package game

import "testing"

// 剧本开启昼夜循环时，夜间状态应按回合号变化。
// 默认边界：19:40 入夜（回收截止）、05:20 天亮（第一轮升空）。
func TestDayNightCycleFollowsClock(t *testing.T) {
	state := State{Turn: 1, DayHours: 12, TurnsPerDay: 72}
	if state.Night {
		t.Fatal("开局正午不应是夜间")
	}
	// 每回合 20 分钟：12:00 之后 7h40m = 23 回合 → 第 24 回合 19:40 入夜。
	state.Turn = 23
	UpdateDayNight(&state)
	if state.Night {
		t.Fatalf("第 23 回合（19:20）还应是白天：%.2f", clockAt(state))
	}
	state.Turn = 24
	UpdateDayNight(&state)
	if !state.Night {
		t.Fatalf("第 24 回合（19:40）应入夜：%.2f", clockAt(state))
	}
	// 次日 05:20 天亮 = 12:00 之后 17h20m = 52 回合后 → 第 53 回合。
	state.Turn = 52
	UpdateDayNight(&state)
	if !state.Night {
		t.Fatalf("第 52 回合（次日 05:00）仍是夜间：%.2f", clockAt(state))
	}
	state.Turn = 53
	UpdateDayNight(&state)
	if state.Night {
		t.Fatalf("第 53 回合（次日 05:20）应当天亮：%.2f", clockAt(state))
	}
}

// 未开启循环时保持剧本固定昼夜。
func TestDayNightStaticWithoutCycle(t *testing.T) {
	state := State{Turn: 40, Night: true}
	UpdateDayNight(&state)
	if !state.Night {
		t.Fatal("未配置 TurnsPerDay 时不应改变夜间状态")
	}
}

// 规则书 199 行：天黑前两回合，自动循环任务集中返航。
// 提前两回合是因为召回命令在回合结束时登记，飞机要到下一回合才真的往回飞。
func TestLoopingAircraftRecallBeforeNight(t *testing.T) {
	state := State{Turn: 22, DayHours: 12, TurnsPerDay: 72}
	state.Aircraft = map[string]*AirUnit{
		"cap":  {ID: "cap", Faction: "A", Model: "F6F", Status: "cap", OnCycle: true, CycleKind: "cap", TaskID: "t1", TaskKind: "cap"},
		"hold": {ID: "hold", Faction: "A", Model: "F6F", Status: "cap", OnCycle: false, TaskID: "t2", TaskKind: "cap"},
		"reco": {ID: "reco", Faction: "A", Model: "F6F", Status: "recon", TaskID: "t3", TaskKind: "recon"},
	}
	events := recallLoopingAircraftBeforeNight(&state)
	if !state.Aircraft["cap"].Returning || !state.Aircraft["reco"].Returning {
		t.Fatal("自动循环的 CAP 和侦察机应当返航")
	}
	// 手动派出的 CAP 也一并召回：它离母舰只要 1 回合，而天黑只剩 2 回合，
	// 不强制掉头就是白白丢飞机——玩家不会因为"我要自己决定"而想损失算子。
	if !state.Aircraft["hold"].Returning {
		t.Fatal("已经来不及的手动 CAP 也应被强制召回")
	}
	recalled := 0
	for _, event := range events {
		if event.Type == "night_recall" {
			recalled++
		}
	}
	if recalled == 0 {
		t.Fatalf("应有返航事件：%+v", events)
	}
	// 还早的时候（第 21 回合，下一回合 19:20 仍属白天）不该召回。
	early := State{Turn: 21, DayHours: 12, TurnsPerDay: 72}
	early.Aircraft = state.Aircraft
	if events := recallLoopingAircraftBeforeNight(&early); len(events) != 0 {
		t.Fatalf("离天黑还有两回合以上时不应召回：%+v", events)
	}
}

// 天黑不是损失：入夜那一刻还在天上的算子继续飞，返航落地时才逐个判夜间摔毁。
// 以前这里一刀切"天黑还在天上=坠海"，远程打击波会整波凭空消失。
func TestNightfallKeepsAircraftFlying(t *testing.T) {
	state := State{Turn: 24, DayHours: 12, TurnsPerDay: 72}
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"}
	carrier := &Unit{ID: "cv", Faction: "A", Name: "列克星敦", Coord: "0505", Card: ShipCard{HangarCapacity: 8}}
	carrier.Deck = &FlightDeck{Length: 4, Elevators: []*AircraftElevator{{ID: "e1", Capacity: 2, Status: "idle"}}}
	state.Units = map[string]*Unit{"cv": carrier}
	state.Aircraft = map[string]*AirUnit{
		// 就在母舰上空：落地要过夜间 70% 判定，但不会被入夜直接删掉。
		"homebound": {ID: "homebound", CarrierID: "cv", Faction: "A", Model: "SBD", Status: "attack", Coord: "0505", MaxRange: 10, RemainingRange: 5},
		// 十几格外：继续飞，燃油与落地判定照常结算。
		"far":  {ID: "far", CarrierID: "cv", Faction: "A", Model: "SBD", Status: "attack", Coord: "0808", MaxRange: 10, RemainingRange: 5},
		"deck": {ID: "deck", CarrierID: "cv", Faction: "A", Model: "F4F", Status: "hangar"},
	}
	events := UpdateDayNight(&state)
	if !state.Night {
		t.Fatalf("第 24 回合应入夜：%.2f", clockAt(state))
	}
	for _, id := range []string{"homebound", "far", "deck"} {
		if state.Aircraft[id] == nil {
			t.Fatalf("%s 在天黑时被凭空删掉了", id)
		}
	}
	for _, event := range events {
		if event.Type == "air_destroyed" || event.Type == "air_night_lost" {
			t.Fatalf("入夜不该判任何损失：%s：%s", event.Type, event.Text)
		}
	}
}

// 陆基机夜间落地要过 20% 摔毁判定；舰载机是 70%。两边都得判，不能只有航母那一套。
func TestNightLandingRiskAppliesToAirfields(t *testing.T) {
	state := State{Turn: 1, Night: true, RNGState: 5}
	airfield := &Airfield{ID: "af", Faction: "A", Type: AirfieldLarge, Coord: "0505", Runways: 3, Strength: 15}
	state.Airfields = map[string]*Airfield{"af": airfield}
	state.GroundAir = map[string]*AirfieldAirUnit{"b17": {
		ID: "b17", AirfieldID: "af", Faction: "A", Model: "B-17 空中堡垒", Kind: "heavy_bomber",
		Status: "attack", Coord: "0505", MaxRange: 37, RemainingRange: 20,
	}}
	crashes := 0
	for attempt := 0; attempt < 200; attempt++ {
		plane := &AirfieldAirUnit{ID: "p", AirfieldID: "af", Faction: "A", Model: "B-17 空中堡垒", Kind: "heavy_bomber",
			Status: "attack", Coord: "0505", MaxRange: 37, RemainingRange: 20}
		state.GroundAir["p"] = plane
		events := landGroundAircraft(&state, plane, airfield)
		if state.GroundAir["p"] == nil {
			crashes++
			if len(events) == 0 || events[len(events)-1].Type != "night_landing_crash" {
				t.Fatalf("摔毁时没有 night_landing_crash 事件：%+v", events)
			}
		}
	}
	if crashes == 0 || crashes >= 200 {
		t.Fatalf("夜间降落机场 20%% 摔毁判定没有生效：200 次里摔了 %d 次", crashes)
	}
}

// 时间线窗口：04:00 之前不整备，05:20 之前不放飞，19:40 之后算夜间。
func TestTimelineWindows(t *testing.T) {
	state := State{Turn: 1, DayHours: 0, TurnsPerDay: 72}
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"}
	cases := []struct {
		turn     int
		clock    string
		service  bool
		launched bool
		night    bool
	}{
		{1, "00:00", false, false, true},
		{13, "04:00", true, false, true},
		{16, "05:00", true, false, true},
		{17, "05:20", true, true, false},
		{60, "19:40", false, false, true},
	}
	for _, test := range cases {
		state.Turn = test.turn
		if got := formatClock(clockAt(state)); got != test.clock {
			t.Fatalf("第 %d 回合时刻应为 %s：%s", test.turn, test.clock, got)
		}
		if got := state.ServiceWindowOpen(); got != test.service {
			t.Fatalf("%s 的整备窗口应为 %v", test.clock, test.service)
		}
		if got := state.LaunchWindowOpen(); got != test.launched {
			t.Fatalf("%s 的放飞窗口应为 %v", test.clock, test.launched)
		}
		if got := state.nightAt(clockAt(state)); got != test.night {
			t.Fatalf("%s 的夜间判定应为 %v", test.clock, got)
		}
	}
}
