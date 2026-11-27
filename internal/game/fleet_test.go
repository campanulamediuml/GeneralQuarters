package game

import "testing"

// 规则书 392–400 行：损失 80% 生命值 / 失去动力 / 起火燃烧 = 重创（进水、倾斜不算），
// 重创舰带走一艘护航舰（DD → CL → CA → BB）脱离主队，航向地图边缘。
func TestCrippledShipDetachesWithEscort(t *testing.T) {
	state := sandboxState(t)
	damaged := state.Units["a-bb-01"]
	EnsureDamage(damaged).Disabled = true // 失去动力
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" && unit.Faction == "A" && unit.ID != damaged.ID && unit.Type != "DD" {
			continue
		}
	}
	events := resolveDetachments(&state)
	if len(events) != 1 || events[0].Type != "unit_detached" {
		t.Fatalf("没有产生重创脱离事件：%+v", events)
	}
	if !damaged.Withdrawing || damaged.FleetID == "TG-A" {
		t.Fatalf("重创舰没有脱离主队：%+v", damaged)
	}
	escort := state.Units["a-dd-01"]
	if escort == nil || !escort.Withdrawing || escort.FleetID != damaged.FleetID {
		t.Fatalf("没有按 DD 优先带走护航舰：%+v", escort)
	}
	// 主队仍有战列舰，不应整队撤退。
	if state.Units["a-bb-02"].Withdrawing {
		t.Fatal("主队仍有战力时不该强制撤退")
	}
}

// 主队只剩航母时整个主队强制撤退（规则书 402 行）。
func TestFleetWithOnlyCarriersForcesWithdrawal(t *testing.T) {
	state := sandboxState(t)
	removable := []string{}
	for id, unit := range state.Units {
		if FleetKey(unit) == "TG-A-CV" && unit.Type != "CV" {
			removable = append(removable, id)
		}
	}
	for _, id := range removable {
		delete(state.Units, id)
	}
	events := resolveDetachments(&state)
	found := false
	for _, event := range events {
		if event.Type == "fleet_forced_withdraw" {
			found = true
		}
	}
	if !found {
		t.Fatalf("只剩航母的主队没有强制撤退：%+v", events)
	}
	if !state.Units["a2-cv-01"].Withdrawing {
		t.Fatal("航母应当进入撤退状态")
	}
}

// 退避队每回合向地图边缘移动，抵达边缘后消失（规则书 403 行）。
func TestWithdrawingFleetMovesToEdgeAndDisappears(t *testing.T) {
	state := sandboxState(t)
	bb := state.Units["a-bb-01"]
	bb.Coord = "0102"
	bb.Heading = "NE"
	bb.Withdrawing = true
	bb.FleetID = "TG-A"
	// 把同队其他船挪走，避免它们一起被处理
	for _, unit := range state.Units {
		if unit.Faction == "A" && FleetKey(unit) == "TG-A" && unit.ID != bb.ID {
			unit.Withdrawing = true
		}
	}
	events := advanceTowardEdge(&state)
	found := false
	for _, event := range events {
		if event.Type == "fleet_withdrawn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("边缘的退避队没有撤出战场：%+v", events)
	}
	if state.Units["a-bb-01"] != nil {
		t.Fatal("撤出战场的舰队应当消失")
	}
}

// 运输船在敌方机场 hex 停留满 3 回合后开始卸载，每回合 1 点（规则书 495–497 行）。
func TestTransportUnloadsAfterThreeTurns(t *testing.T) {
	state := sandboxState(t)
	addTransport(t, &state, "a-trs-01", "0207")
	if state.FleetStays == nil {
		state.FleetStays = map[string]string{}
	}
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
	state.FleetStays["TG-TRS"] = "0207"
	state.FleetSessions["TG-TRS"] = 2
	if events := resolveUnloading(&state); len(events) != 0 {
		t.Fatalf("停留不满 3 回合就开始卸载：%+v", events)
	}
	state.FleetSessions["TG-TRS"] = 3
	events := resolveUnloading(&state)
	// 卸载的舰队同时会向敌方完全暴露（规则书 535 行），所以这里只筛卸载事件本身。
	if !hasEvent(events, "troop_unloading") {
		t.Fatalf("停留满 3 回合没有卸载：%+v", events)
	}
	if supply := shoreSupplies(state, "A", "0207"); supply != 1 {
		t.Fatalf("滩头物资数量错误：%d", supply)
	}
	if state.Units["a-trs-01"].Card.Cargo != 2 {
		t.Fatalf("运输船货量没有减少：%d", state.Units["a-trs-01"].Card.Cargo)
	}
}

// 卸空后舰队强制撤离。
func TestTransportWithdrawsWhenEmpty(t *testing.T) {
	state := sandboxState(t)
	addTransport(t, &state, "a-trs-01", "0207")
	state.Units["a-trs-01"].Card.Cargo = 1
	if state.FleetStays == nil {
		state.FleetStays = map[string]string{}
	}
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
	state.FleetStays["TG-TRS"] = "0207"
	state.FleetSessions["TG-TRS"] = 3
	resolveUnloading(&state)
	if !state.Units["a-trs-01"].Withdrawing {
		t.Fatal("运输船卸空后没有撤离")
	}
}

// 舰队在同一 hex 连续停留才累计停留回合数；换 hex 后从到达当回合算第 1 回合。
func TestFleetSessionsCountConsecutiveStays(t *testing.T) {
	state := sandboxState(t)
	state.FleetStays = map[string]string{"TG-A": "0202"}
	state.FleetSessions = map[string]int{"TG-A": 2}
	trackFleetSessions(&state)
	if state.FleetSessions["TG-A"] != 3 {
		t.Fatalf("连续停留没有累计：%d", state.FleetSessions["TG-A"])
	}
	state.FleetStays["TG-A"] = "0203"
	trackFleetSessions(&state)
	if state.FleetSessions["TG-A"] != 1 {
		t.Fatalf("换 hex 后停留计数应从 1 重新算：%d", state.FleetSessions["TG-A"])
	}
}

// 规则书 421–423 行：炸弹命中航母时，机库/甲板整装待发的飞机会被摧毁至多 2 个算子。
func TestBombHitDestroysReadyAircraft(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	carrier.Card.DamageControl = 10 // 排除 DCT 失败带来的额外影响
	hangar := []*AirUnit{}
	for _, id := range []string{"a2-cv-01-fighter-x1", "a2-cv-01-fighter-x2", "a2-cv-01-dive_bomber-x1"} {
		aircraft := state.Aircraft[id]
		aircraft.Armed = true
		aircraft.Status = "hangar"
		aircraft.TaskKind = "attack"
		hangar = append(hangar, aircraft)
	}
	before := carrier.Card.Aircraft
	applyGunOrBombHit(&state, Hit{TargetID: carrier.ID, Cause: HitBomb})
	after := carrier.Card.Aircraft
	lost := before - after
	if lost < 1 || lost > 2 {
		t.Fatalf("机库整装待发飞机摧毁数量异常：%d", lost)
	}
	for _, aircraft := range hangar {
		if state.Aircraft[aircraft.ID] == nil {
			continue
		}
	}
	if EnsureDamage(carrier).Fire == "" {
		t.Fatal("命中机库整装待发飞机应引起小型火灾")
	}
}

// 整装待发 = 有挂载 + 整备回合走完 + 没损伤/检修（规则书 421–423 行），
// 和任务类型无关：机库里选好挂载整备完就是待发。
func TestArmedFlagFollowsService(t *testing.T) {
	state := sandboxState(t)
	fighter := state.Aircraft["a2-cv-01-fighter-x1"]
	fighter.Status = "hangar"
	fighter.Loadout = LoadoutAirTank
	fighter.ServiceTurns = 0
	syncArmedAircraft(&state, nil)
	if fighter.Armed {
		t.Fatal("还没整备的算子不应标记为整装待发")
	}
	fighter.ServiceTurns = 1
	syncArmedAircraft(&state, nil)
	if !fighter.Armed {
		t.Fatal("整备完成的算子应标记为整装待发")
	}
	fighter.Loadout = LoadoutNone
	syncArmedAircraft(&state, nil)
	if fighter.Armed {
		t.Fatal("卸掉挂载之后不应再是整装待发")
	}
	if fighter.ServiceTurns != 1 {
		t.Fatalf("整备进度不该被 Armed 同步改动：%d", fighter.ServiceTurns)
	}
}

// 带火灾的舰队无法岸轰（规则书 432 行）。
func TestBurningFleetCannotBombard(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	unit.Coord = "0207"
	if state.FleetStays == nil {
		state.FleetStays = map[string]string{}
	}
	state.FleetStays["TG-A"] = "0207"
	// 把主队其余船只挪开，避免同 hex 的判定受干扰
	for _, member := range state.Units {
		if member.Faction == "A" && FleetKey(member) == "TG-A" && member.ID != unit.ID {
			member.Coord = "0303"
		}
	}
	if canBombard(state, unit) == nil {
		t.Fatal("条件满足时应当可以岸轰")
	}
	EnsureDamage(unit).Fire = "small"
	if canBombard(state, unit) != nil {
		t.Fatal("起火燃烧中不应能岸轰")
	}
}

func addTransport(t *testing.T, state *State, id, coord string) {
	t.Helper()
	state.Units[id] = &Unit{
		ID: id, DefinitionID: "trs", FleetID: "TG-TRS", FleetName: "运输船队", Name: "运输船",
		Type: "TRS", Faction: "A", Coord: coord, Heading: "NE", Strength: 2, MaxStrength: 2,
		Card: ShipCard{Cargo: 3, DamageControl: 4, SpeedClass: "slow"},
	}
}

// 重创舰脱离一次后，不应在后续回合被重复摘下。
func TestCrippledShipDetachesOnlyOnce(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	EnsureDamage(unit).Disabled = true // 失去动力
	resolveDetachments(&state)
	firstFleet := state.Units["a-bb-01"].FleetID
	if !state.Units["a-bb-01"].Withdrawing {
		t.Fatal("重创舰应进入撤离状态")
	}
	firstName := state.Units["a-bb-01"].FleetName
	for i := 0; i < 3; i++ {
		resolveDetachments(&state)
	}
	if state.Units["a-bb-01"].FleetID != firstFleet {
		t.Fatalf("重创舰不应被反复脱队：%s → %s", firstFleet, state.Units["a-bb-01"].FleetID)
	}
	if state.Units["a-bb-01"].FleetName != firstName {
		t.Fatalf("舰队名不应反复拼接：%q → %q", firstName, state.Units["a-bb-01"].FleetName)
	}
}
