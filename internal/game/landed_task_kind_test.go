package game

import "testing"

// TestLandedAircraftKeepsTaskKind 回归：着舰不能把任务类型清掉。
// TaskID 保留但 TaskKind 被清空的算子会变成"无类型残骸"：任务栏里既不是攻击也不是侦察，
// 卡在升降机/甲板上既放飞不了也回收不了，玩家看到的是一队回不来的轰炸机。
func TestLandedAircraftKeepsTaskKind(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-01-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	carrier.Deck.Operation = ""
	aircraft.Status = "attack"
	aircraft.TaskKind = "attack"
	aircraft.TaskID = "strike-keep-kind"
	aircraft.Coord = carrier.Coord

	LandAircraft(&state, aircraft, carrier)
	if aircraft.Status != "recovery" {
		t.Fatalf("着舰后应进入回收状态：%s", aircraft.Status)
	}
	if aircraft.TaskKind != "attack" || aircraft.TaskID != "strike-keep-kind" {
		t.Fatalf("着舰不应清掉任务归属：kind=%q task=%q", aircraft.TaskKind, aircraft.TaskID)
	}

	view := VisibleAviation(state, "B")
	found := false
	for _, task := range view.Tasks {
		if task.ID != "strike-keep-kind" {
			continue
		}
		found = true
		if task.Kind != "attack" {
			t.Fatalf("回收中的攻击编队在任务栏里丢了类型：%q", task.Kind)
		}
	}
	if !found {
		t.Fatal("回收中的攻击编队从任务栏消失了")
	}
}

// TestKindlessAircraftNeverLifted 没有任务类型的算子不许被提升到甲板，
// 否则它占着升降机和甲板却永远放飞不了，把整条航空作业链堵死。
func TestKindlessAircraftNeverLifted(t *testing.T) {
	state := sandboxState(t)
	freezeFleets(&state)
	carrier := state.Units["b-cv-01"]
	carrier.Deck.Operation = ""
	aircraft := state.Aircraft["b-cv-01-dive_bomber-x2"]
	aircraft.Status = "service"
	aircraft.TaskID = "ghost-task"
	aircraft.TaskKind = ""
	aircraft.ServiceTurns = 2
	aircraft.Coord = carrier.Coord
	aircraft.SetRange(12)

	for turn := 0; turn < 4; turn++ {
		AdvanceAviation(&state)
		switch aircraft.Status {
		case "deck", "elevator", "ready":
			t.Fatalf("第 %d 回合无类型算子被提升了：%s", turn, aircraft.Status)
		}
	}
	if used := deckUsed(state, carrier.ID); used != 0 {
		t.Fatalf("甲板被无类型算子占用：%d", used)
	}
}
