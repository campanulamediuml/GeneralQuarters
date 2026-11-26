package game

import "testing"

// 拆分出来的新编队以前直接把 fleetID 当舰队名，地图和舰队面板上显示 split-<uuid>，
// 玩家分不清谁是谁；这里固定住"原舰队名·分队"的派生规则。
func TestReorganizedFleetGetsReadableName(t *testing.T) {
	state := State{Map: MapDefinition{Rows: 3, Cols: 3}, Units: map[string]*Unit{
		"a1": {ID: "a1", Name: "法拉格特", FleetID: "TF17", FleetName: "第17特混舰队", Faction: "A", Coord: "0101"},
		"a2": {ID: "a2", Name: "莫里斯", FleetID: "TF17", FleetName: "第17特混舰队", Faction: "A", Coord: "0101"},
		"a3": {ID: "a3", Name: "惠普尔", FleetID: "TF17", FleetName: "第17特混舰队", Faction: "A", Coord: "0101"},
	}}
	order := Order{Type: "REORGANIZE_HEX", Coord: "0101", Assignments: map[string]string{
		"a1": "TF17",
		"a2": "split-mwwjw452",
		"a3": "split-mwwjvyzw",
	}}
	ResolveOrders(&state, []Order{order})

	names := map[string]string{}
	for _, unit := range state.Units {
		names[unit.ID] = unit.FleetName
		if unit.FleetID != "TF17" && unit.FleetName == unit.FleetID {
			t.Fatalf("%s 的舰队名仍然是内部 id %q", unit.ID, unit.FleetName)
		}
	}
	// 先处理的第一支分队沿用"·分队"，第二支撞名后挂上旗舰名；顺序由 unitID 排序固定。
	if names["a2"] != "第17特混舰队·分队" {
		t.Fatalf("第一支分队名 = %q", names["a2"])
	}
	if names["a3"] != "第17特混舰队·惠普尔" {
		t.Fatalf("第二支分队名 = %q", names["a3"])
	}
	if names["a1"] != "第17特混舰队" {
		t.Fatalf("本队舰队名被改成了 %q", names["a1"])
	}
}
