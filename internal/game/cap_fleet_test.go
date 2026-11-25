package game

import "testing"

// CAP 是舰队级任务：翔鹤、瑞鹤同属 MO，同一批 A6M 应该能一起发 CAP，
// 不应被“必须同一母舰”拦住。
func TestCarrierCapAllowsMultipleCarriersInSameFleet(t *testing.T) {
	state := coralCombatState(t)
	state.Night = false
	state.Turn = 30 // 白天整备窗口
	ids := []string{"b-cv-01-fighter-x1", "b-cv-02-fighter-x1"}
	for _, id := range ids {
		plane := state.Aircraft[id]
		if plane == nil {
			t.Fatalf("找不到 %s", id)
		}
		plane.Status = "hangar"
		plane.TaskID = ""
		plane.TaskKind = ""
		plane.Damaged = false
		plane.Maintenance = 0
	}
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: ids}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("同舰队跨母舰 CAP 应被允许：%v", err)
	}
}
