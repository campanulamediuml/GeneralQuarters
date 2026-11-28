package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"generalquarters/internal/game"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "generalquarters.db"))
	if err != nil {
		t.Fatalf("打开测试库失败：%v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// 存档写进去就是 gzip blob：不需要后台 worker，也不会留下"标记和内容对不上"的中间行。
func TestTurnSnapshotIsGzipBlob(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	state := game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 1}
	room := Room{ID: "room1", JoinToken: "tok", Status: "running", Turn: 1, Version: 1, State: state}
	if err := store.CreateRoom(ctx, room); err != nil {
		t.Fatalf("建房失败：%v", err)
	}
	var raw []byte
	if err := store.DB.QueryRow("SELECT state_blob FROM turns WHERE room_id=? AND turn=?", "room1", 1).Scan(&raw); err != nil {
		t.Fatalf("读存档失败：%v", err)
	}
	if !isGzipBlob(raw) {
		t.Fatalf("新建房间的回合快照不是 gzip blob：%d 字节，开头 %x", len(raw), raw[:min(4, len(raw))])
	}
	got, err := store.LoadTurnState(ctx, "room1", 1)
	if err != nil {
		t.Fatalf("读取第 1 回合失败：%v", err)
	}
	if got.ScenarioID != "_test" || got.Turn != 1 {
		t.Fatalf("读回的状态不对：%+v", got)
	}
	// 老库里没压过的明文 JSON 行也必须能读（升级不强制迁移数据）。
	legacy, err := json.Marshal(game.State{ScenarioID: "_old", ScenarioName: "旧库", Turn: 2})
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	if _, err := store.DB.Exec("INSERT INTO turns(room_id,turn,version,state_blob,created_at) VALUES(?,?,?,?,?)", "room1", 2, 2, legacy, "now"); err != nil {
		t.Fatalf("写入明文旧行失败：%v", err)
	}
	second, err := store.LoadTurnState(ctx, "room1", 2)
	if err != nil {
		t.Fatalf("读取明文旧行失败：%v", err)
	}
	if second.ScenarioID != "_old" || second.Turn != 2 {
		t.Fatalf("明文旧行读回的状态不对：%+v", second)
	}
}
