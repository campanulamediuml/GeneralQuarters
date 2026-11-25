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
	if outcome.Winner != "A" {
		t.Fatalf("胜者=%q，期望 A（24:0）", outcome.Winner)
	}
	if outcome.Scores["A"] != 24 || outcome.Scores["B"] != 0 {
		t.Fatalf("比分=%v，期望 A24/B0", outcome.Scores)
	}
	if outcome.Reason != "回合上限 5 到达，按击沉战果分判定" {
		t.Fatalf("结束原因=%q，应该说明按战果分判定", outcome.Reason)
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
