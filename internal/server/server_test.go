package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

type testView struct {
	Turn      int            `json:"turn"`
	Version   int64          `json:"version"`
	Units     []game.Unit    `json:"units"`
	You       store.Player   `json:"you"`
	Players   []store.Player `json:"players"`
	Orders    []game.Order   `json:"orders"`
	JoinToken string         `json:"join_token"`
}

type testClient struct {
	client *http.Client
	base   string
}

func newTestClient(t *testing.T, base string) *testClient {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{client: &http.Client{Jar: jar}, base: base}
}

func (c *testClient) call(t *testing.T, method, path string, input, output any) int {
	t.Helper()
	var body bytes.Buffer
	if input != nil {
		_ = json.NewEncoder(&body).Encode(input)
	}
	request, err := http.NewRequest(method, c.base+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if output != nil {
		_ = json.NewDecoder(response.Body).Decode(output)
	}
	return response.StatusCode
}

func setupTestServer(t *testing.T) (*testClient, string, *store.Store, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := New(database, "../../content", false)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	httpServer := httptest.NewServer(mux)
	cleanup := func() { httpServer.Close(); database.Close() }
	return newTestClient(t, httpServer.URL), path, database, cleanup
}

func TestTwoPlayerCommandAndTurnFlow(t *testing.T) {
	a, dbPath, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	if len(created.JoinToken) != 14 {
		t.Fatalf("邀请token格式错误：%s", created.JoinToken)
	}

	var initial testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial); status != http.StatusOK {
		t.Fatal("读取A状态失败")
	}
	if initial.You.Faction != "A" || initial.JoinToken == "" {
		t.Fatal("房主信息不完整")
	}
	for _, unit := range initial.Units {
		if unit.Faction == "B" {
			t.Fatal("A初始状态泄露远端敌舰")
		}
	}

	b := newTestClient(t, a.base)
	var joined roomResponse
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"name": "B", "token": created.JoinToken}, &joined); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	var bState testView
	_ = b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &bState)
	if bState.You.Faction != "B" || bState.JoinToken != "" {
		t.Fatal("B阵营或token权限错误")
	}
	for _, unit := range bState.Units {
		if unit.Faction == "A" {
			t.Fatal("B初始状态泄露远端敌舰")
		}
	}

	invalid := map[string]any{"command_id": "invalid", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-flag", "coord": "0404"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", invalid, nil); status != http.StatusBadRequest {
		t.Fatalf("非法移动状态码%d", status)
	}
	valid := map[string]any{"command_id": "move-1", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-flag", "coord": "0203"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", valid, nil); status != http.StatusOK {
		t.Fatal("合法移动被拒绝")
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", valid, nil); status != http.StatusOK {
		t.Fatal("幂等重试失败")
	}
	var ordered testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &ordered)
	if len(ordered.Orders) != 1 {
		t.Fatalf("幂等命令产生重复命令：%d", len(ordered.Orders))
	}

	var readyView testView
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 1, "ready": true}, &readyView); status != http.StatusOK || readyView.Version != 1 {
		t.Fatal("单方ready不应结算")
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 1, "ready": true}, &readyView); status != http.StatusOK {
		t.Fatal("双方ready结算失败")
	}
	if readyView.Turn != 2 || readyView.Version != 2 {
		t.Fatalf("回合版本错误：%+v", readyView)
	}
	found := false
	for _, unit := range readyView.Units {
		if unit.ID == "a-flag" && unit.Coord == "0203" {
			found = true
		}
	}
	if !found {
		t.Fatal("移动结果未写入私有快照")
	}

	stale := map[string]any{"command_id": "move-stale", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-flag", "coord": "0204"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", stale, nil); status != http.StatusConflict {
		t.Fatal("过期版本未被拒绝")
	}
	persisted, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer persisted.Close()
	room, err := persisted.Room(t.Context(), created.RoomID)
	if err != nil || room.Turn != 2 || room.State.Units["a-flag"].Coord != "0203" {
		t.Fatalf("持久化恢复失败：%+v %v", room, err)
	}

	third := newTestClient(t, a.base)
	if status := third.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken}, nil); status != http.StatusBadRequest {
		t.Fatal("满员房间允许第三人加入")
	}
}

func TestSoloReadyAndInvalidToken(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	_ = a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{}, &created)
	var view testView
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 1, "ready": true}, &view); status != http.StatusOK || view.Turn != 2 {
		t.Fatalf("单人沙盒未结算：%+v", view)
	}
	other := newTestClient(t, a.base)
	if status := other.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": "XXXX-XXXX-XXXX"}, nil); status != http.StatusBadRequest {
		t.Fatal("无效token未被拒绝")
	}
}
