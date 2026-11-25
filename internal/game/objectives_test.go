package game

import "testing"

func objectiveState() State {
	return State{
		Turn:      1,
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units: map[string]*Unit{
			"a1": {ID: "a1", Faction: "A", Type: "BB", Coord: "0202", Card: ShipCard{SpeedClass: "normal"}},
			"b1": {ID: "b1", Faction: "B", Type: "BB", Coord: "0808", Card: ShipCard{SpeedClass: "normal"}},
		},
		Aircraft:          map[string]*AirUnit{},
		GroundAir:         map[string]*AirfieldAirUnit{},
		ShoreSupplies:     map[string]int{},
		InitialShipCounts: map[string]int{"A": 5, "B": 5},
	}
}

// 一方全灭时对局立即结束。
func TestOutcomeWhenFactionEliminated(t *testing.T) {
	state := objectiveState()
	delete(state.Units, "b1")
	outcome := EvaluateOutcome(state)
	if outcome == nil || outcome.Winner != "A" {
		t.Fatalf("B 全灭时 A 应获胜：%+v", outcome)
	}
}

// 没有目标也没有全灭时继续打。
func TestOutcomeKeepsGoingWithoutObjectives(t *testing.T) {
	if outcome := EvaluateOutcome(objectiveState()); outcome != nil {
		t.Fatalf("对局不应结束：%+v", outcome)
	}
}

// 到达回合上限时按目标点判定。
func TestOutcomeByObjectivesAtTurnCap(t *testing.T) {
	state := objectiveState()
	state.MaxTurns = 3
	state.Turn = 3
	state.Objectives = []Objective{
		{ID: "supply", Kind: ObjectiveSupplies, Coord: "0202", Faction: "A", Value: 2, Points: 3},
		{ID: "control", Kind: ObjectiveControl, Coord: "0808", Faction: "B", Points: 1},
	}
	state.ShoreSupplies["A|0202"] = 2
	outcome := EvaluateOutcome(state)
	if outcome == nil || !outcome.TurnCap || outcome.Winner != "A" || outcome.Scores["A"] != 3 || outcome.Scores["B"] != 1 {
		t.Fatalf("目标点判定错误：%+v", outcome)
	}
}

// 没到回合上限不按目标点提前结束。
func TestOutcomeNotAtTurnCapYet(t *testing.T) {
	state := objectiveState()
	state.MaxTurns = 10
	state.Turn = 3
	state.Objectives = []Objective{{ID: "control", Kind: ObjectiveControl, Coord: "0202", Faction: "A"}}
	if outcome := EvaluateOutcome(state); outcome != nil {
		t.Fatalf("未到回合上限不应结束：%+v", outcome)
	}
}

// 击沉数目标：初始 5 艘只剩 3 艘即达成。
func TestSinkObjective(t *testing.T) {
	state := objectiveState()
	// A 的目标是击沉对方（B）的舰船：统计的是 B 的损失，不是 A 的。
	objective := Objective{ID: "sink", Kind: ObjectiveSink, Faction: "A", Value: 2}
	if !objectiveCompleted(state, "A", objective) {
		t.Fatal("击沉对方舰船达到目标时应达成")
	}
	// 把 B 的初始数改成 1（现存 1 艘、沉 0 艘）：不达成。
	state.InitialShipCounts["B"] = 1
	if objectiveCompleted(state, "A", objective) {
		t.Fatal("击沉数不足时不应达成")
	}
	// 己方被打沉再多也不算完成击沉目标。
	state.InitialShipCounts["A"] = 50
	if objectiveCompleted(state, "A", objective) {
		t.Fatal("自己损失惨重不该算完成击沉目标")
	}
}

// 投送完成只加 20 VP，不提前结束；哪怕日军已经卸完，也要打到 360 回合。
func TestSupplyDeliveryScoresButWaitsForTurnCap(t *testing.T) {
	state := objectiveState()
	state.MaxTurns = 288
	state.Turn = 5
	state.Objectives = []Objective{
		{ID: "moresby", Text: "向莫尔兹比港投送 20 点货物", Kind: ObjectiveSupplies, Coord: "1611", Faction: "B", Value: 20},
		{ID: "vp", Kind: ObjectiveScore},
	}
	state.ShoreSupplies["B|1611"] = 20
	if outcome := EvaluateOutcome(state); outcome != nil {
		t.Fatalf("投送完成不应提前结束：%+v", outcome)
	}
	if scores := VictoryPoints(state); scores["B"] != 20 {
		t.Fatalf("投送完成应给 B 20 VP，实际 %v", scores)
	}
}

// 到回合上限仍未完成投送：目标分值判给阻止方，再按 VP 比值结算。
func TestSupplyObjectiveFailsAtTurnCap(t *testing.T) {
	state := objectiveState()
	state.MaxTurns = 3
	state.Turn = 3
	state.Objectives = []Objective{
		{ID: "moresby", Text: "向莫尔兹比港投送 20 点货物", Kind: ObjectiveSupplies, Coord: "1611", Faction: "B", Value: 20},
		{ID: "vp", Kind: ObjectiveScore},
	}
	state.ShoreSupplies["B|1611"] = 19
	state.Units["trs"] = &Unit{ID: "trs", Faction: "B", Type: "TRS", Coord: "0808", Card: ShipCard{Cargo: 1}}
	outcome := EvaluateOutcome(state)
	if outcome == nil || outcome.Winner != "A" || !outcome.TurnCap {
		t.Fatalf("未完成投送应判 A 胜：%+v", outcome)
	}
	if outcome.Scores["A"] != 20 || outcome.Scores["B"] != 0 {
		t.Fatalf("未完成的 20 分应判给 A：%+v", outcome.Scores)
	}
}

// 运输船全沉或货已卸空也不提前结束；拖到回合上限再判 20 分给阻止方。
func TestSupplyNoCargoWaitsForTurnCap(t *testing.T) {
	state := objectiveState()
	state.MaxTurns = 288
	state.Turn = 10
	state.Objectives = []Objective{
		{ID: "moresby", Kind: ObjectiveSupplies, Coord: "1611", Faction: "B", Value: 20},
		{ID: "vp", Kind: ObjectiveScore},
	}
	state.Units["trs"] = &Unit{ID: "trs", Faction: "B", Type: "TRS", Coord: "1611", Card: ShipCard{Cargo: 0}}
	if outcome := EvaluateOutcome(state); outcome != nil {
		t.Fatalf("无货可卸也不应提前结束：%+v", outcome)
	}
	state.Turn = 288
	outcome := EvaluateOutcome(state)
	if outcome == nil || outcome.Winner != "A" || !outcome.TurnCap {
		t.Fatalf("到回合上限应判 A 胜：%+v", outcome)
	}
}

// settleOutcome 只发布一次结果事件，避免每回合刷屏。
func TestSettleOutcomeOnlyOnce(t *testing.T) {
	state := objectiveState()
	delete(state.Units, "b1")
	events := settleOutcome(&state)
	if len(events) != 1 || events[0].Type != "game_over" {
		t.Fatalf("应发布一次 game_over 事件：%+v", events)
	}
	if again := settleOutcome(&state); len(again) != 0 {
		t.Fatalf("结果只应记录一次：%+v", again)
	}
}

// 母舰全沉后，机库里剩下的飞机不应让该阵营继续"存活"。
func TestFactionWithoutShipsIsEliminatedEvenWithHangarAircraft(t *testing.T) {
	state := objectiveState()
	delete(state.Units, "b1")
	state.Aircraft["orphan"] = &AirUnit{ID: "orphan", Faction: "B", Status: "hangar", Kind: "fighter", Model: "A6M"}
	if factionHasUnits(state, "B") {
		t.Fatal("只剩机库飞机不应算存活")
	}
	outcome := EvaluateOutcome(state)
	if outcome == nil || outcome.Winner != "A" {
		t.Fatalf("B 方舰船全灭应判 A 方获胜：%+v", outcome)
	}
	// 机场上的地面飞机仍算战力。
	state.GroundAir["g"] = &AirfieldAirUnit{ID: "g", Faction: "B", Status: "hangar"}
	if !factionHasUnits(state, "B") {
		t.Fatal("机场地面飞机应算战力")
	}
}
