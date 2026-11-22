package game

import "testing"

// 规则书 597–602 行：基础 100%，每 1hex 美军 -10%、日军 -20%，无轰炸机的编队距离惩罚翻倍。
func TestRadioSuccessChanceByDistance(t *testing.T) {
	state := sandboxState(t)
	bombers := []*AirUnit{{ID: "x", Kind: "dive_bomber"}}
	fighters := []*AirUnit{{ID: "y", Kind: "fighter"}}
	cases := []struct {
		name     string
		faction  string
		coord    string
		aircraft []*AirUnit
		want     float64
		distance int
	}{
		{"美军同格", "A", "0202", bombers, 100, 0},
		{"美军1hex", "A", "0205", bombers, 90, 1},
		{"美军1hex战斗机翻倍", "A", "0205", fighters, 80, 1},
		// 日军舰队本身就在 0208，同格没有距离惩罚。
		{"日军同格", "B", "0208", bombers, 100, 0},
	}
	for _, item := range cases {
		chance, distance := radioSuccessChance(state, item.faction, item.coord, item.aircraft)
		if chance != item.want || distance != item.distance {
			t.Fatalf("%s 成功率错误：%.0f%%（期望%.0f%%）距离%d（期望%d）", item.name, chance, item.want, distance, item.distance)
		}
	}
}

// 日军距离惩罚翻倍（-20%/hex）。
func TestJapanRadioPenaltyIsDouble(t *testing.T) {
	state := sandboxState(t)
	bombers := []*AirUnit{{ID: "b", Kind: "torpedo_bomber"}}
	chance, distance := radioSuccessChance(state, "B", "0205", bombers)
	if distance != 2 || chance != 60 {
		t.Fatalf("日军无线电衰减错误：%.0f%% 距离%d", chance, distance)
	}
	// 没有轰炸机时距离惩罚翻倍（-40%/hex），2hex 只剩 20%。
	chance, _ = radioSuccessChance(state, "B", "0205", []*AirUnit{{ID: "f", Kind: "fighter"}})
	if chance != 20 {
		t.Fatalf("日军战斗机编队通信成功率错误：%.0f%%", chance)
	}
}

// 每个空中编队每回合各有一次机会：三个打击编队可以各发一次。
func TestEachFormationGetsOneRadioPerTurn(t *testing.T) {
	state := sandboxState(t)
	tasks := []string{"strike-1", "strike-2", "strike-3"}
	carrierCoord := state.Units["a2-cv-01"].Coord
	base := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	base.Status = "attack"
	base.TaskKind = "attack"
	base.Coord = carrierCoord
	base.TaskID = tasks[0]
	base.Priority = 3
	for _, taskID := range tasks[1:] {
		copyUnit := *base
		copyUnit.ID = base.ID + "-" + taskID
		copyUnit.TaskID = taskID
		state.Aircraft[copyUnit.ID] = &copyUnit
	}
	orders := []Order{}
	for _, taskID := range tasks {
		orders = append(orders, Order{Type: "RECALL_AIR", TaskID: taskID})
	}
	events := ResolveOrders(&state, orders)
	if len(state.RadioUsed) != 3 {
		t.Fatalf("三个编队应该各自用掉一次无线电机会：%+v", state.RadioUsed)
	}
	for _, taskID := range tasks {
		if owner, ok := state.RadioUsed[taskID]; !ok || owner != "A" {
			t.Fatalf("编队%s的无线电记录缺失：%+v", taskID, state.RadioUsed)
		}
	}
	for _, event := range events {
		if event.Type == "radio_exhausted" {
			t.Fatalf("三个不同编队不应互相挤占机会：%s", event.Text)
		}
	}
}

// 发射失败也算用掉这次机会，之后这个编队不能再发；换一个编队仍然可以。
func TestRadioFailureConsumesFormationSlot(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["a2-cv-01-fighter-x1"]
	aircraft.Status = "cap"
	aircraft.TaskID = "task-cap-1"
	aircraft.Coord = state.Units["a2-cv-01"].Coord
	aircraft.PatrolCoord = aircraft.Coord
	events := ApplyRadioOrder(&state, "A", Order{Type: "RECALL_AIR", TaskID: aircraft.TaskID})
	if len(state.RadioUsed) != 1 {
		t.Fatalf("发射后必须记一次机会（无论成败）：%+v %+v", state.RadioUsed, events)
	}
	if err := ValidateOrder(state, "A", Order{Type: "RECALL_AIR", TaskID: aircraft.TaskID}); err == nil {
		t.Fatal("同一编队本回合不应能发第二次无线电")
	}
	other := state.Aircraft["a2-cv-02-fighter-x1"]
	other.Status = "cap"
	other.TaskID = "task-cap-2"
	other.Coord = state.Units["a2-cv-02"].Coord
	other.PatrolCoord = other.Coord
	if err := ValidateOrder(state, "A", Order{Type: "RECALL_AIR", TaskID: other.TaskID}); err != nil {
		t.Fatalf("不同编队应当有自己的无线电机会：%v", err)
	}
}

// 召回成功后编队返航，并取消 CAP 自动循环。
func TestRadioRecallTurnsAircraftBack(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["a2-cv-01-fighter-x1"]
	aircraft.Status = "cap"
	aircraft.TaskID = "task-cap-1"
	aircraft.OnCycle = true
	aircraft.Coord = state.Units["a2-cv-01"].Coord
	aircraft.PatrolCoord = aircraft.Coord
	if err := ValidateOrder(state, "A", Order{Type: "RECALL_AIR", TaskID: aircraft.TaskID}); err != nil {
		t.Fatalf("召回命令被拒绝：%v", err)
	}
	if err := ValidateOrder(state, "B", Order{Type: "RECALL_AIR", TaskID: aircraft.TaskID}); err == nil {
		t.Fatal("敌方不应能召回我的编队")
	}
	ApplyRadioOrder(&state, "A", Order{Type: "RECALL_AIR", TaskID: aircraft.TaskID})
	if state.RadioUsed[aircraft.TaskID] != "A" {
		t.Fatal("召回后应记录该编队已用掉无线电机会")
	}
	if !aircraft.Returning {
		t.Fatal("召回后编队应该返航")
	}
	if aircraft.OnCycle {
		t.Fatal("主动召回应该取消 CAP 自动循环")
	}
}

// 回合开始时清空各编队的无线电记录。
func TestRadioSlotsResetEachTurn(t *testing.T) {
	state := sandboxState(t)
	state.RadioUsed = map[string]string{"strike-1": "A"}
	ResolveOrders(&state, nil)
	if len(state.RadioUsed) != 0 {
		t.Fatalf("新回合没有重置无线电记录：%+v", state.RadioUsed)
	}
}

// 改目标同样是无线电通信：走结算通道、按编队计次，不能靠即时命令绕过次数限制。
func TestRetargetConsumesRadioSlot(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	aircraft.Status = "attack"
	aircraft.TaskID = "strike-1"
	aircraft.TaskKind = "attack"
	aircraft.Coord = state.Units["a2-cv-01"].Coord

	// 改目标不应再是即时命令，否则可以无限次免费改。
	if IsImmediateOrder("RETARGET_AIR") {
		t.Fatal("RETARGET_AIR 不应走即时通道")
	}
	order := Order{Type: "RETARGET_AIR", TaskID: "strike-1", Coord: "0505"}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("第一次改目标应当合法：%v", err)
	}
	ResolveOrders(&state, []Order{order})
	if _, used := state.RadioUsed["strike-1"]; !used {
		t.Fatal("改目标之后应当记掉该编队的无线电机会")
	}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("同一编队本回合不应能再发一次无线电")
	}
}

// CAP 编队只能通过无线电 RTB，不能改任务或改目标。
func TestCAPCanOnlyRTB(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["a2-cv-01-fighter-x1"]
	aircraft.Status = "cap"
	aircraft.TaskID = "task-cap-1"
	aircraft.TaskKind = "cap"
	aircraft.Coord = state.Units["a2-cv-01"].Coord
	aircraft.PatrolCoord = aircraft.Coord

	if err := ValidateOrder(state, "A", Order{Type: "RECALL_AIR", TaskID: aircraft.TaskID}); err != nil {
		t.Fatalf("CAP 应当可以 RTB：%v", err)
	}
	if err := ValidateOrder(state, "A", Order{Type: "RETARGET_AIR", TaskID: aircraft.TaskID, Coord: "0505"}); err == nil {
		t.Fatal("CAP 不应能改目标")
	}
	if err := ValidateOrder(state, "A", Order{Type: "RETASK_AIR", TaskID: aircraft.TaskID, TaskKind: "attack"}); err == nil {
		t.Fatal("CAP 不应能改任务")
	}
}

// 打击编队和转场编队可以改目标，也可以直接召回。
func TestStrikeCanRetargetOrRTB(t *testing.T) {
	for _, kind := range []string{"attack", "transfer"} {
		state := sandboxState(t)
		aircraft := state.Aircraft["a2-cv-01-dive_bomber-x1"]
		aircraft.Status = "attack"
		aircraft.TaskID = "strike-1"
		aircraft.TaskKind = kind
		aircraft.Coord = state.Units["a2-cv-01"].Coord
		if err := ValidateOrder(state, "A", Order{Type: "RETARGET_AIR", TaskID: "strike-1", Coord: "0505"}); err != nil {
			t.Fatalf("%s 编队应当可以改目标：%v", kind, err)
		}
		if err := ValidateOrder(state, "A", Order{Type: "RECALL_AIR", TaskID: "strike-1"}); err != nil {
			t.Fatalf("%s 编队应当可以召回：%v", kind, err)
		}
	}
}

// 不能在空里把编队改成 CAP。
func TestCannotRetaskToCAP(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	aircraft.Status = "attack"
	aircraft.TaskID = "strike-1"
	aircraft.TaskKind = "attack"
	aircraft.Coord = state.Units["a2-cv-01"].Coord
	if err := ValidateOrder(state, "A", Order{Type: "RETASK_AIR", TaskID: "strike-1", TaskKind: "cap"}); err == nil {
		t.Fatal("打击编队不应能改成 CAP")
	}
	if err := ValidateOrder(state, "A", Order{Type: "RETASK_AIR", TaskID: "strike-1", TaskKind: "transfer"}); err != nil {
		t.Fatalf("打击编队应当可以改成转场：%v", err)
	}
}
