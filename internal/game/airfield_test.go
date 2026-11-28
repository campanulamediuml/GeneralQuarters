package game

import (
	"testing"
)

// TestAirfieldsDeployWithGroundAircraft 机场在剧本里正确落盘：生命值 = 跑道 × 5，可用跑道由生命值推导。
func TestAirfieldsDeployWithGroundAircraft(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-a-lighthouse"]
	if airfield == nil {
		t.Fatal("沙盒剧本应部署灯塔岛机场")
	}
	if airfield.MaxStrength != airfield.Runways*5 || airfield.Strength != airfield.MaxStrength {
		t.Fatalf("机场生命值应为跑道数×5：%+v", airfield)
	}
	if len(sortedGroundAirIDs(state, airfield.ID)) == 0 {
		t.Fatal("机场应带地面飞机")
	}
}

// TestRunwaysDerivedFromStrength 规则：可用跑道 = min(初始跑道, max(生命值,0)/5 向上取整)。
func TestRunwaysDerivedFromStrength(t *testing.T) {
	cases := []struct{ strength, max, want int }{
		{15, 3, 3}, {14, 3, 3}, {11, 3, 3}, {10, 3, 2}, {6, 3, 2}, {5, 3, 1}, {1, 3, 1}, {0, 3, 0}, {-4, 3, 0},
	}
	for _, c := range cases {
		if got := RunwaysAvailable(c.strength, c.max); got != c.want {
			t.Fatalf("生命值 %d/跑道 %d：可用跑道应为 %d，实际 %d", c.strength, c.max, c.want, got)
		}
	}
}

// 敌方机场也要返回实时整备状态：炮击/轰炸后双方都能从地图算子上看到生命值和跑道变化。
func TestVisibleAirfieldsReportDamageToEnemy(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Strength = 4
	airfield.HeavyAA = 2
	airfield.LightAA = 3
	airfield.Coastal = 6
	for _, contact := range VisibleAirfields(state, "A") {
		if contact.ID != airfield.ID {
			continue
		}
		if contact.Strength != 4 || contact.MaxStrength != 15 {
			t.Fatalf("敌方机场生命值应为 4/15，实际 %d/%d", contact.Strength, contact.MaxStrength)
		}
		if contact.Runways != 1 || contact.MaxRunways != 3 {
			t.Fatalf("4 点生命值应剩 1/3 条跑道，实际 %d/%d", contact.Runways, contact.MaxRunways)
		}
		if contact.HeavyAA != 2 || contact.LightAA != 3 {
			t.Fatalf("敌方机场防空应为 2/3，实际 %v/%v", contact.HeavyAA, contact.LightAA)
		}
		if contact.Coastal != 6 {
			t.Fatalf("敌方机场岸炮应为 6，实际 %d", contact.Coastal)
		}
		if contact.HangarUsed == 0 {
			t.Fatal("敌方机场应返回机库占用数")
		}
		return
	}
	t.Fatal("没有找到 af-b-sentry 的可见机场")
}

// TestAirfieldBusycycle 机场整备三回合：service → deck → ready → 起飞。
func TestAirfieldBusycycle(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.GroundAir["af-b-fighter-1"]
	if aircraft == nil {
		t.Fatal("B 机场应有战斗机")
	}
	order := Order{Type: "LAUNCH_CAP", AircraftID: aircraft.ID}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("机场CAP命令应合法：%v", err)
	}
	ApplyGroundAirOrder(&state, order)
	if aircraft.Status != "service" {
		t.Fatalf("分配任务后应进入整备：%s", aircraft.Status)
	}
	AdvanceAirfields(&state)
	if aircraft.Status != "deck" {
		t.Fatalf("第一回合推进后应滑行到跑道：%s", aircraft.Status)
	}
	// 第二回合：滑行到跑道 → 待命 → 自动起飞（同一次推进里连推两步，
	// 与航母 launchReadyTasks 的行为一致，不需要玩家再下一条命令）
	AdvanceAirfields(&state)
	if aircraft.Status != "cap" || aircraft.Coord != "0207" {
		t.Fatalf("整备完成的机场编队应当自动起飞：status=%s coord=%s", aircraft.Status, aircraft.Coord)
	}
}

// TestAirStrikeTargetWeightsFollowRemainingPoints 空袭落点按"跑道生命值:岸炮:驻机"加权摇一次。
func TestAirStrikeTargetWeightsFollowRemainingPoints(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Strength = 15
	parked := len(sortedGroundAirIDs(state, airfield.ID))
	total := 15 + airfield.Coastal + parked
	var rng uint64 = 20261127
	const draws = 30000
	counts := map[airStrikeTarget]int{}
	for i := 0; i < draws; i++ {
		counts[airStrikeTargetRoll(&rng, state, airfield)]++
	}
	wantRunway := float64(15) / float64(total)
	wantCoastal := float64(airfield.Coastal) / float64(total)
	if share := float64(counts[airStrikeRunway]) / draws; share < wantRunway-0.02 || share > wantRunway+0.02 {
		t.Fatalf("落跑道比例应约 %.3f，实际 %.3f", wantRunway, share)
	}
	if share := float64(counts[airStrikeCoastal]) / draws; share < wantCoastal-0.02 || share > wantCoastal+0.02 {
		t.Fatalf("落岸炮比例应约 %.3f，实际 %.3f", wantCoastal, share)
	}
	// 岸炮清零 → 不再落岸炮；没有驻机 → 不再落驻机。
	airfield.Coastal = 0
	if counts = map[airStrikeTarget]int{}; true {
		for i := 0; i < 2000; i++ {
			counts[airStrikeTargetRoll(&rng, state, airfield)]++
		}
	}
	if counts[airStrikeCoastal] != 0 {
		t.Fatalf("岸炮清零后不该再有命中落在岸炮上：%d", counts[airStrikeCoastal])
	}
}

// TestGroundStrikeDamageAirfield 对地空袭：命中减少机场生命值、摧毁地面飞机、生成对地报告。
func TestGroundStrikeDamageAirfield(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	before := airfield.Strength
	beforeGround := len(sortedGroundAirIDs(state, airfield.ID))
	attackers := map[string]*AirUnit{}
	for index, id := range []string{"atk-1", "atk-2", "atk-3"} {
		attackers[id] = &AirUnit{ID: id, CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack",
			Strength: 2, AirCombat: 1, AntiShip: 4, AntiGround: 4, TaskID: "ground-task", TaskKind: "attack", Loadout: LoadoutBombTank,
			Coord: airfield.Coord, TargetCoord: airfield.Coord, RemainingRange: 8, MaxRange: 12}
		_ = index
	}
	state.Aircraft = attackers
	state.RNGState = 20241116
	events := resolveGroundStrike(&state, "ground-task", airfield.Coord)
	if len(state.AirCombats) != 1 {
		t.Fatalf("应生成对地报告：%v", state.AirCombats)
	}
	report := state.AirCombats[0]
	if report.Target != string(StrikeTargetAir) || report.TargetName != airfield.Name {
		t.Fatalf("报告目标类型错误：%+v", report)
	}
	if report.BombAttempts == 0 {
		t.Fatalf("应记录投弹次数：%+v", report)
	}
	// 命中只落在摇中的那一项上：扣机场生命值的只有"落跑道"的命中。
	if airfield.Strength != before-report.AirfieldRunwayHits {
		t.Fatalf("机场生命值应只被落跑道的命中扣减：before=%d after=%d 落跑道=%d", before, airfield.Strength, report.AirfieldRunwayHits)
	}
	if report.AirfieldRunwayHits+report.AirfieldCoastal+report.GroundLoss > report.StrikeHits {
		t.Fatalf("命中拆分不应超过总命中：落跑道%d 岸炮%d 驻机%d 总命中%d", report.AirfieldRunwayHits, report.AirfieldCoastal, report.GroundLoss, report.StrikeHits)
	}
	if report.StrikeHits > 0 && report.AirfieldRunwayHits == 0 && report.AirfieldCoastal == 0 && report.GroundLoss == 0 {
		t.Fatal("有命中却三项都是 0：落点结算没有生效")
	}
	if report.GroundLoss > beforeGround {
		t.Fatalf("摧毁的地面飞机不能超过原有数量：%d/%d", report.GroundLoss, beforeGround)
	}
	if len(events) == 0 {
		t.Fatal("对地空袭应产生战报")
	}
}

// TestAttackRoutesToAirfield 打击目标 hex 上有敌方机场时，空袭走对地结算而不是对舰。
func TestAttackRoutesToAirfield(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	aircraft := &AirUnit{ID: "route-1", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack",
		Strength: 2, AntiGround: 4, TaskID: "route-task", TaskKind: "attack", Coord: airfield.Coord, TargetCoord: airfield.Coord, RemainingRange: 8, MaxRange: 12}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.RNGState = 7
	if strikeAirfieldAt(state, "A", airfield.Coord) == nil {
		t.Fatal("B 机场应能被 A 的对地目标识别")
	}
	resolveGroundStrike(&state, "route-task", airfield.Coord)
	if len(state.AirCombats) != 1 || state.AirCombats[0].Target != string(StrikeTargetAir) {
		t.Fatalf("存在机场时应走对地空袭：%+v", state.AirCombats)
	}
}
