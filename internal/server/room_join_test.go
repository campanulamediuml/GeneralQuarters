package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCreateEmptyRoomThenJoinSymmetricFactions(t *testing.T) {
	coordinator, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	response, err := http.Post(coordinator.base+"/api/v1/rooms", "application/json", strings.NewReader(`{"scenario_id":"_fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", response.StatusCode)
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("创建空房间不应建立玩家会话")
	}
	var created roomResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.RoomID == "" || created.JoinToken == "" || created.PlayerID != "" || created.Faction != "" {
		t.Fatalf("创建响应不应包含玩家席位：%+v", created)
	}

	b := newTestClient(t, coordinator.base)
	joinedB := joinTestRoom(t, b, created, "B")
	if joinedB.RoomID != created.RoomID || joinedB.Faction != "B" || joinedB.PlayerID == "" || joinedB.JoinToken != "" {
		t.Fatalf("B加入响应错误：%+v", joinedB)
	}
	duplicate := newTestClient(t, coordinator.base)
	if status := duplicate.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "faction": "B"}, nil); status != http.StatusConflict {
		t.Fatalf("重复加入B阵营应返回409，实际%d", status)
	}
	a := newTestClient(t, coordinator.base)
	joinedA := joinTestRoom(t, a, created, "A")
	if joinedA.PlayerID == joinedB.PlayerID || joinedA.Faction != "A" {
		t.Fatalf("A/B席位不是独立玩家：A=%+v B=%+v", joinedA, joinedB)
	}

	for name, test := range map[string]struct {
		client  *testClient
		faction string
	}{"A": {a, "A"}, "B": {b, "B"}} {
		var view testView
		if status := test.client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK {
			t.Fatalf("读取%s状态失败：%d", name, status)
		}
		if view.You.Faction != test.faction || view.You.IsHost || view.JoinToken != created.JoinToken {
			t.Fatalf("%s席位权限不对等：%+v token=%q", name, view.You, view.JoinToken)
		}
	}
}

func TestJoinRoomRequiresFactionAndRejectsLegacyCreateFields(t *testing.T) {
	client, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestRoom(t, client, "_fixture", false)
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken}, nil); status != http.StatusBadRequest {
		t.Fatalf("缺少阵营应返回400，实际%d", status)
	}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "faction": "C"}, nil); status != http.StatusBadRequest {
		t.Fatalf("非法阵营应返回400，实际%d", status)
	}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"scenario_id": "_fixture", "name": "旧房主", "faction": "A"}, nil); status != http.StatusBadRequest {
		t.Fatalf("创建房间仍接受旧房主字段，状态码%d", status)
	}
}

func TestSoloRoomRejectsSecondSeat(t *testing.T) {
	client, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestRoom(t, client, "_fixture", true)
	joinTestRoom(t, client, created, "B")
	other := newTestClient(t, client.base)
	if status := other.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "faction": "A"}, nil); status != http.StatusConflict {
		t.Fatalf("单人房第二席位应返回409，实际%d", status)
	}
}
