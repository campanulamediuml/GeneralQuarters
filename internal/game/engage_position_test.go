package game

import "testing"

// TestEngageUsesFleetRealPosition 回归：舰队离开最后已知 hex 后，只要攻击编队贴着它的真实位置，
// 就应按真实距离判定接敌（同 hex 200% / 相邻 100%），而不是因为旧坐标上没船就恒为 0%。
func TestEngageUsesFleetRealPosition(t *testing.T) {
	state := sandboxState(t)
	fleetID := FleetKey(state.Units["b2-bb-01"])
	aircraft := &AirUnit{ID: "strike-x", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C",
		Status: "attack", Strength: 2, AirCombat: 1, AntiShip: 4, TaskID: "strike-x", TaskKind: "attack",
		Coord: "0608", OriginCoord: "0608", TargetCoord: "0505", TargetFleetID: fleetID, RemainingRange: 10, MaxRange: 12}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.RNGState = 5
	tryEngage(&state, aircraft, "0608")
	if !aircraft.Engaged {
		t.Fatalf("编队与舰队同 hex 时应接敌成功（旧目标坐标不该让它 0%%）：%+v", aircraft)
	}
	if aircraft.TargetCoord != "0608" {
		t.Fatalf("接敌成功后应把目标改到舰队真实位置：%s", aircraft.TargetCoord)
	}
}

// TestEngageFailsOnEmptySea 没有情报目标、旧坐标上也没有舰队时，不能盲打空海域。
func TestEngageFailsOnEmptySea(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "strike-y", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C",
		Status: "attack", Strength: 2, AirCombat: 1, AntiShip: 4, TaskID: "strike-y", TaskKind: "attack",
		Coord: "0909", OriginCoord: "0909", TargetCoord: "0909", RemainingRange: 10, MaxRange: 12}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.RNGState = 5
	tryEngage(&state, aircraft, "0909")
	if aircraft.Engaged {
		t.Fatal("空海域不应接敌成功")
	}
}
