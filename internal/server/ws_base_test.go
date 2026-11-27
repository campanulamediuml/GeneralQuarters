package server

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

// 刷新页面时，服务端还留着上一条连接留下的底座；如果新连接一上来就收到补丁，
// 客户端会因为"没有底座"而关连接重连，而每次重连都会撞上同一帧，永远同步不上。
// 这里锁住协议：连接必须自己拉过一份全量，之后才允许收补丁。
func TestPlayerGetsNoPatchesBeforeFetchingBase(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建单人房状态码%d", status)
	}
	var initial testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial); status != http.StatusOK {
		t.Fatal("读取初始状态失败")
	}

	// 新连接（模拟刷新后的页面）：连上了，但还没来得及拉全量。
	fresh := dialTestWS(t, a, "/ws?room_id="+created.RoomID)
	placeSubmarines(t, a, created.RoomID)
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("ready 状态码%d", status)
	}
	_ = fresh.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	for {
		_, data, err := fresh.ReadMessage()
		if err != nil {
			break // 超时：没收到任何东西，符合预期。
		}
		if bytes.Contains(data, []byte("\"type\":\"state\"")) {
			t.Fatalf("还没拉底座的连接不该收到补丁：%s", data)
		}
	}

	// 拉过底座的连接应该照常收到补丁。
	grounded := dialTestWS(t, a, "/ws?room_id="+created.RoomID)
	_ = grounded.WriteJSON(wsRequest{Type: "rpc_request", ID: "state-1", Method: http.MethodGet, Path: "/api/v1/rooms/" + created.RoomID + "/state"})
	readWSResponse(t, grounded, "state-1")
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("第二次 ready 状态码%d", status)
	}
	_ = grounded.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, data, err := grounded.ReadMessage()
		if err != nil {
			t.Fatalf("拉过底座之后应该能收到补丁：%v", err)
		}
		if bytes.Contains(data, []byte("\"type\":\"state\"")) {
			return
		}
	}
}
