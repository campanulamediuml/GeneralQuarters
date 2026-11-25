package game

import "testing"

// 规则书 535 行：运输船卸货和炮击的舰队会自动以最高情报等级暴露行踪，无论白天和黑夜。
//
// 这组回归覆盖四件事：
//   1. 岸轰和卸载真的会写入对方视角的满级情报（真实位置 + 完整编成）；
//   2. 同一回合的回合末情报衰减不能把它降级（岸轰/卸载在衰减之前结算）；
//   3. 只暴露给对手，不会在己方视角里凭空生成接触；
//   4. 非法驻留（航母编队停在机场格/浅滩）不能凭坐标蹭到岸轰或卸载。

func shoreTestState(t *testing.T) (State, *Airfield) {
	t.Helper()
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	if airfield == nil {
		t.Fatal("沙盒缺少 af-b-sentry 机场")
	}
	return state, airfield
}

// ensureFleetTracking 保证停留/停靠计数表可用（部分夹具只建了 Units）。
func ensureFleetTracking(state *State) {
	if state.FleetStays == nil {
		state.FleetStays = map[string]string{}
	}
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
}

func parkFleetOnAirfield(state *State, fleetID, coord string) {
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID {
			unit.Coord = coord
		}
	}
	ensureFleetTracking(state)
	state.FleetStays[fleetID] = coord
}

// 岸轰的舰队对敌方暴露到满级情报：位置、编成、事件一次给全。
func TestBombardmentExposesFleetAtMaxIntel(t *testing.T) {
	state, airfield := shoreTestState(t)
	parkFleetOnAirfield(&state, "TG-A", airfield.Coord)
	state.RNGState = 52
	events := resolveShoreBombardment(&state)

	contact := state.Contacts["B"]["TG-A"]
	if contact == nil {
		t.Fatal("岸轰后敌方没有获得该舰队的接触")
	}
	if contact.Level != intelMaxLevel {
		t.Fatalf("岸轰应暴露到最高情报等级 %d，实际 %d", intelMaxLevel, contact.Level)
	}
	if contact.Coord != airfield.Coord {
		t.Fatalf("满级情报应给出真实位置 %s，实际 %s", airfield.Coord, contact.Coord)
	}
	if len(contact.Members) != len(fleetMembers(&state, "TG-A")) {
		t.Fatalf("满级情报应带完整编成：成员 %d，舰队 %d", len(contact.Members), len(fleetMembers(&state, "TG-A")))
	}
	if !hasEvent(events, "fleet_exposed") {
		t.Fatalf("岸轰暴露应留下战报：%+v", events)
	}
	// 只暴露给对手：己方视角不该凭空出现自己的接触。
	if state.Contacts["A"]["TG-A"] != nil {
		t.Fatal("暴露不应写到己方视角")
	}
}

// 卸载的舰队同样暴露到满级，而且卸载战报排在暴露事件之前。
func TestUnloadingExposesFleetAtMaxIntel(t *testing.T) {
	state, airfield := shoreTestState(t)
	addTransport(t, &state, "a-trs-01", airfield.Coord)
	ensureFleetTracking(&state)
	state.FleetStays["TG-TRS"] = airfield.Coord
	state.FleetSessions["TG-TRS"] = 3

	events := resolveUnloading(&state)
	contact := state.Contacts["B"]["TG-TRS"]
	if contact == nil || contact.Level != intelMaxLevel {
		t.Fatalf("卸载后敌方应获得满级情报：%+v", contact)
	}
	if contact.Coord != airfield.Coord {
		t.Fatalf("满级情报应给出真实位置 %s，实际 %s", airfield.Coord, contact.Coord)
	}
	order := []string{}
	for _, event := range events {
		if event.Type == "troop_unloading" || event.Type == "fleet_exposed" {
			order = append(order, event.Type)
		}
	}
	if len(order) != 2 || order[0] != "troop_unloading" || order[1] != "fleet_exposed" {
		t.Fatalf("卸载战报应排在暴露之前，实际 %v", order)
	}
}

// 同回合覆盖风险：岸轰在回合末的情报衰减之前结算，衰减不能把刚暴露的 4 级情报降到 3 级。
func TestExposureSurvivesSameTurnDecay(t *testing.T) {
	state, airfield := shoreTestState(t)
	parkFleetOnAirfield(&state, "TG-A", airfield.Coord)
	state.RNGState = 52
	resolveShoreBombardment(&state)
	if contact := state.Contacts["B"]["TG-A"]; contact == nil || contact.Level != intelMaxLevel {
		t.Fatalf("岸轰后应为 4 级情报：%+v", contact)
	}
	if !state.IntelFresh["B|TG-A"] {
		t.Fatal("暴露后必须打上本回合情报新鲜标记，否则回合末衰减会立刻降级")
	}

	decayEvents := decayContactIntel(&state)
	contact := state.Contacts["B"]["TG-A"]
	if contact == nil {
		t.Fatal("情报不该在暴露的同一回合消失")
	}
	if contact.Level != intelMaxLevel {
		t.Fatalf("暴露的同一回合不该降级：%d", contact.Level)
	}
	if hasEvent(decayEvents, "contact_decay") || hasEvent(decayEvents, "contact_lost") {
		t.Fatalf("刚暴露的情报不应产生衰减事件：%+v", decayEvents)
	}
}

// 已经满级时不重复发暴露事件，但位置/编成仍按最新真实状态刷新。
func TestExposureRefreshesWithoutDuplicateEvent(t *testing.T) {
	state, airfield := shoreTestState(t)
	parkFleetOnAirfield(&state, "TG-A", airfield.Coord)
	first := exposeFleetAtMaxIntel(&state, "TG-A", "对岸轰击")
	if len(first) != 1 {
		t.Fatalf("首次暴露应该发一条事件：%+v", first)
	}
	second := exposeFleetAtMaxIntel(&state, "TG-A", "卸载物资")
	if len(second) != 0 {
		t.Fatalf("已经满级不应重复发事件：%+v", second)
	}
	if contact := state.Contacts["B"]["TG-A"]; contact == nil || contact.Level != intelMaxLevel {
		t.Fatalf("重复调用不该降低情报等级：%+v", contact)
	}
}

// 非法驻留（含航母的编队停在机场格/浅滩上）不能岸轰；同理带航母的"运输队"不能卸载。
// 这是旧存档的真实风险：航母编队曾经能停在机场格上，新规则下这种驻留不该继续产出行动。
func TestIllegalCarrierFleetCannotShellOrUnload(t *testing.T) {
	state, airfield := shoreTestState(t)
	carrier := state.Units["a2-cv-01"]
	if carrier == nil {
		t.Fatal("沙盒夹具缺少 a2-cv-01")
	}
	fleetID := FleetKey(carrier)
	parkFleetOnAirfield(&state, fleetID, airfield.Coord)
	var gunner *Unit
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID && unit.Card.Gunnery > 0 {
			gunner = unit
			break
		}
	}
	if gunner == nil {
		t.Fatalf("%s 里没有能开炮的舰船，夹具不成立", fleetID)
	}
	if !FleetHasCarrier(state, fleetID) {
		t.Fatalf("%s 应含航母", fleetID)
	}
	if canBombard(state, gunner) != nil {
		t.Fatal("含航母的编队停在机场格上不应能岸轰")
	}

	// 运输队里混进航母：同样不能卸载。
	addTransport(t, &state, "a-trs-01", airfield.Coord)
	ensureFleetTracking(&state)
	state.FleetSessions["TG-TRS"] = 3
	carrier.FleetID = "TG-TRS"
	if unitUnloading(state, state.Units["a-trs-01"]) != nil {
		t.Fatal("带航母的编队停在机场格上不应能卸载")
	}
}
