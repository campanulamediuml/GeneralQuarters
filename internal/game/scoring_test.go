package game

import "testing"

// TestShipVictoryPointsTable 锁死战果分表：航母类按机库容量，其余按舰种折算。
func TestShipVictoryPointsTable(t *testing.T) {
	cases := []struct {
		shipType string
		card     ShipCard
		want     int
	}{
		{"CV", ShipCard{HangarCapacity: 24}, 24},
		{"CVL", ShipCard{HangarCapacity: 9}, 9},
		{"CVE", ShipCard{HangarCapacity: 12}, 12},
		{"cv", ShipCard{HangarCapacity: 18}, 18},
		{"CV", ShipCard{}, 1},
		{"BB", ShipCard{}, 6},
		{"BC", ShipCard{}, 6},
		{"CA", ShipCard{}, 3},
		{"CL", ShipCard{}, 2},
		{"DD", ShipCard{}, 1},
		{"TRS", ShipCard{}, 1},
		{"SS", ShipCard{}, 0},
	}
	for _, testCase := range cases {
		if got := ShipVictoryPoints(testCase.shipType, testCase.card); got != testCase.want {
			t.Errorf("%s（机库%d）=%d，期望%d", testCase.shipType, testCase.card.HangarCapacity, got, testCase.want)
		}
	}
}

// TestVictoryPointsCountsEnemyLosses 校验记分方向：B 沉了算 A 的分，反之亦然。
func TestVictoryPointsCountsEnemyLosses(t *testing.T) {
	state := State{Losses: map[string]*LossLedger{
		"B": {
			Ships: []ShipLoss{
				{Name: "翔鹤", Type: "CV", Card: ShipCard{HangarCapacity: 18}},
				{Name: "有明", Type: "DD"},
				{Name: "青叶", Type: "CA"},
			},
			AircraftTotal: 5,
		},
		"A": {Ships: []ShipLoss{{Name: "法拉格特", Type: "BB"}}, AircraftTotal: 2},
	}}
	scores := VictoryPoints(state)
	// A 击沉翔鹤18 + 有明1 + 青叶3，外加击落 B 的 5 架 ×0.25 = 1.25
	if scores["A"] != 18+1+3+1.25 {
		t.Fatalf("A 的战果分=%v，期望 23.25（击沉 22 + 5 架×0.25）", scores["A"])
	}
	// B 击沉一艘战列舰 6 分 + 击落 A 的 2 架 ×0.25 = 0.5
	if scores["B"] != 6.5 {
		t.Fatalf("B 的战果分=%v，期望 6.5", scores["B"])
	}
}

// TestOutcomeAtTurnCapUsesVictoryPoints 校验声明 score 目标的剧本在回合上限按战果分判胜负。
func TestOutcomeAtTurnCapUsesVictoryPoints(t *testing.T) {
	state := State{
		Turn:       5,
		MaxTurns:   5,
		Objectives: []Objective{{ID: "vp", Kind: ObjectiveScore}},
		Units: map[string]*Unit{
			"a-dd": {ID: "a-dd", Faction: "A", Type: "DD"},
			"b-dd": {ID: "b-dd", Faction: "B", Type: "DD"},
		},
		Losses: map[string]*LossLedger{
			"B": {Ships: []ShipLoss{{Name: "翔鹤", Type: "CV", Card: ShipCard{HangarCapacity: 24}}}},
		},
	}
	outcome := EvaluateOutcome(state)
	if outcome == nil {
		t.Fatal("回合上限已到，应该有结果")
	}
	if !outcome.TurnCap {
		t.Fatal("应该按回合上限判定")
	}
	if outcome.Winner != "A" || outcome.Level != VictoryLevelStrategic {
		t.Fatalf("结果=%+v，期望 A 战略胜利（24:0）", outcome)
	}
	if outcome.Scores["A"] != 24 || outcome.Scores["B"] != 0 {
		t.Fatalf("比分=%v，期望 A24/B0", outcome.Scores)
	}
	if outcome.Reason != "回合上限 5 到达；VP 24:0（∞:1），A方战略胜利" {
		t.Fatalf("结束原因=%q，应该说明 VP 与胜利等级", outcome.Reason)
	}
}

// TestVictoryPointRatioLevels 锁死 1.25 / 2 两个分界：
// 小于 1.25 平分秋色，1.25 到 2 战术胜利，大于 2 战略胜利。
func TestVictoryPointRatioLevels(t *testing.T) {
	cases := []struct {
		name       string
		a, b       float64
		wantWinner string
		wantLevel  VictoryLevel
	}{
		{"双方为零", 0, 0, "", VictoryLevelDraw},
		{"差距不到1.25", 12, 10, "", VictoryLevelDraw},
		{"刚好1.25", 12.5, 10, "A", VictoryLevelTactical},
		{"1.5倍", 15, 10, "A", VictoryLevelTactical},
		{"刚好2倍", 20, 10, "A", VictoryLevelTactical},
		{"超过2倍", 21, 10, "A", VictoryLevelStrategic},
		{"B超过2倍", 10, 21, "B", VictoryLevelStrategic},
	}
	state := objectiveState()
	state.Objectives = []Objective{{ID: "vp", Kind: ObjectiveScore}}
	for _, testCase := range cases {
		outcome := victoryPointOutcome(state, map[string]float64{"A": testCase.a, "B": testCase.b}, "测试", true)
		if outcome.Winner != testCase.wantWinner || outcome.Level != testCase.wantLevel {
			t.Errorf("%s：结果=%+v，期望胜者%q等级%q", testCase.name, outcome, testCase.wantWinner, testCase.wantLevel)
		}
	}
}

// TestJapanCannotGetStrategicWithoutSupply 锁死"日军未投送满20单位不能战略胜利"。
func TestJapanCannotGetStrategicWithoutSupply(t *testing.T) {
	state := objectiveState()
	state.Objectives = []Objective{
		{ID: "moresby", Kind: ObjectiveSupplies, Coord: "1611", Faction: "B", Value: 20},
		{ID: "vp", Kind: ObjectiveScore},
	}
	outcome := victoryPointOutcome(state, map[string]float64{"A": 10, "B": 30}, "测试", true)
	if outcome.Winner != "B" || outcome.Level != VictoryLevelTactical {
		t.Fatalf("日军未投送满20单位时应为战术胜利：%+v", outcome)
	}
	if outcome.Reason == "" {
		t.Fatal("应说明战略胜利被降级")
	}

	state.ShoreSupplies = map[string]int{"B|1611": 20}
	outcome = victoryPointOutcome(state, map[string]float64{"A": 10, "B": 30}, "测试", true)
	if outcome.Winner != "B" || outcome.Level != VictoryLevelStrategic {
		t.Fatalf("投送满20单位后应允许战略胜利：%+v", outcome)
	}
}

// TestFinalVictoryPointsAwardsDeniedSupply 校验没送到的货物折算成阻止方 VP。
func TestFinalVictoryPointsAwardsDeniedSupply(t *testing.T) {
	state := objectiveState()
	state.Objectives = []Objective{{ID: "moresby", Kind: ObjectiveSupplies, Coord: "1611", Faction: "B", Value: 20}}
	state.ShoreSupplies = map[string]int{"B|1611": 7}
	scores := FinalVictoryPoints(state)
	if scores["B"] != 0 || scores["A"] != 20 {
		t.Fatalf("未完成投送时 A 应拿 20 点阻止分，实际 %v", scores)
	}
	state.ShoreSupplies["B|1611"] = 20
	scores = FinalVictoryPoints(state)
	if scores["B"] != 20 || scores["A"] != 0 {
		t.Fatalf("完成投送时 B 应拿 20 点投送分，实际 %v", scores)
	}
}

// TestOutcomeKeepsObjectivePointsWithoutScoreObjective 校验没有 score 目标的老剧本不受影响。
func TestOutcomeKeepsObjectivePointsWithoutScoreObjective(t *testing.T) {
	state := State{
		Turn:     3,
		MaxTurns: 3,
		Objectives: []Objective{
			{ID: "control", Kind: ObjectiveControl, Coord: "0202", Faction: "A", Points: 4},
		},
		Units: map[string]*Unit{
			"a-dd": {ID: "a-dd", Faction: "A", Type: "DD", Coord: "0202"},
			"b-dd": {ID: "b-dd", Faction: "B", Type: "DD", Coord: "0909"},
		},
		Losses: map[string]*LossLedger{
			"B": {Ships: []ShipLoss{{Name: "翔鹤", Type: "CV", Card: ShipCard{HangarCapacity: 24}}}},
		},
	}
	outcome := EvaluateOutcome(state)
	if outcome == nil || outcome.Winner != "A" {
		t.Fatalf("应该按控制点判定 A 胜：%+v", outcome)
	}
	if outcome.Scores["A"] != 4 {
		t.Fatalf("老剧本仍按目标点计分，A=%v，期望 4", outcome.Scores["A"])
	}
}

// TestScoreBreakdownMatchesVictoryPoints 锁死"明细之和 = 总分"：界面上的得分明细
// 和侧栏 VP 是同一笔账，任何一边单算都可能悄悄错位。
func TestScoreBreakdownMatchesVictoryPoints(t *testing.T) {
	state := State{
		Losses: map[string]*LossLedger{
			"B": {
				Ships: []ShipLoss{
					{Turn: 4, ID: "b-shokaku", Name: "翔鹤", Type: "CV", Card: ShipCard{HangarCapacity: 18}},
					{Turn: 6, ID: "b-ushio", Name: "潮", Type: "DD"},
				},
				// 先瘫后沉的航母只按整艘算，不能领两次。
				Crippled:      []CrippledCarrier{{Turn: 3, ID: "b-shokaku", Name: "翔鹤", Reason: "飞行甲板被摧毁", Card: ShipCard{HangarCapacity: 18}}},
				AircraftTotal: 4,
			},
			"A": {
				Ships: []ShipLoss{{Turn: 7, ID: "a-lexington", Name: "列克星敦", Type: "CV", Card: ShipCard{HangarCapacity: 24}}},
			},
		},
	}
	lines := ScoreBreakdown(state)
	if got := linePoints(lines["A"]); got != 18+1+4*0.25 {
		t.Fatalf("A 的明细合计=%v，期望 20（翔鹤18 + 潮1 + 4架×0.25）", got)
	}
	if got := linePoints(lines["B"]); got != 24 {
		t.Fatalf("B 的明细合计=%v，期望 24（列克星敦按机库24计）", got)
	}
	scores := VictoryPoints(state)
	for _, faction := range []string{"A", "B"} {
		if scores[faction] != linePoints(lines[faction]) {
			t.Fatalf("%s 方总分 %v 与明细合计 %v 不一致", faction, scores[faction], linePoints(lines[faction]))
		}
	}
	// 条目要能告诉玩家"亏的是谁"：己方的损失记在对方账上。
	for _, line := range lines["A"] {
		if line.Faction != "B" {
			t.Fatalf("A 的得分条目应来自 B 的损失：%+v", line)
		}
		if line.Text == "" || line.Points <= 0 {
			t.Fatalf("条目缺少文案或分数：%+v", line)
		}
	}
	// 瘫痪那条已经被沉没覆盖，不能出现在明细里。
	for _, line := range lines["A"] {
		if line.Kind == "crippled" {
			t.Fatalf("先瘫后沉的航母不该在明细里再领一次：%+v", line)
		}
	}
}

func linePoints(lines []ScoreLine) float64 {
	total := 0.0
	for _, line := range lines {
		total += line.Points
	}
	return total
}
