package game

import "testing"

// advanceUntilStatus 推进航空作业，直到算子进入期望状态或回合用尽。
func advanceUntilStatus(state *State, id, status string, turns int) bool {
	for turn := 0; turn < turns; turn++ {
		AdvanceAviation(state)
		if aircraft := state.Aircraft[id]; aircraft != nil && aircraft.Status == status {
			return true
		}
	}
	return false
}

// 规则书 188 行：CAP 降落后自动重新整备，再按原巡逻空域再次升空。
func TestCAPCycleRelaunchesAfterLanding(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	state.Units["b-cv-02"].Coord = carrier.Coord
	cap := &AirUnit{ID: "cap-cycle", CarrierID: carrier.ID, Faction: "B", Kind: "fighter", Model: "A6M2",
		Status: "cap", Strength: 2, AirCombat: 5, TaskID: "cap-TG-B", TaskKind: "cap",
		Coord: carrier.Coord, PatrolCoord: carrier.Coord, Returning: true, OnCycle: true, CycleTaskID: "cap-TG-B"}
	cap.SetRange(3)
	state.Aircraft = map[string]*AirUnit{cap.ID: cap}
	if !advanceUntilStatus(&state, cap.ID, "service", 6) {
		t.Fatalf("CAP 落舰后应自动重新进入整备：%s", cap.Status)
	}
	if !cap.OnCycle || cap.TaskKind != "cap" || cap.TaskID != "cap-TG-B" {
		t.Fatalf("重新整备应恢复 CAP 任务与循环开关：%+v", cap)
	}
	if cap.PatrolCoord != carrier.Coord {
		t.Fatalf("重新出动应回到母舰巡逻空域：%s", cap.PatrolCoord)
	}
}

// 玩家手动关掉循环后，落地不再自动恢复（规则书 191 行）。
func TestCycleOptOutSurvivesLanding(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	cap := &AirUnit{ID: "cap-stop", CarrierID: carrier.ID, Faction: "B", Kind: "fighter", Model: "A6M2",
		Status: "cap", Strength: 2, AirCombat: 5, TaskID: "cap-TG-B", TaskKind: "cap",
		Coord: carrier.Coord, PatrolCoord: carrier.Coord, Returning: true, OnCycle: true}
	cap.SetRange(3)
	state.Aircraft = map[string]*AirUnit{cap.ID: cap}
	order := Order{Type: "SET_AIR_CYCLE", AircraftID: cap.ID}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("取消循环的命令被拒绝：%v", err)
	}
	ApplyAirOrder(&state, order)
	if cap.OnCycle {
		t.Fatal("命令应当关闭循环开关")
	}
	if !advanceUntilStatus(&state, cap.ID, "hangar", 6) {
		t.Fatalf("算子应正常落回机库：%s", cap.Status)
	}
	for turn := 0; turn < 3; turn++ {
		AdvanceAviation(&state)
	}
	if cap.Status != "hangar" || cap.OnCycle {
		t.Fatalf("取消循环后不应自己再出动：status=%s on_cycle=%v", cap.Status, cap.OnCycle)
	}
}

// 侦察循环要带着原来的航向、扇面和半径重新出动，否则等于飞一个空任务。
func TestReconCycleKeepsSearchParams(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-04"]
	recon := &AirUnit{ID: "recon-cycle", CarrierID: carrier.ID, Faction: "B", Kind: "dive_bomber", Model: "D3A1",
		Status: "recon", Strength: 1, TaskID: "recon-cycle", TaskKind: "recon",
		Heading: "W", SectorWidth: 60, ReconRadius: 5, Coord: carrier.Coord, OriginCoord: carrier.Coord,
		Returning: true, OnCycle: true, BaseRange: 18, MaxRange: 23}
	recon.SetRange(3)
	state.Aircraft = map[string]*AirUnit{recon.ID: recon}
	if !advanceUntilStatus(&state, recon.ID, "service", 8) {
		t.Fatalf("侦察机落舰后应自动重新整备：%s", recon.Status)
	}
	// 半径不可设置，一律是机型上限；航向和扇面必须保住。
	if limit := ReconRadiusLimit(*recon); recon.TaskKind != "recon" || recon.Heading != "W" || recon.SectorWidth != 60 || recon.ReconRadius != limit {
		t.Fatalf("侦察循环丢了搜索参数（期望半径 %d）：%+v", limit, recon)
	}
	if recon.ReconDistance != 0 || recon.Coord != carrier.Coord {
		t.Fatalf("重新出动应从母舰、距离 0 开始：coord=%s distance=%d", recon.Coord, recon.ReconDistance)
	}
}

// 空中掩护循环：队友舰队动了，下一轮掩护点跟着舰队走，而不是回到那片空海。
func TestCoverCycleFollowsProtectedFleet(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	escort := state.Units["b-dd-01"]
	escort.FleetID = "TG-B-ESCORT"
	escort.FleetName = "护卫队"
	escort.Coord = "0206"
	cover := &AirUnit{ID: "cover-cycle", CarrierID: carrier.ID, Faction: "B", Kind: "fighter", Model: "A6M2",
		Status: "cover", Strength: 2, AirCombat: 5, TaskID: "cover-cycle", TaskKind: "cover",
		TargetCoord: "0202", Coord: carrier.Coord, Returning: true, OnCycle: true}
	cover.SetRange(3)
	state.Aircraft = map[string]*AirUnit{cover.ID: cover}
	if !advanceUntilStatus(&state, cover.ID, "service", 8) {
		t.Fatalf("掩护编队落舰后应自动重新整备：%s", cover.Status)
	}
	if cover.TaskKind != "cover" {
		t.Fatalf("循环任务类型应为 cover：%s", cover.TaskKind)
	}
	if cover.TargetCoord != escort.Coord {
		t.Fatalf("掩护点应跟着被掩护的舰队：%s（期望 %s）", cover.TargetCoord, escort.Coord)
	}
}

// 母舰不可用时循环结束，并且给出明确原因，而不是静默停摆。
func TestCycleEndsWhenCarrierLost(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	cap := &AirUnit{ID: "cap-lost", CarrierID: carrier.ID, Faction: "B", Kind: "fighter", Model: "A6M2",
		Status: "hangar", Strength: 2, AirCombat: 5, TaskID: "cap-TG-B", TaskKind: "cap",
		Coord: carrier.Coord, OnCycle: true, CycleKind: "cap"}
	state.Aircraft = map[string]*AirUnit{cap.ID: cap}
	delete(state.Units, carrier.ID)
	events := AdvanceAviation(&state)
	if cap.OnCycle {
		t.Fatal("母舰沉没后循环应当结束")
	}
	ended := false
	for _, event := range events {
		if event.Type == "air_cycle_end" {
			ended = true
		}
	}
	if !ended {
		t.Fatalf("循环结束时应有说明事件：%+v", events)
	}
}

// 剧本时间线：04:00 之前不进整备，05:20 之前不放飞。
func TestTimelineBlocksServiceAndLaunch(t *testing.T) {
	state := sandboxState(t)
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40", NightLoss: true}
	state.TurnsPerDay = 72
	state.DayHours = 0
	state.Turn = 1

	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"b-cv-01-fighter-x1"}}
	ApplyAirOrder(&state, order)
	aircraft := state.Aircraft["b-cv-01-fighter-x1"]
	for turn := 0; turn < 10; turn++ {
		AdvanceAviation(&state)
	}
	if aircraft.Status != "service" || aircraft.ServiceTurns != 0 {
		t.Fatalf("04:00 之前不应推进机库整备：status=%s turns=%d", aircraft.Status, aircraft.ServiceTurns)
	}

	// 04:00（第 13 回合）开始整备。
	state.Turn = 13
	AdvanceAviation(&state)
	if aircraft.ServiceTurns != 1 {
		t.Fatalf("04:00 之后应开始整备：%d", aircraft.ServiceTurns)
	}

	// 05:00（第 16 回合）整理完了也不许起飞。
	aircraft.Status = "deck"
	aircraft.ServiceTurns = 2
	state.Turn = 16
	AdvanceAviation(&state)
	if aircraft.Status == "cap" {
		t.Fatal("05:20 之前不应放飞")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftIDs: []string{aircraft.ID}}); err == nil {
		t.Fatal("甲板算子提前起飞应被拒绝")
	}

	// 05:20（第 17 回合）放行。
	state.Turn = 17
	AdvanceAviation(&state)
	if aircraft.Status != "cap" {
		t.Fatalf("05:20 之后应正常放飞：%s", aircraft.Status)
	}
}
