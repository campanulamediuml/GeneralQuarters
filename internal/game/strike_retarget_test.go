package game

import "testing"

func retargetFixture() State {
	state := State{
		Map:      MapDefinition{Rows: 7, Cols: 9},
		Units:    map[string]*Unit{},
		Contacts: map[string]map[string]*FleetContact{"A": {"B1": {FleetID: "B1", Faction: "B", Level: intelMaxLevel, Coord: "0204"}}},
		Aircraft: map[string]*AirUnit{},
	}
	leader := &AirUnit{ID: "l", TaskID: "T1", TaskKind: "attack", Faction: "A", Status: "attack", Coord: "0602", TargetCoord: "0204"}
	member := &AirUnit{ID: "m", TaskID: "T1", TaskKind: "attack", Faction: "A", Status: "attack", Coord: "0602", TargetCoord: "0204"}
	state.Aircraft["l"] = leader
	state.Aircraft["m"] = member
	return state
}

// TestStrikeRetargetsWhenTargetFleetMoves 校验打击编队认领舰队之后，目标 hex 会跟着情报走。
func TestStrikeRetargetsWhenTargetFleetMoves(t *testing.T) {
	state := retargetFixture()
	leader := state.Aircraft["l"]
	member := state.Aircraft["m"]

	// 第一次：目标 hex 上有这支舰队的情报，编队认领它，目标不动。
	if events := retargetStrikeToContact(&state, leader, []*AirUnit{leader, member}); len(events) != 0 {
		t.Fatalf("目标未变化时不该改目标：%+v", events)
	}
	if leader.TargetFleetID != "B1" {
		t.Fatalf("没有认领目标舰队：%q", leader.TargetFleetID)
	}

	// 舰队移动：情报更新到 0205，编队应该改追过去。
	state.Contacts["A"]["B1"].Coord = "0205"
	events := retargetStrikeToContact(&state, leader, []*AirUnit{leader, member})
	if len(events) != 1 || events[0].Type != "strike_retarget" {
		t.Fatalf("舰队移动后没有改追：%+v", events)
	}
	if leader.TargetCoord != "0205" || member.TargetCoord != "0205" {
		t.Fatalf("目标没有同步改追：leader=%s member=%s", leader.TargetCoord, member.TargetCoord)
	}

	// 情报丢失：保持当前目标，不要乱飞。
	delete(state.Contacts["A"], "B1")
	leader.TargetCoord, member.TargetCoord = "0205", "0205"
	if events := retargetStrikeToContact(&state, leader, []*AirUnit{leader, member}); len(events) != 0 {
		t.Fatalf("情报丢失后不该改目标：%+v", events)
	}
	if leader.TargetCoord != "0205" {
		t.Fatalf("情报丢失后目标被改动：%s", leader.TargetCoord)
	}
}

// TestStrikeDoesNotRetargetAirfield 校验对固定机场的打击不会被"追踪舰队"逻辑影响。
func TestStrikeDoesNotRetargetAirfield(t *testing.T) {
	state := retargetFixture()
	leader := state.Aircraft["l"]
	member := state.Aircraft["m"]
	leader.TargetCoord, member.TargetCoord = "0304", "0304"
	if events := retargetStrikeToContact(&state, leader, []*AirUnit{leader, member}); len(events) != 0 {
		t.Fatalf("对机场的打击不该改目标：%+v", events)
	}
	if leader.TargetCoord != "0304" || leader.TargetFleetID != "" {
		t.Fatalf("机场目标被误改：%+v", leader)
	}
}

// TestStrikeRetargetUsesIntelDisplayCoord 校验看不见目标时按情报显示坐标追，而不是真实坐标。
func TestStrikeRetargetUsesIntelDisplayCoord(t *testing.T) {
	state := retargetFixture()
	leader := state.Aircraft["l"]
	member := state.Aircraft["m"]
	leader.TargetFleetID = "B1"
	member.TargetFleetID = "B1"
	state.Contacts["A"]["B1"].Level = 2
	state.Contacts["A"]["B1"].Coord = "0208"
	events := retargetStrikeToContact(&state, leader, []*AirUnit{leader, member})
	if len(events) != 1 {
		t.Fatalf("情报位置变化后没有改追：%+v", events)
	}
	want := intelDisplayCoord(*state.Contacts["A"]["B1"])
	if leader.TargetCoord != want {
		t.Fatalf("改追坐标=%s，期望情报显示坐标%s", leader.TargetCoord, want)
	}
}
