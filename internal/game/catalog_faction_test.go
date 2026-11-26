package game

import "testing"

// TestFactionSpecificShipCards 校验同名舰型在美日两侧取到不同的角色卡。
func TestFactionSpecificShipCards(t *testing.T) {
	state := sandboxState(t)
	cases := []struct {
		unitID string
		heavy  float64
		light  float64
	}{
		{"a2-cv-01", 6.0, 30.0},
		{"b-cv-01", 8.0, 9.0},
		{"a-bb-01", 16.8, 16.8},
		{"b-bb-01", 9.8, 11.2},
		{"a-ca-01", 2.0, 6.0},
		{"b-ca-01", 4.0, 2.0},
		{"a-dd-01", 2.0, 3.0},
		{"b-dd-01", 3.0, 1.0},
	}
	for _, test := range cases {
		unit := state.Units[test.unitID]
		if unit == nil {
			t.Fatalf("缺少舰船%s", test.unitID)
		}
		if unit.Card.HeavyAA != test.heavy || unit.Card.LightAA != test.light {
			t.Fatalf("%s防空=%v/%v，期望%v/%v", test.unitID, unit.Card.HeavyAA, unit.Card.LightAA, test.heavy, test.light)
		}
	}
}

// TestFactionSpecificAircraft 校验同 ID 机种按载机阵营解析成不同型号与航程。
func TestFactionSpecificAircraft(t *testing.T) {
	state := sandboxState(t)
	cases := []struct {
		aircraftID string
		model      string
		baseRange  int
		loadout    Loadout
		maxRange   int
		radius     int
	}{
		// 美军：F6F/SB2C/TBF 14，SOC 12
		{"a2-cv-01-fighter-x1", "F4F 野猫", 10, LoadoutAirTank, 13, 6},
		{"a2-cv-01-dive_bomber-x1", "SBD 无畏", 13, LoadoutBombTank, 17, 8},
		{"a2-cv-01-torpedo_bomber-x1", "TBD 蹂躏者", 14, LoadoutTorpedoTank, 18, 9},
		{"air-a-11", "SOC 海鸥", 9, LoadoutReconTank, 12, 6},
		// 日军：零战 32、B5N 20、D3A 18、E13A 16
		{"b-cv-01-fighter-x1", "A6M2 零战", 32, LoadoutAirTank, 42, 21},
		{"b-cv-01-dive_bomber-x1", "D3A1 九九舰爆", 16, LoadoutBombTank, 21, 10},
		{"b-cv-01-torpedo_bomber-x1", "B5N2 九七舰攻", 16, LoadoutTorpedoTank, 21, 10},
		{"air-b-11", "E13A 零式水侦", 14, LoadoutReconTank, 18, 9},
	}
	for _, test := range cases {
		aircraft := state.Aircraft[test.aircraftID]
		if aircraft == nil {
			t.Fatalf("缺少飞机%s", test.aircraftID)
		}
		if aircraft.Model != test.model || aircraft.BaseRange != test.baseRange {
			t.Fatalf("%s型号/基础航程=%s/%d，期望%s/%d", test.aircraftID, aircraft.Model, aircraft.BaseRange, test.model, test.baseRange)
		}
		if aircraft.Loadout != test.loadout {
			t.Fatalf("%s默认挂载=%s，期望%s", test.aircraftID, aircraft.Loadout, test.loadout)
		}
		if aircraft.MaxRange != test.maxRange || aircraft.Range() != test.maxRange {
			t.Fatalf("%s航程=%d/%d，期望%d", test.aircraftID, aircraft.MaxRange, aircraft.Range(), test.maxRange)
		}
		if radius := aircraft.MaxRange / 2; radius != test.radius {
			t.Fatalf("%s作战半径=%d，期望%d", test.aircraftID, radius, test.radius)
		}
		if aircraft.Fuel.Reserve != ReserveRange {
			t.Fatalf("%s备用油=%d，期望%d", test.aircraftID, aircraft.Fuel.Reserve, ReserveRange)
		}
	}
}

// TestAirfieldAircraftUsesFaction 校验机场上的飞机同样按机场阵营取机种。
func TestAirfieldAircraftUsesFaction(t *testing.T) {
	state := sandboxState(t)
	japanese := state.GroundAir["af-b-fighter-1"]
	if japanese == nil || japanese.Model != "A6M2 零战" || japanese.BaseRange != 32 {
		t.Fatalf("日军机场战斗机解析错误：%+v", japanese)
	}
	american := state.GroundAir["af-a-fighter-1"]
	if american == nil || american.Model != "F4F 野猫" || american.BaseRange != 10 {
		t.Fatalf("美军机场战斗机解析错误：%+v", american)
	}
}

// TestDefinitionLookupFallsBackToGeneric 校验没写阵营的通用定义仍然可用。
func TestDefinitionLookupFallsBackToGeneric(t *testing.T) {
	catalog := UnitCatalog{
		Units: []UnitDefinition{{ID: "dd", Type: "DD", Name: "驱逐舰"}},
		Air:   []AircraftDefinition{{ID: "fighter", Kind: "fighter", Name: "战斗机", Model: "通用", Range: 10}},
	}
	units, err := indexUnitDefinitions(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if definition, ok := units.lookup("A", "dd"); !ok || definition.Name != "驱逐舰" {
		t.Fatalf("通用舰船定义回退失败：%+v", definition)
	}
	aircraft, err := indexAircraftDefinitions(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if definition, ok := aircraft.resolve("B", "af-b-fighter-1"); !ok || definition.Model != "通用" {
		t.Fatalf("通用机型定义回退失败：%+v", definition)
	}
}

// TestDuplicateFactionDefinitionRejected 校验同阵营同 ID 的重复定义会在装载时暴露。
func TestDuplicateFactionDefinitionRejected(t *testing.T) {
	catalog := UnitCatalog{Units: []UnitDefinition{
		{ID: "cv", Faction: "A"}, {ID: "cv", Faction: "A"},
	}}
	if _, err := indexUnitDefinitions(catalog); err == nil {
		t.Fatal("重复的阵营单位定义没有被拒绝")
	}
}
