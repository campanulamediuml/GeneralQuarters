package game

import (
	"math"
	"strings"
	"testing"
)

// 夜间水上飞机规则（规则书"夜间航空行动"）：只对"从水上飞机基地出发的水上飞机"开口子——
// 回基地不判 20% 摔毁、天黑不收场、可以继续排航空任务；代价是夜间侦察效率在全部修正之后
// 再 ×50%。舰队自带的水侦不算，夜里回收照样难。
func loadNightMarianas(t *testing.T) State {
	t.Helper()
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}
	return state
}

func TestNightSeaplaneLandingExempt(t *testing.T) {
	state := loadNightMarianas(t)
	state.Night = true

	seaplane := state.GroundAir["af-b-saipan-sea-h8k-1"]
	landPlane := state.GroundAir["af-b-saipan-a6m5-1"]
	if seaplane == nil || landPlane == nil {
		t.Fatal("马里亚纳缺少水上飞机基地算子或陆基算子")
	}
	if !fromSeaplaneBase(state, seaplane) {
		t.Fatalf("水上飞机基地的 %s 没被认成基地水机", seaplane.Model)
	}
	if fromSeaplaneBase(state, landPlane) {
		t.Fatalf("陆基机场的 %s 被认成了基地水机", landPlane.Model)
	}

	// 回水上飞机基地：掷多少都不摔。
	for i := 0; i < 300; i++ {
		if crashed, _ := nightGroundLandingCrash(&state, seaplane); crashed {
			t.Fatal("水上飞机回基地被判了夜间降落摔毁")
		}
	}

	// 同一个夜里，陆基机场的零战照旧按 20% 判。
	template := *landPlane
	crashes := 0
	for i := 0; i < 300; i++ {
		copy := template
		state.GroundAir[template.ID] = &copy
		if crashed, _ := nightGroundLandingCrash(&state, &copy); crashed {
			crashes++
		}
	}
	if crashes == 0 {
		t.Fatal("陆基机场夜间降落一次都没摔，判定没生效")
	}
	if crashes > 120 {
		t.Fatalf("陆基机场夜间摔毁 %d/300，明显超过 20%%", crashes)
	}
}

func TestNightSeaplaneReconHalved(t *testing.T) {
	state := loadNightMarianas(t)
	aircraft := state.GroundAir["af-b-saipan-sea-h8k-1"]
	if aircraft == nil {
		t.Fatal("缺少水上飞机基地算子")
	}
	// 拿一艘美军驱逐舰当目标，并把起飞点压到它身上：省掉每格 -1% 的距离修正，
	// 也避免发现率本来就贴着 100% 被封顶、让"折半"看不出比例。
	// 跟踪成功率里带天气系数，而天气按被跟踪舰队的坐标查：遍历 map 随手取第一艘驱逐舰，
	// 会在"恰好取到坏天气那一队"时把成功率压到 18%，用例随机失败。按 id 排序取天气最好的
	// 那艘，前提就稳定了（这里测的是夜间折半，不是天气）。
	var members []Unit
	bestWeather := -1.0
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit.Faction != "A" || unit.Type != "DD" {
			continue
		}
		// 这个用例要的是"没封顶的发现率"：只取一艘船当目标，编队加成最小时才看得出折半。
		if factor := weatherFactor(weatherAt(state.Map, unit.Coord)); factor > bestWeather {
			bestWeather = factor
			members = []Unit{*unit}
		}
	}
	if len(members) == 0 {
		t.Fatal("剧本里没有美军驱逐舰")
	}
	aircraft.OriginCoord = members[0].Coord

	state.Night = false
	day := groundReconChance(state, aircraft, members)
	state.Night = true
	night := groundReconChance(state, aircraft, members)
	if day <= 0 {
		t.Fatalf("白天发现率为 %.2f，测试前提不成立", day)
	}
	if day >= 100 {
		t.Fatalf("白天发现率已经封顶（%.2f），没法验证折半", day)
	}
	if math.Abs(night-day*0.5) > 0.001 {
		t.Fatalf("夜间发现率 %.2f，期望是白天 %.2f 的一半", night, day)
	}
}

func TestNightSeaplaneOrdersAllowed(t *testing.T) {
	state := loadNightMarianas(t)
	nightTurn := 0
	for turn := 1; turn <= 72; turn++ {
		probe := state
		probe.Turn = turn
		if !probe.ServiceWindowOpen() {
			nightTurn = turn
			break
		}
	}
	if nightTurn == 0 {
		t.Fatal("找不到夜间回合")
	}
	state.Turn = nightTurn
	if state.ServiceWindowOpen() {
		t.Fatalf("第 %d 回合不是夜间窗口", nightTurn)
	}

	seaplaneOrder := Order{Type: "LAUNCH_RECON", AircraftIDs: []string{"af-b-saipan-sea-h8k-1"}, Heading: "E", SectorWidth: 60}
	if err := ValidateGroundAirOrder(state, seaplaneOrder); err != nil {
		t.Fatalf("水上飞机夜间侦察被拦下了：%v", err)
	}
	landOrder := Order{Type: "LAUNCH_RECON", AircraftIDs: []string{"af-b-saipan-a6m5-1"}, Heading: "E", SectorWidth: 60}
	err := ValidateGroundAirOrder(state, landOrder)
	if err == nil || !strings.Contains(err.Error(), "夜间") {
		t.Fatalf("陆基零战夜间也能安排任务了：%v", err)
	}
}

func TestShipFloatplaneNightRecoveryStillHard(t *testing.T) {
	state := loadNightMarianas(t)
	state.Night = true
	var template *AirUnit
	for _, aircraft := range state.Aircraft {
		if aircraft.Seaplane && state.Units[aircraft.CarrierID] != nil {
			copied := *aircraft
			template = &copied
			break
		}
	}
	if template == nil {
		t.Skip("剧本里没有舰载水侦")
	}
	carrier := state.Units[template.CarrierID]
	crashes := 0
	for i := 0; i < 60; i++ {
		craft := *template
		craft.Coord = carrier.Coord
		craft.Status = "returning"
		craft.Returning = true
		state.Aircraft[craft.ID] = &craft
		for _, event := range advanceAircraftReturn(&state, state.Aircraft[craft.ID]) {
			if event.Type == "night_landing_crash" {
				crashes++
			}
		}
	}
	if crashes == 0 {
		t.Fatal("舰队自带的水侦夜间回收一次都没摔，太便宜了")
	}
}

// 夜间水上飞机基地的水上飞机：除了发现率，持续跟踪的成功率也在全部修正之后 ×50%。
func TestNightSeaplaneTrackingHalved(t *testing.T) {
	state := loadNightMarianas(t)
	aircraft := state.GroundAir["af-b-saipan-sea-h8k-1"]
	if aircraft == nil {
		t.Fatal("缺少水上飞机基地算子")
	}
	// 跟踪是按舰队算的（50% + 编队里每艘船的加成），生产代码走的是
	// fleetMembers(state, TrackingFleetID)。这里以前手工塞了一艘驱逐舰当 members，
	// 于是编队加成只有 +1%，而且天气随"map 遍历先碰到哪艘船"漂移，用例会随机失败。
	var members []Unit
	bestWeather := -1.0
	trackedFleet := ""
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit.Faction != "A" || unit.Type != "DD" {
			continue
		}
		fleet := fleetMembers(&state, FleetKey(unit))
		if len(fleet) == 0 {
			continue
		}
		if factor := weatherFactor(weatherAt(state.Map, fleet[0].Coord)); factor > bestWeather {
			bestWeather = factor
			members = fleet
			trackedFleet = FleetKey(unit)
		}
	}
	if len(members) == 0 {
		t.Fatal("剧本里没有美军驱逐舰")
	}
	aircraft.TrackingFleetID = trackedFleet

	tracked := func(night bool, trials int) int {
		state.Night = night
		hits := 0
		for i := 0; i < trials; i++ {
			for _, event := range resolveGroundTracking(&state, aircraft, members) {
				if event.Type == "recon_tracking" && strings.Contains(event.Text, "提升至") {
					hits++
				}
			}
		}
		return hits
	}

	const trials = 6000
	day := tracked(false, trials)
	night := tracked(true, trials)
	if day < trials/4 {
		t.Fatalf("白天跟踪成功 %d/%d，成功率太低，测试前提不成立", day, trials)
	}
	ratio := float64(night) / float64(day)
	if math.Abs(ratio-0.5) > 0.05 {
		t.Fatalf("夜间跟踪成功 %d、白天 %d（比例 %.2f），期望正好折半", night, day, ratio)
	}
}

// 同样的夜间折半也作用在"扇面扫到敌方机场"的 30% 判定上。
func TestNightSeaplaneAirfieldDiscoveryHalved(t *testing.T) {
	state := loadNightMarianas(t)
	aircraft := state.GroundAir["af-b-saipan-sea-h8k-1"]
	if aircraft == nil {
		t.Fatal("缺少水上飞机基地算子")
	}
	// 造一座美军机场放在扇面里，别去动剧本自带的日军机场。
	const coord = "2384"
	state.Airfields["test-us-airfield"] = &Airfield{ID: "test-us-airfield", Faction: "A", Name: "测试机场", Coord: coord, Type: AirfieldSmall, Runways: 2, Strength: 10}

	discovered := func(night bool, trials int) int {
		state.Night = night
		hits := 0
		for i := 0; i < trials; i++ {
			if len(discoverEnemyAirfieldsAt(&state, aircraft, []string{coord})) > 0 {
				hits++
			}
		}
		return hits
	}

	const trials = 6000
	day := discovered(false, trials)
	night := discovered(true, trials)
	if day < trials/4 {
		t.Fatalf("白天机场发现 %d/%d，30%% 判定没生效", day, trials)
	}
	ratio := float64(night) / float64(day)
	if math.Abs(ratio-0.5) > 0.06 {
		t.Fatalf("夜间机场发现 %d、白天 %d（比例 %.2f），期望折半", night, day, ratio)
	}
}
