package game

import "testing"

// 攻击编队不能盲打舰队：没有情报时拒绝，侦察发现后才能出击。
func TestStrikeFleetRequiresReconContact(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}

	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1"}, Coord: "0202"}); err == nil {
		t.Fatal("没有情报时不应允许对舰队派出攻击编队")
	}
	state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID = "task-x"
	state.Aircraft["b-cv-01-dive_bomber-x1"].TaskKind = "attack"
	state.Aircraft["b-cv-01-dive_bomber-x1"].Status = "attack"
	state.Aircraft["b-cv-01-dive_bomber-x1"].Loadout = LoadoutBombTank
	if err := ValidateAirOrder(state, "B", Order{Type: "SET_STRIKE_TARGET", AircraftID: "task-x", Coord: "0202"}); err == nil {
		t.Fatal("没有情报时不应允许对舰队设置打击目标")
	}

	contact := &FleetContact{FleetID: "TG-A", Faction: "A", Level: 2, Coord: "0202", Count: 14}
	state.Contacts["B"]["TG-A"] = contact
	displayed := intelDisplayCoord(*contact)
	if err := ValidateAirOrder(state, "B", Order{Type: "SET_STRIKE_TARGET", AircraftID: "task-x", Coord: displayed}); err != nil {
		t.Fatalf("侦察发现后应允许打击显示位置 %s：%v", displayed, err)
	}
	far := "0709"
	if far == displayed {
		far = "0107"
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "SET_STRIKE_TARGET", AircraftID: "task-x", Coord: far}); err == nil {
		t.Fatalf("情报之外的坐标 %s 不应允许打击", far)
	}
}

// 机场固定不动、永远显示在地图上，所以不需要情报也能直接打（盲打机场允许）。
func TestStrikeAirfieldWorksWithoutContact(t *testing.T) {
	state := sandboxState(t)
	state.AirfieldContacts = nil
	state.Contacts = map[string]map[string]*FleetContact{}
	if err := ValidateStrikeTarget(state, "A", "0207"); err != nil {
		t.Fatalf("未侦察的敌方机场也应允许打击：%v", err)
	}
	// 己方机场不是打击目标。
	if err := ValidateStrikeTarget(state, "A", "0304"); err == nil {
		t.Fatal("己方机场不应允许作为打击目标")
	}
}

// 情报等级 0 的接触不算情报。
func TestStrikeIgnoresZeroLevelContact(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Faction: "A", Level: 0, Coord: "0202", Count: 14},
	}
	if knownStrikeCoord(state, "B", "0202") {
		t.Fatal("情报等级 0 不应算作已发现")
	}
}

// 接敌成功后下一回合发起空袭：即便接敌时不在目标格，也应结算而不是静默跳过。
func TestEngagedFormationStrikesNextTurn(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	contact := &FleetContact{FleetID: "TG-A", Faction: "A", Level: 4, Coord: "0202", Count: 14}
	state.Contacts["B"]["TG-A"] = contact
	ship := state.Units["a-bb-01"]
	ship.Coord = "0202"
	hide := func() {
		for _, u := range state.Units {
			if u.Faction == "A" {
				u.Coord = "0202"
			}
		}
	}
	hide()
	target := intelDisplayCoord(*contact)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1"}, Coord: target}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("有情报后应允许出击：%v", err)
	}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	// 直接把编队放到目标格并标记已接敌，模拟"接敌成功"之后的状态
	for _, m := range taskAircraft(state, taskID) {
		m.Status = "attack"
		m.Coord = target
		m.TargetCoord = target
		m.Engaged = true
	}
	before := len(state.AirCombats)
	advanceAttack(&state, state.Aircraft["b-cv-01-dive_bomber-x1"])
	if len(state.AirCombats) == before {
		t.Fatal("已接敌的编队下一回合应当发起空袭并生成战报")
	}
}

// 真实场景：接敌成功时编队可能停在目标邻近格（接敌判定用 TargetCoord），
// 下一回合仍必须发起空袭，不能因为 Coord != TargetCoord 被静默跳过。
func TestEngagedOffTargetStillStrikes(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	contact := &FleetContact{FleetID: "TG-A", Faction: "A", Level: 4, Coord: "0202", Count: 14}
	state.Contacts["B"]["TG-A"] = contact
	for _, u := range state.Units {
		if u.Faction == "A" {
			u.Coord = "0202"
		}
	}
	target := intelDisplayCoord(*contact)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1"}, Coord: target}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("有情报后应允许出击：%v", err)
	}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	// 编队停在目标格旁边的格子上，但已经接敌成功（这正是引擎里 tryEngage 的产物）。
	for _, m := range taskAircraft(state, taskID) {
		m.Status = "attack"
		m.Coord = "0201"
		m.TargetCoord = target
		m.Engaged = true
	}
	before := len(state.AirCombats)
	advanceAttack(&state, state.Aircraft["b-cv-01-dive_bomber-x1"])
	if len(state.AirCombats) == before {
		t.Fatal("已接敌但不在目标格的编队，下一回合也应当发起空袭")
	}
}

// 接敌成功后即使坐标与 TargetCoord 不一致，也必须结算空袭（防止永远卡住）。
func TestEngagedOffCoordStillStrikes(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	contact := &FleetContact{FleetID: "TG-A", Faction: "A", Level: 4, Coord: "0202", Count: 14}
	state.Contacts["B"]["TG-A"] = contact
	for _, u := range state.Units {
		if u.Faction == "A" {
			u.Coord = "0202"
		}
	}
	target := intelDisplayCoord(*contact)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1"}, Coord: target}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	// 接敌时编队停在别的格子上（真实对局里就是这样），TargetCoord 指向显示位置。
	for _, m := range taskAircraft(state, taskID) {
		m.Status = "attack"
		m.Coord = "0105"
		m.TargetCoord = target
		m.Engaged = true
	}
	before := len(state.AirCombats)
	advanceAttack(&state, state.Aircraft["b-cv-01-dive_bomber-x1"])
	if len(state.AirCombats) == before {
		t.Fatal("已接敌的编队即使坐标不一致也必须发起空袭")
	}
}
