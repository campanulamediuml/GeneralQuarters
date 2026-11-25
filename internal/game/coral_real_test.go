package game

import "testing"

// 珊瑚海 · 真实比例：双方在各自的矩形出生区里随机出现，时间线按 20 分钟一回合配置。
func TestCoralRealSpawnsInBoxes(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	// 25 海里/hex 的真实比例：64 列 × 25 = 1600 海里，52 行 × 21.65 = 1126 海里，
	// 正好覆盖珊瑚海图幅（143°E–170°E，3.6°S–22.5°S）。
	if gameMap.Rows != 52 || gameMap.Cols != 64 {
		t.Fatalf("真实比例地图尺寸错误：%d×%d", gameMap.Rows, gameMap.Cols)
	}
	if scenario.Timeline == nil {
		t.Fatal("珊瑚海剧本必须带作战时间线")
	}
	if scenario.Timeline.ServiceStart != "04:00" || scenario.Timeline.LaunchStart != "05:20" || scenario.Timeline.RecoverBy != "19:40" || !scenario.Timeline.NightLoss {
		t.Fatalf("时间线配置错误：%+v", *scenario.Timeline)
	}
	if scenario.TurnsPerDay != 72 || scenario.DayHours != 0 {
		t.Fatalf("20 分钟一回合应配 72 回合/天、00:00 开局：%d / %.1f", scenario.TurnsPerDay, scenario.DayHours)
	}
	inBox := func(depBox DeployBox, coord string) bool {
		row, col, err := ParseCoord(coord)
		if err != nil {
			return false
		}
		return row >= depBox.RowMin && row <= depBox.RowMax && col >= depBox.ColMin && col <= depBox.ColMax
	}
	// 每个舰队各自一个出生框（MO 航母本队和第六战队分开部署）。
	boxes := map[string]DeployBox{}
	for _, deployment := range scenario.Deployments {
		if deployment.Box == nil {
			t.Fatalf("部署%s缺少出生区", deployment.ID)
		}
		key := deployment.Faction + "/" + deployment.FleetID
		if existing, ok := boxes[key]; ok && existing != *deployment.Box {
			t.Fatalf("同舰队出生区不一致：%+v / %+v", existing, *deployment.Box)
		}
		boxes[key] = *deployment.Box
	}
	spots := map[string]map[string]bool{"A": {}, "B": {}}
	for seed := uint64(1); seed <= 20; seed++ {
		state, err := NewState(scenario, gameMap, catalog, seed)
		if err != nil {
			t.Fatalf("种子%d部署失败：%v", seed, err)
		}
		coords := map[string]string{}
		fleetCoords := map[string]string{}
		for _, unit := range state.Units {
			key := unit.Faction + "/" + unit.FleetID
			if !inBox(boxes[key], unit.Coord) {
				t.Fatalf("种子%d %s落到了出生区外：%s", seed, key, unit.Coord)
			}
			if existing, ok := fleetCoords[key]; ok && existing != unit.Coord {
				t.Fatalf("同一舰队的舰船没有落在同一个 hex：%s / %s", existing, unit.Coord)
			}
			fleetCoords[key] = unit.Coord
			if unit.FleetID == "TF17" || unit.FleetID == "MO" {
				coords[unit.Faction] = unit.Coord
			}
		}
		distance := Distance(coords["A"], coords["B"])
		if distance < 12 || distance > 45 {
			t.Fatalf("种子%d双方出生距离异常：%d hex", seed, distance)
		}
		spots["A"][coords["A"]] = true
		spots["B"][coords["B"]] = true
	}
	if len(spots["A"]) < 3 || len(spots["B"]) < 3 {
		t.Fatalf("出生点没有随机化：%v", spots)
	}
}

// 真实比例的珊瑚海开局就是深夜，第一轮放飞要等到 05:20。
func TestCoralRealFirstTurnIsNight(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	UpdateDayNight(&state)
	if !state.Night {
		t.Fatal("00:00 开局应当是夜间")
	}
	if state.ServiceWindowOpen() {
		t.Fatal("04:00 之前不应该开始整备")
	}
	state.Turn = 17
	if !state.LaunchWindowOpen() {
		t.Fatalf("05:20（第 17 回合）应该可以放飞：%s", formatClock(clockAt(state)))
	}
}

// 珊瑚海（1942年5月）日军航母还没有对空雷达：翔鹤、瑞鹤、祥凤都不该带。
func TestCoralRealJapaneseCarriersHaveNoAirRadar(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"b-cv-01", "b-cv-02", "b-6s-cvl-01"} {
		unit := state.Units[id]
		if unit == nil {
			t.Fatalf("缺少航母 %s", id)
		}
		if unit.Card.AirRadar {
			t.Fatalf("%s在1942年5月不应有对空雷达", unit.Name)
		}
		if unit.Card.SurfaceRadar {
			t.Fatalf("%s在1942年5月不应有对海雷达", unit.Name)
		}
	}
}

// 1942年5月：日军舰队没有雷达；机场里只有汤斯维尔有对空雷达，凯恩斯已撤掉。
func TestCoralRealRadarFits(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range state.Units {
		if unit.Faction != "B" {
			continue
		}
		if unit.Card.SurfaceRadar || unit.Card.AirRadar {
			t.Fatalf("%s在1942年5月不应有雷达（对海=%v 对空=%v）", unit.Name, unit.Card.SurfaceRadar, unit.Card.AirRadar)
		}
	}
	for _, airfield := range state.Airfields {
		want := airfield.ID == "af-a-townsville"
		if airfield.Radar != want {
			t.Fatalf("%s雷达配置错误：%v（期望%v）", airfield.Name, airfield.Radar, want)
		}
	}
}

// 美军按史实拆成 TF17 / TF11 / TF44，并按卡面改成 CA 炮8、DD 炮5。
func TestCoralRealUSTaskForces(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"TF17": {"a-cv-02", "a-ca-03", "a-ca-04", "a-dd-02", "a-dd-04", "a-dd-01", "a-dd-05", "a-dd-06"},
		"TF11": {"a-cv-01", "a-ca-01", "a-ca-02", "a-ca-05", "a-dd-07", "a-dd-08", "a-dd-09", "a-dd-10", "a-dd-03"},
		"TF44": {"a-ca-06", "a-ca-07", "a-cl-01", "a-dd-11", "a-dd-12"},
	}
	for fleetID, ids := range want {
		seen := []string{}
		for _, unit := range state.Units {
			if unit.Faction == "A" && unit.FleetID == fleetID {
				seen = append(seen, unit.ID)
			}
		}
		if len(seen) != len(ids) {
			t.Fatalf("%s 编成数量错误：%v（期望%v）", fleetID, seen, ids)
		}
		for _, id := range ids {
			if state.Units[id] == nil {
				t.Fatalf("%s 缺少 %s", fleetID, id)
			}
		}
	}
	for _, id := range []string{"a-ca-03", "a-ca-04", "a-ca-01", "a-ca-06"} {
		unit := state.Units[id]
		if unit.Card.Gunnery != 8 || unit.Card.DamageControl != 3 || unit.Strength != 3 {
			t.Fatalf("%s 卡面错误：炮%d 损管%d 强度%d", unit.Name, unit.Card.Gunnery, unit.Card.DamageControl, unit.Strength)
		}
	}
	for _, id := range []string{"a-dd-01", "a-dd-04", "a-dd-07", "a-dd-11"} {
		unit := state.Units[id]
		if unit.Card.Gunnery != 5 {
			t.Fatalf("%s 火炮能力应为5，实际%d", unit.Name, unit.Card.Gunnery)
		}
	}
	if state.Units["a-ca-07"].Card.Nation != "australia" || state.Units["a-cl-01"].Card.Nation != "australia" {
		t.Fatal("澳大利亚与霍巴特应带 australia 国别标识")
	}
}

// 双方每个舰队都在自己的区域里随机出生：位置够分散，且不同舰队不会叠在同一个 hex。
func TestCoralRealFleetSpawnsAreRandomAndSeparate(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	spots := map[string]map[string]bool{}
	for seed := uint64(1); seed <= 40; seed++ {
		state, err := NewState(scenario, gameMap, catalog, seed)
		if err != nil {
			t.Fatalf("种子%d部署失败：%v", seed, err)
		}
		occupied := map[string]string{}
		for _, unit := range state.Units {
			key := unit.Faction + "/" + unit.FleetID
			if spots[key] == nil {
				spots[key] = map[string]bool{}
			}
			spots[key][unit.Coord] = true
			if other, exists := occupied[unit.Coord]; exists && other != key {
				t.Fatalf("种子%d 两个舰队叠在 %s：%s / %s", seed, unit.Coord, other, key)
			}
			occupied[unit.Coord] = key
		}
	}
	for fleet, coords := range spots {
		if len(coords) < 5 {
			t.Fatalf("%s 的出生点不够随机：40 个种子只有 %d 个位置", fleet, len(coords))
		}
	}
}

// 珊瑚海剧本的决定性目标是日军向莫尔兹比港投送 20 点货物，
// 运输船总载货必须足够，否则这个目标从开局就不可能完成。
func TestCoralRealMoresbySupplyObjective(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	var found *Objective
	for index := range scenario.Objectives {
		if scenario.Objectives[index].ID == "moresby-supply" {
			found = &scenario.Objectives[index]
		}
	}
	if found == nil || !found.Decisive || found.Kind != ObjectiveSupplies || found.Faction != "B" || found.Coord != "1611" || found.Value != 20 {
		t.Fatalf("莫尔兹比投送目标配置错误：%+v", found)
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	if cargo := factionCargoLeft(state, "B"); cargo < 20 {
		t.Fatalf("日军运输船总载货不足 20 点：%d", cargo)
	}
}

// 1942年5月美军舰队只有切斯特、约克城、芝加哥、澳大利亚、列克星敦有对空雷达，
// 全部美军舰船都没有对海雷达。
func TestCoralRealUSRadar(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	airRadar := map[string]bool{"chester": true, "yorktown": true, "chicago": true, "australia": true, "lexington": true}
	seen := 0
	for _, unit := range state.Units {
		if unit.Faction != "A" {
			continue
		}
		seen++
		if unit.Card.SurfaceRadar {
			t.Fatalf("%s不应有对海雷达", unit.Name)
		}
		if unit.Card.AirRadar != airRadar[unit.DefinitionID] {
			t.Fatalf("%s对空雷达配置错误：%v（期望%v）", unit.Name, unit.Card.AirRadar, airRadar[unit.DefinitionID])
		}
	}
	if seen == 0 {
		t.Fatal("没有找到美军舰船")
	}
}
