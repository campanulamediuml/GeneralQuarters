package server

import (
	"net/http"
	"testing"
)

// 就绪即冻结：点了"完成部署"以后本回合不能再改任何东西。
// 以前这里会自动撤回 ready 再收下命令，等于"准备"只是个装饰：
// 对手看到双方都就绪了，结算出来的却是你又改过的版本。
func TestReadyFreezesCommands(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestPlayerRoom(t, a, "_coral_real", "A", false)
	b := newTestClient(t, a.base)
	var joined roomResponse
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]any{"name": "B", "token": created.JoinToken, "faction": "B"}, &joined); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	var initial testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial)
	// 先生效部署阶段，再验证"就绪即冻结"这条对局规则。
	settleDeployment(t, created.RoomID, a, b)

	// ready 只回一条回执，就绪标记要从状态里读。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("ready 失败：%d", status)
	}
	var readyView testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &readyView)
	if !readyView.You.Ready {
		t.Fatal("就绪标记没有生效")
	}

	// ready 不能撤销：必须拒绝 ready:false，且就绪标记保持。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": false}, nil); status != http.StatusBadRequest {
		t.Fatalf("ready:false 应被拒绝（400），实际 %d", status)
	}
	var stillReady testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &stillReady)
	if !stillReady.You.Ready {
		t.Fatal("ready:false 被拒绝后不应把 ready 撤掉")
	}

	// 就绪之后再发任何命令都必须被拒绝，而且不能悄悄把 ready 撤掉。
	var state testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &state)
	unitID := ""
	for _, unit := range state.Units {
		if unit.Faction == state.You.Faction && len(unit.LegalMoves) > 0 {
			unitID = unit.ID
			break
		}
	}
	if unitID == "" {
		t.Fatal("找不到可以移动的己方单位")
	}
	status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{
		"command_id": "freeze-move-1", "type": "SET_MOVE", "unit_id": unitID, "coord": "0202",
	}, nil)
	if status != http.StatusConflict {
		t.Fatalf("就绪后应拒绝命令（409），实际 %d", status)
	}
	var after testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &after)
	if !after.You.Ready {
		t.Fatal("被拒绝的命令不应该顺手把 ready 撤回")
	}
	if len(after.Orders) != 0 {
		t.Fatalf("被拒绝的命令不应该落成待结算命令：%+v", after.Orders)
	}

	// 对手就绪后正常结算，回合推进。
	var resolved testView
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, &resolved); status != http.StatusOK {
		t.Fatalf("双方就绪结算失败：%d", status)
	}
	if resolved.Turn != 2 {
		t.Fatalf("双方就绪后应推进到第2回合：%d", resolved.Turn)
	}
	if resolved.You.Ready {
		t.Fatal("新回合开始后应当解除就绪")
	}
}
