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

// 熟练度直接改写卡面数值：对空全机种修正，俯冲/鱼雷机再加对舰；新手不会把数值压到 0 以下。
func TestApplyPilotSkill(t *testing.T) {
	if ac, as := applyPilotSkill(PilotVeteran, "fighter", 4, 0); ac != 5 || as != 0 {
		t.Fatalf("老兵战斗机对空应为 5，实际 %d/%d", ac, as)
	}
	if ac, as := applyPilotSkill(PilotRookie, "torpedo_bomber", 1, 3); ac != 1 || as != 2 {
		t.Fatalf("新手鱼雷机应为 1/2，实际 %d/%d", ac, as)
	}
	if ac, _ := applyPilotSkill(PilotRookie, "fighter", 1, 0); ac != 1 {
		t.Fatalf("新手空战值最低压到 1，实际 %d", ac)
	}
	if ac, as := applyPilotSkill(PilotAce, "dive_bomber", 2, 2); ac != 4 || as != 4 {
		t.Fatalf("王牌俯冲轰炸机应为 4/4，实际 %d/%d", ac, as)
	}
	if ac, as := applyPilotSkill(PilotRegular, "fighter", 3, 0); ac != 3 || as != 0 {
		t.Fatalf("普通机组不应改动数值，实际 %d/%d", ac, as)
	}
}

// 部署时把熟练度落到算子上：同一机型的两种熟练度，对空值必须拉开，且 Skill 字段要能投影出去。
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
	if vet.Skill != PilotVeteran || vet.AirCombat != 5 {
		t.Fatalf("老兵应为 5 空战，实际 %s/%d", vet.Skill, vet.AirCombat)
	}
	if rook.Skill != PilotRookie || rook.AirCombat != 3 {
		t.Fatalf("新手应为 3 空战，实际 %s/%d", rook.Skill, rook.AirCombat)
	}
	if plain.Skill != PilotRegular || plain.AirCombat != 4 {
		t.Fatalf("缺省应是普通 4 空战，实际 %s/%d", plain.Skill, plain.AirCombat)
	}
}
