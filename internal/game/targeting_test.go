package game

import "testing"

func TestStrikeTargetValidationAndInheritance(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	first := Order{Type: "LAUNCH_STRIKE", AircraftID: "air-b-03"}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatalf("允许先创建编队再设置目标：%v", err)
	}
	ApplyAirOrder(&state, first)
	target := Order{Type: "SET_STRIKE_TARGET", AircraftID: "air-b-03", Coord: "0101"}
	if err := ValidateAirOrder(state, "B", target); err == nil {
		t.Fatal("允许打击未侦测目标")
	}
	state.Contacts["B"]["TG-A"] = &FleetContact{FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14}
	target.Coord = "0202"
	if err := ValidateAirOrder(state, "B", target); err != nil {
		t.Fatalf("已知目标未通过校验：%v", err)
	}
	ApplyAirOrder(&state, target)
	join := Order{Type: "LAUNCH_STRIKE", AircraftID: "air-b-06", TaskID: "air-b-03", Coord: "0101"}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("加入已有编队失败：%v", err)
	}
	ApplyAirOrder(&state, join)
	for _, id := range []string{"air-b-03", "air-b-06"} {
		if state.Aircraft[id].TargetCoord != "0202" {
			t.Fatalf("编队成员没有继承目标：%+v", state.Aircraft[id])
		}
	}
}

func TestAttackMovesTowardTargetAndConsumesRange(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "strike", Faction: "B", Model: "SB2C", Status: "attack", Coord: "0208", OriginCoord: "0208", TargetCoord: "0202", RemainingRange: 12, MaxRange: 12}
	state.Aircraft["strike"] = aircraft
	advanceAttack(&state, aircraft)
	if aircraft.Coord != "0206" || aircraft.RemainingRange != 10 {
		t.Fatalf("攻击编队移动或耗油错误：%+v", aircraft)
	}
	advanceAttack(&state, aircraft)
	events := advanceAttack(&state, aircraft)
	if aircraft.Coord != "0202" || aircraft.RemainingRange != 6 || len(events) != 1 || events[0].Type != "attack_arrived" {
		t.Fatalf("攻击编队没有抵达目标：%+v %+v", aircraft, events)
	}
}

func TestAttackWaitsForTargetBeforeDeparture(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	setAircraftReady(&state, "air-b-03")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftID: "air-b-03"})
	if state.Aircraft["air-b-03"].Status != "attack_holding" {
		t.Fatalf("未设目标的编队不应离场：%+v", state.Aircraft["air-b-03"])
	}
	state.Contacts["B"]["TG-A"] = &FleetContact{FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14}
	ApplyAirOrder(&state, Order{Type: "SET_STRIKE_TARGET", AircraftID: "air-b-03", Coord: "0202"})
	if state.Aircraft["air-b-03"].Status != "attack" {
		t.Fatalf("设置目标后编队没有离场：%+v", state.Aircraft["air-b-03"])
	}
}
