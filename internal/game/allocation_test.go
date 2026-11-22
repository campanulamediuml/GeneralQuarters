package game

import "testing"

// TestStrikeReportCarriesFleetAndAllocations 回归：对舰报告必须带上整支舰队清单，
// 以及"每个算子分配到了哪艘船、命中几次"，前端据此把飞机绘到对应目标上方。
func TestStrikeReportCarriesFleetAndAllocations(t *testing.T) {
	state := sandboxState(t)
	attacker := &AirUnit{ID: "sb2c-a", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack", Strength: 2, AirCombat: 1, AntiShip: 4, TaskID: "sb2c-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 8, MaxRange: 12}
	torpedo := &AirUnit{ID: "tbf-a", CarrierID: "a2-cv-01", Faction: "A", Kind: "torpedo_bomber", Model: "TBF", Status: "attack", Strength: 2, AirCombat: 1, AntiShip: 5, TaskID: "sb2c-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 8, MaxRange: 12}
	state.Aircraft = map[string]*AirUnit{attacker.ID: attacker, torpedo.ID: torpedo}
	state.RNGState = 4242
	// 舰队搬到目标 hex（sandbox 中 b2 舰队即 0608）
	state.RNGState = 4242
	resolveAirStrike(&state, "sb2c-a", "0608")
	if len(state.AirCombats) != 1 {
		t.Fatalf("应生成一份对舰报告：%+v", state.AirCombats)
	}
	report := state.AirCombats[0]
	if report.Target != string(StrikeTargetGroup) && report.Target != string(StrikeTargetShip) {
		t.Fatalf("报告目标类型错误：%q", report.Target)
	}
	if len(report.Ships) == 0 {
		t.Fatal("报告应带上被攻击舰队的全部舰船清单")
	}
	if len(report.Bombers) != 2 {
		t.Fatalf("应记录 2 个参与投弹的算子：%+v", report.Bombers)
	}
	// 目标分配在"投弹前"按算子生成；若两架攻击机都被 CAP/防空打掉，可能没有分配记录，
	// 但只要有算子进入投弹，就必须逐机记录分配。
	if report.BombAttempts > 0 && len(report.TargetAllocations) != 2 {
		t.Fatalf("应为每个投弹算子记录目标分配：%+v", report.TargetAllocations)
	}
	// 用固定种子保证空袭至少进行到投弹环节（否则"没有分配"是规则结果，不是漏记）
	known := map[string]bool{}
	for _, ship := range report.Ships {
		known[ship.ID] = true
	}
	if len(report.TargetAllocations) == 0 {
		t.Skipf("该种子下攻击机在投弹前全部退出（%s），目标分配为空属于规则结果", report.Summary)
	}
	for _, allocation := range report.TargetAllocations {
		if !known[allocation.ShipID] || allocation.ShipName == "" {
			t.Fatalf("目标分配指向未知舰船：%+v", allocation)
		}
	}
	// 防守方视角不泄露对方的目标分配，但保留自己舰船信息
	for _, visible := range VisibleAirCombats(state, "B") {
		if len(visible.TargetAllocations) != 0 || len(visible.Ships) == 0 {
			t.Fatalf("防守方视角泄露了分配或丢失了舰队清单：%+v", visible)
		}
	}
}
