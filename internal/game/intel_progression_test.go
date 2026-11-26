package game

import "testing"

// TestIntelLevelRisesOnePerTurn 锁死"情报等级按回合 1-2-3-4 走"：
// 同一回合里目视、舰队雷达、航空搜索、跟踪可能同时命中，按调用次数累加会让
// 一次接触两回合内从 0 冲到 4（珊瑚海实测 165 回合两源叠加直接满级）。
func TestIntelLevelRisesOnePerTurn(t *testing.T) {
	state := sandboxState(t)
	state.Contacts = map[string]map[string]*FleetContact{"A": {}}
	fleetID, members := enemyFleetForTest(&state)
	if fleetID == "" || len(members) == 0 {
		t.Fatal("沙盒剧本应有敌方舰队")
	}
	// 同一回合命中四次：只升一级。
	for i := 0; i < 4; i++ {
		if level := recordContact(&state, "A", fleetID, members); level != 1 {
			t.Fatalf("同一回合第 %d 次发现后情报等级应为 1，实际 %d", i+1, level)
		}
	}
	// 之后每个回合各来一次：2、3、4，然后封顶。
	for want := 2; want <= intelMaxLevel; want++ {
		advanceTurnStart(&state)
		if level := recordContact(&state, "A", fleetID, members); level != want {
			t.Fatalf("下一回合发现后情报等级应为 %d，实际 %d", want, level)
		}
	}
	advanceTurnStart(&state)
	if level := recordContact(&state, "A", fleetID, members); level != intelMaxLevel {
		t.Fatalf("情报等级应封顶在 %d，实际 %d", intelMaxLevel, level)
	}
}

// TestMultipleIntelSourcesSameTurnRaiseOnce 目视 + 其他侦查同回合命中，也只升一级。
func TestMultipleIntelSourcesSameTurnRaiseOnce(t *testing.T) {
	state := State{
		Map: MapDefinition{Rows: 4, Cols: 9, Cells: []MapCell{{Coord: "0202", Terrain: "water"}, {Coord: "0204", Terrain: "water"}}},
		Units: map[string]*Unit{
			"a": {ID: "a", FleetID: "A1", FleetName: "我方", Name: "我方", Type: "BB", Faction: "A", Coord: "0202", Strength: 6, MaxStrength: 6},
			"b": {ID: "b", FleetID: "B1", FleetName: "敌方", Name: "敌方", Type: "CA", Faction: "B", Coord: "0204", Strength: 4, MaxStrength: 4},
		},
		Contacts: map[string]map[string]*FleetContact{},
	}
	members := []Unit{*state.Units["b"]}
	advanceTurnStart(&state)
	// 先记一次"舰队侦查"，再走目视：同一回合两次命中。
	recordContact(&state, "A", "B1", members)
	detectVisualContacts(&state)
	if level := state.Contacts["A"]["B1"].Level; level != 1 {
		t.Fatalf("同回合多种侦查手段应只升一级，实际 %d", level)
	}
}

// TestStrikeContactJumpsToMax 空袭接敌 / 岸轰暴露走"直接满级"，不进每回合 +1 的节奏。
func TestStrikeContactJumpsToMax(t *testing.T) {
	state := sandboxState(t)
	state.Contacts = map[string]map[string]*FleetContact{"A": {}}
	fleetID, members := enemyFleetForTest(&state)
	if fleetID == "" || len(members) == 0 {
		t.Fatal("沙盒剧本应有敌方舰队")
	}
	if level := raiseContactToMax(&state, "A", fleetID, members); level != intelMaxLevel {
		t.Fatalf("接敌应直接给满级情报，实际 %d", level)
	}
	// 再调一次不能把等级推过头，也不能空转（recordContact 的 +1 换成循环会死循环）。
	if level := raiseContactToMax(&state, "A", fleetID, members); level != intelMaxLevel {
		t.Fatalf("重复接敌应停在满级，实际 %d", level)
	}
}
