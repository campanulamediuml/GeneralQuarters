package server

import (
	"net/http"
	"testing"
)

func fetchViewMap(t *testing.T, client *testClient, roomID string) map[string]any {
	t.Helper()
	var view map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatalf("读取状态失败：%d", status)
	}
	return view
}

// 部署阶段整条链路：舰队在部署区里出生、只能拖到区内、双方都点部署完毕才进第 1 回合。
func TestDeployPhaseFlow(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestPlayerRoom(t, a, "_coral_real", "A", false)
	b := newTestClient(t, a.base)
	var joined roomResponse
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]any{"name": "B", "token": created.JoinToken, "faction": "B"}, &joined); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}

	view := fetchViewMap(t, a, created.RoomID)
	if view["phase"] != "deploy" {
		t.Fatalf("新房间应处于部署阶段，实际 %v", view["phase"])
	}
	zone, _ := view["deploy_zone"].(map[string]any)
	if zone == nil {
		t.Fatal("部署阶段应下发 deploy_zone")
	}
	if anchor, _ := zone["anchor"].(string); anchor != "3250" {
		t.Fatalf("美军部署区锚点应为 3250，实际 %v", zone["anchor"])
	}
	cells, _ := zone["cells"].(string)
	if len(cells) != 52*64 {
		t.Fatalf("部署区格位串长度不对：%d", len(cells))
	}

	// 拿一支己方舰队和它的合法落点（部署阶段就是整片部署区）。
	you, _ := view["you"].(map[string]any)
	faction, _ := you["faction"].(string)
	unitID, from, target := "", "", ""
	for _, raw := range view["units"].([]any) {
		unit, _ := raw.(map[string]any)
		if unit["faction"] != faction {
			continue
		}
		moves, _ := unit["legal_moves"].([]any)
		if len(moves) == 0 {
			continue
		}
		unitID, _ = unit["id"].(string)
		from, _ = unit["coord"].(string)
		for _, move := range moves {
			if coord, _ := move.(string); coord != from {
				target = coord
				break
			}
		}
		if target != "" {
			break
		}
	}
	if unitID == "" || target == "" {
		t.Fatal("找不到可部署的己方舰队")
	}

	// 区外（日军部署区附近）必须被拒绝。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{
		"command_id": "deploy-out-1", "type": "SET_MOVE", "unit_id": unitID, "coord": "0831",
	}, nil); status != http.StatusBadRequest {
		t.Fatalf("部署区外移动应被拒绝（400），实际 %d", status)
	}

	// 区内可以整片拖：立刻生效，没有冷却。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{
		"command_id": "deploy-in-1", "type": "SET_MOVE", "unit_id": unitID, "coord": target,
	}, nil); status != http.StatusOK {
		t.Fatalf("部署区内移动应被接受，实际 %d", status)
	}
	moved := fetchViewMap(t, a, created.RoomID)
	if !ownFleetAt(moved, faction, target) {
		t.Fatalf("部署移动没有生效，舰队仍在原位")
	}

	// 潜艇也要摆完才能点"部署完毕"。
	placeSubmarines(t, a, created.RoomID)

	// 点过"部署完毕"就是部署结束：之后的部署操作一律拒绝，ready 也不能被撤回。
	// 以前这里允许再拖一次并自动撤回 ready，玩家手滑挪一下舰队，准备状态就无声消失了。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("部署完毕失败：%d", status)
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{
		"command_id": "deploy-in-2", "type": "SET_MOVE", "unit_id": unitID, "coord": from,
	}, nil); status != http.StatusConflict {
		t.Fatalf("部署完毕之后的部署操作应被锁定（409），实际 %d", status)
	}
	afterMove := fetchViewMap(t, a, created.RoomID)
	youAfter, _ := afterMove["you"].(map[string]any)
	if ready, _ := youAfter["ready"].(bool); !ready {
		t.Fatal("被拒绝的部署操作不应撤回「部署完毕」")
	}
	if !ownFleetAt(afterMove, faction, target) {
		t.Fatal("被拒绝的部署操作不应真的移动舰队")
	}

	// 双方都点部署完毕：进入第 1 回合，回合号不变。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("部署完毕失败：%d", status)
	}
	placeSubmarines(t, b, created.RoomID)
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("部署完毕失败：%d", status)
	}
	battle := fetchViewMap(t, a, created.RoomID)
	if battle["phase"] != "battle" {
		t.Fatalf("双方部署完毕后应开战，实际 %v", battle["phase"])
	}
	if turn, _ := battle["turn"].(float64); int(turn) != 1 {
		t.Fatalf("部署结束应停在第 1 回合（部署不算回合），实际 %v", battle["turn"])
	}

	// 开战之后 ready 恢复"就绪即结算"的语义。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("ready 失败：%d", status)
	}
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("ready 失败：%d", status)
	}
	resolved := fetchViewMap(t, a, created.RoomID)
	if turn, _ := resolved["turn"].(float64); int(turn) != 2 {
		t.Fatalf("开战后双方 ready 应结算到第 2 回合，实际 %v", resolved["turn"])
	}
}

// ownFleetAt 判断己方是否有舰队停在某个 hex。
func ownFleetAt(view map[string]any, faction, coord string) bool {
	fleets, _ := view["fleets"].([]any)
	for _, raw := range fleets {
		fleet, _ := raw.(map[string]any)
		if fleet["faction"] != faction || fleet["coord"] != coord {
			continue
		}
		return true
	}
	return false
}
