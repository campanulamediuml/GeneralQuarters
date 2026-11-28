package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"generalquarters/internal/game"
)

// SavePlannedOrder 原子保存房间版本、幂等回执和待结算订单。
func (s *Store) SavePlannedOrder(ctx context.Context, room Room, playerID, commandID string, order game.Order, response, fingerprint string) (Room, error) {
	data, err := json.Marshal(order)
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
	result, err := tx.ExecContext(ctx, "UPDATE rooms SET version=?,updated_at=? WHERE id=? AND version=?", newVersion, now, room.ID, room.Version)
	if err != nil {
		return room, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return room, fmt.Errorf("房间版本冲突")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE players SET ready=0 WHERE id=?", playerID); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO orders VALUES(?,?,?,?,?) ON CONFLICT(room_id,turn,player_id,unit_id) DO UPDATE SET order_json=excluded.order_json`, room.ID, room.Turn, playerID, order.UnitID, data); err != nil {
		return room, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO commands VALUES(?,?,?,?,?)", playerID, commandID, response, now, fingerprint); err != nil {
		return room, err
	}
	if err := tx.Commit(); err != nil {
		return room, err
	}
	room.Version = newVersion
	return room, nil
}
