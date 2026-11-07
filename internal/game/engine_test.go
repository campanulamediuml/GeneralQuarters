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
	unit := state.Units["a-flag"]
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0203"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOrder(state, "B", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0203"}); err == nil {
		t.Fatal("允许操作敌方单位")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0304"}); err == nil {
		t.Fatal("允许非相邻移动")
	}

	state.Units["a-flag"].Coord = "0303"
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: unit.ID, Coord: "0304"}); err == nil {
		t.Fatal("允许进入岛屿")
	}
	state.Units["a-flag"].Coord = "0405"
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
