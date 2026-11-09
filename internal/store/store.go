package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"generalquarters/internal/game"
	_ "modernc.org/sqlite"
)

type Room struct {
	ID        string
	JoinToken string
	Status    string
	Turn      int
	Version   int64
	State     game.State
}

type Player struct {
	ID        string `json:"id"`
	RoomID    string `json:"-"`
	Faction   string `json:"faction"`
	Name      string `json:"name"`
	IsHost    bool   `json:"is_host"`
	Ready     bool   `json:"ready"`
	Connected bool   `json:"connected"`
}

type Event struct {
	ID        int64     `json:"id"`
	Turn      int       `json:"turn"`
	Sequence  int       `json:"sequence"`
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, err
	}
	store := &Store{DB: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS rooms (
 id TEXT PRIMARY KEY, join_token TEXT NOT NULL UNIQUE, scenario_id TEXT NOT NULL,
 status TEXT NOT NULL, turn INTEGER NOT NULL, version INTEGER NOT NULL,
 state_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS players (
 id TEXT PRIMARY KEY, room_id TEXT NOT NULL, faction TEXT NOT NULL, name TEXT NOT NULL,
 is_host INTEGER NOT NULL, ready INTEGER NOT NULL DEFAULT 0, connected INTEGER NOT NULL DEFAULT 1,
 UNIQUE(room_id, faction), FOREIGN KEY(room_id) REFERENCES rooms(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS sessions (
 token_hash TEXT PRIMARY KEY, player_id TEXT NOT NULL, room_id TEXT NOT NULL, expires_at TEXT NOT NULL,
 FOREIGN KEY(player_id) REFERENCES players(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS commands (
 player_id TEXT NOT NULL, command_id TEXT NOT NULL, response_json TEXT NOT NULL, created_at TEXT NOT NULL,
 PRIMARY KEY(player_id, command_id)
);
CREATE TABLE IF NOT EXISTS orders (
 room_id TEXT NOT NULL, turn INTEGER NOT NULL, player_id TEXT NOT NULL, unit_id TEXT NOT NULL,
 order_json TEXT NOT NULL, PRIMARY KEY(room_id, turn, player_id, unit_id)
);
CREATE TABLE IF NOT EXISTS turns (
 room_id TEXT NOT NULL, turn INTEGER NOT NULL, version INTEGER NOT NULL, state_json TEXT NOT NULL,
 created_at TEXT NOT NULL, PRIMARY KEY(room_id, turn)
);
CREATE TABLE IF NOT EXISTS events (
 id INTEGER PRIMARY KEY AUTOINCREMENT, room_id TEXT NOT NULL, turn INTEGER NOT NULL,
 sequence INTEGER NOT NULL, type TEXT NOT NULL, text TEXT NOT NULL, audience TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_room ON events(room_id, id);
`)
	return err
}

func (s *Store) Close() error { return s.DB.Close() }

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) CreateRoom(ctx context.Context, room Room, player Player, sessionHash string) error {
	stateJSON, err := json.Marshal(room.State)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO rooms VALUES(?,?,?,?,?,?,?,?,?)`, room.ID, room.JoinToken, room.State.ScenarioID, room.Status, room.Turn, room.Version, stateJSON, now, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO players(id,room_id,faction,name,is_host,ready,connected) VALUES(?,?,?,?,?,0,1)`, player.ID, room.ID, player.Faction, player.Name, boolInt(player.IsHost))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions VALUES(?,?,?,?)`, sessionHash, player.ID, room.ID, time.Now().Add(30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO turns VALUES(?,?,?,?,?)`, room.ID, room.Turn, room.Version, stateJSON, now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events(room_id,turn,sequence,type,text,audience,created_at) VALUES(?,?,?,?,?,?,?)`, room.ID, room.Turn, 0, "room_created", "房间已创建，沙盒剧本载入完成", "all", now)
	return tx.Commit()
}

func (s *Store) JoinRoom(ctx context.Context, joinToken, name, playerID, sessionHash string) (Player, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Player{}, err
	}
	defer tx.Rollback()
	var roomID string
	var player Player
	err = tx.QueryRowContext(ctx, `SELECT id FROM rooms WHERE join_token=?`, joinToken).Scan(&roomID)
	if err == sql.ErrNoRows {
		return player, fmt.Errorf("房间token无效")
	}
	if err != nil {
		return player, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM players WHERE room_id=?`, roomID).Scan(&count); err != nil {
		return player, err
	}
	if count >= 2 {
		return player, fmt.Errorf("房间已满")
	}
	faction := "A"
	var existing string
	err = tx.QueryRowContext(ctx, `SELECT faction FROM players WHERE room_id=? ORDER BY is_host DESC LIMIT 1`, roomID).Scan(&existing)
	if err == nil && existing == "A" {
		faction = "B"
	}
	player = Player{ID: playerID, RoomID: roomID, Faction: faction, Name: name, IsHost: false, Connected: true}
	if name == "" {
		player.Name = "玩家" + faction
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO players(id,room_id,faction,name,is_host,ready,connected) VALUES(?,?,?,?,0,0,1)`, player.ID, roomID, faction, player.Name)
	if err != nil {
		return player, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions VALUES(?,?,?,?)`, sessionHash, player.ID, roomID, time.Now().Add(30*24*time.Hour).UTC().Format(time.RFC3339Nano))
	if err != nil {
		return player, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO events(room_id,turn,sequence,type,text,audience,created_at) VALUES((SELECT turn FROM rooms WHERE id=?),(SELECT turn FROM rooms WHERE id=?),0,?,?,?,?)`, roomID, roomID, "player_joined", player.Name+"加入房间，阵营"+faction, "all", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return player, err
	}
	return player, tx.Commit()
}

func (s *Store) PlayerBySession(ctx context.Context, tokenHash string) (Player, error) {
	var player Player
	var host, ready, connected int
	err := s.DB.QueryRowContext(ctx, `SELECT p.id,p.room_id,p.faction,p.name,p.is_host,p.ready,p.connected FROM sessions s JOIN players p ON p.id=s.player_id WHERE s.token_hash=? AND s.expires_at>?`, tokenHash, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&player.ID, &player.RoomID, &player.Faction, &player.Name, &host, &ready, &connected)
	player.IsHost, player.Ready, player.Connected = host == 1, ready == 1, connected == 1
	return player, err
}

func (s *Store) RoomIDByToken(ctx context.Context, token string) (string, error) {
	var roomID string
	err := s.DB.QueryRowContext(ctx, `SELECT id FROM rooms WHERE join_token=?`, token).Scan(&roomID)
	return roomID, err
}

func (s *Store) Room(ctx context.Context, roomID string) (Room, error) {
	var room Room
	var stateJSON string
	err := s.DB.QueryRowContext(ctx, `SELECT id,join_token,status,turn,version,state_json FROM rooms WHERE id=?`, roomID).Scan(&room.ID, &room.JoinToken, &room.Status, &room.Turn, &room.Version, &stateJSON)
	if err != nil {
		return room, err
	}
	if err := json.Unmarshal([]byte(stateJSON), &room.State); err != nil {
		return room, err
	}
	game.EnsureAviation(&room.State)
	return room, nil
}

func (s *Store) Players(ctx context.Context, roomID string) ([]Player, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,room_id,faction,name,is_host,ready,connected FROM players WHERE room_id=? ORDER BY faction`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	players := []Player{}
	for rows.Next() {
		var player Player
		var host, ready, connected int
		if err := rows.Scan(&player.ID, &player.RoomID, &player.Faction, &player.Name, &host, &ready, &connected); err != nil {
			return nil, err
		}
		player.IsHost, player.Ready, player.Connected = host == 1, ready == 1, connected == 1
		players = append(players, player)
	}
	return players, rows.Err()
}

func (s *Store) SetReady(ctx context.Context, playerID string, ready bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE players SET ready=? WHERE id=?`, boolInt(ready), playerID)
	return err
}

func (s *Store) SetConnected(ctx context.Context, playerID string, connected bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE players SET connected=? WHERE id=?`, boolInt(connected), playerID)
	return err
}

func (s *Store) CommandResponse(ctx context.Context, playerID, commandID string) (string, bool, error) {
	var response string
	err := s.DB.QueryRowContext(ctx, `SELECT response_json FROM commands WHERE player_id=? AND command_id=?`, playerID, commandID).Scan(&response)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	return response, err == nil, err
}

func (s *Store) SaveOrder(ctx context.Context, roomID string, turn int, playerID, commandID string, order game.Order, response string) error {
	data, err := json.Marshal(order)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if order.Type == "CLEAR_MOVE" {
		_, err = tx.ExecContext(ctx, `DELETE FROM orders WHERE room_id=? AND turn=? AND player_id=? AND unit_id=?`, roomID, turn, playerID, order.UnitID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO orders VALUES(?,?,?,?,?) ON CONFLICT(room_id,turn,player_id,unit_id) DO UPDATE SET order_json=excluded.order_json`, roomID, turn, playerID, order.UnitID, data)
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO commands VALUES(?,?,?,?)`, playerID, commandID, response, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Orders(ctx context.Context, roomID string, turn int, playerID string) ([]OrderRecord, error) {
	query := `SELECT player_id,order_json FROM orders WHERE room_id=? AND turn=?`
	args := []any{roomID, turn}
	if playerID != "" {
		query += ` AND player_id=?`
		args = append(args, playerID)
	}
	query += ` ORDER BY player_id,unit_id`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []OrderRecord{}
	for rows.Next() {
		var record OrderRecord
		var data string
		if err := rows.Scan(&record.PlayerID, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &record.Order); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

type OrderRecord struct {
	PlayerID string
	Order    game.Order
}

func (s *Store) ApplyImmediate(ctx context.Context, room Room, playerID, commandID, response string, newState game.State, events []game.EngineEvent) (Room, error) {
	data, err := json.Marshal(newState)
	if err != nil {
		return room, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return room, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	newVersion := room.Version + 1
	result, err := tx.ExecContext(ctx, `UPDATE rooms SET state_json=?,version=?,updated_at=? WHERE id=? AND version=?`, data, newVersion, now, room.ID, room.Version)
	if err != nil {
		return room, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return room, fmt.Errorf("房间版本冲突")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE players SET ready=0 WHERE id=?`, playerID); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE turns SET version=?,state_json=? WHERE room_id=? AND turn=?`, newVersion, data, room.ID, room.Turn); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO commands VALUES(?,?,?,?)`, playerID, commandID, response, now); err != nil {
		return room, err
	}
	for sequence, event := range events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(room_id,turn,sequence,type,text,audience,created_at) VALUES(?,?,?,?,?,?,?)`, room.ID, room.Turn, sequence, event.Type, event.Text, event.Audience, now); err != nil {
			return room, err
		}
	}
	if err := tx.Commit(); err != nil {
		return room, err
	}
	room.Version, room.State = newVersion, newState
	return room, nil
}

func (s *Store) Resolve(ctx context.Context, room Room, newState game.State, events []game.EngineEvent) (Room, error) {
	data, err := json.Marshal(newState)
	if err != nil {
		return room, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return room, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	newVersion := room.Version + 1
	newTurn := room.Turn + 1
	result, err := tx.ExecContext(ctx, `UPDATE rooms SET state_json=?,turn=?,version=?,updated_at=? WHERE id=? AND version=?`, data, newTurn, newVersion, now, room.ID, room.Version)
	if err != nil {
		return room, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return room, fmt.Errorf("房间版本冲突")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE players SET ready=0 WHERE room_id=?`, room.ID); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM orders WHERE room_id=? AND turn=?`, room.ID, room.Turn); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO turns VALUES(?,?,?,?,?)`, room.ID, newTurn, newVersion, data, now); err != nil {
		return room, err
	}
	for sequence, event := range events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(room_id,turn,sequence,type,text,audience,created_at) VALUES(?,?,?,?,?,?,?)`, room.ID, room.Turn, sequence, event.Type, event.Text, event.Audience, now); err != nil {
			return room, err
		}
	}
	if len(events) == 0 {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(room_id,turn,sequence,type,text,audience,created_at) VALUES(?,?,?,?,?,?,?)`, room.ID, room.Turn, 0, "turn_resolved", "本回合没有移动或交战，回合结算完成", "all", now); err != nil {
			return room, err
		}
	}
	if err := tx.Commit(); err != nil {
		return room, err
	}
	room.Turn, room.Version, room.State = newTurn, newVersion, newState
	return room, nil
}

func (s *Store) Events(ctx context.Context, roomID, faction string) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,turn,sequence,type,text,created_at FROM events WHERE room_id=? AND (audience='all' OR audience=?) ORDER BY id DESC LIMIT 100`, roomID, faction)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []Event{}
	for rows.Next() {
		var event Event
		var created string
		if err := rows.Scan(&event.ID, &event.Turn, &event.Sequence, &event.Type, &event.Text, &created); err != nil {
			return nil, err
		}
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		events = append(events, event)
	}
	return events, rows.Err()
}
