package game

import "testing"

// 熟练度写在机场算子 ID 里：机种定义按 "-" 分段解析，熟练度也是一段。
func TestPilotSkillFromID(t *testing.T) {
	cases := map[string]string{
		"af-b-yap-a6m5-rookie-1":   PilotRookie,
		"af-b-saipan-a6m5-3":       PilotRegular,
		"af-b-rota-a6m5-veteran-2": PilotVeteran,
		"af-b-guam-p1y-ace-7":      PilotAce,
	}
	for id, want := range cases {
		if got := pilotSkillFromID(id); got != want {
			t.Fatalf("%s 熟练度应为 %s，实际 %s", id, want, got)
		}
	}
}

// 熟练度不再是卡面数值的加减，而是 DC 判定的骰子档位：
// 劣势（新手）= 2 骰取劣，普通 = 单骰，2 骰优势（老兵），3 骰优势（王牌）。
func TestPilotDiceAdvantageTiers(t *testing.T) {
	cases := map[string]struct {
		level int
		label string
	}{
		PilotRookie:  {0, "劣势"},
		PilotRegular: {1, "普通"},
		PilotVeteran: {2, "2 骰优势"},
		PilotAce:     {3, "3 骰优势"},
	}
	for skill, want := range cases {
		if got := PilotDiceAdvantage(skill); got != want.level {
			t.Fatalf("%s 骰子档位应为 %d，实际 %d", skill, want.level, got)
		}
		if got := PilotAdvantageLabel(skill); got != want.label {
			t.Fatalf("%s 档位名应为 %s，实际 %s", skill, want.label, got)
		}
	}
	// 认不出来的写法按普通。
	if got := PilotDiceAdvantage("weird"); got != 1 {
		t.Fatalf("未知熟练度应按普通，实际 %d", got)
	}
}

// 部署时把熟练度落到算子上：同一机型的两种熟练度卡面数值一样（差值只在 DC 掷骰里），
// 但 Skill 字段必须能投影出去，前端角标和 DC 判定都要认它。
func TestDeploymentAppliesPilotSkill(t *testing.T) {
	scenario := ScenarioDefinition{
		ID: "t", Version: 1, Units: "t",
		DeployZones: []DeployZone{{Faction: "A", Anchor: "0303", Radius: 2}},
		Deployments: []Deployment{{ID: "cv1", Faction: "A", Name: "航母", FleetID: "F", FleetName: "F", Ship: "cv"}},
		AirDeployments: []AirDeployment{
			{ID: "vet", Unit: "fighter", Carrier: "cv1", Skill: "veteran"},
			{ID: "rook", Unit: "fighter", Carrier: "cv1", Skill: "rookie"},
			{ID: "plain", Unit: "fighter", Carrier: "cv1"},
		},
	}
	gameMap := MapDefinition{ID: "t", Version: 1, Rows: 6, Cols: 6}
	catalog := UnitCatalog{Version: 1,
		Units: []UnitDefinition{{ID: "cv", Faction: "A", Type: "CV", Name: "航母", Strength: 4,
			Card: ShipCard{HangarCapacity: 10, DeckCapacity: 4, ElevatorCount: 1}}},
		Air: []AircraftDefinition{{ID: "fighter", Faction: "A", Name: "战斗机", Model: "F6F", Kind: "fighter", Strength: 2, Range: 10, AirCombat: 4}},
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatalf("建状态失败：%v", err)
	}
	vet, rook, plain := state.Aircraft["vet"], state.Aircraft["rook"], state.Aircraft["plain"]
	if vet == nil || rook == nil || plain == nil {
		t.Fatal("熟练度部署没有生成算子")
	}
	if vet.Skill != PilotVeteran || vet.AirCombat != 4 {
		t.Fatalf("老兵卡面空战值仍是机型原值 4，实际 %s/%d", vet.Skill, vet.AirCombat)
	}
	if rook.Skill != PilotRookie || rook.AirCombat != 4 {
		t.Fatalf("新手卡面空战值仍是机型原值 4，实际 %s/%d", rook.Skill, rook.AirCombat)
	}
	if plain.Skill != PilotRegular || plain.AirCombat != 4 {
		t.Fatalf("缺省应是普通 4 空战，实际 %s/%d", plain.Skill, plain.AirCombat)
	}
	if PilotDiceAdvantage(vet.Skill) != 2 || PilotDiceAdvantage(rook.Skill) != 0 {
		t.Fatal("老兵/新手的骰子档位没有拉开")
	}
}
