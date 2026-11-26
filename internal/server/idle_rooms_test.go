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

// 闲置超过 12 小时的房间只释放内存：worker 停掉、连接断开，
// 但房间、事件、回合快照一行都不删——那是复盘用的。再进来会从库里重新载入。
func TestSweepIdleRoomsFreesActorButKeepsHistory(t *testing.T) {
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

	if _, ok := app.actors.Load(created.RoomID); ok {
		t.Fatal("闲置房间的 actor 还挂在内存里")
	}
	// 库里的东西一样不能少：房间、事件、回合快照。
	if _, err := database.Room(ctx, created.RoomID); err != nil {
		t.Fatalf("闲置房间被误删：%v", err)
	}
	var events, turns int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM events WHERE room_id=?", created.RoomID).Scan(&events); err != nil {
		t.Fatalf("统计事件失败：%v", err)
	}
	if events == 0 {
		t.Fatal("对战日志被清掉了，复盘就没了")
	}
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM turns WHERE room_id=?", created.RoomID).Scan(&turns); err != nil {
		t.Fatalf("统计回合快照失败：%v", err)
	}
	if turns == 0 {
		t.Fatal("回合快照被清掉了")
	}
	// 再访问一次：按需从库里重新载入，状态照常返回，而不是 404。
	var reloaded map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &reloaded); status != http.StatusOK {
		t.Fatalf("释放内存后重进应当能重新载入，实际 %d", status)
	}
	if _, ok := app.actors.Load(created.RoomID); !ok {
		t.Fatal("重新载入后应当有新的 room actor")
	}
}

// 有人在线的房间即使很久没出过命令也不能动：玩家开着页面看海图不算废弃。
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

	if _, ok := app.actors.Load(created.RoomID); !ok {
		t.Fatal("在线房间的 actor 不该被清掉")
	}
}
