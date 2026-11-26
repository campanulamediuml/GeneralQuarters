package game

import "testing"

// 机库计数以"实际在舰的算子"为准：摔毁的飞机从来没有上舰，
// 旧版在备用油透支和夜间着舰失败两条路径上把它加了回去，计数会一路涨到 21/18。
func TestCarrierAircraftCountReconcilesToAboard(t *testing.T) {
	state := coralCombatState(t)
	carrier := firstFactionUnit(&state, "A", func(unit *Unit) bool { return IsCarrier(unit) })
	if carrier == nil {
		t.Fatal("找不到美军航母")
	}
	aboard := aboardAircraft(state, carrier.ID)
	// 模拟历史脏数据：机库占位被撞高了三格。
	carrier.Card.Aircraft = aboard + 3

	ReconcileCarrierAircraftCounts(&state)

	if got := carrier.Card.Aircraft; got != aboard {
		t.Fatalf("对账后机库应回到实际在舰数 %d，实际 %d", aboard, got)
	}
	if carrier.Card.Aircraft > carrier.Card.HangarCapacity {
		t.Fatalf("对账后不该超过机库上限：%d/%d", carrier.Card.Aircraft, carrier.Card.HangarCapacity)
	}
}

// 夜间着舰摔毁不该改变机库占位：算子被删除后，在舰数不变、Card.Aircraft 也不能变。
func TestNightLandingCrashKeepsHangarCount(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 60
	state.Night = true
	carrier := firstFactionUnit(&state, "A", func(unit *Unit) bool { return IsCarrier(unit) })
	plane := &AirUnit{ID: "night-crash", CarrierID: carrier.ID, Faction: "A", Model: "SBD", Kind: "dive_bomber",
		Status: "attack", Coord: carrier.Coord, Returning: true, TaskID: "strike", TaskKind: "attack"}
	plane.SetRange(12)
	state.Aircraft[plane.ID] = plane
	before := carrier.Card.Aircraft

	// 反复跑到真的摔死为止（70% 一回合一次，几乎必然很快命中）。
	crashed := false
	for attempt := 0; attempt < 40 && state.Aircraft[plane.ID] != nil; attempt++ {
		advanceAircraftReturn(&state, plane)
	}
	if state.Aircraft["night-crash"] != nil {
		// 它可能落回甲板而不是摔毁；那就直接跳过断言（这条用例只关心"摔毁时不涨"）。
		t.Skip("这一局里算子着舰成功，没有触发摔毁判定")
	}
	crashed = true
	if crashed && carrier.Card.Aircraft != before {
		t.Fatalf("摔毁的算子不该占机库位置：%d -> %d", before, carrier.Card.Aircraft)
	}
}
