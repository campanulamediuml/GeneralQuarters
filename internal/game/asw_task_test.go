package game

import "testing"

// 找某个阵营的第一架指定机种（测试用）。
func firstAircraftOfKind(state State, faction, kind string) *AirUnit {
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft != nil && aircraft.Faction == faction && aircraft.Kind == kind {
			return aircraft
		}
	}
	return nil
}

// TestAswPatrolRejectsFightersAndArmsDepthCharges 锁定反潜巡逻：
// 战斗机不能接这个任务；接单后自动挂反潜深弹，并且只有挂着深弹的巡逻机才算航空支援。
func TestAswPatrolRejectsFightersAndArmsDepthCharges(t *testing.T) {
	state := coralCombatState(t)
	// 靶场要的是白天：夜里航空命令本来就会被拦，测不出机型校验。
	state.Timeline = nil
	state.Night = false
	fighter := firstAircraftOfKind(state, "B", "fighter")
	bomber := firstAircraftOfKind(state, "B", "dive_bomber")
	if fighter == nil || bomber == nil {
		t.Fatal("测试剧本里找不到战斗机或俯冲轰炸机")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_ASW", AircraftIDs: []string{fighter.ID}}); err == nil {
		t.Fatal("战斗机不应该能执行反潜巡逻")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_ASW", AircraftIDs: []string{bomber.ID}}); err != nil {
		t.Fatalf("俯冲轰炸机应能执行反潜巡逻：%v", err)
	}
	ApplyAirOrder(&state, Order{Type: "LAUNCH_ASW", AircraftIDs: []string{bomber.ID}})
	if got := bomber.Loadout.Weapon(); got != WeaponDepthCharge {
		t.Fatalf("反潜巡逻应自动挂反潜深弹，实际 %s", bomber.Loadout)
	}
	if !bomber.OnCycle {
		t.Fatal("反潜巡逻是循环任务：接单后应自动开启任务循环")
	}
	carrier := state.Units[bomber.CarrierID]
	if carrier == nil {
		t.Fatal("找不到母舰")
	}
	// 挂深弹、在空中巡逻，才算反潜航空支援。
	bomber.Status = "asw"
	bomber.Coord = carrier.Coord
	if !aswAirPatrolAt(state, "B", carrier.Coord) {
		t.Fatal("挂着深弹的反潜巡逻应当算航空支援")
	}
	// 换成炸弹挂载就不算：深弹是判定条件。
	bomber.Loadout = LoadoutBombTank
	if aswAirPatrolAt(state, "B", carrier.Coord) {
		t.Fatal("没有深弹的飞机不应该算反潜航空支援")
	}
}

// TestAswPatrolIsCyclicAndStopsAtNight 反潜巡逻是循环任务，且入夜自动中止。
func TestAswPatrolIsCyclicAndStopsAtNight(t *testing.T) {
	if !nightBlockedAirOrder("LAUNCH_ASW") {
		t.Fatal("夜间不应允许新编反潜巡逻")
	}
	if !cyclicTaskKind("asw") {
		t.Fatal("反潜巡逻应当是循环任务")
	}
	state := coralCombatState(t)
	bomber := firstAircraftOfKind(state, "B", "dive_bomber")
	if bomber == nil {
		t.Fatal("找不到俯冲轰炸机")
	}
	// 循环重启时的机型校验：战斗机不能续上反潜巡逻。
	bomber.CycleKind = "asw"
	bomber.Kind = "fighter"
	if ok, _ := canRestartCycle(state, bomber); ok {
		t.Fatal("战斗机不应能重启反潜巡逻循环")
	}
}
