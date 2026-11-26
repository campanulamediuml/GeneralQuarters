package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"generalquarters/internal/game"
)

// 房间状态在库里存的是 gzip 压缩后的 JSON blob（rooms.state_blob / turns.state_blob）。
//
// 为什么不直接存 JSON 文本：整份 game.State 每回合要写两次（房间当前状态 + 回合历史），
// 明文 300 KB 上下；同样内容 gzip 完十几 KB，写库和磁盘都省一个数量级。以前是
// "先写明文、后台 worker 慢慢压"，现在在编码这一步压掉：不用 worker、不用
// compressed 标记，也不存在"压到一半"的中间状态。
//
// 老库照样能读：那时存的是明文 JSON（TEXT），或者被老 worker 压过的 gzip blob。
// 按魔数判断即可，不需要额外的列来标记编码。
const stateGzipLevel = gzip.BestSpeed

func encodeState(state game.State) ([]byte, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("编码房间状态失败：%w", err)
	}
	var buffer bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buffer, stateGzipLevel)
	if err != nil {
		return nil, err
	}
	if _, err := writer.Write(raw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// decodeState 认三种存档：gzip(JSON) blob（现在写的）、明文 JSON（老库，TEXT）、空数据。
func decodeState(raw []byte) (game.State, error) {
	var state game.State
	if len(raw) == 0 {
		return state, fmt.Errorf("房间状态为空")
	}
	if isGzipBlob(raw) {
		reader, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return state, fmt.Errorf("解压房间状态失败：%w", err)
		}
		defer reader.Close()
		plain, err := io.ReadAll(reader)
		if err != nil {
			return state, fmt.Errorf("解压房间状态失败：%w", err)
		}
		raw = plain
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("解析房间状态失败：%w", err)
	}
	return state, nil
}

func isGzipBlob(raw []byte) bool {
	return len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b
}

// ReplaceState 是给测试和工具用的"直接换掉房间状态"入口：按存档格式写回
// rooms.state_blob（gzip JSON）。
func (s *Store) ReplaceState(ctx context.Context, roomID string, state game.State) error {
	data, err := encodeState(state)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "UPDATE rooms SET state_blob=? WHERE id=?", data, roomID)
	return err
}
