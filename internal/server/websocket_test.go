package server

import (
	"bytes"
	"encoding/json"
	"io"
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
	var turn int
	_ = json.Unmarshal(view["turn"], &turn)
	if turn != 1 {
		t.Fatalf("turn=%d", turn)
	}
	if _, ok := view["version"]; ok {
		t.Fatal("协议里不该再有 version：状态变更是靠补丁下发的")
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
	view := readStateView(t, conn, 1)
	if reader := viewReader(conn); reader.kind != "snapshot" {
		t.Fatalf("frame type=%q", reader.kind)
	}
	if view["spectator"] != true {
		t.Fatalf("not a spectator view: %#v", view["spectator"])
	}
	var readyView testView
	if status := host.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, &readyView); status != http.StatusOK {
		t.Fatalf("ready status %d", status)
	}
	nextView := readStateView(t, conn, 2)
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

// ready 触发结算后，两名玩家都应该被直接推一份新的阵营私有状态，
// 而不是收到一条失效提示再自己发 RPC 去拉。
func TestWebSocketPlayersReceivePushedState(t *testing.T) {
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
	seedPlayerView(t, hostConn, created.RoomID)
	seedPlayerView(t, guestConn, created.RoomID)

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
		view := readStateView(t, conn, 2)
		if view["turn"] != float64(2) {
			t.Fatalf("%s 推过来的仍是第 %v 回合", name, view["turn"])
		}
		if view["you"] == nil || view["orders"] == nil {
			t.Fatalf("%s 推过来的不是私有视图", name)
		}
	}
}

// 对手进房也算"视图变了"：房间版本号没有变化，但玩家的在线状态变了，
// 推送必须照发，不能被"内容没变就不发"的判重吃掉。
func TestWebSocketPresencePushesState(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	hostConn := dialTestWS(t, host, "/ws?room_id="+created.RoomID)
	if view := seedPlayerView(t, hostConn, created.RoomID); len(view["players"].([]any)) != 1 {
		t.Fatalf("初始玩家数=%d", len(view["players"].([]any)))
	}
	guest := newTestClient(t, host.base)
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B"}, nil); status != http.StatusOK {
		t.Fatalf("join room status %d", status)
	}
	readStateViewWhere(t, hostConn, func(view map[string]any) bool {
		players, ok := view["players"].([]any)
		return ok && len(players) == 2
	})
}

// 命令落库之后，服务端必须直接把新状态推给这名玩家：
// 客户端不再“提交完再自己拉一次”。推过来的 orders 里应该有刚才那条命令。
func TestWebSocketCommandPushesState(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	conn := dialTestWS(t, host, "/ws?room_id="+created.RoomID)
	seedPlayerView(t, conn, created.RoomID)
	_ = conn.WriteJSON(wsRequest{Type: "rpc_request", ID: "move", Method: http.MethodPost, Path: "/api/v1/rooms/" + created.RoomID + "/commands", Body: json.RawMessage(`{"command_id":"ws-move","type":"SET_MOVE","unit_id":"a-bb-01","coord":"0203"}`)})
	if response := readWSResponse(t, conn, "move"); response.Status != http.StatusOK {
		t.Fatalf("command status=%d body=%s", response.Status, response.Body)
	}
	merged := readStateViewWhere(t, conn, func(view map[string]any) bool {
		orders, ok := view["orders"].([]any)
		if !ok || len(orders) != 1 {
			return false
		}
		order, _ := orders[0].(map[string]any)
		return order != nil && order["coord"] == "0203"
	})
	if orders := merged["orders"].([]any); len(orders) != 1 {
		t.Fatalf("推过来的命令数=%d", len(orders))
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

// 推送靠"和上一帧逐字节相同就不发"去掉空转，所以同一份状态两次投影必须一模一样。
// 以前视图里带了一个 server_time，每次都不同，判重永远命中不了——这条测试盯住这个前提。
func TestStateViewIsDeterministic(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	first := rawState(t, host, created.RoomID)
	second := rawState(t, host, created.RoomID)
	if !bytes.Equal(first, second) {
		t.Fatalf("同一份状态两次投影不一致: %s / %s", first, second)
	}
}

func rawState(t *testing.T, client *testClient, roomID string) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, client.base+"/api/v1/rooms/"+roomID+"/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("state status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
