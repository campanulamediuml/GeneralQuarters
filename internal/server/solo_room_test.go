package server

import (
	"net/http"
	"testing"
)

// 双人对局里只有一名玩家时不能自己 ready 就推进回合：
// 之前一个人在房里点一下"完成部署"，回合会一直往后跑，等于自己跟自己下。
func TestTwoPlayerRoomRejectsLoneReady(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	var initial testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial)
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": initial.Version, "ready": true}, nil); status != http.StatusBadRequest {
		t.Fatalf("单人 ready 应被拒绝：%d", status)
	}
	var after testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &after)
	if after.Turn != 1 || after.You.Ready {
		t.Fatalf("被拒绝的 ready 不应改变回合或就绪状态：turn=%d ready=%v", after.Turn, after.You.Ready)
	}

	// 第二名玩家进来之后，双方 ready 就正常结算。
	b := newTestClient(t, a.base)
	var joined roomResponse
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]any{"name": "B", "token": created.JoinToken}, &joined); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	var bState testView
	_ = b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &bState)
	var readyView testView
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": bState.Version, "ready": true}, &readyView); status != http.StatusOK {
		t.Fatalf("两人都在时应能 ready：%d", status)
	}
	if readyView.Turn != 1 {
		t.Fatalf("只有一方 ready 不该结算：turn=%d", readyView.Turn)
	}
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": readyView.Version, "ready": true}, &readyView); status != http.StatusOK {
		t.Fatalf("双方 ready 结算失败：%d", status)
	}
	if readyView.Turn != 2 {
		t.Fatalf("双方 ready 后应推进到第2回合：%d", readyView.Turn)
	}
}

// 显式创建的单人房仍然一名玩家 ready 就结算。
func TestSoloRoomResolvesAlone(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建单人房状态码%d", status)
	}
	var initial testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial)
	if !initial.Solo {
		t.Fatal("单人房应把 solo 标记返回给前端")
	}
	var readyView testView
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": initial.Version, "ready": true}, &readyView); status != http.StatusOK {
		t.Fatalf("单人房 ready 失败：%d", status)
	}
	if readyView.Turn != 2 {
		t.Fatalf("单人房 ready 应直接结算：turn=%d", readyView.Turn)
	}
}
