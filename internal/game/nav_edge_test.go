package game

import "testing"

// DayHours/TurnsPerDay 通过 State 参与昼夜推算。
func TestScenarioDayNightConfigFlowsIntoState(t *testing.T) {
	state := State{}
	state.TurnsPerDay = 72
	state.DayHours = 12
	state.Turn = 1
	UpdateDayNight(&state)
	if state.Night {
		t.Fatal("正午不应是夜间")
	}
	state.Turn = 30
	UpdateDayNight(&state)
	if !state.Night {
		t.Fatalf("第30回合应为夜间（约20:40），clock=%.2f", clockAt(state))
	}
}

// 自动航行不应在编队锁定（整编后3回合）时硬走。
func TestAutoNavRespectsMovementLock(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	LockFleetMovement(&state, FleetKey(unit), 3)
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: "0205"})
	start := unit.Coord
	ResolveOrders(&state, nil)
	if state.Units["a-bb-01"].Coord != start {
		t.Fatalf("锁定期间不应移动：%s → %s", start, state.Units["a-bb-01"].Coord)
	}
}

// 抵达目的地后目的地必须清空，否则舰队会卡在原地反复判定。
func TestAutoNavClearsDestinationOnArrival(t *testing.T) {
	state := sandboxState(t)
	dest := "0205"
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: dest})
	for i := 0; i < 40 && FleetDestination(state, "TG-A") != ""; i++ {
		ResolveOrders(&state, nil)
	}
	if FleetDestination(state, "TG-A") != "" {
		t.Fatal("抵达后目的地没有清空")
	}
	if state.Units["a-bb-01"].Coord != dest {
		t.Fatalf("应在 %s，实际 %s", dest, state.Units["a-bb-01"].Coord)
	}
}

// 手动 SET_MOVE 与自动航行共用移动机会：手动动过后，本回合不能再被自动推一格。
func TestManualMoveThenAutoNavInSameTurn(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: "0205"})
	moves := LegalMoves(state, unit)
	if len(moves) == 0 {
		t.Skip("没有合法手动移动")
	}
	manual := moves[0]
	ResolveOrders(&state, []Order{{Type: "SET_MOVE", UnitID: "a-bb-01", Coord: manual}})
	if got := state.Units["a-bb-01"].Coord; got != manual {
		t.Fatalf("手动移动后应在 %s，实际 %s", manual, got)
	}
}

// 目的地被清空后，舰队不应继续被 resolveAutoNavigation 驱动。
func TestAutoNavIdleWhenDestinationEmpty(t *testing.T) {
	state := sandboxState(t)
	before := state.Units["a-bb-01"].Coord
	events := resolveAutoNavigation(&state)
	if state.Units["a-bb-01"].Coord != before {
		t.Fatalf("没有目的地时不应移动：%s → %s", before, state.Units["a-bb-01"].Coord)
	}
	if len(events) != 0 {
		t.Fatalf("没有自动航行时不应产生事件：%+v", events)
	}
}

// 脱离后的舰队如果还保留目的地，也要能继续走到目的地。
func TestDetachedFleetKeepsItsOwnDestination(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: "0205"})
	if FleetDestination(state, FleetKey(unit)) != "0205" {
		t.Fatal("目的地未设置")
	}
	// 模拟重创脱离：换 fleet id
	unit.FleetID = "TG-A-detach-1"
	if FleetDestination(state, FleetKey(unit)) != "0205" {
		t.Fatalf("脱离后应保留自己的目的地，实际 %q", FleetDestination(state, FleetKey(unit)))
	}
}
