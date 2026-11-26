package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialTestWS(t *testing.T, client *testClient, path string) *websocket.Conn {
	t.Helper()
	u, err := url.Parse(client.base + path)
	if err != nil {
		t.Fatal(err)
	}
	u.Scheme = "ws"
	headers := http.Header{}
	for _, c := range client.client.Jar.Cookies(mustURL(t, client.base)) {
		headers.Add("Cookie", c.Name+"="+c.Value)
	}
	conn, response, err := websocket.DefaultDialer.Dial(u.String(), headers)
	if err != nil {
		if response != nil {
			t.Fatalf("WS handshake status=%d: %v", response.StatusCode, err)
		}
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func readWSResponse(t *testing.T, conn *websocket.Conn, id string) wsResponse {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var msg wsResponse
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatal(err)
		}
		if msg.Type == "rpc_response" && msg.ID == id {
			return msg
		}
	}
}

func TestWebSocketRPCUsesRoomAPIAndEnforcesRoomScope(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	conn := dialTestWS(t, host, "/ws?room_id="+created.RoomID)
	_ = conn.WriteJSON(wsRequest{Type: "rpc_request", ID: "state-1", Method: http.MethodGet, Path: "/api/v1/rooms/" + created.RoomID + "/state"})
	state := readWSResponse(t, conn, "state-1")
	if state.Status != http.StatusOK {
		t.Fatalf("state status=%d body=%s", state.Status, state.Body)
	}
	var view map[string]json.RawMessage
	if err := json.Unmarshal(state.Body, &view); err != nil {
		t.Fatal(err)
	}
	var version int64
	_ = json.Unmarshal(view["version"], &version)
	if version != 1 {
		t.Fatalf("version=%d", version)
	}
	_ = conn.WriteJSON(wsRequest{Type: "rpc_request", ID: "other-room", Method: http.MethodGet, Path: "/api/v1/rooms/other-room/state"})
	denied := readWSResponse(t, conn, "other-room")
	if denied.Status != http.StatusForbidden {
		t.Fatalf("cross-room status=%d", denied.Status)
	}
	_ = conn.WriteJSON(wsRequest{Type: "rpc_request", ID: "spectate", Method: http.MethodGet, Path: "/api/v1/rooms/" + created.RoomID + "/spectate"})
	denied = readWSResponse(t, conn, "spectate")
	if denied.Status != http.StatusForbidden {
		t.Fatalf("spectate RPC status=%d", denied.Status)
	}
	_ = conn.WriteJSON(wsRequest{Type: "rpc_request", ID: "ready", Method: http.MethodPost, Path: "/api/v1/rooms/" + created.RoomID + "/ready", Body: json.RawMessage("{\"ready\":true}")})
	ready := readWSResponse(t, conn, "ready")
	if ready.Status != http.StatusOK {
		t.Fatalf("ready status=%d body=%s", ready.Status, ready.Body)
	}
}

func TestWebSocketSpectatorSubscribesByRoomAndToken(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	watcher := newTestClient(t, host.base) // no session cookie
	conn := dialTestWS(t, watcher, "/ws")
	_ = conn.WriteJSON(map[string]string{"type": "subscribe", "room_id": created.RoomID, "token": created.JoinToken})
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var envelope map[string]json.RawMessage
	if err := conn.ReadJSON(&envelope); err != nil {
		t.Fatal(err)
	}
	var kind string
	_ = json.Unmarshal(envelope["type"], &kind)
	if kind != "snapshot" {
		t.Fatalf("frame type=%q", kind)
	}
	var view map[string]any
	if err := json.Unmarshal(envelope["view"], &view); err != nil {
		t.Fatal(err)
	}
	if view["spectator"] != true {
		t.Fatalf("not a spectator view: %#v", view["spectator"])
	}
	var readyView testView
	if status := host.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, &readyView); status != http.StatusOK {
		t.Fatalf("ready status %d", status)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var next map[string]json.RawMessage
	if err := conn.ReadJSON(&next); err != nil {
		t.Fatal(err)
	}
	var nextType string
	_ = json.Unmarshal(next["type"], &nextType)
	if nextType != "snapshot" {
		t.Fatalf("update frame type=%q", nextType)
	}
	var nextView map[string]any
	_ = json.Unmarshal(next["view"], &nextView)
	if nextView["turn"] != float64(2) {
		t.Fatalf("spectator update turn=%v", nextView["turn"])
	}
}

func readWSEvent(t *testing.T, conn *websocket.Conn, eventType string) map[string]json.RawMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var envelope map[string]json.RawMessage
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatal(err)
		}
		var kind string
		_ = json.Unmarshal(envelope["type"], &kind)
		if kind == eventType {
			return envelope
		}
	}
}

// 玩家进入房间后由服务端主动推送变更：ready 触发结算时，
// 两名玩家的 WebSocket 都必须收到 turn_resolved，而不是靠客户端轮询发现。
func TestWebSocketPlayerReceivesTurnResolved(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	guest := newTestClient(t, host.base)
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B"}, nil); status != http.StatusOK {
		t.Fatalf("join room status %d", status)
	}
	hostConn := dialTestWS(t, host, "/ws?room_id="+created.RoomID)
	guestConn := dialTestWS(t, guest, "/ws?room_id="+created.RoomID)

	if err := hostConn.WriteJSON(wsRequest{Type: "rpc_request", ID: "ready-host", Method: http.MethodPost, Path: "/api/v1/rooms/" + created.RoomID + "/ready", Body: json.RawMessage(`{"ready":true}`)}); err != nil {
		t.Fatal(err)
	}
	if response := readWSResponse(t, hostConn, "ready-host"); response.Status != http.StatusOK {
		t.Fatalf("host ready status=%d body=%s", response.Status, response.Body)
	}
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("guest ready status %d", status)
	}

	for name, conn := range map[string]*websocket.Conn{"host": hostConn, "guest": guestConn} {
		event := readWSEvent(t, conn, "turn_resolved")
		var version int64
		_ = json.Unmarshal(event["version"], &version)
		if version != 2 {
			t.Fatalf("%s turn_resolved version=%d", name, version)
		}
	}
}

// 没有玩家 Cookie 的连接如果第一条发的是 RPC，说明会话已经失效：
// 必须回一个 401 rpc_response，前端才能读到"请先创建或加入房间"并回首页，
// 而不是被当成无效观战订阅后无限重连。
func TestWebSocketRejectsUnauthenticatedPlayerRPC(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	anonymous := newTestClient(t, host.base)
	conn := dialTestWS(t, anonymous, "/ws?room_id="+created.RoomID)
	_ = conn.WriteJSON(wsRequest{Type: "rpc_request", ID: "no-session", Method: http.MethodGet, Path: "/api/v1/rooms/" + created.RoomID + "/state"})
	response := readWSResponse(t, conn, "no-session")
	if response.Status != http.StatusUnauthorized {
		t.Fatalf("unauth RPC status=%d", response.Status)
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Error, "请先创建") {
		t.Fatalf("unauth RPC error=%q", body.Error)
	}
}
