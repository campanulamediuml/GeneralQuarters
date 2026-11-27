package game

import "testing"

// submarineFixture 造一个最小状态：10x10 海图、双方各一支舰队。
func submarineFixture() State {
	return State{
		Map:       MapDefinition{Rows: 10, Cols: 10},
		Doctrines: map[string]string{"A": DoctrineUS, "B": DoctrineJapan},
		Year:      1944,
		RNGState:  12345,
		Units: map[string]*Unit{
			"a-ca": {ID: "a-ca", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "旧金山", Type: "CA", Strength: 4, MaxStrength: 4, Card: ShipCard{DepthCharges: 0}},
			"a-dd": {ID: "a-dd", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "弗莱彻", Type: "DD", Strength: 2, MaxStrength: 2, Card: ShipCard{DepthCharges: 2}},
			"b-cv": {ID: "b-cv", FleetID: "TF-B", FleetName: "TF-B", Faction: "B", Coord: "0808", Name: "瑞鹤", Type: "CV", Strength: 4, MaxStrength: 4},
		},
	}
}

// TestSubmarineDeployKeepsReservesHidden 锁定"可部署 N 艘 + 每方 3 艘随机预备艇"：
// 部署阶段只看得到本方可部署的潜艇，预备艇连本方都看不到。
func TestSubmarineDeployKeepsReservesHidden(t *testing.T) {
	state := submarineFixture()
	state.Phase = PhaseDeploy
	scenario := ScenarioDefinition{Submarines: []SubmarineDeployment{
		{Faction: "A", Count: 2, Name: "S级", Picture: "a.png"},
		{Faction: "B", Count: 1, Name: "伊号", Picture: "b.png"},
	}}
	if err := deploySubmarines(&state, scenario); err != nil {
		t.Fatalf("展开潜艇失败：%v", err)
	}
	if len(state.Submarines) != 2+1+submarineReserveCount*2 {
		t.Fatalf("潜艇总数应为 3 艘可部署 + 6 艘预备艇，实际 %d", len(state.Submarines))
	}
	if got := len(VisibleSubmarines(state, "A")); got != 2 {
		t.Fatalf("部署阶段 A 应看到自己 2 艘待部署潜艇，实际 %d", got)
	}
	if got := len(VisibleSubmarines(state, "B")); got != 1 {
		t.Fatalf("部署阶段 B 应看到自己 1 艘待部署潜艇，实际 %d", got)
	}
	if got := UndeployedSubmarines(state, "A"); got != 2 {
		t.Fatalf("A 应有 2 艘未部署，实际 %d", got)
	}
	// 摆完一艘：未部署数减一，坐标落在可潜海格里。
	sub := VisibleSubmarines(state, "A")[0]
	order := Order{Type: "DEPLOY_SUB", UnitID: sub.ID, Coord: "0505"}
	if err := validateSubDeploy(state, "A", order); err != nil {
		t.Fatalf("海格部署应通过：%v", err)
	}
	ApplySubDeploy(&state, order)
	if got := UndeployedSubmarines(state, "A"); got != 1 {
		t.Fatalf("摆完一艘后应剩 1 艘未部署，实际 %d", got)
	}
	// 预备艇不能被玩家摆位。
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		if state.Submarines[id].Reserve {
			if err := validateSubDeploy(state, "A", Order{Type: "DEPLOY_SUB", UnitID: id, Coord: "0504"}); err == nil {
				t.Fatalf("预备艇 %s 不应该能被摆位", id)
			}
			break
		}
	}
}

// TestStartSubmarinesHidesAndRollsPhase 开战之后：每艘潜艇掷 1d6 定第一次巡逻回合，
// 并且从双方视图里消失。
func TestStartSubmarinesHidesAndRollsPhase(t *testing.T) {
	state := submarineFixture()
	state.Phase = PhaseDeploy
	scenario := ScenarioDefinition{Submarines: []SubmarineDeployment{{Faction: "A", Count: 1, Name: "S级"}}}
	if err := deploySubmarines(&state, scenario); err != nil {
		t.Fatalf("展开潜艇失败：%v", err)
	}
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		if state.Submarines[id].Reserve {
			continue
		}
		ApplySubDeploy(&state, Order{Type: "DEPLOY_SUB", UnitID: id, Coord: "0202"})
	}
	state.Phase = PhaseBattle
	StartSubmarines(&state)
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if !sub.Deployed || sub.Coord == "" {
			t.Fatalf("开战后潜艇 %s 应已落位：%+v", id, sub)
		}
		if sub.FirstMoveTurn < 1 || sub.FirstMoveTurn > 6 {
			t.Fatalf("第一次巡逻回合应是 1d6，实际 %d", sub.FirstMoveTurn)
		}
	}
	if got := VisibleSubmarines(state, "A"); len(got) != 0 {
		t.Fatalf("开战后本方也看不到潜艇，实际 %d 艘", len(got))
	}
}

// TestSubmarinePatrolPhaseAndPinning 巡逻只在 1d6 相位上每 6 回合一次；被发现后钉在原地。
func TestSubmarinePatrolPhaseAndPinning(t *testing.T) {
	state := submarineFixture()
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0504", Deployed: true, FirstMoveTurn: 3, MoveInterval: 6},
	}
	state.Turn = 2
	AdvanceSubmarines(&state)
	if state.Submarines["sub-b-1"].Coord != "0504" {
		t.Fatalf("第 2 回合不该巡逻：%s", state.Submarines["sub-b-1"].Coord)
	}
	state.Turn = 3
	AdvanceSubmarines(&state)
	sub := state.Submarines["sub-b-1"]
	if Distance(sub.Coord, "0504") > 1 {
		t.Fatalf("巡逻只走 1 hex，实际到了 %s", sub.Coord)
	}
	// 下一次巡逻在第 9 回合；第 4 回合不该动。
	after := sub.Coord
	state.Turn = 4
	AdvanceSubmarines(&state)
	if state.Submarines["sub-b-1"].Coord != after {
		t.Fatalf("巡逻间隔应是 6 回合，第 4 回合不该动")
	}
	// 被发现之后钉死：即使到了巡逻回合也不动。
	state.Submarines["sub-b-1"].Discovered = true
	state.Submarines["sub-b-1"].DiscoveredBy = "A"
	state.Submarines["sub-b-1"].TrackedFleetID = "TF-A"
	state.Turn = 9
	AdvanceSubmarines(&state)
	if state.Submarines["sub-b-1"].Coord != after {
		t.Fatalf("被发现的潜艇应被钉在原地")
	}
}

// TestSubmarineBattleReportsPointsAndEffects 潜艇战：差额为战斗点数，点数再兑换成效果。
func TestSubmarineBattleReportsPointsAndEffects(t *testing.T) {
	for seed := uint64(1); seed < 60; seed++ {
		state := submarineFixture()
		state.RNGState = seed
		state.Submarines = map[string]*Submarine{
			"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true, MoveInterval: 6},
		}
		events := resolveSubmarineBattle(&state, state.Submarines["sub-b-1"], "TF-A")
		if len(state.SubBattles) != 1 {
			t.Fatalf("种子 %d：应记一条潜艇战报", seed)
		}
		report := state.SubBattles[0]
		diff := report.SubRoll - report.FleetRoll
		if diff < 0 {
			diff = -diff
		}
		if report.SubPoints+report.FleetPoints != diff {
			t.Fatalf("种子 %d：战斗点数应等于骰差 %d，实际 潜艇%d 舰队%d", seed, diff, report.SubPoints, report.FleetPoints)
		}
		if diff > 0 && len(report.Effects) == 0 && report.PointsLeft > 0 {
			t.Fatalf("种子 %d：有 %d 点战斗点数和效果都没兑换", seed, diff)
		}
		if report.Summary == "" || len(events) == 0 {
			t.Fatalf("种子 %d：战报和事件都不能为空", seed)
		}
	}
}

// TestSubmarineTorpedoChanceTable 锁定鱼雷命中率：基础 25，近距离 +20，日军 +10，美军 1944 前 -20，
// 夜间 +10，小型舰艇 -10，最低 5；-4 的远距离偷袭白天固定 5、夜间 25。
func TestSubmarineTorpedoChanceTable(t *testing.T) {
	state := submarineFixture()
	jp := &Submarine{Faction: "B"}
	us := &Submarine{Faction: "A"}
	dd := &Unit{Type: "DD", Card: ShipCard{SpeedClass: "fast"}}
	cv := &Unit{Type: "CV"}
	if got := submarineTorpedoChance(state, jp, cv, false); got != 55 {
		t.Fatalf("日军近距离白天应为 25+20+10=55，实际 %.0f", got)
	}
	state.Night = true
	if got := submarineTorpedoChance(state, jp, cv, false); got != 65 {
		t.Fatalf("日军夜间近距离应为 25+20+10+10=65，实际 %.0f", got)
	}
	state.Night = false
	// 1944 年起取消美军 -20：夜间近距离 25+20=45。
	if got := submarineTorpedoChance(state, us, cv, false); got != 45 {
		t.Fatalf("美军 1944 近距离应为 25+20=45，实际 %.0f", got)
	}
	state.Year = 1943
	if got := submarineTorpedoChance(state, us, cv, false); got != 25 {
		t.Fatalf("美军 1943 近距离应为 25+20-20=25，实际 %.0f", got)
	}
	state.Year = 1944
	if got := submarineTorpedoChance(state, jp, dd, false); got != 45 {
		t.Fatalf("日军近距离打驱逐舰应为 55-10=45，实际 %.0f", got)
	}
	// -4 的白天是远距离偷袭：基础 25%，不吃近距离 +20。
	if got := submarineTorpedoChance(state, us, cv, true); got != 25 {
		t.Fatalf("白天远距离偷袭应为 25%%，实际 %.0f", got)
	}
	// 夜间 -4 贴近到近距离：25+20+10=55（美军 1944 不再吃 -20）。
	state.Night = true
	if got := submarineTorpedoChance(state, us, cv, !state.Night); got != 55 {
		t.Fatalf("夜间近距离偷袭应为 55%%，实际 %.0f", got)
	}
}

// TestDepthChargeConsumesChargesAndEndsTracking 反潜作业：每回合消耗 1 组深弹，深弹耗尽后追踪失败。
func TestDepthChargeConsumesChargesAndEndsTracking(t *testing.T) {
	state := submarineFixture()
	state.Phase = PhaseBattle
	sub := &Submarine{
		ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true,
		Discovered: true, DiscoveredBy: "A", TrackedFleetID: "TF-A",
	}
	state.Submarines = map[string]*Submarine{"sub-b-1": sub}
	before := state.Units["a-dd"].Card.DepthCharges
	for turn := 0; turn < 20; turn++ {
		sub := state.Submarines["sub-b-1"]
		if sub == nil || !sub.Discovered {
			break
		}
		resolveDepthChargeTracking(&state, sub)
	}
	charges := state.Units["a-dd"].Card.DepthCharges
	if charges >= before {
		t.Fatalf("投放深弹应消耗弹药：投放前 %d，现在 %d", before, charges)
	}
	tracked := state.Submarines["sub-b-1"]
	if tracked != nil && tracked.Discovered {
		t.Fatalf("每回合都该有结果：深弹耗尽后追踪应失败，实际仍在追踪（剩余深弹 %d）", charges)
	}
}

// TestDiscoveredSubmarineVisibleToFleetSideOnly 被发现的潜艇只给舰队方看，潜艇方看不到。
func TestDiscoveredSubmarineVisibleToFleetSideOnly(t *testing.T) {
	state := submarineFixture()
	state.Phase = PhaseBattle
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true,
			Discovered: true, DiscoveredBy: "A", TrackedFleetID: "TF-A"},
	}
	seen := VisibleSubmarines(state, "A")
	if len(seen) != 1 || seen[0].ID != "sub-b-1" || seen[0].Coord != "0505" {
		t.Fatalf("A 应该看到被发现潜艇的位置：%+v", seen)
	}
	if got := VisibleSubmarines(state, "B"); len(got) != 0 {
		t.Fatalf("潜艇方看不到自己的潜艇，实际 %d 艘", len(got))
	}
}
