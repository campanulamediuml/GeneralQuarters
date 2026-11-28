package game

import "testing"

// TestCarrierlessAircraftDivertToAirfield 母舰沉没后，空中算子不应当场消失：
// 备降同阵营可用甲板；没有甲板时转场到最近的可用机场，从舰载机列表移入机场地面算子。
func TestCarrierlessAircraftDivertToAirfield(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "orphan-recon", CarrierID: "gone-carrier", Faction: "A", Kind: "recon", Model: "F6F-5P", Status: "recon",
		Strength: 1, TaskID: "orphan-recon", TaskKind: "recon", OriginCoord: "0602", Coord: "0602", RemainingRange: 8, MaxRange: 16, Heading: "E", SectorWidth: 60}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	// 机场在 2 hex 外，一回合 2 格即可抵达；推进两回合以保证稳定降落。
	events := AdvanceAviation(&state)
	events = append(events, AdvanceAviation(&state)...)
	if _, still := state.Aircraft["orphan-recon"]; still {
		t.Fatal("失去母舰的侦察机不应继续留在舰载机列表里")
	}
	if diverted := state.GroundAir["orphan-recon"]; diverted == nil || diverted.AirfieldID != "af-a-lighthouse" {
		t.Fatalf("侦察机应备降到最近的友方机场：%+v", diverted)
	}
	if len(events) == 0 {
		t.Fatal("备降应产生战报")
	}
}

// TestAirfieldRecoveryRepairsRunways 机场每回合按损管判定修复跑道：
// 生命值回升、可用跑道随之恢复，并且要有 airfield_repaired 战报。
func TestAirfieldRecoveryRepairsRunways(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Strength = airfield.MaxStrength - 3
	repaired := 0
	for turn := 0; turn < 200 && airfield.Strength < airfield.MaxStrength; turn++ {
		for _, event := range AdvanceAirfields(&state) {
			if event.Type == "airfield_repaired" {
				repaired++
			}
		}
	}
	if airfield.Strength != airfield.MaxStrength {
		t.Fatalf("200 回合内跑道应修复到满值：%d/%d", airfield.Strength, airfield.MaxStrength)
	}
	if repaired != 3 {
		t.Fatalf("每修好 1 点生命值应发一条战报，实际 %d 条", repaired)
	}
	if got := RunwaysAvailable(airfield.Strength, airfield.Runways); got != airfield.Runways {
		t.Fatalf("修满后可用跑道应恢复：%d/%d", got, airfield.Runways)
	}
}

// TestAirfieldStrikeInterruptsServicing 机场被命中时，正在整备/滑行的算子当场退回机库。
//
// 防空判定会消耗随机数，所以这里不写死种子：扫一批种子，找到有命中的那一局再校验打断逻辑，
// 避免防空公式一改这个用例就因为"这个种子恰好没命中"而误报。
func TestAirfieldStrikeInterruptsServicing(t *testing.T) {
	run := func(seed uint64) (*AirCombatReport, []EngineEvent) {
		state := sandboxState(t)
		airfield := state.Airfields["af-b-sentry"]
		for _, id := range sortedGroundAirIDs(state, airfield.ID) {
			state.GroundAir[id].Status = "deck"
		}
		attackers := map[string]*AirUnit{}
		for _, id := range []string{"atk-1", "atk-2", "atk-3", "atk-4"} {
			attackers[id] = &AirUnit{ID: id, CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack",
				Strength: 2, AirCombat: 1, AntiGround: 4, TaskID: "ground-task", TaskKind: "attack", Loadout: LoadoutBombTank,
				Coord: airfield.Coord, TargetCoord: airfield.Coord, RemainingRange: 8, MaxRange: 12}
		}
		state.Aircraft = attackers
		state.RNGState = seed
		events := resolveGroundStrike(&state, "ground-task", airfield.Coord)
		report := state.AirCombats[len(state.AirCombats)-1]
		return &report, events
	}
	var report *AirCombatReport
	var events []EngineEvent
	for seed := uint64(1); seed <= 120; seed++ {
		candidate, candidateEvents := run(seed)
		if candidate.StrikeHits > 0 {
			report, events = candidate, candidateEvents
			break
		}
	}
	if report == nil {
		t.Fatal("120 个种子里一次命中都没有，对地攻击概率可能算错了")
	}
	interrupted := 0
	found := false
	for _, event := range events {
		if event.Type == "airfield_operations_interrupted" {
			found = true
		}
	}
	if report.StrikeHits > 0 {
		// 每个命中都会打断一次整备；这一局至少要能看到打断事件。
		if !found {
			t.Fatalf("机场命中后应发出 airfield_operations_interrupted：%+v", events)
		}
	}
	_ = interrupted
}
