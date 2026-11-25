package server

import (
	"net/http"
	"testing"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

type spectateView struct {
	Turn      int                        `json:"turn"`
	Units     []game.Unit                `json:"units"`
	Fleets    []game.FleetGroup          `json:"fleets"`
	Airfields []game.AirfieldContact     `json:"airfields"`
	Aviation  game.AviationView          `json:"aviation"`
	Events    []store.Event              `json:"events"`
	Losses    map[string]game.LossLedger `json:"losses"`
	Spectator bool                       `json:"spectator"`
}

// TestSpectateMergesBothFactions 校验观战快照：两边各自的合法视图按单位归属合并，
// 且必须用 join_token 认证——玩家 Cookie 那套权限不能拿来偷看对方。
func TestSpectateMergesBothFactions(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_coral_real", "faction": "A"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	guest := newTestClient(t, host.base)
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B"}, nil); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	watcher := newTestClient(t, host.base)
	var view spectateView
	path := "/api/v1/rooms/" + created.RoomID + "/spectate?token=" + created.JoinToken
	if status := watcher.call(t, http.MethodGet, path, nil, &view); status != http.StatusOK {
		t.Fatalf("观战状态码%d", status)
	}
	if !view.Spectator {
		t.Fatal("观战快照没有标记 spectator")
	}
	sides := map[string]bool{}
	for _, unit := range view.Units {
		sides[unit.Faction] = true
	}
	if !sides["A"] || !sides["B"] {
		t.Fatalf("观战快照缺少某一方的舰船：%v", sides)
	}
	fleetSides := map[string]bool{}
	for _, fleet := range view.Fleets {
		fleetSides[fleet.Faction] = true
	}
	if !fleetSides["A"] || !fleetSides["B"] {
		t.Fatalf("观战快照缺少某一方的舰队：%v", fleetSides)
	}
	// 机场也要合并：珊瑚海剧本里 A 有莫尔兹比港、凯恩斯和鲍恩，B 有拉包尔、莱城、肖特兰和莱卡塔。
	airfieldSides := map[string]int{}
	for _, airfield := range view.Airfields {
		airfieldSides[airfield.Faction]++
	}
	if airfieldSides["A"] == 0 || airfieldSides["B"] == 0 {
		t.Fatalf("观战快照缺少某一方的机场：%v", airfieldSides)
	}
	if bad := watcher.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/spectate?token=XXXX-XXXX-XXXX", nil, nil); bad != http.StatusForbidden {
		t.Fatalf("错误 token 应该 403，实际 %d", bad)
	}
	// 没有 Cookie 的玩家接口仍然要求会话。
	if status := watcher.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, nil); status != http.StatusUnauthorized {
		t.Fatalf("未认证的 state 应该 401，实际 %d", status)
	}
}
