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
	// Solo 表示"单人房"：允许一名玩家 ready 就结算（沙盒自测用）。
	// 双人对局必须是两名玩家都 ready 才推进，否则一个人进房点一下就自己跟自己跑起来。
	Solo bool
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
 state_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
 solo INTEGER NOT NULL DEFAULT 0
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
 request_fingerprint TEXT NOT NULL DEFAULT '',
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
	if err != nil {
		return err
	}
	// 老库的 commands 表没有 request_fingerprint 列：补上，否则幂等校验退化成
	// "沿用旧 command_id 就静默返回旧回执"，客户端重启后第一部分命令会被吞掉。
	if !s.hasColumn("commands", "request_fingerprint") {
		if _, err := s.DB.Exec(`ALTER TABLE commands ADD COLUMN request_fingerprint TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// 老库的 rooms 表没有 solo 列：补上，默认 0（双人对局）。
	if !s.hasColumn("rooms", "solo") {
		if _, err := s.DB.Exec(`ALTER TABLE rooms ADD COLUMN solo INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// turns 的历史快照由后台 worker 压缩（见 turns.go）：compressed=0 表示还是原样 JSON。
	// 老库补列时全部记成 0，worker 会按"已结束回合"的规则慢慢把它们压掉。
	if !s.hasColumn("turns", "compressed") {
		if _, err := s.DB.Exec(`ALTER TABLE turns ADD COLUMN compressed INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// 部分索引：worker 每轮只需要找还没压的行，别整表扫。
	if _, err := s.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_turns_pending ON turns(turn) WHERE compressed = 0`); err != nil {
		return err
	}
	return nil
}

func (s *Store) hasColumn(table, column string) bool {
	rows, err := s.DB.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
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
	_, err = tx.ExecContext(ctx, `INSERT INTO rooms(id,join_token,scenario_id,status,turn,version,state_json,created_at,updated_at,solo) VALUES(?,?,?,?,?,?,?,?,?,?)`, room.ID, room.JoinToken, room.State.ScenarioID, room.Status, room.Turn, room.Version, stateJSON, now, now, boolInt(room.Solo))
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
	_, err = tx.ExecContext(ctx, `INSERT INTO turns(room_id,turn,version,state_json,created_at,compressed) VALUES(?,?,?,?,?,0)`, room.ID, room.Turn, room.Version, stateJSON, now)
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

// RoomJoinToken 只取邀请 token：观战握手要拿它比对，没必要把整份 146KB 的
// state_json 读回来解析一遍（那一次读比这个查询贵三个数量级）。
func (s *Store) RoomJoinToken(ctx context.Context, roomID string) (string, error) {
	var token string
	err := s.DB.QueryRowContext(ctx, `SELECT join_token FROM rooms WHERE id=?`, roomID).Scan(&token)
	return token, err
}

func (s *Store) Room(ctx context.Context, roomID string) (Room, error) {
	var room Room
	var stateJSON string
	var solo int
	err := s.DB.QueryRowContext(ctx, `SELECT id,join_token,status,turn,version,state_json,solo FROM rooms WHERE id=?`, roomID).Scan(&room.ID, &room.JoinToken, &room.Status, &room.Turn, &room.Version, &stateJSON, &solo)
	room.Solo = solo != 0
	if err != nil {
		return room, err
	}
	if err := json.Unmarshal([]byte(stateJSON), &room.State); err != nil {
		return room, err
	}
	game.EnsureAviation(&room.State)
	game.NormalizeFleetNames(&room.State)
	game.NormalizeAirfieldHangar(&room.State)
	return room, nil
}

// StalledReadyRooms 找出"所有玩家都已 ready、但房间已经超过 olderThan 没动过"的对局。
// 服务重启后 ready 标记还留在库里，可新的 ready 请求不会再来，结算逻辑就永远等不到
// 第二次触发，房间会卡死在同一回合（T108 这次就是这么卡住的）。巡检靠这个把房间捞出来。
func (s *Store) StalledReadyRooms(ctx context.Context, olderThan time.Duration) ([]Room, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
	rows, err := s.DB.QueryContext(ctx, "SELECT r.id,r.join_token,r.status,r.turn,r.version,r.state_json,r.solo FROM rooms r WHERE r.status='running' AND r.updated_at < ? AND EXISTS(SELECT 1 FROM players p WHERE p.room_id=r.id) AND NOT EXISTS(SELECT 1 FROM players p WHERE p.room_id=r.id AND p.ready=0)", cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rooms := []Room{}
	for rows.Next() {
		var room Room
		var stateJSON string
		var solo int
		if err := rows.Scan(&room.ID, &room.JoinToken, &room.Status, &room.Turn, &room.Version, &stateJSON, &solo); err != nil {
			return nil, err
		}
		room.Solo = solo != 0
		if err := json.Unmarshal([]byte(stateJSON), &room.State); err != nil {
			return nil, err
		}
		game.EnsureAviation(&room.State)
		game.NormalizeFleetNames(&room.State)
		game.NormalizeAirfieldHangar(&room.State)
		rooms = append(rooms, room)
	}
	return rooms, rows.Err()
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

// CommandResponse 返回某个 command_id 之前写下的回执，以及当时的请求指纹。
// 指纹让服务端能区分"同一命令的重试"和"复用 ID 的另一条命令"：前者原样返回，
// 后者必须报错，否则客户端重启后沿用旧 ID 会静默拿到旧回执、新命令根本不执行。
func (s *Store) CommandResponse(ctx context.Context, playerID, commandID string) (string, string, bool, error) {
	var response, fingerprint string
	err := s.DB.QueryRowContext(ctx, `SELECT response_json, COALESCE(request_fingerprint,'') FROM commands WHERE player_id=? AND command_id=?`, playerID, commandID).Scan(&response, &fingerprint)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	return response, fingerprint, err == nil, err
}

func (s *Store) SaveOrder(ctx context.Context, roomID string, turn int, playerID, commandID string, order game.Order, response, fingerprint string) error {
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
		if order.TaskID != "" {
			// 无线电命令按任务登记，需要用 task_id 才能撤回。
			_, err = tx.ExecContext(ctx, `DELETE FROM orders WHERE room_id=? AND turn=? AND player_id=? AND unit_id=?`, roomID, turn, playerID, order.TaskID)
		} else {
			_, err = tx.ExecContext(ctx, `DELETE FROM orders WHERE room_id=? AND turn=? AND player_id=? AND unit_id=?`, roomID, turn, playerID, order.UnitID)
		}
	} else if order.Type == "RECALL_AIR" || order.Type == "RETASK_AIR" || order.Type == "RETARGET_AIR" {
		// 无线电命令挂在任务上，而不是某一艘船上。
		_, err = tx.ExecContext(ctx, `INSERT INTO orders VALUES(?,?,?,?,?) ON CONFLICT(room_id,turn,player_id,unit_id) DO UPDATE SET order_json=excluded.order_json`, roomID, turn, playerID, order.TaskID, data)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO orders VALUES(?,?,?,?,?) ON CONFLICT(room_id,turn,player_id,unit_id) DO UPDATE SET order_json=excluded.order_json`, roomID, turn, playerID, order.UnitID, data)
	}
	if err != nil {
		return err
	}
	// 幂等响应可能已经由 BumpVersion 写过（规划移动先推进版本再存命令），用 upsert 覆盖即可。
	_, err = tx.ExecContext(ctx, `INSERT INTO commands VALUES(?,?,?,?,?) ON CONFLICT(player_id,command_id) DO UPDATE SET response_json=excluded.response_json, request_fingerprint=excluded.request_fingerprint`, playerID, commandID, response, time.Now().UTC().Format(time.RFC3339Nano), fingerprint)
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

func (s *Store) ApplyImmediate(ctx context.Context, room Room, playerID, commandID, fingerprint, response string, newState game.State, events []game.EngineEvent) (Room, error) {
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
	// 回合内的这条 UPDATE 把"当回合快照"更新成玩家当前看到的状态。
	// compressed 必须显式归零：万一 worker 刚好压过这一行，不重置就会变成
	// "压缩标志=1、内容是明文"，读的时候按 gzip 解压直接失败。
	if _, err := tx.ExecContext(ctx, `UPDATE turns SET version=?,state_json=?,compressed=0 WHERE room_id=? AND turn=?`, newVersion, data, room.ID, room.Turn); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO commands VALUES(?,?,?,?,?)`, playerID, commandID, response, now, fingerprint); err != nil {
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO turns(room_id,turn,version,state_json,created_at,compressed) VALUES(?,?,?,?,?,0)`, room.ID, newTurn, newVersion, data, now); err != nil {
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

// AllEvents 是观战视角的战报：不按 audience 过滤，双方阵营的内部事件都能看到。
func (s *Store) AllEvents(ctx context.Context, roomID string) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,turn,sequence,type,text,created_at FROM events WHERE room_id=? ORDER BY id DESC LIMIT 200`, roomID)
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
