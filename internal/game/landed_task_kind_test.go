package game

import "testing"

// TestLandedAttackAircraftLeavesTaskBar：打击任务栏只放"还在整备、准备出击"的编队。
// 编队离场后属于海图，落回甲板/机库的算子属于回收面板——落地还给任务栏留一张
// "任务整备中"的编队卡，玩家会以为这波又回来了。
// 同时守住以前的幽灵状态：TaskID 留着、TaskKind 清掉 = 既不能放飞也不能重新分配。
func TestLandedAttackAircraftLeavesTaskBar(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-01-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	carrier.Deck.Operation = ""
	aircraft.Status = "attack"
	aircraft.TaskKind = "attack"
	aircraft.TaskID = "strike-leave"
	aircraft.Coord = carrier.Coord

	LandAircraft(&state, aircraft, carrier)
	if aircraft.Status != "recovery" {
		t.Fatalf("着舰后应进入回收状态：%s", aircraft.Status)
	}
	if aircraft.TaskID != "" || aircraft.TaskKind != "" {
		t.Fatalf("落地应退出打击编队：kind=%q task=%q", aircraft.TaskKind, aircraft.TaskID)
	}
	if aircraft.TaskLane != 0 {
		t.Fatalf("落地应归还任务栏位：%d", aircraft.TaskLane)
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.ID == "strike-leave" {
			t.Fatalf("落地后编队不该还留在任务栏：%+v", task)
		}
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
