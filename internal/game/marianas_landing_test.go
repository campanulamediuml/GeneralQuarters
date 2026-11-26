package game

import "testing"

// TestMarianasLandingGroups 校验马里亚纳新加的三个登陆运输队：每队 12 艘慢速运输船 + 12 艘驱逐舰，
// 以及塞班/天宁/关岛各 40 点的投送目标；并且真的能在关岛卸到 40 点。
func TestMarianasLandingGroups(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	groups := map[string]struct{ trs, dd int }{}
	for _, unit := range state.Units {
		if unit.FleetID == "" || len(unit.FleetID) < 3 || unit.FleetID[:3] != "TRN" {
			continue
		}
		group := groups[unit.FleetID]
		switch unit.Type {
		case "TRS":
			group.trs++
			if unit.Card.Cargo <= 0 {
				t.Fatalf("%s 没有货", unit.Name)
			}
			if unit.Card.SpeedClass != "slow" {
				t.Fatalf("%s 不是慢速运输船：%s", unit.Name, unit.Card.SpeedClass)
			}
		case "DD":
			group.dd++
		}
		groups[unit.FleetID] = group
	}
	if len(groups) != 3 {
		t.Fatalf("运输队数量=%d，期望 3", len(groups))
	}
	for id, group := range groups {
		if group.trs != 12 || group.dd != 12 {
			t.Fatalf("%s 编成 %d 运输 + %d 驱逐，期望 12+12", id, group.trs, group.dd)
		}
		if interval := FleetMoveInterval(state, state.Units[firstUnitOfFleet(state, id)]); interval != 6 {
			t.Fatalf("%s 每 %d 回合移动一格，慢速运输队应为 6", id, interval)
		}
	}

	// 舰队航母：企业号就是原来的提康德罗加（us-cv-07），航空队 10 F6F + 8 SB2C + 6 TBF。
	enterprise := state.Units["us-cv-07"]
	if enterprise == nil || enterprise.Name != "企业" {
		t.Fatalf("us-cv-07 应该是企业号，实际 %+v", enterprise)
	}
	for _, id := range []string{"us-cv-01", "us-cv-07", "us-cv-08"} {
		counts := map[string]int{}
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID == id {
				counts[aircraft.DefinitionID]++
			}
		}
		if counts["f6f"] != 10 || counts["sb2c"] != 8 || counts["tbf"] != 6 {
			t.Fatalf("%s 航空队 %v，期望 10 F6F + 8 SB2C + 6 TBF", id, counts)
		}
	}

	// 日军：栗田的第一游击部队 7 艘战列舰（含架空复活的比叡/雾岛），三个机动部队各 10 艘。
	if got := factionFleetSize(state, "KURITA"); got != 13 {
		t.Fatalf("第一游击部队 %d 艘，期望 13", got)
	}
	bb := map[string]bool{}
	for _, unit := range state.Units {
		if unit.FleetID == "KURITA" && unit.Type == "BB" {
			bb[unit.Name] = true
		}
	}
	if len(bb) != 7 || !bb["比叡"] || !bb["雾岛"] {
		t.Fatalf("栗田部队战列舰 %v，期望 7 艘且含比叡/雾岛", bb)
	}
	for _, fleet := range []string{"TG1", "TG2", "TG3"} {
		if got := factionFleetSize(state, fleet); got != 10 {
			t.Fatalf("%s %d 艘，期望 10", fleet, got)
		}
	}

	// 六座岛屿机场各 6 点岸炮，水上飞机基地不带岸炮。
	for _, id := range []string{"af-b-saipan", "af-b-tinian", "af-b-guam", "af-b-rota", "af-b-yap", "af-b-palau"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coastal != 6 {
			t.Fatalf("%s 岸炮不是 6：%+v", id, airfield)
		}
	}
	for _, id := range []string{"af-b-saipan-sea", "af-b-guam-sea", "af-b-palau-sea"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coastal != 0 {
			t.Fatalf("%s 水上基地不该有岸炮：%+v", id, airfield)
		}
	}

	supplyTargets := map[string]bool{}
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Faction == "A" && objective.Value == 40 {
			supplyTargets[objective.Coord] = true
		}
	}
	for _, coord := range []string{"1874", "2073", "2574"} {
		if !supplyTargets[coord] {
			t.Fatalf("缺少 %s 的 40 点投送目标", coord)
		}
	}

	// 把关岛目标队放到关岛机场格，停满 3 回合后开始卸货。
	const fleet = "TRN1"
	for _, unit := range state.Units {
		if unit.FleetID == fleet {
			unit.Coord = "2574"
		}
	}
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
	state.FleetSessions[fleet] = 3
	for turn := 0; turn < 12; turn++ {
		resolveUnloading(&state)
	}
	if got := state.ShoreSupplies["A|2574"]; got < 40 {
		t.Fatalf("关岛累计只卸下 %d 点，没到 40", got)
	}
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Coord == "2574" {
			if !objectiveCompleted(state, "A", objective) {
				t.Fatalf("关岛卸到 %d 点了，目标仍未完成", state.ShoreSupplies["A|2574"])
			}
		}
	}
}

func factionFleetSize(state State, fleetID string) int {
	count := 0
	for _, unit := range state.Units {
		if unit.FleetID == fleetID {
			count++
		}
	}
	return count
}

func firstUnitOfFleet(state State, fleetID string) string {
	for id, unit := range state.Units {
		if unit.FleetID == fleetID {
			return id
		}
	}
	return ""
}
