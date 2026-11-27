package game

import "testing"

// 甲板上的空载算子没有油弹，不能直接出击：拖进任务栏要先在甲板整备一轮。
func TestEmptyDeckAircraftServicesBeforeLaunch(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	plane.Status = "deck"
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("空载甲板算子拖进任务栏被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	if plane.Status != "service" || !plane.DeckService {
		t.Fatalf("空载甲板算子应先在甲板整备：status=%s deck_service=%v", plane.Status, plane.DeckService)
	}
	if plane.TaskID == "" || plane.TaskKind != "cap" {
		t.Fatalf("任务应当记在这架飞机上：%+v", plane)
	}
	if used := deckUsed(state, "a2-cv-01"); used != 1 {
		t.Fatalf("甲板整备要占甲板位，实际 %d", used)
	}
	// 这一回合还在甲板整备，飞不了。
	AdvanceAviation(&state)
	if plane.Status != "service" || !plane.DeckService {
		t.Fatalf("甲板整备第一回合不该放飞：status=%s", plane.Status)
	}
	// 下一回合整备完成，当回合转成甲板待发并放飞。
	AdvanceAviation(&state)
	if plane.Status != "cap" {
		t.Fatalf("甲板整备完成后应当放飞，实际 %s", plane.Status)
	}
}

// 甲板上已经整备好的算子拖进任务栏就是"整备完毕、直接出击"，不再多等一轮。
func TestReadyDeckAircraftLaunchesImmediately(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "cap", "a2-cv-01-fighter-x1")
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("整备好的甲板算子拖进任务栏被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	if plane.Status != "cap" {
		t.Fatalf("整备好的甲板算子应当立即升空，实际 %s", plane.Status)
	}
}

// 打击编队混着整备好的和没整备的：整备好的必须在机库等，等全组都整备完再一起提升。
func TestStrikeGroupWaitsForWholeGroupService(t *testing.T) {
	state := sandboxState(t)
	ready := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	slow := state.Aircraft["a2-cv-01-dive_bomber-x2"]
	applyArm(t, &state, []string{ready.ID}, LoadoutBombTank)
	advanceAircraftService(&state)
	if ready.ServiceTurns != 1 {
		t.Fatalf("前置整备没完成：%d", ready.ServiceTurns)
	}
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{ready.ID, slow.ID}, Coord: "0708"}
	ApplyAirOrder(&state, order)
	if ready.Status != "service" || slow.Status != "service" {
		t.Fatalf("编队应当一起在机库整备：%s / %s", ready.Status, slow.Status)
	}
	// 整备好的那架不能先上甲板：同组还有没整备完的。
	scheduleElevators(&state)
	if ready.Status != "service" || slow.Status != "service" {
		t.Fatalf("整组没整备完时不该先提升：%s / %s", ready.Status, slow.Status)
	}
	// 下一回合另一架整备完成，两架一起进升降机。
	scheduleElevators(&state)
	if slow.ServiceTurns != 1 {
		t.Fatalf("另一架整备没完成：%d", slow.ServiceTurns)
	}
	if ready.Status != "elevator" || slow.Status != "elevator" {
		t.Fatalf("全组整备完成后应一起提升：%s / %s", ready.Status, slow.Status)
	}
}

// CAP/侦察是单机快反任务：同批里整备好的先走，不等别人。
func TestCapGroupDoesNotWaitForOthers(t *testing.T) {
	state := sandboxState(t)
	first := state.Aircraft["a2-cv-01-fighter-x1"]
	second := state.Aircraft["a2-cv-01-fighter-x2"]
	applyArm(t, &state, []string{first.ID}, LoadoutAirTank)
	advanceAircraftService(&state)
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{first.ID, second.ID}}
	ApplyAirOrder(&state, order)
	if first.ServiceTurns != 1 || second.ServiceTurns != 0 {
		t.Fatalf("前置状态不对：%d / %d", first.ServiceTurns, second.ServiceTurns)
	}
	scheduleElevators(&state)
	if first.Status != "elevator" {
		t.Fatalf("CAP 整备好的那架应该先提升，实际 %s", first.Status)
	}
	if second.Status != "service" {
		t.Fatalf("没整备完的那架应当还在机库，实际 %s", second.Status)
	}
}
