package game

import "testing"

// 远距离侦察机必须按自己的返航时间提前召回：
// 13 hex 外的水上侦察机飞回来要 14 回合，固定"提前两回合"等于把人送进海里。
func TestFarReconRecalledByDistance(t *testing.T) {
	state := State{Turn: 30, DayHours: 0, TurnsPerDay: 72}
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40", NightLoss: true}
	carrier := &Unit{ID: "cv", Faction: "B", Name: "翔鹤", Coord: "1030"}
	state.Units = map[string]*Unit{"cv": carrier}
	// 水侦速度 1 hex/回合，扇面半径 6：走完扇面还要再飞 6 hex 回来 = 13 回合（含着舰余量）。
	far := &AirUnit{ID: "far", CarrierID: "cv", Faction: "B", Model: "E13A", Kind: "recon", Status: "recon", Seaplane: true,
		Coord: "1036", Heading: "E", SectorWidth: 60, ReconRadius: 6, OnCycle: true, CycleKind: "recon", TaskID: "r1", TaskKind: "recon"}
	near := &AirUnit{ID: "near", CarrierID: "cv", Faction: "B", Model: "A6M2", Kind: "fighter", Status: "cap",
		Coord: "1030", OnCycle: true, CycleKind: "cap", TaskID: "c1", TaskKind: "cap"}
	state.Aircraft = map[string]*AirUnit{"far": far, "near": near}
	// 第 52 回合 = 17:00，距 19:40 还有 8 回合：6 hex 外的水侦刚好够回去，该立刻掉头。
	state.Turn = 52
	events := recallLoopingAircraftBeforeNight(&state)
	if !far.Returning {
		t.Fatalf("13 hex 外的侦察机应立刻返航：%+v", far)
	}
	// 母舰上空的 CAP 只要 1 回合就能落回来，离天黑还有 8 回合，不该现在收。
	if state.Aircraft["near"].Returning {
		t.Fatal("还够时间的时候不该收 CAP")
	}
	// 按"从当前位置回家"算：6 hex + 1 着舰 = 7 回合，距天黑 8 回合，刚好该掉头。
	warned := false
	for _, event := range events {
		if event.Type == "night_warning" || event.Type == "night_recall" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("来不及的侦察机应有返航或警告事件：%+v", events)
	}
}

// 还来得及的编队要到"刚好够回"的那一刻才收，不提前浪费一轮任务时间。
func TestRecallFiresAtLastChance(t *testing.T) {
	state := State{Turn: 52, DayHours: 0, TurnsPerDay: 72} // 17:00，距 19:40 还有 8 回合
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40", NightLoss: true}
	state.Units = map[string]*Unit{"cv": {ID: "cv", Faction: "B", Coord: "1030"}}
	// 已经扫完扇面、正在往回飞的水侦，在母舰 6 hex 外：6 回合航程 + 1 着舰余量 = 7 回合，
	// 而距天黑只剩 8 回合——正好是"现在必须掉头"的那一回合。
	tight := &AirUnit{ID: "tight", CarrierID: "cv", Faction: "B", Model: "E13A", Kind: "recon", Status: "recon", Seaplane: true,
		Coord: "1036", Heading: "E", SectorWidth: 60, ReconRadius: 6, ReconDistance: 6, OnCycle: true, CycleKind: "recon", TaskID: "r2", TaskKind: "recon"}
	state.Aircraft = map[string]*AirUnit{"tight": tight}
	events := recallLoopingAircraftBeforeNight(&state)
	if !tight.Returning {
		t.Fatal("刚好够回去的编队应该现在就掉头")
	}
	recalled := false
	for _, event := range events {
		if event.Type == "night_recall" {
			recalled = true
		}
	}
	if !recalled {
		t.Fatalf("应产生强制返航事件：%+v", events)
	}
}

// 已经来不及的飞机不再召回（召也回不来），只发一条明确的警告。
func TestDoomedAircraftWarning(t *testing.T) {
	state := State{Turn: 55, DayHours: 0, TurnsPerDay: 72}
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40", NightLoss: true}
	state.Units = map[string]*Unit{"cv": &Unit{ID: "cv", Faction: "B", Coord: "1030"}}
	doomed := &AirUnit{ID: "far", CarrierID: "cv", Faction: "B", Model: "E13A", Kind: "recon", Status: "recon",
		Coord: "1055", OnCycle: true, CycleKind: "recon", TaskID: "r1", TaskKind: "recon"}
	state.Aircraft = map[string]*AirUnit{"far": doomed}
	events := recallLoopingAircraftBeforeNight(&state)
	warned := false
	for _, event := range events {
		if event.Type == "night_warning" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("赶不回来的飞机应有明确警告：%+v", events)
	}
}
