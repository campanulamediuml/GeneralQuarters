package game

import "testing"

// 剧本可以对单舰属性做覆盖（例如珊瑚海四艘航母统一甲板长度 8），
// 没写的字段必须继续沿用单位库，不能被覆盖逻辑清空。
func TestDeploymentOverridesApply(t *testing.T) {
	_, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	strength, deck, lightAA := 5, 8, 12.5
	scenario := ScenarioDefinition{
		ID: "override-test", Name: "覆盖测试", Year: 1942,
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Deployments: []Deployment{{
			ID: "cv1", Unit: "cv", Faction: "A", Name: "测试航母", Coord: "2015",
			Overrides: &DeploymentOverrides{Strength: &strength, DeckLength: &deck, LightAA: &lightAA},
		}},
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	unit := state.Units["cv1"]
	if unit == nil {
		t.Fatal("部署没有生成舰船")
	}
	if unit.Strength != 5 || unit.MaxStrength != 5 {
		t.Fatalf("强度覆盖失败：%d/%d", unit.Strength, unit.MaxStrength)
	}
	if unit.Card.LightAA != 12.5 {
		t.Fatalf("轻防空覆盖失败：%.1f", unit.Card.LightAA)
	}
	EnsureAviation(&state)
	if unit.Deck == nil || unit.Deck.BaseLength != 8 {
		t.Fatalf("甲板长度覆盖失败：%+v", unit.Deck)
	}
	// 没写覆盖的字段必须保持单位库原值（这里取重防空做对照）。
	var baseCard *ShipCard
	for index := range catalog.Units {
		if catalog.Units[index].ID == "cv" && catalog.Units[index].Faction == "A" {
			card := catalog.Units[index].Card
			baseCard = &card
			break
		}
	}
	if baseCard == nil {
		t.Fatal("单位库缺少 cv")
	}
	if unit.Card.HeavyAA != baseCard.HeavyAA {
		t.Fatalf("未覆盖的重防空被改动了：%.2f vs %.2f", unit.Card.HeavyAA, baseCard.HeavyAA)
	}
}
