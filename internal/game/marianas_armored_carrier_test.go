package game

import "testing"

// 马里亚纳里日军只有大凤、信浓算装甲航母（大凤级和信浓改大和都是用装甲飞行甲板
// 换机库容积的设计），其余日本航母——包括云龙级和改装的轻母——都不是。
// 这张清单直接决定炸弹命中后的甲板损坏概率和机库保护，改卡时别把它改丢。
func TestMarianasArmoredCarriers(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	armored := map[string]bool{}
	for _, unit := range state.Units {
		if unit.Card.ArmoredCarrier {
			armored[unit.Name] = true
			if unit.Type != "CV" {
				t.Fatalf("%s 是 %s，装甲航母只可能是 CV", unit.Name, unit.Type)
			}
			if unit.Faction != "B" {
				t.Fatalf("%s 属于 %s，美军这一局没有装甲航母", unit.Name, unit.Faction)
			}
		}
	}
	if len(armored) != 2 || !armored["大凤"] || !armored["信浓"] {
		t.Fatalf("装甲航母应只有大凤、信浓，实际 %v", armored)
	}

	// 卡面数值和剧本附录那张表对得上：甲板容量、机库容量、损管。
	// 两艘的损管都是 2——装甲甲板只保护机库，新舰和管损不力的账照算。
	want := map[string]struct {
		deck, hangar int
		dc           int
	}{
		"大凤": {8, 20, 2},
		"信浓": {7, 15, 2},
	}
	for _, unit := range state.Units {
		expect, ok := want[unit.Name]
		if !ok {
			continue
		}
		if unit.Card.DeckCapacity != expect.deck || unit.Card.HangarCapacity != expect.hangar || unit.Card.DamageControl != expect.dc {
			t.Fatalf("%s 卡面应为甲板 %d / 机库 %d / 损管 %d，实际 %d / %d / %d",
				unit.Name, expect.deck, expect.hangar, expect.dc,
				unit.Card.DeckCapacity, unit.Card.HangarCapacity, unit.Card.DamageControl)
		}
	}
}
