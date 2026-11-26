package game

import "testing"

// 打击编队落地就离开任务组：算子回甲板/机库展示，任务组里只留还在空中的成员。
// 以前落地后还留着 TaskID，编队会以"任务整备中"的样子冒回打击任务栏。
func TestLandedStrikeAircraftLeavesFormation(t *testing.T) {
	state := coralCombatState(t)
	ids := carrierTaskCandidates(&state, "A", 2)
	if len(ids) < 2 {
		t.Fatalf("需要两架待命舰载机，实际 %v", ids)
	}
	ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids, Coord: "1611"})
	first := state.Aircraft[ids[0]]
	second := state.Aircraft[ids[1]]
	taskID := first.TaskID
	if taskID == "" {
		t.Fatal("打击任务没有建立")
	}
	// 全员升空 → 编队离场。
	for _, member := range taskAircraft(state, taskID) {
		member.Status = "attack"
		member.Launched = true
		member.Coord = "1611"
	}
	carrier := state.Units[first.CarrierID]
	if carrier == nil {
		t.Fatal("找不到母舰")
	}
	LandAircraft(&state, first, carrier)
	if state.Aircraft[ids[0]].TaskID != "" {
		t.Fatalf("落地后的算子应该退出打击编队，实际还挂着 %q", state.Aircraft[ids[0]].TaskID)
	}
	if got := state.Aircraft[ids[1]].TaskID; got != taskID {
		t.Fatalf("还在空中的成员应保留编队号 %q，实际 %q", taskID, got)
	}
	LandAircraft(&state, second, carrier)
	for _, task := range VisibleAviation(state, "A").Tasks {
		if task.ID == taskID {
			t.Fatalf("全员落地后编队不该还留在任务列表里：%+v", task)
		}
	}
}

// 下降回机库属于回收流程，编队阶段不能再说成"升降机提升中"。
func TestTaskPhaseReportsLoweringAsRecovering(t *testing.T) {
	carrier := &Unit{ID: "cv-1", Faction: "A", Type: "CV", Deck: &FlightDeck{Elevators: []*AircraftElevator{
		{ID: "el-1", Status: "lowering"},
		{ID: "el-2", Status: "lifting"},
	}}}
	state := State{Units: map[string]*Unit{"cv-1": carrier}}

	descending := &AirUnit{ID: "cap-1", CarrierID: "cv-1", Faction: "A", Status: "elevator", ElevatorID: "el-1", TaskKind: "cap"}
	if got := taskPhase(state, []*AirUnit{descending}); got != "recovering" {
		t.Fatalf("下降中的算子应报回收阶段，实际 %q", got)
	}
	lifting := &AirUnit{ID: "cap-2", CarrierID: "cv-1", Faction: "A", Status: "elevator", ElevatorID: "el-2", TaskKind: "cap"}
	if got := taskPhase(state, []*AirUnit{lifting}); got != "lifting" {
		t.Fatalf("提升中的算子仍应报提升阶段，实际 %q", got)
	}
}

// TestGroundTaskPhaseFollowsAirfieldFlow 机场编队的阶段要跟着真实流程走：
// 机库整备 → 滑行到跑道 → 跑道待命 → 升空。以前只有 service / active 两档，
// 飞机已经在跑道上待命，任务卡还写"任务整备中"。
func TestGroundTaskPhaseFollowsAirfieldFlow(t *testing.T) {
	state := coralCombatState(t)
	ids := groundTaskCandidates(&state, "af-a-moresby", 1)
	if len(ids) == 0 {
		t.Fatal("莫尔兹比机场没有待命飞机")
	}
	ApplyGroundAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids, Coord: "1611"})
	aircraft := state.GroundAir[ids[0]]
	taskID := aircraft.TaskID

	phaseOf := func() string {
		for _, task := range GroundAviationView(state, "A").Tasks {
			if task.ID == taskID {
				return task.Phase
			}
		}
		return ""
	}
	aircraft.Status = "service"
	if got := phaseOf(); got != "servicing" {
		t.Fatalf("机库整备阶段应为 servicing，实际 %q", got)
	}
	aircraft.Status = "deck"
	if got := phaseOf(); got != "taxiing" {
		t.Fatalf("滑行至跑道应为 taxiing，实际 %q", got)
	}
	aircraft.Status = "ready"
	if got := phaseOf(); got != "preparing" {
		t.Fatalf("跑道待命应为 preparing，实际 %q", got)
	}
	aircraft.Status = "attack"
	if got := phaseOf(); got != "active" {
		t.Fatalf("升空后应为 active，实际 %q", got)
	}
}
