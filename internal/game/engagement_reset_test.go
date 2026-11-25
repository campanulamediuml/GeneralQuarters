package game

import "testing"

// 上一轮打击留下的接敌标记不能带进新一轮出击：离场那一刻必须清掉。
// 否则编队一进入 attack 状态，就会在"还没飞到目标"的位置按目标 hex 结算一次空袭，
// 或者卡在"已接敌但不推进"——珊瑚海第 115 回合那 12 架留在空中的就是这种情况。
func TestDepartureClearsStaleEngagement(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	carrier := state.Units["b-cv-01"]
	if carrier == nil {
		t.Fatal("找不到翔鹤")
	}
	plane := &AirUnit{ID: "strike-1", CarrierID: carrier.ID, Faction: "B", Kind: "dive_bomber", Model: "D3A1",
		Status: "attack_holding", Strength: 1, AirCombat: 1, AntiShip: 3,
		TaskID: "strike-1", TaskKind: "attack", Coord: carrier.Coord, OriginCoord: carrier.Coord,
		TargetCoord: "2533", Engaged: true, SearchTurns: 2, SearchHeading: "E"}
	plane.SetRange(20)
	state.Aircraft = map[string]*AirUnit{plane.ID: plane}

	events := updateAttackFormation(&state, "strike-1")
	if plane.Status != "attack" {
		t.Fatalf("全员升空后应当离场：%s", plane.Status)
	}
	if plane.Engaged || plane.SearchTurns != 0 || plane.SearchHeading != "" {
		t.Fatalf("离场应清掉上一轮的接敌与搜索状态：%+v", plane)
	}
	if !hasEvent(events, "attack_departed") {
		t.Fatalf("离场应当留下战报：%+v", events)
	}
}

// 换任务时必须把上一轮的接敌/搜索状态一起清掉（clearAirTask 是所有接单路径的共同入口）。
func TestClearAirTaskDropsEngagement(t *testing.T) {
	plane := &AirUnit{ID: "x", Status: "attack", Engaged: true, SearchTurns: 3, SearchHeading: "E", TargetCoord: "1234"}
	clearAirTask(plane)
	if plane.Engaged || plane.SearchTurns != 0 || plane.SearchHeading != "" {
		t.Fatalf("换任务后应清掉接敌与搜索状态：%+v", plane)
	}
}
