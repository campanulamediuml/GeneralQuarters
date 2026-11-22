package game

import "testing"

// 珊瑚海 · 真实比例：双方在各自的矩形出生区里随机出现，时间线按 20 分钟一回合配置。
func TestCoralRealSpawnsInBoxes(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	if gameMap.Rows != 30 || gameMap.Cols != 60 {
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
