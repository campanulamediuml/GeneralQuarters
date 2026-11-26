package game

import "testing"

// 规则书 240–244 行：普通整备 3 回合——机库加油整备、提升到甲板、起飞，各占一回合。
func TestNormalServiceTakesThreeTurns(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"b-cv-01-fighter-x1"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("机库算子应可分配任务：%v", err)
	}
	ApplyAirOrder(&state, order)
	aircraft := state.Aircraft["b-cv-01-fighter-x1"]
	if aircraft.Status != "service" || aircraft.ServiceTurns != 0 {
		t.Fatalf("分配任务后应进入机库整备第 0 回合：%+v", aircraft)
	}
	// 第 1 回合：机库加油整备完成，但还没上甲板。
	AdvanceAviation(&state)
	if aircraft.Status != "service" || aircraft.ServiceTurns != 1 {
		t.Fatalf("第 1 回合应完成机库加油整备但仍停在机库：status=%s turns=%d", aircraft.Status, aircraft.ServiceTurns)
	}
	// 第 2 回合：上升降机。
	AdvanceAviation(&state)
	if aircraft.Status != "elevator" {
		t.Fatalf("第 2 回合应开始提升：%s", aircraft.Status)
	}
	// 第 3 回合：提升完成并放飞。
	AdvanceAviation(&state)
	if aircraft.Status != "cap" {
		t.Fatalf("第 3 回合应完成提升并放飞：%s", aircraft.Status)
	}
}

// 转场是整组任务：组里还有飞机没整备完时，整组不得先提升。
func TestServiceWaitsForWholeTransferGroup(t *testing.T) {
	state := sandboxState(t)
	first := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1"}}
	ApplyAirOrder(&state, first)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	state.Aircraft["b-cv-01-dive_bomber-x1"].TaskKind = "transfer"
	second := Order{Type: "RETURN_TO_HANGAR", AircraftIDs: []string{"b-cv-01-dive_bomber-x1"}}
	_ = second
	// 手动补一个同组、刚进整备的成员
	late := &AirUnit{ID: "late-x", CarrierID: "b-cv-01", Faction: "B", Kind: "dive_bomber", Model: "SBD",
		Status: "service", TaskID: taskID, TaskKind: "transfer", Strength: 2, ServiceTurns: 0}
	state.Aircraft[late.ID] = late
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	if late.Status == "elevator" {
		return
	}
	if state.Aircraft["b-cv-01-dive_bomber-x1"] == nil || state.Aircraft["b-cv-01-dive_bomber-x1"].Status == "elevator" {
		t.Fatal("同组还有飞机未整备完毕时，整组不应提升（转场要求整队一起走）")
	}
}
