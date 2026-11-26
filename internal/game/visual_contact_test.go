package game

import "testing"

// TestVisualContactMakesFleetStrikeable 校验"地图上看得见的舰队就能打"：
// 目视范围内（2 hex）的舰队即使没有侦察机，也要留下情报记录，
// 否则前端点得到目标、服务端却因为没有情报拒绝打击。
func TestVisualContactMakesFleetStrikeable(t *testing.T) {
	state := State{
		Map: MapDefinition{Rows: 4, Cols: 9, Cells: []MapCell{{Coord: "0202", Terrain: "water"}, {Coord: "0204", Terrain: "water"}, {Coord: "0205", Terrain: "water"}}},
		Units: map[string]*Unit{
			"a": {ID: "a", FleetID: "A1", FleetName: "我方", Name: "我方", Type: "BB", Faction: "A", Coord: "0202", Strength: 6, MaxStrength: 6},
			"b": {ID: "b", FleetID: "B1", FleetName: "敌方", Name: "敌方", Type: "CA", Faction: "B", Coord: "0204", Strength: 4, MaxStrength: 4},
			"c": {ID: "c", FleetID: "B2", FleetName: "远敌", Name: "远敌", Type: "CA", Faction: "B", Coord: "0205", Strength: 4, MaxStrength: 4},
		},
		Contacts: map[string]map[string]*FleetContact{},
	}
	// 目视接触按正常节奏升级：每回合 +1，不直接刷到情报 4。
	for round := 1; round <= intelMaxLevel; round++ {
		advanceTurnStart(&state)
		detectVisualContacts(&state)
		contact := state.Contacts["A"]["B1"]
		if contact == nil || contact.Level != round {
			t.Fatalf("第%d回合目视接触等级错误：%+v", round, contact)
		}
		// 等级没满也要打得着：地图上画的是真实坐标，打击校验必须认可这个 hex。
		if err := ValidateStrikeTarget(state, "A", "0204"); err != nil {
			t.Fatalf("情报%d级时目视可见的舰队无法被设为打击目标：%v", round, err)
		}
	}
	detectVisualContacts(&state)
	if level := state.Contacts["A"]["B1"].Level; level != intelMaxLevel {
		t.Fatalf("情报等级超过上限：%d", level)
	}
	if err := ValidateStrikeTarget(state, "A", "0205"); err == nil {
		t.Fatal("视野外的舰队不应该能在没有侦察的情况下被打击")
	}
}
