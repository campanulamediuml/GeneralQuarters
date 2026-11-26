package store

import (
	"context"
	"database/sql"
	"fmt"

	"generalquarters/internal/game"
)

// turns 表是回放用的历史快照：每回合一条完整的 game.State，存的是 gzip(JSON) blob
// （编码见 statecodec.go）。
//
// 压缩就在写库那一步做，没有后台 worker、没有 compressed 标记：以前是"先写明文、
// worker 隔几秒再压"，现在结算事务里直接写压好的 blob（十几 KB），既省了写库量，
// 也不存在"压缩标志和实际内容对不上"的中间状态。老库里明文 JSON 和老的 gzip 行
// 都还能读，decodeState 按魔数判断。

// LoadTurnState 读某一回合的历史快照。回放、查历史都走这个入口，
// 调用方不需要知道这一行是老明文还是 gzip blob。
func (s *Store) LoadTurnState(ctx context.Context, roomID string, turn int) (game.State, error) {
	var state game.State
	var raw []byte
	err := s.DB.QueryRowContext(ctx, "SELECT state_blob FROM turns WHERE room_id=? AND turn=?", roomID, turn).Scan(&raw)
	if err == sql.ErrNoRows {
		return state, fmt.Errorf("第 %d 回合没有快照", turn)
	}
	if err != nil {
		return state, err
	}
	state, err = decodeState(raw)
	if err != nil {
		return state, fmt.Errorf("解析第 %d 回合快照失败：%w", turn, err)
	}
	return state, nil
}
