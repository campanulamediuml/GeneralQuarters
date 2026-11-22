package store

import (
	"context"
	"fmt"
	"time"
)

// BumpVersion 只推进房间版本号（保存待结算命令时用）：让"该版本已被处理"对并发命令可见，
// 同时把该玩家置为未准备，语义与 ApplyImmediate 一致。
func (s *Store) BumpVersion(ctx context.Context, room Room, playerID, commandID, fingerprint, response string) (Room, error) {
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
	if _, err := tx.ExecContext(ctx, "INSERT INTO commands VALUES(?,?,?,?,?)", playerID, commandID, response, now, fingerprint); err != nil {
		return room, err
	}
	room.Version = newVersion
	return room, tx.Commit()
}
