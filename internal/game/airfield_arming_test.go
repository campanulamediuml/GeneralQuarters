package game

import "testing"

// 岸基没有升降机：机库里提前挂好弹的算子不必再走一遍机库整备，拖进任务栏直接滑行。
func TestGroundPreparedAircraftSkipsHangarService(t *testing.T) {
	state := sandboxState(t)
	plane := state.GroundAir["af-b-fighter-1"]
	if plane == nil {
		t.Fatal("缺少机场算子")
	}
	arm := Order{Type: "SET_LOADOUT", AircraftID: plane.ID, AircraftIDs: []string{plane.ID}, Loadout: string(LoadoutAirTank)}
	if err := ValidateOrder(state, "B", arm); err != nil {
		t.Fatalf("机场选挂载被拒：%v", err)
	}
	ApplyLoadoutOrder(&state, arm)
	if plane.Loadout != LoadoutAirTank || !plane.LoadoutManual {
		t.Fatalf("机场挂载没生效：%+v", plane)
	}
	cap := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{plane.ID}}
	if err := ValidateGroundAirOrder(state, cap); err != nil {
		t.Fatalf("CAP 被拒：%v", err)
	}
	ApplyGroundAirOrder(&state, cap)
	if plane.Status != "deck" {
		t.Fatalf("提前挂好弹的岸基算子应直接滑行，实际 %s", plane.Status)
	}
	// 滑行→跑道待命→起飞都在这一回合走完：比没挂弹的算子少一整回合（20 分钟）。
	AdvanceAirfields(&state)
	if plane.Status != "cap" {
		t.Fatalf("提前挂弹的算子应当下回合起飞，实际 %s", plane.Status)
	}
}

// 没有提前挂弹的岸基算子拖进任务栏仍然从机库整备开始（多一回合）。
func TestGroundUnpreparedAircraftServicesInHangar(t *testing.T) {
	state := sandboxState(t)
	plane := state.GroundAir["af-b-fighter-2"]
	cap := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{plane.ID}}
	if err := ValidateGroundAirOrder(state, cap); err != nil {
		t.Fatalf("CAP 被拒：%v", err)
	}
	ApplyGroundAirOrder(&state, cap)
	if plane.Status != "service" {
		t.Fatalf("没挂弹的岸基算子应从机库整备开始，实际 %s", plane.Status)
	}
	if plane.Loadout == LoadoutNone {
		t.Fatal("任务应当给算子配上挂载")
	}
	AdvanceAirfields(&state)
	if plane.Status != "deck" {
		t.Fatalf("机库整备一回合后应滑行，实际 %s", plane.Status)
	}
	AdvanceAirfields(&state)
	if plane.Status != "cap" {
		t.Fatalf("滑行一回合后应升空，实际 %s", plane.Status)
	}
}

// 岸基的"甲板栏"就是跑道：机库算子可以手动滑行过去，不占跑道起降名额。
func TestGroundLiftToDeckTaxisToRunway(t *testing.T) {
	state := sandboxState(t)
	plane := state.GroundAir["af-b-dive_bomber-1"]
	order := Order{Type: "LIFT_TO_DECK", AircraftIDs: []string{plane.ID}}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("滑行到跑道被拒：%v", err)
	}
	ApplyGroundAirOrder(&state, order)
	if plane.Status != "deck" {
		t.Fatalf("手动滑行应进入滑行阶段，实际 %s", plane.Status)
	}
	AdvanceAirfields(&state)
	if plane.Status != "ready" {
		t.Fatalf("滑行一回合后应在跑道待命，实际 %s", plane.Status)
	}
	// 跑道待命的空载算子拖进任务栏：任务会配挂载，然后当回合就能起飞。
	dispatch := Order{Type: "LAUNCH_RECON", AircraftIDs: []string{plane.ID}, Heading: "W", SectorWidth: 60}
	if err := ValidateOrder(state, "B", dispatch); err != nil {
		t.Fatalf("跑道待命算子派任务被拒：%v", err)
	}
	ApplyGroundAirOrder(&state, dispatch)
	if plane.Status != "recon" {
		t.Fatalf("跑道待命的算子应该直接起飞，实际 %s", plane.Status)
	}
	// 滑行/待命不占起降名额：起飞前 OpsRemaining 还是满的。
}
