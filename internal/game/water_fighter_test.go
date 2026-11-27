package game

import "testing"

// 伊势、日向是架空的"航空战列舰"：每艘 3 个零式水侦 + 3 个二式水战，
// 二式水战能从战列舰弹射起飞做 CAP，水侦做不了。
func TestWaterFighterCarriesAndCaps(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	for _, shipID := range []string{"jp-bb-10", "jp-bb-11"} {
		ship := state.Units[shipID]
		if ship == nil {
			t.Fatalf("缺少 %s", shipID)
		}
		if ship.Card.HangarCapacity != 6 || ship.Card.Aircraft != 6 {
			t.Fatalf("%s 卡面 %d/%d，期望 6/6", ship.Name, ship.Card.Aircraft, ship.Card.HangarCapacity)
		}
		fighter, recon := "", ""
		counts := map[string]int{}
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID != shipID {
				continue
			}
			counts[aircraft.DefinitionID]++
			if aircraft.DefinitionID == "f1m" && fighter == "" {
				fighter = aircraft.ID
			}
			if aircraft.DefinitionID == "e13a" && recon == "" {
				recon = aircraft.ID
			}
		}
		if counts["f1m"] != 3 || counts["e13a"] != 3 {
			t.Fatalf("%s 在舰算子 %v，期望 3 个二式水战 + 3 个零式水侦", ship.Name, counts)
		}
		if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: fighter}); err != nil {
			t.Fatalf("%s 的二式水战无法起飞做 CAP：%v", ship.Name, err)
		}
		if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: fighter, Heading: "W", SectorWidth: 60}); err == nil {
			t.Fatalf("%s 的二式水战不该能执行扇面搜索", ship.Name)
		}
		if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: recon}); err == nil {
			t.Fatalf("%s 的零式水侦不该能执行 CAP", ship.Name)
		}
	}
}
