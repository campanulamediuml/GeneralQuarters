package game

import "testing"

func TestStrikeTargetValidationAndInheritance(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	first := Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-01-dive_bomber-x1"}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatalf("允许先创建编队再设置目标：%v", err)
	}
	ApplyAirOrder(&state, first)
	strikeTaskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	target := Order{Type: "SET_STRIKE_TARGET", AircraftID: "b-cv-01-dive_bomber-x1", Coord: "0101"}
	// 没有情报的格子不能作为打击目标：攻击编队必须先由侦察机发现目标才能出击。
	if err := ValidateAirOrder(state, "B", target); err == nil {
		t.Fatal("没有情报的坐标不应允许派出攻击编队")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "SET_STRIKE_TARGET", AircraftID: "b-cv-01-dive_bomber-x1", Coord: "9999"}); err == nil {
		t.Fatal("越界坐标仍应被拒绝")
	}
	// 情报等级 1 的位置并不可靠（规则书 144 行）：玩家只能拿到"显示位置"，
	// 打击目标也必须是显示位置，用真实坐标反而会被判为不在情报范围内。
	contact := &FleetContact{FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14}
	state.Contacts["B"]["TG-A"] = contact
	displayed := intelDisplayCoord(*contact)
	target.Coord = displayed
	if err := ValidateAirOrder(state, "B", target); err != nil {
		t.Fatalf("已知目标未通过校验：%v", err)
	}
	ApplyAirOrder(&state, target)
	join := Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-02-dive_bomber-x1", TaskID: strikeTaskID, Coord: "0101"}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("加入已有编队失败：%v", err)
	}
	ApplyAirOrder(&state, join)
	for _, id := range []string{"b-cv-01-dive_bomber-x1", "b-cv-02-dive_bomber-x1"} {
		if state.Aircraft[id].TargetCoord != displayed {
			t.Fatalf("编队成员没有继承目标：%+v", state.Aircraft[id])
		}
	}
}

func TestAttackMovesTowardTargetAndConsumesRange(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "strike", Faction: "B", Model: "SB2C", Status: "attack", Coord: "0208", OriginCoord: "0208", TargetCoord: "0202", RemainingRange: 12, MaxRange: 12}
	state.Aircraft["strike"] = aircraft
	advanceAttack(&state, aircraft)
	if aircraft.Coord != "0206" || aircraft.Range() != 10 {
		t.Fatalf("攻击编队移动或耗油错误：%+v", aircraft)
	}
	advanceAttack(&state, aircraft)
	events := advanceAttack(&state, aircraft)
	arrived := false
	for _, event := range events {
		if event.Type == "attack_arrived" {
			arrived = true
		}
	}
	// 同一回合里还可能有雷达拦截/改追目标的事件，不能要求事件列表恰好只有一条。
	if aircraft.Coord != "0202" || aircraft.Range() != 6 || !arrived {
		t.Fatalf("攻击编队没有抵达目标：%+v %+v", aircraft, events)
	}
}

func TestAttackWaitsForTargetBeforeDeparture(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	setAircraftReady(&state, "attack", "b-cv-01-dive_bomber-x1")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-01-dive_bomber-x1"})
	if state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "attack_holding" {
		t.Fatalf("未设目标的编队不应离场：%+v", state.Aircraft["b-cv-01-dive_bomber-x1"])
	}
	state.Contacts["B"]["TG-A"] = &FleetContact{FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14}
	ApplyAirOrder(&state, Order{Type: "SET_STRIKE_TARGET", AircraftID: "b-cv-01-dive_bomber-x1", Coord: "0202"})
	if state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "attack" {
		t.Fatalf("设置目标后编队没有离场：%+v", state.Aircraft["b-cv-01-dive_bomber-x1"])
	}
}

// 盲打：目标 hex 上空没有舰队时，编队抵达后进行接敌判定并失败，不能发起空袭。
func TestBlindStrikeSearchesAndFailsOnEmptyHex(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "blind", Faction: "B", Model: "D3A", Kind: "dive_bomber", Status: "attack",
		Coord: "0404", OriginCoord: "0208", TargetCoord: "0404", RemainingRange: 12, MaxRange: 12, AntiShip: 4,
	}
	state.Aircraft[aircraft.ID] = aircraft
	events := tryEngage(&state, aircraft, "0404")
	if aircraft.Engaged {
		t.Fatal("空格子上空不应接敌成功")
	}
	found := false
	for _, event := range events {
		if event.Type == "attack_searching" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应当产生搜索失败事件：%+v", events)
	}
}

// 盲打的坐标如果正好压着敌方舰队，同格基础 200%% 必中，接敌成功并把情报拉到 4。
func TestBlindStrikeEngagesWhenTargetIsThere(t *testing.T) {
	state := sandboxState(t)
	fleet := state.Units["a-bb-01"]
	const coord = "0404"
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = coord
		}
	}
	aircraft := &AirUnit{
		ID: "blind2", Faction: "B", Model: "D3A", Kind: "dive_bomber", Status: "attack",
		Coord: coord, OriginCoord: "0208", TargetCoord: coord, RemainingRange: 12, MaxRange: 12, AntiShip: 4,
	}
	state.Aircraft[aircraft.ID] = aircraft
	tryEngage(&state, aircraft, coord)
	if !aircraft.Engaged {
		t.Fatal("同格应当接敌成功")
	}
	if fleet == nil {
		t.Fatal("舰队丢失")
	}
	if contact := state.Contacts["B"]["TG-A"]; contact == nil || contact.Level != 4 {
		t.Fatalf("接敌成功后情报应提升到4：%+v", contact)
	}
}
