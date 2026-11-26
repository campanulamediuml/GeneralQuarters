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
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间失败：%d", status)
	}
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B"}, nil); status != http.StatusOK {
		t.Fatalf("加入房间失败：%d", status)
	}
	cleanup := func() { httpServer.Close(); database.Close() }
	return a, database, app, created.RoomID, cleanup
}

// 重启之后双方 ready 标记还在库里，但不会再有 ready 请求触发结算：
// 巡检必须把这局推进一回合，否则房间永远停在原地。
func TestReconcilerResolvesStalledReadyRoom(t *testing.T) {
	a, database, app, roomID, cleanup := stalledFixture(t)
	defer cleanup()
	if _, err := database.DB.Exec("UPDATE players SET ready=1 WHERE room_id=?", roomID); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339Nano)
	if _, err := database.DB.Exec("UPDATE rooms SET updated_at=? WHERE id=?", old, roomID); err != nil {
		t.Fatal(err)
	}
	app.settleStalledRooms(context.Background())

	var view testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatal("读取状态失败")
	}
	if view.Turn != 2 {
		t.Fatalf("巡检应当把卡住的房间结算到下一回合，实际停在 %d", view.Turn)
	}
	for _, player := range view.Players {
		if player.Ready {
			t.Fatalf("%s 的 ready 应在结算后清空", player.Name)
		}
	}
}

// 刚 ready 的房间不能抢跑：正常请求正在结算，巡检只处理静置超过阈值的。
func TestReconcilerLeavesFreshReadyRoomAlone(t *testing.T) {
	a, database, app, roomID, cleanup := stalledFixture(t)
	defer cleanup()
	if _, err := database.DB.Exec("UPDATE players SET ready=1 WHERE room_id=?", roomID); err != nil {
		t.Fatal(err)
	}
	app.settleStalledRooms(context.Background())

	var view testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatal("读取状态失败")
	}
	if view.Turn != 1 {
		t.Fatalf("静置时间没超过阈值时不应结算，实际推进到 %d", view.Turn)
	}
}
