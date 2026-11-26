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

func TestTurnSnapshotCompression(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	state := game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 1}
	room := Room{ID: "room1", JoinToken: "tok", Status: "running", Turn: 1, Version: 1, State: state}
	player := Player{ID: "p1", Faction: "A", Name: "甲", IsHost: true}
	if err := store.CreateRoom(ctx, room, player, "hash"); err != nil {
		t.Fatalf("建房失败：%v", err)
	}
	// 正在进行的回合不能压：回放读到的永远是完整状态。
	if n, err := store.compressPendingTurns(ctx, 10); err != nil || n != 0 {
		t.Fatalf("进行中的回合被压缩了：n=%d err=%v", n, err)
	}
	// 推进到第 2 回合：第 1 回合变成"已结束"，worker 可以压它。
	if _, err := store.DB.Exec("UPDATE rooms SET turn=2, version=2 WHERE id=?", "room1"); err != nil {
		t.Fatalf("推进回合失败：%v", err)
	}
	next, err := json.Marshal(game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 2})
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	if _, err := store.DB.Exec(`INSERT INTO turns(room_id,turn,version,state_json,created_at,compressed) VALUES(?,?,?,?,?,0)`, "room1", 2, 2, next, "now"); err != nil {
		t.Fatalf("写入第 2 回合失败：%v", err)
	}
	n, err := store.compressPendingTurns(ctx, 10)
	if err != nil {
		t.Fatalf("压缩失败：%v", err)
	}
	if n != 1 {
		t.Fatalf("应压缩 1 行，实际 %d", n)
	}
	var compressed int
	var size int
	if err := store.DB.QueryRow(`SELECT compressed, length(state_json) FROM turns WHERE room_id=? AND turn=?`, "room1", 1).Scan(&compressed, &size); err != nil {
		t.Fatalf("查压缩标记失败：%v", err)
	}
	if compressed != 1 {
		t.Fatalf("第 1 回合没有被标记为已压缩")
	}
	if size >= 1024 {
		t.Fatalf("压缩后仍有 %d 字节，测试状态太小说明不了问题", size)
	}
	got, err := store.LoadTurnState(ctx, "room1", 1)
	if err != nil {
		t.Fatalf("读取第 1 回合失败：%v", err)
	}
	if got.ScenarioID != "_test" || got.Turn != 1 {
		t.Fatalf("读回的状态不对：%+v", got)
	}
	// 未压缩的第 2 回合也要能读。
	second, err := store.LoadTurnState(ctx, "room1", 2)
	if err != nil {
		t.Fatalf("读取第 2 回合失败：%v", err)
	}
	if second.Turn != 2 {
		t.Fatalf("第 2 回合状态不对：%+v", second)
	}
	// 重复跑 worker 不应重复计数。
	if n, err := store.compressPendingTurns(ctx, 10); err != nil || n != 0 {
		t.Fatalf("重复压缩：n=%d err=%v", n, err)
	}
}
