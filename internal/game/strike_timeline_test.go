package game

import "testing"

// 舰载打击编队：距离 4 hex（每回合 2 hex）、甲板只剩 2 个空位、每回合能下降 1 架，
// 所以前两架本波次能落，第三架要多等一回合。
func TestEstimateCarrierStrikeLandsWithQueue(t *testing.T) {
	state := State{Turn: 1, DayHours: 12, TurnsPerDay: 72}
	home := &Unit{ID: "cv1", Coord: "1010", Deck: &FlightDeck{Length: 2, Elevators: []*AircraftElevator{{ID: "e1", Capacity: 1, Status: "idle"}}}}
	members := []*AirUnit{
		{ID: "a1", CarrierID: "cv1", Status: "attack", Coord: "1010", TargetCoord: "1014"},
		{ID: "a2", CarrierID: "cv1", Status: "attack", Coord: "1010", TargetCoord: "1014"},
		{ID: "a3", CarrierID: "cv1", Status: "attack", Coord: "1010", TargetCoord: "1014"},
	}
	toTarget, toLanded := EstimateCarrierStrike(state, members, home)
	// 2 回合飞到目标 + 1 回合接敌空袭。
	if toTarget != 3 {
		t.Fatalf("到目标应 3 回合，实际 %d", toTarget)
	}
	// 3 + 返航 2 + 着舰 1 + 第三架排队 1。
	if toLanded != 7 {
		t.Fatalf("全部落舰应 7 回合（含排队），实际 %d", toLanded)
	}
}

// 还没起飞的编队要先把整备/提升算进去，不能按"已经在天上"估算。
func TestEstimateCarrierStrikeCountsPrep(t *testing.T) {
	state := State{Turn: 1, DayHours: 12, TurnsPerDay: 72}
	home := &Unit{ID: "cv1", Coord: "1010", Deck: &FlightDeck{Length: 12, Elevators: []*AircraftElevator{{ID: "e1", Capacity: 4, Status: "idle"}}}}
	members := []*AirUnit{
		{ID: "a1", CarrierID: "cv1", Status: "service", ServiceTurns: 0, Coord: "1010", TargetCoord: "1012"},
		{ID: "a2", CarrierID: "cv1", Status: "hangar", Coord: "1010", TargetCoord: "1012"},
	}
	toTarget, toLanded := EstimateCarrierStrike(state, members, home)
	// service 剩 2 回合整备 + 1 回合提升 = 3，再飞 1 回合 + 接敌 1 = 5；机库那架同样 3+1+1。
	if toTarget != 5 {
		t.Fatalf("整备中的编队到目标应 5 回合，实际 %d", toTarget)
	}
	if toLanded <= toTarget {
		t.Fatalf("落舰回合必须晚于命中回合：target=%d landed=%d", toTarget, toLanded)
	}
}

// 机场编队按跑道数排队；跑道上一次只能收下这么多。
func TestEstimateGroundStrikeQueuesByRunways(t *testing.T) {
	state := State{Turn: 1, DayHours: 12, TurnsPerDay: 72}
	airfield := &Airfield{ID: "af1", Coord: "1010", Strength: 10, Runways: 2}
	if got := RunwaysAvailable(airfield.Strength, airfield.Runways); got != 2 {
		t.Fatalf("可用跑道应为 2，实际 %d", got)
	}
	members := []*AirfieldAirUnit{
		{ID: "g1", AirfieldID: "af1", Status: "attack", Coord: "1010", TargetCoord: "1014"},
		{ID: "g2", AirfieldID: "af1", Status: "attack", Coord: "1010", TargetCoord: "1014"},
		{ID: "g3", AirfieldID: "af1", Status: "attack", Coord: "1010", TargetCoord: "1014"},
	}
	toTarget, toLanded := EstimateGroundStrike(state, members, airfield)
	if toTarget != 3 {
		t.Fatalf("机场编队到目标应 3 回合，实际 %d", toTarget)
	}
	// 3 + 返航 2 + 着舰 1 + 第三架排在第二条跑道之后 → 多 1 回合。
	if toLanded != 7 {
		t.Fatalf("机场编队全部落回应 7 回合，实际 %d", toLanded)
	}
}

// 时刻换算：12:00 起算，每回合 20 分钟。
func TestEtaClockFormatting(t *testing.T) {
	state := State{Turn: 1, DayHours: 12, TurnsPerDay: 72}
	if got := etaClock(state, 6); got != "14:00" {
		t.Fatalf("6 回合后应为 14:00，实际 %s", got)
	}
	if got := etaClock(state, 24); got != "20:00" {
		t.Fatalf("24 回合后应为 20:00，实际 %s", got)
	}
	// 跨午夜仍然按当天时刻写（19:40 之后就是夜间）。
	if got := etaClock(State{Turn: 1, DayHours: 23, TurnsPerDay: 72}, 3); got != "00:00" {
		t.Fatalf("跨午夜应回绕到 00:00，实际 %s", got)
	}
	if got := etaClock(State{Turn: 1, DayHours: 12, TurnsPerDay: 0}, 6); got != "" {
		t.Fatalf("没有昼夜剧本的时刻应为空串，实际 %s", got)
	}
}

// 落舰落在夜间：弹出"返航时间在夜间"的依据。
func TestFillStrikeETAMarksNightLanding(t *testing.T) {
	// 19:00 起算，再过 3 回合 = 20:00，已经过了 19:40 的回收截止。
	state := State{Turn: 1, DayHours: 19, TurnsPerDay: 72}
	task := AirTask{}
	fillStrikeETA(state, &task, 1, 3)
	if task.ETATarget != "19:20" || task.ETALanded != "20:00" {
		t.Fatalf("时刻应为 19:20 / 20:00，实际 %s / %s", task.ETATarget, task.ETALanded)
	}
	if !task.LandsAtNight {
		t.Fatal("20:00 落舰应标记为夜间")
	}
	state.DayHours = 12
	task = AirTask{}
	fillStrikeETA(state, &task, 1, 3)
	if task.LandsAtNight {
		t.Fatal("13:00 落舰不该标记为夜间")
	}
}
