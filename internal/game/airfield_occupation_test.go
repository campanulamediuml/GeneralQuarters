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

	start := airfield.Strength
	events := AdvanceAirfields(&state)
	if airfield.Strength == 0 {
		t.Fatal("滩头物资不该把机场直接打成 0")
	}
	if airfield.Strength != start {
		t.Fatalf("滩头物资还在时不该修复：%d -> %d", start, airfield.Strength)
	}
	for _, event := range events {
		if event.Type == "airfield_occupied" {
			t.Fatal("机场没有被压毁时不该发 airfield_occupied 战报")
		}
		if event.Type == "airfield_repaired" {
			t.Fatal("滩头物资还在时不该出现修复事件")
		}
	}
}

// 物资造成的伤害是永久的：清滩之后能修，但只能修回 MaxStrength - SupplyDamage。
func TestSupplyDamageNeverRepairsBack(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	airfield.MaxStrength = 8
	airfield.Strength = 3
	airfield.SupplyDamage = 3
	state.ShoreSupplies = map[string]int{}
	for turn := 0; turn < 200 && airfield.Strength < airfield.MaxStrength; turn++ {
		AdvanceAirfields(&state)
	}
	if airfield.Strength != 5 {
		t.Fatalf("维修上限应为 8-3=5，实际 %d", airfield.Strength)
	}
}

// 炮击清空物资之后维修重新开始。
func TestRepairResumesAfterSuppliesCleared(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	airfield.MaxStrength = 8
	airfield.Strength = 2
	key := "B|" + airfield.Coord
	state.ShoreSupplies = map[string]int{key: 2}
	for turn := 0; turn < 20; turn++ {
		AdvanceAirfields(&state)
	}
	if airfield.Strength != 2 {
		t.Fatalf("物资还在时不该修复，实际 %d", airfield.Strength)
	}
	delete(state.ShoreSupplies, key)
	for turn := 0; turn < 200 && airfield.Strength < airfield.MaxStrength; turn++ {
		AdvanceAirfields(&state)
	}
	if airfield.Strength != 8 {
		t.Fatalf("清滩后应修回满值，实际 %d", airfield.Strength)
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
	if airfield.SupplyDamage != 3 {
		t.Fatalf("物资伤害应记 3 点永久损毁，实际 %d", airfield.SupplyDamage)
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
