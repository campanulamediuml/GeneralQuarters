package game

import "testing"

// 滩头物资不再把机场生命值按 0 计：卸一点就整座机场停摆是旧规则，
// 现在每点物资只扣 1 点生命值，机场照常走修复逻辑。
func TestEnemySuppliesDoNotZeroAirfield(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	airfield.Strength = airfield.MaxStrength
	state.ShoreSupplies = map[string]int{"B|" + airfield.Coord: 6}

	events := AdvanceAirfields(&state)
	if airfield.Strength == 0 {
		t.Fatal("滩头物资不该把机场直接打成 0")
	}
	for _, event := range events {
		if event.Type == "airfield_occupied" {
			t.Fatal("机场没有被压毁时不该发 airfield_occupied 战报")
		}
	}
}

// 每点物资 -1 生命值：一次卸 3 点就掉 3 点，打空之后才瘫痪。
func TestUnloadingDamagesAirfieldPerPoint(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	airfield.Strength = 5
	state.ShoreSupplies = map[string]int{}
	state.FleetSessions = map[string]int{"TRS-B": unloadStartTurns}
	state.FleetStays = map[string]string{"TRS-B": airfield.Coord}
	state.Units = map[string]*Unit{
		"t1": {ID: "t1", FleetID: "TRS-B", FleetName: "运输队", Name: "运输船1", Type: "TRS", Faction: "B",
			Coord: airfield.Coord, Strength: 2, MaxStrength: 2, Card: ShipCard{Cargo: 3, SpeedClass: "normal", DamageControl: 3}},
		"t2": {ID: "t2", FleetID: "TRS-B", FleetName: "运输队", Name: "运输船2", Type: "TRS", Faction: "B",
			Coord: airfield.Coord, Strength: 2, MaxStrength: 2, Card: ShipCard{Cargo: 3, SpeedClass: "normal", DamageControl: 3}},
		"t3": {ID: "t3", FleetID: "TRS-B", FleetName: "运输队", Name: "运输船3", Type: "TRS", Faction: "B",
			Coord: airfield.Coord, Strength: 2, MaxStrength: 2, Card: ShipCard{Cargo: 3, SpeedClass: "normal", DamageControl: 3}},
	}
	events := resolveUnloading(&state)
	if airfield.Strength != 2 {
		t.Fatalf("3 点物资应扣 3 点机场生命值：5 -> %d", airfield.Strength)
	}
	unloaded := false
	for _, event := range events {
		if event.Type == "troop_unloading" {
			unloaded = true
		}
		if event.Type == "airfield_occupied" {
			t.Fatal("机场还有 2 点生命值，不该判瘫痪")
		}
	}
	if !unloaded {
		t.Fatal("应产生卸载战报")
	}
}

// 前端/观战靠 AirfieldContact 上的 enemy_supplies 画红字，字段必须带出去。
func TestAirfieldContactExposesEnemySupplies(t *testing.T) {
	state := coralCombatState(t)
	airfield := state.Airfields["af-a-moresby"]
	state.ShoreSupplies = map[string]int{"B|" + airfield.Coord: 9}
	for _, contact := range VisibleAirfields(state, "A") {
		if contact.ID == airfield.ID {
			if contact.EnemySupplies != 9 {
				t.Fatalf("机场情报应带敌方滩头物资数量，实际 %d", contact.EnemySupplies)
			}
			return
		}
	}
	t.Fatal("没找到莫尔兹比机场的情报")
}
