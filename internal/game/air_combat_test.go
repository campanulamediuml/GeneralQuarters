package game

import "testing"

func TestAirCombatChanceTable(t *testing.T) {
	cases := []struct {
		difference int
		want       float64
	}{
		{-8, 5}, {-3, 10}, {-2, 15}, {-1, 20}, {0, 25}, {1, 35}, {2, 45}, {3, 55}, {4, 65}, {5, 75}, {6, 85}, {9, 95},
	}
	for _, test := range cases {
		if got := airCombatChance(test.difference, 0); got != test.want {
			t.Fatalf("差值%d命中率=%v，期望%v", test.difference, got, test.want)
		}
	}
}

func TestStrikeAllowsEscortFighters(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"air-b-01", "air-b-03"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("护航战斗机不能加入攻击编队：%v", err)
	}
}

func TestAirStrikeResolvesCAPAndReturns(t *testing.T) {
	state := sandboxState(t)
	state.Aircraft = map[string]*AirUnit{
		"strike-a": {ID: "strike-a", CarrierID: "a2-cv-01", Faction: "A", Model: "SB2C", Kind: "dive_bomber", Status: "attack", Strength: 2, AirCombat: 2, AntiShip: 4, TaskID: "strike-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 12, MaxRange: 12},
		"escort-a": {ID: "escort-a", CarrierID: "a2-cv-01", Faction: "A", Model: "F6F", Kind: "fighter", Status: "attack", Strength: 2, AirCombat: 5, TaskID: "strike-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 13, MaxRange: 13},
		"cap-b":    {ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Model: "A6M", Kind: "fighter", Status: "cap", Strength: 2, AirCombat: 5, TaskID: "cap-b", TaskKind: "cap", Coord: "0608", RemainingRange: 13, MaxRange: 13},
	}
	state.Units["b-cv-01"].Coord = "0608"
	state.RNGState = 42
	events := resolveAirStrike(&state, "strike-a", "0608")
	if len(state.AirCombats) != 1 {
		t.Fatalf("没有生成空战报告：%+v", events)
	}
	report := state.AirCombats[0]
	if report.AttackerCount != 2 || report.DefenderCount != 1 || report.AttackerFaction != "A" || report.DefenderFaction != "B" {
		t.Fatalf("空战报告双方错误：%+v", report)
	}
	if report.Winner != "draw" {
		t.Fatalf("幸存攻击机被返航状态误判为全灭：%+v", report)
	}
	if contact := state.Contacts["A"]["TG-B"]; contact == nil || contact.Level != 4 {
		t.Fatalf("空袭接敌后情报没有提升到4：%+v", contact)
	}
	if attacker := state.Aircraft["strike-a"]; attacker != nil && !attacker.Returning {
		t.Fatalf("完成空袭后攻击机没有返航：%+v", attacker)
	}
	if len(events) == 0 || events[len(events)-1].Type != "air_strike_resolved" {
		t.Fatalf("缺少空袭完成战报：%+v", events)
	}
}

func TestAirStrikeWithoutCAPSkipsDogfightReport(t *testing.T) {
	state := sandboxState(t)
	state.Aircraft = map[string]*AirUnit{
		"strike-a": {ID: "strike-a", CarrierID: "a2-cv-01", Faction: "A", Model: "TBF", Kind: "torpedo_bomber", Status: "attack", Strength: 2, AirCombat: 1, AntiShip: 5, TaskID: "strike-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 12, MaxRange: 12},
	}
	state.RNGState = 7
	resolveAirStrike(&state, "strike-a", "0608")
	if len(state.AirCombats) != 0 {
		t.Fatal("没有CAP时不应生成空战动画报告")
	}
	if attacker := state.Aircraft["strike-a"]; attacker != nil && !attacker.Returning {
		t.Fatalf("无CAP空袭后没有返航：%+v", attacker)
	}
}

func TestAttackReturnLandsAndClearsConfusion(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "strike-a", CarrierID: "a2-cv-01", Faction: "A", Model: "SB2C", Kind: "dive_bomber", Status: "attack", Strength: 2, TaskID: "strike-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 6, MaxRange: 12, Returning: true, Confusion: true}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	carrier := state.Units[aircraft.CarrierID]
	carrier.Deck.Operation = ""
	for range 3 {
		advanceAircraftReturn(&state, aircraft)
		carrier.Deck.Operation = ""
	}
	if aircraft.Status != "recovery" || aircraft.Confusion || aircraft.TaskID != "" {
		t.Fatalf("返航降落状态错误：%+v", aircraft)
	}
}

func TestAviationQueuePositionsFollowPriority(t *testing.T) {
	state := sandboxState(t)
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"})
	ApplyAirOrder(&state, Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60})
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 2 || view.Tasks[0].QueuePosition != 1 || view.Tasks[1].QueuePosition != 2 {
		t.Fatalf("队列位置不连续：%+v", view.Tasks)
	}
	ApplyAirOrder(&state, Order{Type: "PRIORITIZE_AIR_TASK", AircraftID: "air-b-10"})
	view = VisibleAviation(state, "B")
	if view.Tasks[0].Kind != "recon" || view.Tasks[0].QueuePosition != 1 || view.Tasks[1].QueuePosition != 2 {
		t.Fatalf("优先级调整后队列错误：%+v", view.Tasks)
	}
}

func TestAmmunitionDepletionThresholds(t *testing.T) {
	cases := []struct {
		faction  string
		own      int
		opposing int
		want     float64
	}{
		{"A", 5, 1, 10}, {"A", 3, 1, 25}, {"A", 2, 1, 50}, {"A", 1, 2, 75},
		{"B", 6, 1, 10}, {"B", 4, 1, 25}, {"B", 2, 1, 50}, {"B", 1, 2, 75},
	}
	for _, test := range cases {
		if got := ammunitionDepletionChance(test.faction, test.own, test.opposing); got != test.want {
			t.Fatalf("%s弹药比例%d/%d=%v，期望%v", test.faction, test.own, test.opposing, got, test.want)
		}
	}
}

func TestStrikeWeightsAndHitModifiers(t *testing.T) {
	weights := map[string]int{"BB": 10, "BC": 10, "CV": 100, "CA": 3, "CL": 1, "DD": 1, "TRS": 11}
	for shipType, want := range weights {
		if got := strikeShipWeight(shipType); got != want {
			t.Fatalf("%s目标权重=%d，期望%d", shipType, got, want)
		}
	}
	state := sandboxState(t)
	attacker := &AirUnit{Kind: "dive_bomber"}
	target := state.Units["b-cv-01"]
	if got := strikeHitChance(state, attacker, target, true, false); got != 30 {
		t.Fatalf("混合编队且无CAP时命中率=%v，期望30", got)
	}
	if got := strikeHitChance(state, attacker, target, false, true); got != 15 {
		t.Fatalf("轻防空修正错误：%v", got)
	}
	attacker.Confusion = true
	state.Map.Cells = append(state.Map.Cells, MapCell{Coord: target.Coord, Weather: "rain"})
	if got := strikeHitChance(state, attacker, target, false, true); got != 5 {
		t.Fatalf("天气和混乱没有正确降低命中率：%v", got)
	}
}

func TestLightAAModifiers(t *testing.T) {
	state := sandboxState(t)
	target := state.Units["a2-cv-01"]
	torpedo := &AirUnit{Kind: "torpedo_bomber"}
	if got := lightAAValue(state, torpedo, target, false); got != 60 {
		t.Fatalf("鱼雷机与美国航母轻防空修正=%v，期望60", got)
	}
	if got := lightAAValue(state, torpedo, target, true); got != 40 {
		t.Fatalf("混合编队轻防空修正=%v，期望40", got)
	}
	dive := &AirUnit{Kind: "dive_bomber"}
	if got := lightAAValue(state, dive, target, true); got != 30 {
		t.Fatalf("混合俯冲轰炸机轻防空修正=%v，期望30", got)
	}
}

func TestAAWeatherAndVTModifiers(t *testing.T) {
	state := sandboxState(t)
	state.Year = 1943
	state.Map.Cells = append(state.Map.Cells, MapCell{Coord: "0608", Weather: "rain"})
	if got := aaHitChance(state, "0608", true, "A"); got != 4 {
		t.Fatalf("降雨盟军重防空=%v，期望4", got)
	}
	if got := aaHitChance(state, "0608", false, "B"); got != 2 {
		t.Fatalf("降雨轻防空=%v，期望2", got)
	}
	state.Map.Cells[len(state.Map.Cells)-1].Weather = "storm"
	if got := aaHitChance(state, "0608", true, "A"); got != 3 {
		t.Fatalf("风暴盟军重防空=%v，期望3", got)
	}
}

func TestAircraftHitTableProducesAllOutcomes(t *testing.T) {
	for strength := 1; strength <= 4; strength++ {
		expected := map[string]bool{"撤退": false, "损伤": false, "摧毁": false}
		if strength == 4 {
			expected["无效果"] = false
		}
		for seed := uint64(1); seed <= 2000; seed++ {
			aircraft := &AirUnit{ID: "target", Model: "F6F", Strength: strength}
			state := State{Aircraft: map[string]*AirUnit{aircraft.ID: aircraft}, RNGState: seed}
			outcome, _ := applyAircraftHit(&state, aircraft)
			if _, ok := expected[outcome]; ok {
				expected[outcome] = true
			}
		}
		for outcome, found := range expected {
			if !found {
				t.Fatalf("机体强度%d没有覆盖%s结果", strength, outcome)
			}
		}
	}
}

func TestStrikeCAPGroupsUseDistanceAndPrivacy(t *testing.T) {
	state := sandboxState(t)
	state.Units["b-cv-01"].Coord = "0608"
	state.Units["b2-bb-01"].Coord = "0607"
	state.Aircraft["cap-near"] = &AirUnit{ID: "cap-near", CarrierID: "b-cv-01", Faction: "B", Status: "cap", TaskID: "cap-near"}
	state.Aircraft["cap-far"] = &AirUnit{ID: "cap-far", CarrierID: "b2-bb-01", Faction: "B", Status: "cap", TaskID: "cap-far"}
	groups := strikeCAPGroups(state, "B", "0608")
	if len(groups) != 2 || groups[0][0].ID != "cap-near" || groups[1][0].ID != "cap-far" {
		t.Fatalf("CAP增援顺序错误：%+v", groups)
	}
	state.AirCombats = []AirCombatReport{{ID: "visible", AttackerFaction: "A", DefenderFaction: "B"}, {ID: "hidden", AttackerFaction: "B", DefenderFaction: "B"}}
	visible := VisibleAirCombats(state, "A")
	if len(visible) != 1 || visible[0].ID != "visible" {
		t.Fatalf("空战报告隐私过滤错误：%+v", visible)
	}
}
