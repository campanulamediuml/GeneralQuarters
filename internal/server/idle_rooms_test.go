package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"generalquarters/internal/store"
)

// 闲置超过 12 小时的房间要被真删掉：库里连战报一起清空，内存里的 worker 停掉，
// 之后再访问就是 404——不能出现"库删了、actor 还留着空壳"的泄漏。
func TestSweepIdleRoomsEvictsActorAndRoom(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	app := New(database, testContentRoot(t), false)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := newTestClient(t, server.URL)

	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	// 进门读一次状态：这一下才会把房间 worker 和它的 146KB 状态载进内存，
	// 也就是"占内存"的那一份。
	var first map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &first); status != http.StatusOK {
		t.Fatalf("读状态失败：%d", status)
	}
	if _, ok := app.actors.Load(created.RoomID); !ok {
		t.Fatal("读状态后应该已经有 room actor")
	}

	// 把房间的活动时间推到 13 小时前，模拟废弃对局。创建房间时玩家 connected=0，
	// 所以这里不需要额外断开连接。
	old := time.Now().UTC().Add(-13 * time.Hour).Format(time.RFC3339Nano)
	if _, err := database.DB.Exec("UPDATE rooms SET updated_at=?", old); err != nil {
		t.Fatalf("改写活动时间失败：%v", err)
	}

	app.sweepIdleRooms(ctx)

	if _, err := database.Room(ctx, created.RoomID); err == nil {
		t.Fatal("闲置房间还留在库里")
	}
	if _, ok := app.actors.Load(created.RoomID); ok {
		t.Fatal("闲置房间的 actor 还挂在内存里")
	}
	// 房间和会话一起删了，所以再拿原 cookie 访问是 401（会话已失效、前端会回首页），
	// 空房间壳则是 404。两种都算"访问不到了"，只要不是 200 就对了。
	var state map[string]any
	status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &state)
	if status != http.StatusUnauthorized && status != http.StatusNotFound {
		t.Fatalf("房间删掉后不应还能读状态，实际 %d", status)
	}
}

// 有人在线的房间即使很久没出过命令也不能删：玩家开着页面看海图不算废弃。
func TestSweepIdleRoomsKeepsConnectedRoom(t *testing.T) {
	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	app := New(database, testContentRoot(t), false)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := newTestClient(t, server.URL)

	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	var first map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &first); status != http.StatusOK {
		t.Fatalf("读状态失败：%d", status)
	}
	// 先标在线（会刷新活动时间），再把活动时间推回 13 小时前：
	// 判定时应由"有人在线"这条兜住。
	if err := database.SetConnected(ctx, created.PlayerID, true); err != nil {
		t.Fatalf("置在线失败：%v", err)
	}
	old := time.Now().UTC().Add(-13 * time.Hour).Format(time.RFC3339Nano)
	if _, err := database.DB.Exec("UPDATE rooms SET updated_at=?", old); err != nil {
		t.Fatalf("改写活动时间失败：%v", err)
	}

	app.sweepIdleRooms(ctx)

	if _, err := database.Room(ctx, created.RoomID); err != nil {
		t.Fatalf("有人在线时不应当回收：%v", err)
	}
	if _, ok := app.actors.Load(created.RoomID); !ok {
		t.Fatal("在线房间的 actor 不该被清掉")
	}
}
