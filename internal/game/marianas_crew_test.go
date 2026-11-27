package game

import "testing"

// TestMarianasCarrierCrewTiers 校验马里亚纳日军的机组分层：
// 复活的中途岛四艘老航母（赤城/加贺/苍龙/飞龙）只能开老飞机，但机组全是王牌；
// 翔鹤/瑞鹤/隼鹰/飞鹰 是普通机组；其余舰载机（大凤、云龙级、信浓、轻航母）全新手。
func TestMarianasCarrierCrewTiers(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	carrierName := map[string]string{}
	for _, unit := range state.Units {
		if unit.Type == "CV" || unit.Type == "CVL" || unit.Type == "CVE" {
			carrierName[unit.ID] = unit.Name
		}
	}

	aced := map[string]bool{"赤城": true, "加贺": true, "苍龙": true, "飞龙": true}
	regular := map[string]bool{"翔鹤": true, "瑞鹤": true, "隼鹰": true, "飞鹰": true}
	oldAircraft := map[string]bool{"a6m2": true, "d3a1": true, "b5n2": true}

	counts := map[string]int{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != "B" {
			continue
		}
		name := carrierName[aircraft.CarrierID]
		if name == "" {
			continue
		}
		counts[aircraft.Skill]++
		switch {
		case aced[name]:
			if aircraft.Skill != PilotAce {
				t.Fatalf("%s 的机组应该是王牌，实际 %s", name, aircraft.Skill)
			}
			if !oldAircraft[aircraft.DefinitionID] {
				t.Fatalf("%s 是复活的老航母，只能开老飞机，实际机型 %s", name, aircraft.DefinitionID)
			}
		case regular[name]:
			if aircraft.Skill != PilotRegular {
				t.Fatalf("%s 的机组应该是普通，实际 %s", name, aircraft.Skill)
			}
		default:
			if aircraft.Skill != PilotRookie {
				t.Fatalf("%s 的机组应该是新手，实际 %s", name, aircraft.Skill)
			}
		}
	}
	if counts[PilotAce] != 66 {
		t.Fatalf("王王牌算子 %d 个，期望 66（赤城 18 + 加贺 18 + 苍龙 15 + 飞龙 15）", counts[PilotAce])
	}
	if counts[PilotRegular] != 60 {
		t.Fatalf("普通算子 %d 个，期望 60（翔鹤 18 + 瑞鹤 18 + 隼鹰 12 + 飞鹰 12）", counts[PilotRegular])
	}
	if counts[PilotVeteran] != 0 {
		t.Fatalf("日军舰载机不该有老兵档：%d", counts[PilotVeteran])
	}
}

// TestMarianasReferenceShipCards 把参考兵牌上的数字钉住：中途岛四艘航母的
// 生命/损管/防空、以及扶桑级两艘的重/轻防空。改数据时别把参考值撞掉。
func TestMarianasReferenceShipCards(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	want := map[string]struct {
		strength     int
		dc           int
		heavy, light float64
	}{
		"加贺": {4, 2, 8, 7},
		"赤城": {4, 2, 3, 7},
		"苍龙": {2, 2, 6, 7},
		"飞龙": {2, 2, 6, 7},
		"山城": {5, 5, 4, 21},
		"扶桑": {5, 5, 4, 21},
	}
	found := map[string]bool{}
	for _, unit := range state.Units {
		expect, ok := want[unit.Name]
		if !ok {
			continue
		}
		found[unit.Name] = true
		card := unit.Card
		if unit.Strength != expect.strength || card.DamageControl != expect.dc ||
			card.HeavyAA != expect.heavy || card.LightAA != expect.light {
			t.Fatalf("%s 兵牌 %d/%d/%.0f-%.0f，期望 %d/%d/%.0f-%.0f", unit.Name,
				unit.Strength, card.DamageControl, card.HeavyAA, card.LightAA,
				expect.strength, expect.dc, expect.heavy, expect.light)
		}
	}
	for name := range want {
		if !found[name] {
			t.Fatalf("没找到 %s", name)
		}
	}
}
