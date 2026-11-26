package game

import "testing"

// deployTestState 是部署阶段的最小夹具：双方各有一片 3 格半径的部署区，
// 每边两支舰队，随机出生在区内。
func deployTestState(t *testing.T) State {
	t.Helper()
	scenario := ScenarioDefinition{
		ID: "_deploy", Name: "部署测试", Version: 1, Year: 1942, Wind: "SE",
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		DeployZones: []DeployZone{
			{Faction: "A", Anchor: "1010", Radius: 3},
			{Faction: "B", Anchor: "2020", Radius: 3},
		},
		Deployments: []Deployment{
			{ID: "a1", Faction: "A", Unit: "dd", FleetID: "TF1", FleetName: "TF1"},
			{ID: "a2", Faction: "A", Unit: "dd", FleetID: "TF2", FleetName: "TF2"},
			{ID: "b1", Faction: "B", Unit: "dd", FleetID: "JP", FleetName: "JP"},
		},
	}
	gameMap := MapDefinition{Version: 1, Rows: 24, Cols: 24}
	catalog := UnitCatalog{Version: 1, Units: []UnitDefinition{
		{ID: "dd", Type: "DD", Name: "驱逐舰", Strength: 3, Card: ShipCard{SpeedClass: "normal", Gunnery: 2, DamageControl: 10}},
	}}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// 配了部署区的剧本：新局先进部署阶段，舰队随机出生在各自的区里。
func TestDeployPhaseSpawnsInsideZone(t *testing.T) {
	state := deployTestState(t)
	if state.Phase != PhaseDeploy {
		t.Fatalf("应处于部署阶段，实际 %q", state.Phase)
	}
	zones := map[string]DeployZone{}
	for _, zone := range state.DeployZones {
		zones[zone.Faction] = zone
	}
	if len(state.Units) != 3 {
		t.Fatalf("舰船数量不对：%d", len(state.Units))
	}
	for _, unit := range state.Units {
		zone := zones[unit.Faction]
		if Distance(unit.Coord, zone.Anchor) > zone.Radius {
			t.Fatalf("%s 出生在部署区外：%s", unit.Name, unit.Coord)
		}
	}
	// 同阵营两支舰队不能出生在同一个 hex。
	if state.Units["a1"].Coord == state.Units["a2"].Coord {
		t.Fatalf("两支舰队叠在同一格：%s", state.Units["a1"].Coord)
	}
}

// 部署阶段整片部署区都是可落点，不要求相邻；开战后才回到"只能走一格"。
func TestDeployMovesCoverWholeZone(t *testing.T) {
	state := deployTestState(t)
	unit := state.Units["a1"]
	moves := DeployMoves(state, unit)
	if len(moves) < 10 {
		t.Fatalf("部署区可用格太少：%d", len(moves))
	}
	far := false
	for _, coord := range moves {
		if Distance(coord, unit.Coord) > 1 {
			far = true
			break
		}
	}
	if !far {
		t.Fatal("部署区应当包含不相邻的格子")
	}
	// LegalMoves 在部署阶段直接返回部署区（前端拖拽高亮用）。
	if len(LegalMoves(state, unit)) != len(moves) {
		t.Fatalf("部署阶段 LegalMoves 应等于部署区：%d / %d", len(LegalMoves(state, unit)), len(moves))
	}
}

// 部署阶段：区内任意格可放，区外和开战类命令都要被拒绝。
func TestDeployValidation(t *testing.T) {
	state := deployTestState(t)
	unit := state.Units["a1"]
	moves := DeployMoves(state, unit)
	target := ""
	for _, coord := range moves {
		if coord != unit.Coord && Distance(coord, unit.Coord) > 1 {
			target = coord
			break
		}
	}
	if target == "" {
		t.Fatal("找不到区内的远距离落点")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: target}); err != nil {
		t.Fatalf("区内移动应被允许：%v", err)
	}
	// 区外（另一方的锚点附近）
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "2020"}); err == nil {
		t.Fatal("部署区外的移动应被拒绝")
	}
	// 开战类命令在部署阶段一律拒绝
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: unit.ID, Heading: "NE"}); err == nil {
		t.Fatal("部署阶段不该允许调整航向")
	}
	if err := ValidateOrder(state, "A", Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"x"}}); err == nil {
		t.Fatal("部署阶段不该允许放飞")
	}
}

// 部署移动整队瞬移，不产生冷却、不标记已移动。
func TestApplyDeployMoveKeepsFleetClean(t *testing.T) {
	state := deployTestState(t)
	unit := state.Units["a1"]
	target := ""
	for _, coord := range DeployMoves(state, unit) {
		if Distance(coord, unit.Coord) > 1 {
			target = coord
			break
		}
	}
	events := ApplyDeployMove(&state, Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: target})
	if len(events) != 1 || events[0].Type != "unit_deployed" {
		t.Fatalf("部署移动应产生一条 unit_deployed，实际 %+v", events)
	}
	for _, item := range state.Units {
		if item.FleetID != "TF1" {
			continue
		}
		if item.Coord != target {
			t.Fatalf("%s 没有跟着整队移动：%s", item.ID, item.Coord)
		}
		if item.MoveCooldown != 0 || item.Moved {
			t.Fatalf("部署移动不该留冷却：cooldown=%d moved=%v", item.MoveCooldown, item.Moved)
		}
	}
}

// 开战：清掉部署期间的整编锁与停留计时，回合号不动。
func TestStartBattleClearsDeployArtifacts(t *testing.T) {
	state := deployTestState(t)
	for _, unit := range state.Units {
		unit.MoveCooldown = 3
		unit.Moved = true
	}
	state.FleetStays = map[string]string{"TF1": "1010"}
	state.FleetSessions = map[string]int{"TF1": 2}
	events := StartBattle(&state)
	if state.Phase != PhaseBattle {
		t.Fatalf("开战后应在战斗阶段，实际 %q", state.Phase)
	}
	for _, unit := range state.Units {
		if unit.MoveCooldown != 0 || unit.Moved {
			t.Fatalf("开战后应清掉部署冷却：%+v", unit)
		}
	}
	if len(state.FleetStays) != 0 || len(state.FleetSessions) != 0 {
		t.Fatalf("开战后应重置停留计时：%v / %v", state.FleetStays, state.FleetSessions)
	}
	if len(events) != 1 || events[0].Type != "battle_start" {
		t.Fatalf("开战应产生 battle_start 事件，实际 %+v", events)
	}
	// 再点一次不应该重复开战。
	if again := StartBattle(&state); again != nil {
		t.Fatalf("重复开战应无事件，实际 %+v", again)
	}
	// 开战后整编重新带上冷却，部署区也不再是合法落点（回到相邻移动）。
	unit := state.Units["a1"]
	if moves := LegalMoves(state, unit); len(moves) > 6 {
		t.Fatalf("开战后只应能走相邻格，实际 %d 个落点", len(moves))
	}
}

// 没配部署区的剧本直接就是战斗阶段（老剧本/测试夹具不受影响）。
func TestNoZonesMeansBattlePhase(t *testing.T) {
	scenario := ScenarioDefinition{
		ID: "_battle", Name: "无部署区", Version: 1, Year: 1942, Wind: "SE",
		Deployments: []Deployment{{ID: "a1", Faction: "A", Unit: "dd", Coord: "0505"}},
	}
	gameMap := MapDefinition{Version: 1, Rows: 12, Cols: 12}
	catalog := UnitCatalog{Version: 1, Units: []UnitDefinition{
		{ID: "dd", Type: "DD", Name: "驱逐舰", Strength: 3, Card: ShipCard{SpeedClass: "normal", Gunnery: 2}},
	}}
	state, err := NewState(scenario, gameMap, catalog, 3)
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != PhaseBattle {
		t.Fatalf("没有部署区应直接进战斗阶段，实际 %q", state.Phase)
	}
	if len(state.DeployZones) != 0 {
		t.Fatalf("不该有部署区：%+v", state.DeployZones)
	}
}
