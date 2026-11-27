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

	// 日军：栗田的第一游击部队把联合舰队的家底都带上了——11 艘战列舰（含架空复活的比叡/雾岛、
	// 老式扶桑/山城/伊势/日向）+ 3 艘重巡 + 2 艘轻巡 + 6 艘驱逐，共 22 艘。
	if got := factionFleetSize(state, "KURITA"); got != 22 {
		t.Fatalf("第一游击部队 %d 艘，期望 22", got)
	}
	bb := map[string]bool{}
	for _, unit := range state.Units {
		if unit.FleetID == "KURITA" && unit.Type == "BB" {
			bb[unit.Name] = true
		}
	}
	if len(bb) != 11 || !bb["比叡"] || !bb["雾岛"] || !bb["扶桑"] || !bb["伊势"] {
		t.Fatalf("栗田部队战列舰 %v，期望 11 艘且含比叡/雾岛/扶桑/伊势", bb)
	}
	// 甲部队把两个航空战队合编成六航母主力（20 艘，88 个航空算子挤一个 hex），丙部队仍是 3 航母编队。
	if got := factionFleetSize(state, "TG1"); got != 20 {
		t.Fatalf("甲部队 %d 艘，期望 20", got)
	}
	carriers := map[string]bool{}
	for _, unit := range state.Units {
		if unit.FleetID == "TG1" && unit.Type == "CV" {
			carriers[unit.Name] = true
		}
	}
	// 甲部队是复活的中途岛四艘老航母 + 两艘改装航母：精英机组只能开老飞机。
	for _, name := range []string{"赤城", "加贺", "飞龙", "苍龙", "隼鹰", "飞鹰"} {
		if !carriers[name] {
			t.Fatalf("六航母主力缺少 %s：%v", name, carriers)
		}
	}
	if got := factionFleetSize(state, "TG3"); got != 13 {
		t.Fatalf("丙部队 %d 艘，期望 13", got)
	}
	// 丁部队：信浓 + 千岁/千代田/瑞凤 + 高雄级全家 + 阿贺野 + 4 驱逐。
	if got := factionFleetSize(state, "TG4"); got != 14 {
		t.Fatalf("丁部队 %d 艘，期望 14", got)
	}
	for _, name := range []string{"信浓", "千岁", "千代田", "瑞凤", "高雄", "爱宕", "摩耶", "鸟海"} {
		found := false
		for _, unit := range state.Units {
			if unit.FleetID == "TG4" && unit.Name == name {
				found = true
			}
		}
		if !found {
			t.Fatalf("丁部队缺少 %s", name)
		}
	}
	// 信浓带 12 个算子（4 零战 + 4 彗星 + 4 天山），三艘轻航母各 7 个。
	jpCounts := map[string]map[string]int{}
	for _, aircraft := range state.Aircraft {
		if aircraft.CarrierID == "jp-cv-10" || aircraft.CarrierID == "jp-cvl-01" || aircraft.CarrierID == "jp-cvl-02" || aircraft.CarrierID == "jp-cvl-03" {
			if jpCounts[aircraft.CarrierID] == nil {
				jpCounts[aircraft.CarrierID] = map[string]int{}
			}
			jpCounts[aircraft.CarrierID][aircraft.DefinitionID]++
		}
	}
	if c := jpCounts["jp-cv-10"]; c["a6m5"] != 4 || c["d4y"] != 4 || c["b6n"] != 4 {
		t.Fatalf("信浓航空队 %v，期望 4/4/4", c)
	}
	for _, id := range []string{"jp-cvl-01", "jp-cvl-02", "jp-cvl-03"} {
		if c := jpCounts[id]; c["a6m5"] != 3 || c["d4y"] != 2 || c["b6n"] != 2 {
			t.Fatalf("%s 航空队 %v，期望 3/2/2", id, c)
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

	// 80×60 的地图（2026-11 往东加宽到 80 列），加上特鲁克/硫磺岛两座日军基地。
	if state.Map.Rows != 60 || state.Map.Cols != 80 {
		t.Fatalf("地图尺寸 %dx%d，期望 60x80", state.Map.Rows, state.Map.Cols)
	}
	if state.Airfields["af-b-truk"] == nil || state.Airfields["af-b-iwo"] == nil {
		t.Fatal("缺少特鲁克/硫磺岛机场")
	}
	if state.GroundAir["af-b-iwo-a6m5-1"] == nil || state.GroundAir["af-b-iwo-g4m2-1"] == nil {
		t.Fatal("硫磺岛没有驻机")
	}
	if state.GroundAir["af-b-truk-a6m5-1"] == nil || state.GroundAir["af-b-truk-g4m2-1"] == nil {
		t.Fatal("特鲁克没有驻机")
	}
	// 日军两个部署区：机动部队在帕劳以北，栗田的第一游击部队在硫磺岛西南。
	if zone := deployZoneForFleet(state.DeployZones, "B", "TG1"); zone == nil || zone.Anchor != "3717" {
		t.Fatalf("TG1 应该出生在帕劳以北：%+v", zone)
	}
	if zone := deployZoneForFleet(state.DeployZones, "B", "KURITA"); zone == nil || zone.Anchor != "1226" {
		t.Fatalf("KURITA 应该出生在硫磺岛西南：%+v", zone)
	}
	if zone := deployZoneForFleet(state.DeployZones, "B", "TG4"); zone == nil || zone.Anchor != "1226" {
		t.Fatalf("丁部队应该跟栗田同区出生：%+v", zone)
	}
	if zone := deployZoneForFleet(state.DeployZones, "A", "TF58.1"); zone == nil || zone.Anchor != "2365" {
		t.Fatalf("美军部署区锚点不对：%+v", zone)
	}

	supplyTargets := map[string]bool{}
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Faction == "A" && objective.Value == 40 {
			supplyTargets[objective.Coord] = true
		}
	}
	for _, coord := range []string{"2843", "2943", "3241"} {
		if !supplyTargets[coord] {
			t.Fatalf("缺少 %s 的 40 点投送目标", coord)
		}
	}

	// 把关岛目标队放到关岛机场格，停满 3 回合后开始卸货。
	const fleet = "TRN1"
	for _, unit := range state.Units {
		if unit.FleetID == fleet {
			unit.Coord = "3241"
		}
	}
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
	state.FleetSessions[fleet] = 3
	for turn := 0; turn < 12; turn++ {
		resolveUnloading(&state)
	}
	if got := state.ShoreSupplies["A|3241"]; got < 40 {
		t.Fatalf("关岛累计只卸下 %d 点，没到 40", got)
	}
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Coord == "3241" {
			if !objectiveCompleted(state, "A", objective) {
				t.Fatalf("关岛卸到 %d 点了，目标仍未完成", state.ShoreSupplies["A|3241"])
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
