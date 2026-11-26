package game

import "testing"

// 机场打击编队不再被服务器强制掉头：能不能赶在天黑前回来是玩家自己的赌注，
// 服务器只提醒一次（night_warning），入夜那一刻仍按 forceNightLanding 结算。
//
// 以前这里一刀切召回，于是傍晚从拉包尔打莫尔兹比（19 hex）会在起飞那一刻被判
// "走完任务再返航要 38 回合"，整队掉头回场——远程陆攻等于被取消了。
func TestGroundStrikeNotForceRecalledBeforeNight(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	plane := state.GroundAir["af-b-rabaul-g4m-1"]
	if plane == nil {
		t.Fatal("缺少陆攻算子")
	}
	plane.Status = "attack"
	plane.TaskID = "ground-night"
	plane.TaskKind = "attack"
	plane.OriginCoord = "0428"
	plane.Coord = "1620"
	plane.TargetCoord = "1620"
	plane.Returning = false
	plane.SetRange(20)

	// 造一个"快到天黑"的时间线：TurnsPerDay 与当前回合配合，使剩余回合很小。
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"}
	state.TurnsPerDay = 72
	state.DayHours = 0
	state.Turn = 71 // 23:40 之前两回合天黑
	state.Night = false

	events := recallLoopingAircraftBeforeNight(&state)
	if plane.Returning {
		t.Fatalf("打击编队不应被强制返航：%+v", events)
	}
	if !plane.NightWarned || !hasEvent(events, "night_warning") {
		t.Fatalf("应给一次天黑前回不来的警告：%+v", events)
	}
	if again := recallLoopingAircraftBeforeNight(&state); hasEvent(again, "night_warning") {
		t.Fatal("同一个编队只该警告一次，否则每回合刷一条")
	}
}

// 巡逻／侦察这类本来就要一直留在空中的任务照样强制返航：它们没有"打完就回家"的终点。
func TestGroundReconRecalledBeforeNight(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	plane := state.GroundAir["af-b-rabaul-g3m-1"]
	if plane == nil {
		t.Fatal("缺少 G3M")
	}
	plane.Status = "recon"
	plane.TaskID = "ground-recon"
	plane.TaskKind = "recon"
	plane.OriginCoord = "0223"
	plane.Coord = "1620"
	plane.Heading = "W"
	plane.SectorWidth = 60
	plane.Returning = false
	plane.SetRange(20)

	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"}
	state.TurnsPerDay = 72
	state.DayHours = 0
	state.Turn = 71
	state.Night = false

	events := recallLoopingAircraftBeforeNight(&state)
	if !plane.Returning {
		t.Fatalf("侦察编队天黑前必须返航：%+v", events)
	}
	if plane.TargetCoord != "" {
		t.Fatalf("返航时应清掉任务目标：%q", plane.TargetCoord)
	}
}
