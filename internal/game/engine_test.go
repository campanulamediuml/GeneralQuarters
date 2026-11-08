package game

import (
	"reflect"
	"testing"
)

func TestCoordNeighborsAndDistance(t *testing.T) {
	row, col, err := ParseCoord("0203")
	if err != nil || row != 2 || col != 3 {
		t.Fatalf("坐标解析失败：%d %d %v", row, col, err)
	}
	if _, _, err := ParseCoord("2A03"); err == nil {
		t.Fatal("无效坐标未被拒绝")
	}
	neighbors := Neighbors("0202")
	expected := []string{"0201", "0203", "0102", "0302", "0101", "0301"}
	if !reflect.DeepEqual(neighbors, expected) {
		t.Fatalf("相邻坐标错误：%v", neighbors)
	}
	for _, coord := range neighbors {
		if Distance("0202", coord) != 1 {
			t.Fatalf("相邻坐标%s被错误计算为%d距离", coord, Distance("0202", coord))
		}
	}
	for _, origin := range []string{"0202", "0303", "0405"} {
		for _, coord := range Neighbors(origin) {
			if Distance(origin, coord) != 1 {
				t.Fatalf("奇偶行邻接不一致：%s到%s为%d", origin, coord, Distance(origin, coord))
			}
		}
	}
	if Distance("0202", "0103") != 2 {
		t.Fatal("隔格hex被错误计算为相邻")
	}
	if Distance("0202", "0203") != 1 {
		t.Fatal("相邻hex距离应为1")
	}
	if Distance("0202", "0208") <= 2 {
		t.Fatal("远距离hex计算错误")
	}
}

func TestScenarioValidationAndMovement(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_sandbox")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Units) != 9 || len(state.Units) != 62 || len(state.Aircraft) != 32 {
		t.Fatalf("沙盒部署数量错误：definitions=%d units=%d aircraft=%d", len(catalog.Units), len(state.Units), len(state.Aircraft))
	}
	fleetA := VisibleFleets(state, "A")
	fleetB := VisibleFleets(state, "B")
	if len(fleetA) != 2 || len(fleetB) != 2 {
		t.Fatalf("双方初始舰队数量错误：A=%+v B=%+v", fleetA, fleetB)
	}
	expectedFleets := map[string]int{"TG-A": 14, "TG-A-CV": 17, "TG-B": 17, "TG-B-BB": 14}
	for _, fleet := range append(fleetA, fleetB...) {
		if expectedFleets[fleet.FleetID] != fleet.Count {
			t.Fatalf("舰队%s编制错误：%+v", fleet.FleetID, fleet)
		}
	}
	types := map[string]bool{}
	for _, definition := range catalog.Units {
		types[definition.Type] = true
	}
	for _, shipType := range []string{"BB", "BC", "CA", "CL", "DD", "CV", "CVL", "TRS", "CVE"} {
		if !types[shipType] {
			t.Fatalf("单位库缺少舰船类型%s", shipType)
		}
	}
	unit := state.Units["a-bb-01"]
	if unit.DefinitionID != "bb" || unit.Card.Gunnery != 16 || unit.Card.DamageControl != 7 || !unit.Card.SurfaceRadar {
		t.Fatalf("战列舰角色卡生成错误：%+v", unit)
	}
	carrier := state.Units["b-cv-01"]
	if carrier.Card.Gunnery != 4 || carrier.Card.DeckCapacity != 9 || carrier.Card.ElevatorCount != 3 || carrier.Card.Aircraft != 4 || carrier.Card.HangarCapacity != 24 {
		t.Fatalf("航母角色卡生成错误：%+v", carrier)
	}
	if state.Definitions["units"] != "3" {
		t.Fatalf("未记录单位目录版本：%+v", state.Definitions)
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0203"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOrder(state, "B", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0203"}); err == nil {
		t.Fatal("允许操作敌方单位")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0304"}); err == nil {
		t.Fatal("允许非相邻移动")
	}

	state.Units["a-bb-01"].Coord = "0303"
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0304"}); err == nil {
		t.Fatal("允许进入岛屿")
	}
	state.Units["a-bb-01"].Coord = "0405"
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0506"}); err == nil {
		t.Fatal("允许进入暗礁")
	}
}

func TestResolveAndFogProjection(t *testing.T) {
	newState := func() State {
		return State{RNGState: 42, Map: MapDefinition{Rows: 3, Cols: 3}, Units: map[string]*Unit{
			"a": {ID: "a", Name: "A", Type: "BB", Faction: "A", Coord: "0101", Strength: 3, MaxStrength: 3},
			"b": {ID: "b", Name: "B", Type: "CA", Faction: "B", Coord: "0103", Strength: 3, MaxStrength: 3},
		}}
	}
	first, second := newState(), newState()
	orders := []Order{{Type: "SET_MOVE", UnitID: "a", Coord: "0102"}, {Type: "SET_MOVE", UnitID: "b", Coord: "0102"}}
	firstEvents := ResolveOrders(&first, orders)
	secondEvents := ResolveOrders(&second, orders)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstEvents, secondEvents) {
		t.Fatal("相同种子和命令未得到确定结果")
	}
	if len(firstEvents) == 0 {
		t.Fatal("接触战没有产生事件")
	}
	if first.Units["a"] != nil && first.Units["a"].Coord != "0102" {
		t.Fatal("移动未结算")
	}

	scenario, gameMap, catalog, _ := LoadScenario("../../content", "_sandbox")
	state, _ := NewState(scenario, gameMap, catalog, 42)
	for _, unit := range VisibleUnits(state, "A") {
		if unit.Faction == "B" {
			t.Fatalf("初始视野不应看到远端敌舰：%+v", unit)
		}
	}
}

func TestManualFleetMergeAndFleetMovement(t *testing.T) {
	state := State{RNGState: 42, Map: MapDefinition{Rows: 3, Cols: 3}, Units: map[string]*Unit{
		"a1": {ID: "a1", FleetID: "a1", Name: "旗舰", Type: "BB", Faction: "A", Coord: "0101", Strength: 3, MaxStrength: 3},
		"a2": {ID: "a2", FleetID: "a2", Name: "护航", Type: "DD", Faction: "A", Coord: "0101", Strength: 2, MaxStrength: 2},
	}}
	merge := Order{Type: "MERGE_FLEET", UnitID: "a2", TargetUnitID: "a1"}
	if err := ValidateOrder(state, "A", merge); err != nil {
		t.Fatal(err)
	}
	state.Units["a2"].Coord = "0102"
	if err := ValidateOrder(state, "A", merge); err == nil {
		t.Fatal("允许跨hex整编")
	}
	state.Units["a2"].Coord = "0101"
	events := ResolveOrders(&state, []Order{merge, {Type: "SET_MOVE", UnitID: "a1", Coord: "0102"}})
	if FleetKey(state.Units["a1"]) != "a1" || FleetKey(state.Units["a2"]) != "a1" {
		t.Fatal("整编未统一fleet_id")
	}
	if state.Units["a1"].Coord != "0102" || state.Units["a2"].Coord != "0102" {
		t.Fatal("编队成员未一起移动")
	}
	found := false
	for _, event := range events {
		if event.Type == "fleet_merged" {
			found = true
		}
	}
	if !found {
		t.Fatal("缺少整编战报")
	}
}

func TestHexReorganizationValidationAndApply(t *testing.T) {
	state := State{Map: MapDefinition{Rows: 3, Cols: 3}, Units: map[string]*Unit{
		"a1": {ID: "a1", FleetID: "a1", Faction: "A", Coord: "0101"},
		"a2": {ID: "a2", FleetID: "a1", Faction: "A", Coord: "0101"},
		"a3": {ID: "a3", FleetID: "a3", Faction: "A", Coord: "0101"},
		"b1": {ID: "b1", FleetID: "b1", Faction: "B", Coord: "0101"},
	}}
	order := Order{Type: "REORGANIZE_HEX", Coord: "0101", Assignments: map[string]string{"a1": "a1", "a2": "detached", "a3": "a1"}}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOrder(state, "B", order); err == nil {
		t.Fatal("整编允许包含其他阵营舰船")
	}
	invalid := order
	invalid.Assignments = map[string]string{"a1": "a1", "a2": "a1"}
	if err := ValidateOrder(state, "A", invalid); err == nil {
		t.Fatal("整编允许遗漏hex内舰船")
	}
	events := ResolveOrders(&state, []Order{order})
	if state.Units["a1"].FleetID != "a1" || state.Units["a3"].FleetID != "a1" || state.Units["a2"].FleetID != "detached" {
		t.Fatal("整编未正确应用转移、合并与拆分")
	}
	found := false
	for _, event := range events {
		if event.Type == "fleet_reorganized" {
			found = true
		}
	}
	if !found {
		t.Fatal("缺少整编事件")
	}
}

func TestVisibleFleetsAuthoritativeProjection(t *testing.T) {
	state := State{Map: MapDefinition{Rows: 3, Cols: 3}, Units: map[string]*Unit{
		"a1": {ID: "a1", FleetID: "fleet", Name: "旗舰", Type: "BB", Faction: "A", Coord: "0101", Heading: "E", Strength: 3, MaxStrength: 4, Moved: true},
		"a2": {ID: "a2", FleetID: "fleet", Name: "护航", Type: "DD", Faction: "A", Coord: "0101", Heading: "E", Strength: 2, MaxStrength: 2},
		"b1": {ID: "b1", FleetID: "b1", Name: "远敌", Type: "CA", Faction: "B", Coord: "0303", Strength: 3, MaxStrength: 3},
	}}
	fleets := VisibleFleets(state, "A")
	if len(fleets) != 1 {
		t.Fatalf("服务端舰队投影数量或视野错误：%+v", fleets)
	}
	fleet := fleets[0]
	if fleet.FleetID != "fleet" || fleet.Count != 2 || len(fleet.Members) != 2 {
		t.Fatalf("服务端舰队成员聚合错误：%+v", fleet)
	}
	if fleet.Strength != 5 || fleet.MaxStrength != 6 || !fleet.Moved || len(fleet.LegalMoves) != 0 {
		t.Fatalf("服务端舰队状态计算错误：%+v", fleet)
	}
	move := Order{Type: "SET_MOVE", UnitID: "a2", Coord: "0102"}
	if err := ValidateOrder(state, "A", move); err == nil {
		t.Fatal("混合编队通过未移动成员绕过了整队移动限制")
	}
	if moves := LegalMoves(state, state.Units["a2"]); len(moves) != 0 {
		t.Fatalf("已移动编队的未移动成员仍收到合法目标：%v", moves)
	}
}
