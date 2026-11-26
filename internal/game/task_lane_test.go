package game

import "testing"

// 面板栏位（打击任务 I/II）属于"正在组队的编队"：编队全部升空之后栏位交还，
// 下一支新编队应该重新拿回 I。以前前端按"任务在列表里的位置 % 2"分栏，
// 已出发的编队还挂在列表里照样占一个位置，新编队就只能落到 II。
func TestGroundTaskLaneFreesAfterDeparture(t *testing.T) {
	state := coralCombatState(t)
	ids := groundTaskCandidates(&state, "af-a-moresby", 3)
	if len(ids) < 3 {
		t.Fatalf("莫尔兹比机场需要至少 3 架待命飞机，实际 %v", ids)
	}
	applyStrike := func(id string, lane int) {
		order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{id}, Coord: "1611", Lane: lane}
		ApplyGroundAirOrder(&state, order)
		if state.GroundAir[id].TaskID == "" {
			t.Fatalf("%s 的打击任务没有生效", id)
		}
	}

	applyStrike(ids[0], 0)
	first := state.GroundAir[ids[0]]
	if first.TaskLane != 1 {
		t.Fatalf("第一支编队应占 I 栏，实际 %d", first.TaskLane)
	}
	// 全组升空：栏位该交还。
	for _, member := range groundTaskAircraft(state, first.TaskID) {
		member.Status = "attack"
		member.Coord = "1611"
	}

	applyStrike(ids[1], 0)
	second := state.GroundAir[ids[1]]
	if second.TaskID == first.TaskID {
		t.Fatal("新编队不能并进已经出发的编队")
	}
	if second.TaskLane != 1 {
		t.Fatalf("上一支飞走之后新编队应重新拿 I 栏，实际 %d", second.TaskLane)
	}

	// 两支都还在组队：一支 I、一支 II，编号不重叠。
	applyStrike(ids[2], 0)
	third := state.GroundAir[ids[2]]
	if third.TaskLane != 2 {
		t.Fatalf("第二支在组队的编队应占 II 栏，实际 %d", third.TaskLane)
	}
}

// 拖进哪一栏就放哪一栏：空着的 II 栏被点名时不该被"最小空位"规则塞回 I。
func TestGroundTaskLaneHonorsPlayerChoice(t *testing.T) {
	state := coralCombatState(t)
	ids := groundTaskCandidates(&state, "af-a-moresby", 1)
	if len(ids) == 0 {
		t.Fatal("莫尔兹比机场没有待命飞机")
	}
	ApplyGroundAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids, Coord: "1611", Lane: 2})
	if lane := state.GroundAir[ids[0]].TaskLane; lane != 2 {
		t.Fatalf("玩家指定 II 栏时应放在 II，实际 %d", lane)
	}
	// 已经被占的栏位再点名也没用：退回最小空位（I）。
	ids = groundTaskCandidates(&state, "af-a-moresby", 3)
	ApplyGroundAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids[2:], Coord: "1611", Lane: 2})
	if lane := state.GroundAir[ids[2]].TaskLane; lane != 1 {
		t.Fatalf("II 栏已被占时应退回 I 栏，实际 %d", lane)
	}
}

// 视图要把栏位带给前端，否则前端只能继续按位置轮排。
func TestGroundTaskLaneAppearsInView(t *testing.T) {
	state := coralCombatState(t)
	ids := groundTaskCandidates(&state, "af-a-moresby", 1)
	ApplyGroundAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids, Coord: "1611"})
	taskID := state.GroundAir[ids[0]].TaskID
	found := false
	for _, task := range GroundAviationView(state, "A").Tasks {
		if task.ID != taskID {
			continue
		}
		found = true
		if task.Lane != state.GroundAir[ids[0]].TaskLane || task.Lane <= 0 {
			t.Fatalf("任务卡应带出栏位：task.lane=%d aircraft.task_lane=%d", task.Lane, state.GroundAir[ids[0]].TaskLane)
		}
	}
	if !found {
		t.Fatal("机场任务没有出现在投影里")
	}
}

// 舰载机同一套规则：编队离场后栏位交还，新编队拿回 I。
func TestCarrierTaskLaneFreesAfterDeparture(t *testing.T) {
	state := coralCombatState(t)
	ids := carrierTaskCandidates(&state, "A", 2)
	if len(ids) < 2 {
		t.Fatalf("需要至少 2 架待命舰载机，实际 %v", ids)
	}
	applyStrike := func(id string) {
		order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{id}, Coord: "1611"}
		ApplyAirOrder(&state, order)
		if state.Aircraft[id].TaskID == "" {
			t.Fatalf("%s 的打击任务没有生效：status=%s", id, state.Aircraft[id].Status)
		}
	}
	applyStrike(ids[0])
	first := state.Aircraft[ids[0]]
	if first.TaskLane != 1 {
		t.Fatalf("第一支编队应占 I 栏，实际 %d", first.TaskLane)
	}
	for _, member := range taskAircraft(state, first.TaskID) {
		member.Status = "attack"
		member.Launched = true
		member.Coord = "1611"
	}
	applyStrike(ids[1])
	second := state.Aircraft[ids[1]]
	if second.TaskID == first.TaskID {
		t.Fatal("新编队不能并进已经出发的编队")
	}
	if second.TaskLane != 1 {
		t.Fatalf("上一支飞走之后新编队应重新拿 I 栏，实际 %d", second.TaskLane)
	}
}

// groundTaskCandidates 挑出指定机场里还没上天的飞机。
func groundTaskCandidates(state *State, airfieldID string, want int) []string {
	ids := []string{}
	for _, id := range sortedGroundAirIDsByFaction(*state, "A") {
		aircraft := state.GroundAir[id]
		if aircraft.AirfieldID != airfieldID || airfieldAirborne(aircraft) {
			continue
		}
		ids = append(ids, id)
		if len(ids) >= want {
			break
		}
	}
	return ids
}

// carrierTaskCandidates 挑出还在机库里的己方舰载机。
func carrierTaskCandidates(state *State, faction string, want int) []string {
	ids := []string{}
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft.Faction != faction || aircraft.Status != "hangar" || aircraft.CarrierID == "" {
			continue
		}
		ids = append(ids, id)
		if len(ids) >= want {
			break
		}
	}
	return ids
}
