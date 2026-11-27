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
			if unit.Card.SpeedClass != "very_slow" {
				t.Fatalf("%s 不是极慢运输船：%s", unit.Name, unit.Card.SpeedClass)
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
		// 运输船单舰现实航速约 14 节（5,5,6 交替），24 艘的运输队是大型编队，再 -2 节 = 12 节。
		leader := state.Units[firstUnitOfFleet(state, id)]
		if knots := FleetCycleKnots(state, leader); knots != 12 {
			t.Fatalf("%s 航速 %d 节，24 艘的运输队应为 14-2=12 节", id, knots)
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

	// 日军五支任务群：甲/乙/丙/丁四支航母群 + 栗田的第一游击部队（含架空复活的陆奥）。
	// 舰名逐个点名，免得以后重排编成时数字对上了、船却放错了群。
	fleetWant := map[string]struct {
		size    int
		capital []string
	}{
		"TG1": {11, []string{"天城", "葛城", "云龙", "信浓"}},
		"TG2": {13, []string{"赤城", "加贺", "飞龙", "苍龙", "金刚", "榛名"}},
		"TG3": {13, []string{"翔鹤", "瑞鹤", "大凤", "龙凤"}},
		"TG4": {14, []string{"千岁", "千代田", "飞鹰", "隼鹰", "瑞凤", "扶桑", "山城"}},
		"TG5": {19, []string{"大和", "武藏", "陆奥", "长门", "比叡", "雾岛", "伊势", "日向"}},
	}
	for fleet, want := range fleetWant {
		if got := factionFleetSize(state, fleet); got != want.size {
			t.Fatalf("%s %d 艘，期望 %d", fleet, got, want.size)
		}
		for _, name := range want.capital {
			found := false
			for _, unit := range state.Units {
				if unit.FleetID == fleet && unit.Name == name {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s 缺少 %s", fleet, name)
			}
		}
	}
	// 陆奥是架空的复活船（史实 1943-06 主炮弹药库自爆），日军战列舰共 12 艘。
	battleships := 0
	for _, unit := range state.Units {
		if unit.Faction == "B" && unit.Type == "BB" {
			battleships++
		}
	}
	if battleships != 12 {
		t.Fatalf("日军战列舰 %d 艘，期望 12", battleships)
	}
	// 丁部队的轻航母屏护：铃谷/熊野 + 阿贺野 + 4 驱逐。
	for _, name := range []string{"铃谷", "熊野", "阿贺野"} {
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

	// 九座陆上机场各 8 点岸炮（2026-11 从 6 提到 8），水上飞机基地不带岸炮。
	for _, id := range []string{"af-b-saipan", "af-b-tinian", "af-b-guam", "af-b-rota", "af-b-yap", "af-b-palau", "af-b-truk", "af-b-iwo", "af-b-davao"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coastal != 8 {
			t.Fatalf("%s 岸炮不是 8：%+v", id, airfield)
		}
	}
	for _, id := range []string{"af-b-saipan-sea", "af-b-guam-sea", "af-b-palau-sea"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coastal != 0 {
			t.Fatalf("%s 水上基地不该有岸炮：%+v", id, airfield)
		}
	}
	// 一座岛一格：机场压在岛上，塞班 2763、天宁 2962、罗塔 3161、关岛 3361。
	for id, coord := range map[string]string{"af-b-saipan": "2763", "af-b-tinian": "2962", "af-b-rota": "3160", "af-b-guam": "3361"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coord != coord {
			t.Fatalf("%s 应该在 %s：%+v", id, coord, airfield)
		}
	}
	// 水上飞机基地停在岛外侧：塞班 2663、阿普拉 3360。
	for id, coord := range map[string]string{"af-b-saipan-sea": "2663", "af-b-guam-sea": "3360"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coord != coord {
			t.Fatalf("%s 应该在 %s：%+v", id, coord, airfield)
		}
	}
	// 四座岛机场旁边都得留一格开阔水面，不然舰队进不去、登陆和岸轰全都做不了（规则书 304 行）。
	for _, id := range []string{"af-b-saipan", "af-b-tinian", "af-b-rota", "af-b-guam"} {
		airfield := state.Airfields[id]
		if airfield == nil {
			t.Fatalf("缺少 %s", id)
		}
		if !fleetCanEnter(state, airfield.Coord, false) {
			t.Fatalf("%s 的 %s 没有开阔水面通道，舰队进不去，也就没法登陆/岸轰", id, airfield.Coord)
		}
	}
	// 并掉/搬走的岛格（2862 塞班、2963 天宁、3061 罗塔、3260 关岛）现在都是海。
	for _, cell := range state.Map.Cells {
		switch cell.Coord {
		case "2862", "2963", "3061", "3260":
			if cell.Terrain == "island" {
				t.Fatalf("%s 不应该还是岛", cell.Coord)
			}
		}
	}

	// 89×60 的地图（2026-11 左侧加宽 19 列、右侧砍掉 10 列），加上特鲁克/硫磺岛两座日军基地。
	if state.Map.Rows != 60 || state.Map.Cols != 89 {
		t.Fatalf("地图尺寸 %dx%d，期望 60x89", state.Map.Rows, state.Map.Cols)
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
	// 达沃是日军在菲律宾的航空中心：大型机场，6 零战 52 + 3 银河 + 3 一式陆攻。
	davao := state.Airfields["af-b-davao"]
	if davao == nil || davao.Type != AirfieldLarge || davao.Coord != "4815" {
		t.Fatalf("达沃机场不对：%+v", davao)
	}
	counts := map[string]int{}
	for _, aircraft := range state.GroundAir {
		if aircraft.AirfieldID == "af-b-davao" {
			counts[aircraft.DefinitionID]++
		}
	}
	if counts["a6m5"] != 6 || counts["g4m2"] != 3 || counts["p1y"] != 3 {
		t.Fatalf("达沃驻机 %v，期望 6 零战 52 + 3 银河 + 3 一式陆攻", counts)
	}
	// 日军两个部署区：甲/乙/丙三支航母群在地图西北角，丁部队和栗田的第一游击部队在西南、帕劳以北。
	for fleet, anchor := range map[string]string{"TG1": "1023", "TG2": "1023", "TG3": "1023", "TG4": "4221", "TG5": "4221"} {
		if zone := deployZoneForFleet(state.DeployZones, "B", fleet); zone == nil || zone.Anchor != anchor {
			t.Fatalf("%s 应该出生在 %s：%+v", fleet, anchor, zone)
		}
	}
	if zone := deployZoneForFleet(state.DeployZones, "A", "TF58.1"); zone == nil || zone.Anchor != "2384" {
		t.Fatalf("美军部署区锚点不对：%+v", zone)
	}

	supplyTargets := map[string]bool{}
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Faction == "A" && objective.Value == 40 {
			supplyTargets[objective.Coord] = true
		}
	}
	for _, coord := range []string{"2763", "2962", "3361"} {
		if !supplyTargets[coord] {
			t.Fatalf("缺少 %s 的 40 点投送目标", coord)
		}
	}

	// 把关岛目标队放到关岛机场格，停满 3 回合后开始卸货。
	const fleet = "TRN1"
	for _, unit := range state.Units {
		if unit.FleetID == fleet {
			unit.Coord = "3361"
		}
	}
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
	state.FleetSessions[fleet] = 3
	for turn := 0; turn < 12; turn++ {
		resolveUnloading(&state)
	}
	if got := state.ShoreSupplies["A|3361"]; got < 40 {
		t.Fatalf("关岛累计只卸下 %d 点，没到 40", got)
	}
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Coord == "3361" {
			if !objectiveCompleted(state, "A", objective) {
				t.Fatalf("关岛卸到 %d 点了，目标仍未完成", state.ShoreSupplies["A|3361"])
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
