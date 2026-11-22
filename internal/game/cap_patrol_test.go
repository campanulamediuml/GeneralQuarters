package game

import "testing"

// TestCAPProtectsSevenHexesAroundPatrol 规则：CAP 掩护自己所在 hex 及周围 1 hex（共 7 格），判定按巡逻点。
func TestCAPProtectsSevenHexesAroundPatrol(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "cap-1", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Model: "A6M", Status: "cap",
		Strength: 2, AirCombat: 5, TaskID: "cap-b", TaskKind: "cap", PatrolCoord: "0608", RemainingRange: 10, MaxRange: 13}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.Units["b-cv-01"].Coord = "0608" // CAP 现在锚定母舰当前位置
	// 巡逻点及周围 1 hex 内的攻击编队会被拦截
	if len(strikeCAPGroups(state, "B", "0608")) != 1 {
		t.Fatal("巡逻点上空应被 CAP 掩护")
	}
	neighbors := Neighbors("0608")
	if len(neighbors) != 6 {
		t.Fatalf("hex 周围应有 6 个邻格，实际 %d", len(neighbors))
	}
	for _, coord := range neighbors {
		if len(strikeCAPGroups(state, "B", coord)) != 1 {
			t.Fatalf("邻格 %s 应被 CAP 掩护", coord)
		}
	}
	if len(strikeCAPGroups(state, "B", "0405")) != 0 {
		t.Fatal("超过 1 hex 不应被掩护")
	}
}

// TestRadarContactForwardsCAP 雷达截获成功时 CAP 自动前出最多 2 hex 拦截，并记录前出点。
func TestRadarContactForwardsCAP(t *testing.T) {
	state := sandboxState(t)
	attacker := &AirUnit{ID: "atk", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack",
		Strength: 2, AirCombat: 1, AntiShip: 4, TaskID: "atk", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 10, MaxRange: 12}
	cap := &AirUnit{ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Model: "A6M", Status: "cap",
		Strength: 2, AirCombat: 5, TaskID: "cap-b", TaskKind: "cap", PatrolCoord: "0608", RemainingRange: 12, MaxRange: 13}
	state.Aircraft = map[string]*AirUnit{attacker.ID: attacker, cap.ID: cap}
	state.Units["b-cv-01"].Coord = "0608"
	state.RNGState = 1
	events := interceptIncoming(&state, "A", "0608", []*AirUnit{attacker}, "B")
	forwarded := false
	for _, event := range events {
		if event.Type == "cap_intercepted" {
			forwarded = true
		}
	}
	if !forwarded {
		t.Fatalf("雷达截获应触发 CAP 前出拦截：%v", events)
	}
	if cap.ForwardCoord == "" && state.Aircraft[cap.ID] != nil {
		t.Fatal("前出拦截后应记录前出点（除非被击落）")
	}
	if len(state.AirCombats) != 1 || state.AirCombats[0].Target != "intercept" {
		t.Fatalf("应生成前出拦截报告：%+v", state.AirCombats)
	}
}

// TestCAPReturnsToPatrol 前出拦截后 CAP 自动返回巡逻点。
func TestCAPReturnsToPatrol(t *testing.T) {
	state := sandboxState(t)
	cap := &AirUnit{ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Model: "A6M", Status: "cap",
		Strength: 2, AirCombat: 5, TaskID: "cap-b", TaskKind: "cap", PatrolCoord: "0608", ForwardCoord: "0609", RemainingRange: 12, MaxRange: 13}
	state.Aircraft = map[string]*AirUnit{cap.ID: cap}
	state.Units["b-cv-01"].Coord = "0608"
	AdvanceAviation(&state)
	if cap.ForwardCoord != "" {
		t.Fatalf("前出 1 hex 应一回合内返回巡逻点：%s", cap.ForwardCoord)
	}
}

// TestCAPMissionCycleRestarts CAP 降落后自动重新整备（任务循环）。
func TestCAPMissionCycleRestarts(t *testing.T) {
	state := sandboxState(t)
	cap := &AirUnit{ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Model: "A6M", Status: "cap",
		Strength: 2, AirCombat: 5, TaskID: "cap-b", TaskKind: "cap", Coord: "0208", Returning: true, RemainingRange: 3, MaxRange: 13}
	state.Aircraft = map[string]*AirUnit{cap.ID: cap}
	carrier := state.Units["b-cv-01"]
	carrier.Deck.Operation = ""
	AdvanceAviation(&state)
	// 返航 → 着舰(recovery) → 升降机下降(elevator) → 机库(hangar) 任一步都算进入回收流程
	switch cap.Status {
	case "recovery", "elevator", "hangar", "service":
	default:
		t.Fatalf("CAP 返航后应进入回收流程：%s", cap.Status)
	}
	if !cap.OnCycle {
		t.Fatal("CAP 应标记为任务循环")
	}
}
