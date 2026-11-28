package game

import "testing"

// 珊瑚海 · 真实比例：美军在 3250 周围 8 hex、日军在 0831 周围 8 hex 的部署区里随机出生，
// 时间线按 20 分钟一回合配置。
func TestCoralRealSpawnsInDeployZones(t *testing.T) {
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
	if scenario.Timeline.ServiceStart != "04:00" || scenario.Timeline.LaunchStart != "05:20" || scenario.Timeline.RecoverBy != "19:40" {
		t.Fatalf("时间线配置错误：%+v", *scenario.Timeline)
	}
	if scenario.TurnsPerDay != 72 || scenario.DayHours != 0 {
		t.Fatalf("20 分钟一回合应配 72 回合/天、00:00 开局：%d / %.1f", scenario.TurnsPerDay, scenario.DayHours)
	}
	zones := map[string]DeployZone{}
	for _, zone := range scenario.DeployZones {
		zones[zone.Faction] = zone
	}
	if zones["A"].Anchor != "3250" || zones["A"].Radius != 8 {
		t.Fatalf("美军部署区配置错误：%+v", zones["A"])
	}
	if zones["B"].Anchor != "0831" || zones["B"].Radius != 8 {
		t.Fatalf("日军部署区配置错误：%+v", zones["B"])
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != PhaseDeploy {
		t.Fatalf("配了部署区的剧本新局应先进部署阶段，实际 %q", state.Phase)
	}
	for _, unit := range state.Units {
		zone, ok := zones[unit.Faction]
		if !ok {
			t.Fatalf("%s(%s) 没有部署区", unit.Name, unit.Faction)
		}
		if Distance(unit.Coord, zone.Anchor) > zone.Radius {
			t.Fatalf("%s出生在部署区外：%s（锚点%s 半径%d）", unit.Name, unit.Coord, zone.Anchor, zone.Radius)
		}
		carrier := IsCarrier(unit) || FleetHasCarrier(state, FleetKey(unit))
		if !fleetTerrainAllowsEntry(state.Map, unit.Coord, carrier, false) {
			t.Fatalf("%s出生在不可通行的格子：%s", unit.Name, unit.Coord)
		}
	}
}

func TestCoralRealProvidesSixDeployableSubmarinesPerSide(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, faction := range []string{"A", "B"} {
		deployable := 0
		reserve := 0
		for _, submarine := range state.Submarines {
			if submarine.Faction != faction {
				continue
			}
			if submarine.Reserve {
				reserve++
			} else {
				deployable++
			}
		}
		if deployable != 6 {
			t.Fatalf("%s 应有 6 艘可部署潜艇，实际 %d", faction, deployable)
		}
		if reserve != submarineReserveCount {
			t.Fatalf("%s 应有 %d 艘随机巡逻潜艇，实际 %d", faction, submarineReserveCount, reserve)
		}
	}
}

// 出生点要随机，但同一阵营的两支舰队不能叠在同一格：开局就叠在一起等于逼玩家先拆队。
func TestCoralRealFleetSpawnsAreRandomAndSeparate(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	spots := map[string]map[string]bool{"A": {}, "B": {}}
	for seed := uint64(1); seed <= 20; seed++ {
		state, err := NewState(scenario, gameMap, catalog, seed)
		if err != nil {
			t.Fatalf("种子%d部署失败：%v", seed, err)
		}
		fleetCoords := map[string]string{}
		for _, unit := range state.Units {
			key := unit.Faction + "/" + unit.FleetID
			if existing, ok := fleetCoords[key]; ok && existing != unit.Coord {
				t.Fatalf("同一舰队的舰船没有落在同一个 hex：%s / %s", existing, unit.Coord)
			}
			fleetCoords[key] = unit.Coord
		}
		seen := map[string]map[string]bool{}
		for key, coord := range fleetCoords {
			faction := key[:1]
			if seen[faction] == nil {
				seen[faction] = map[string]bool{}
			}
			if seen[faction][coord] {
				t.Fatalf("种子%d %s 两支舰队叠在 %s", seed, faction, coord)
			}
			seen[faction][coord] = true
			spots[faction][coord] = true
		}
	}
	for faction, set := range spots {
		if len(set) < 3 {
			t.Fatalf("%s出生点没有随机化：%v", faction, set)
		}
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

// 珊瑚海剧本的核心目标是日军向莫尔兹比港投送 20 点货物，
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
	if found == nil || found.Decisive || found.Kind != ObjectiveSupplies || found.Faction != "B" || found.Coord != "1611" || found.Value != 20 {
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

// 珊瑚海是 5 天 / 360 回合；航母不单独立目标，打沉航母只按机库容量计 VP。
func TestCoralRealTurnCap(t *testing.T) {
	scenario, _, _, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	if scenario.MaxTurns != 360 {
		t.Fatalf("珊瑚海回合上限应为 360（5天×72），实际 %d", scenario.MaxTurns)
	}
	if len(scenario.Objectives) != 2 {
		t.Fatalf("珊瑚海只应有投送和计分两条目标，实际 %+v", scenario.Objectives)
	}
	for _, objective := range scenario.Objectives {
		if objective.Kind == "carriers" {
			t.Fatalf("航母不应再有独立目标：%+v", objective)
		}
	}
}

// 珊瑚海日军 6 艘重巡各只有 1 个水机单位，机库容量也是 1。
func TestCoralRealHeavyCruisersOneFloatplane(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 11)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"b-ca-01", "b-ca-02", "b-6s-aoba", "b-6s-furutaka", "b-6s-kako", "b-6s-kinugasa"} {
		unit := state.Units[id]
		if unit == nil || unit.Card.HangarCapacity != 1 || unit.Card.Aircraft != 1 {
			t.Fatalf("%s 应为 1 个水机单位 / 1 机库：%+v", id, unit)
		}
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

// 祥凤的航空队：1 架 A5M 九六舰战、2 架 A6M2 零战、1 架 B5N2 九七舰攻。
func TestCoralRealShohoAirGroup(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, aircraft := range state.Aircraft {
		if aircraft.CarrierID == "b-6s-cvl-01" {
			counts[aircraft.DefinitionID]++
		}
	}
	want := map[string]int{"a5m": 1, "fighter": 2, "torpedo_bomber": 1}
	for id, count := range want {
		if counts[id] != count {
			t.Fatalf("祥凤的 %s 应为 %d 架，实际 %d（全部：%v）", id, count, counts[id], counts)
		}
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	if total != 4 {
		t.Fatalf("祥凤应载 4 架，实际 %d（%v）", total, counts)
	}
	if shoho := state.Units["b-6s-cvl-01"]; shoho == nil || shoho.Card.Aircraft != 4 {
		t.Fatalf("祥凤的载机计数应与实际算子一致：%+v", shoho)
	}
}
