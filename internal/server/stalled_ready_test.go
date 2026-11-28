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

// stalledFixture 造一个"双方已入座、还没结算"的房间，并返回服务端实例。
func stalledFixture(t *testing.T) (*testClient, *store.Store, *Server, string, func()) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	app := New(database, testContentRoot(t), false)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	httpServer := httptest.NewServer(mux)
	a := newTestClient(t, httpServer.URL)
	b := newTestClient(t, httpServer.URL)
	created := createTestPlayerRoom(t, a, "_fixture", "A", false)
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B", "faction": "B"}, nil); status != http.StatusOK {
		t.Fatalf("加入房间失败：%d", status)
	}
	cleanup := func() { httpServer.Close(); database.Close() }
	return a, database, app, created.RoomID, cleanup
}

// restartServer 在同一个库上新起一个服务端实例，模拟进程重启：
// 新实例里没有任何房间状态，房间的存档要等第一条消息才被读进来。
// 房间状态现在只存在 worker 的内存里，所以"库里改了、进程没重启"本来就不该生效——
// 巡检要覆盖的恰恰是重启后 ready 标记还留在库里的那种情况，测试也必须按重启来造。
func restartServer(t *testing.T, database *store.Store) *Server {
	t.Helper()
	return New(database, testContentRoot(t), false)
}

// 重启之后双方 ready 标记还在库里，但不会再有 ready 请求触发结算：
// 巡检必须把这局推进一回合，否则房间永远停在原地。
func TestReconcilerResolvesStalledReadyRoom(t *testing.T) {
	_, database, _, roomID, cleanup := stalledFixture(t)
	defer cleanup()
	if _, err := database.DB.Exec("UPDATE players SET ready=1 WHERE room_id=?", roomID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	if _, err := database.DB.Exec("UPDATE rooms SET updated_at=? WHERE id=?", old, roomID); err != nil {
		t.Fatal(err)
	}
	app := restartServer(t, database)
	app.settleStalledRooms(context.Background())

	// 巡检是往房间队列里投一条消息，异步的；这里等它落地。
	// 结果直接看库：新实例没有旧连接，走 HTTP 还得再登录一次，没有必要。
	deadline := time.Now().Add(3 * time.Second)
	var room store.Room
	var err error
	for {
		room, err = database.Room(context.Background(), roomID)
		if err == nil && room.Turn == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("巡检没有把卡住的房间结算到下一回合，停在 %d（err=%v）", room.Turn, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	players, err := database.Players(context.Background(), roomID)
	if err != nil {
		t.Fatal(err)
	}
	for _, player := range players {
		if player.Ready {
			t.Fatalf("%s 的 ready 应在结算后清空", player.Name)
		}
	}
}

// 刚 ready 的房间不能抢跑：正常请求正在结算，巡检只处理静置超过阈值的。
func TestReconcilerLeavesFreshReadyRoomAlone(t *testing.T) {
	_, database, _, roomID, cleanup := stalledFixture(t)
	defer cleanup()
	if _, err := database.DB.Exec("UPDATE players SET ready=1 WHERE room_id=?", roomID); err != nil {
		t.Fatal(err)
	}
	app := restartServer(t, database)
	app.settleStalledRooms(context.Background())
	// 给可能的结算留出时间，再确认它没发生。
	time.Sleep(100 * time.Millisecond)

	room, err := database.Room(context.Background(), roomID)
	if err != nil {
		t.Fatal(err)
	}
	if room.Turn != 1 {
		t.Fatalf("静置时间没超过阈值时不应结算，实际推进到 %d", room.Turn)
	}
}
