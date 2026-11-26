package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"generalquarters/internal/game"
)

// turns 表是回放用的历史快照：每回合一条完整的 game.State。
//
// 写入路径（CreateRoom / Resolve）只写原样 JSON，压缩交给后台 worker：
//   - 回合结算在关键路径上，不能为了省磁盘在那里压 3 MB 的状态；
//   - 但这份 JSON 的重复度极高（同一套键名、同一批坐标），gzip 后只剩 8%~12%，
//     3 MB 的状态压完约 300 KB，历史库能小一个数量级。
//
// 压缩只处理"已经结束的回合"（turn < 房间当前回合），正在进行的那一行永远不碰，
// 所以回放和查历史永远读到完整状态。
const (
	turnCompressionLevel = 3
	turnCompressionBatch = 8
	turnCompressionEvery = 3 * time.Second
	turnCompressionLog   = 50
)

func compressTurnJSON(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, turnCompressionLevel)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(raw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressTurnJSON(raw []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// LoadTurnState 读某一回合的历史快照；压缩过的行自动解压。
// 回放、查历史都走这个入口，调用方不需要知道这一行压没压过。
func (s *Store) LoadTurnState(ctx context.Context, roomID string, turn int) (game.State, error) {
	var state game.State
	var raw []byte
	var compressed int
	err := s.DB.QueryRowContext(ctx, "SELECT state_json, compressed FROM turns WHERE room_id=? AND turn=?", roomID, turn).Scan(&raw, &compressed)
	if err == sql.ErrNoRows {
		return state, fmt.Errorf("第 %d 回合没有快照", turn)
	}
	if err != nil {
		return state, err
	}
	if compressed != 0 {
		raw, err = decompressTurnJSON(raw)
		if err != nil {
			return state, fmt.Errorf("解压第 %d 回合快照失败：%w", turn, err)
		}
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("解析第 %d 回合快照失败：%w", turn, err)
	}
	return state, nil
}

// StartTurnCompressor 起一个后台 worker，定期把已结束回合的快照压成 gzip。
// interval/batch 为零时用默认值；ctx 取消即退出（服务停止时用）。
func (s *Store) StartTurnCompressor(ctx context.Context, interval time.Duration, batch int) {
	if interval <= 0 {
		interval = turnCompressionEvery
	}
	if batch <= 0 {
		batch = turnCompressionBatch
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		total := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			count, err := s.compressPendingTurns(ctx, batch)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("回合快照压缩失败：%v", err)
				}
				continue
			}
			if count == 0 {
				continue
			}
			total += count
			if total >= turnCompressionLog {
				log.Printf("回合快照压缩完成 %d 条", total)
				total = 0
			}
		}
	}()
}

// compressPendingTurns 压缩一批已结束回合的快照，返回实际压缩的条数。
// 新回合优先：回放和查历史多半先看最近几十回合。
func (s *Store) compressPendingTurns(ctx context.Context, batch int) (int, error) {
	if batch <= 0 {
		batch = turnCompressionBatch
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT t.room_id, t.turn, t.state_json FROM turns t JOIN rooms r ON r.id = t.room_id WHERE t.compressed = 0 AND t.turn < r.turn ORDER BY t.turn DESC LIMIT ?", batch)
	if err != nil {
		return 0, err
	}
	type pending struct {
		roomID string
		turn   int
		raw    []byte
	}
	items := make([]pending, 0, batch)
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.roomID, &item.turn, &item.raw); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	done := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return done, nil
		}
		if len(item.raw) == 0 {
			continue
		}
		packed, err := compressTurnJSON(item.raw)
		if err != nil {
			return done, err
		}
		value := packed
		result, err := s.DB.ExecContext(ctx, "UPDATE turns SET state_json=?, compressed=1 WHERE room_id=? AND turn=? AND compressed=0", value, item.roomID, item.turn)
		if err != nil {
			return done, err
		}
		if affected, _ := result.RowsAffected(); affected == 1 {
			done++
		}
	}
	return done, nil
}
