package game

import "testing"

// 高空投弹的判定：战略/重型/中型/水平轰炸机挂炸弹算高空，鱼雷和战斗机不算。
func TestHighAltitudeClassification(t *testing.T) {
	cases := []struct {
		name     string
		aircraft *AirUnit
		want     bool
	}{
		{"B-17 挂炸弹", &AirUnit{Kind: "strategic_bomber", Model: "B-17 空中堡垒", Loadout: LoadoutBomb}, true},
		{"B-17 挂副油箱炸弹", &AirUnit{Kind: "strategic_bomber", Model: "B-17 空中堡垒", Loadout: LoadoutBombTank}, true},
		{"B-25 挂炸弹", &AirUnit{Kind: "level_bomber", Model: "B-25 米切尔", Loadout: LoadoutBomb}, true},
		{"B-26 挂炸弹", &AirUnit{Kind: "medium_bomber", Model: "B-26 劫掠者", Loadout: LoadoutBombTank}, true},
		{"B-26 挂鱼雷", &AirUnit{Kind: "medium_bomber", Model: "B-26 劫掠者", Loadout: LoadoutTorpedo}, false},
		{"一式陆攻挂鱼雷", &AirUnit{Kind: "torpedo_bomber", Model: "G4M 一式陆攻", Loadout: LoadoutTorpedoTank}, false},
		{"挂弹战斗机", &AirUnit{Kind: "fighter", Model: "F4F 野猫", Loadout: LoadoutBomb}, false},
		{"俯冲轰炸机", &AirUnit{Kind: "dive_bomber", Model: "SBD 无畏", Loadout: LoadoutBomb}, false},
	}
	for _, test := range cases {
		if got := ignoresLightAA(test.aircraft); got != test.want {
			t.Fatalf("%s：轻防空免疫应为 %v，实际 %v", test.name, test.want, got)
		}
	}
}

// airfieldStrikeFixture 造一次对机场的空袭：只有攻击机在空，没有 CAP、没有重防空。
func airfieldStrikeFixture(t *testing.T, lightAA float64, attackers ...*AirUnit) (State, *Airfield) {
	t.Helper()
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	if airfield == nil {
		t.Fatal("找不到哨兵机场")
	}
	airfield.LightAA = lightAA
	airfield.HeavyAA = 0
	state.Aircraft = map[string]*AirUnit{}
	for _, aircraft := range attackers {
		aircraft.Faction = "A"
		aircraft.Status = "attack"
		aircraft.TaskID = "ground-task"
		aircraft.TaskKind = "attack"
		aircraft.Coord = airfield.Coord
		aircraft.TargetCoord = airfield.Coord
		aircraft.Strength = 2
		aircraft.RemainingRange = 8
		state.Aircraft[aircraft.ID] = aircraft
	}
	return state, airfield
}

func highAltitudeStrikePlane(id, kind, model string, antiGround int) *AirUnit {
	return &AirUnit{ID: id, Kind: kind, Model: model, AntiGround: antiGround, Loadout: LoadoutBomb}
}

func diveStrikePlane(id string) *AirUnit {
	return &AirUnit{ID: id, Kind: "dive_bomber", Model: "SBD 无畏", AntiGround: 2, Loadout: LoadoutBomb}
}

// 战略轰炸机和中型轰炸机在高空投弹，机场的 25mm 轻防空够不着：
// 100 点轻防空掷满一轮，命中数仍然必须是 0。
func TestAirfieldLightAADoesNotReachHighAltitudeBombers(t *testing.T) {
	state, airfield := airfieldStrikeFixture(t, 100,
		highAltitudeStrikePlane("b17-1", "strategic_bomber", "B-17 空中堡垒", 2),
		highAltitudeStrikePlane("b25-1", "level_bomber", "B-25 米切尔", 2),
		highAltitudeStrikePlane("b26-1", "medium_bomber", "B-26 劫掠者", 2),
	)
	state.RNGState = 7
	resolveGroundStrike(&state, "ground-task", airfield.Coord)
	report := state.AirCombats[len(state.AirCombats)-1]
	if report.AAHits != 0 {
		t.Fatalf("高空轰炸机不应被机场轻防空命中，实际命中 %d 次", report.AAHits)
	}
	if report.AttackerLosses != 0 || report.AttackerDisabled != 0 {
		t.Fatalf("高空轰炸机不应因轻防空损失/退出：损失 %d，退出 %d", report.AttackerLosses, report.AttackerDisabled)
	}
}

// 同一编队里的俯冲轰炸机仍然吃轻防空：高空机免疫不影响低空机。
func TestAirfieldLightAAStillHitsLowAltitudeAttackers(t *testing.T) {
	state, airfield := airfieldStrikeFixture(t, 100,
		highAltitudeStrikePlane("b17-1", "strategic_bomber", "B-17 空中堡垒", 2),
		diveStrikePlane("sbd-1"),
	)
	state.RNGState = 3
	resolveGroundStrike(&state, "ground-task", airfield.Coord)
	report := state.AirCombats[len(state.AirCombats)-1]
	if report.AAHits == 0 {
		t.Fatal("100 点轻防空应该能打到俯冲轰炸机")
	}
}

// 机场轻防空是"整队一副骰子"：骰数等于机场轻防空值，不随攻击机数量翻倍。
// 旧实现每架攻击机各掷一整副，6 架飞机就会把期望命中数放大 6 倍。
func TestAirfieldLightAARollsOncePerStrike(t *testing.T) {
	run := func(seed uint64) int {
		state, airfield := airfieldStrikeFixture(t, 10,
			diveStrikePlane("sbd-1"), diveStrikePlane("sbd-2"), diveStrikePlane("sbd-3"),
			diveStrikePlane("sbd-4"), diveStrikePlane("sbd-5"), diveStrikePlane("sbd-6"),
		)
		state.RNGState = seed
		resolveGroundStrike(&state, "ground-task", airfield.Coord)
		return state.AirCombats[len(state.AirCombats)-1].AAHits
	}
	total := 0
	for seed := uint64(1); seed <= 300; seed++ {
		total += run(seed)
	}
	// 整队一副：300 局 × 10 点 × 约 3% ≈ 90 次；每架一副会是 6 倍、约 540 次。
	if total > 200 {
		t.Fatalf("300 局轻防空共命中 %d 次，骰子像是按每架飞机各掷了一副", total)
	}
	if total == 0 {
		t.Fatal("300 局一次都没命中，轻防空判定可能根本没掷")
	}
}

// 陆基轰炸机经机场算子投影后同样享受高空免疫（真实流程走的就是这条投影）。
func TestProjectedGroundBomberIgnoresAirfieldLightAA(t *testing.T) {
	state, airfield := airfieldStrikeFixture(t, 100)
	// 把三架 B-17 放进机场地面算子，再按真实流程投影成空中算子。
	for index, id := range []string{"gb-1", "gb-2", "gb-3"} {
		state.GroundAir[id] = &AirfieldAirUnit{
			ID: id, AirfieldID: "af-a-lighthouse", Faction: "A", Kind: "strategic_bomber",
			Model: "B-17 空中堡垒", Status: "attack", TaskID: "ground-task", TaskKind: "attack",
			Coord: airfield.Coord, TargetCoord: airfield.Coord, AntiGround: 2, Loadout: LoadoutBomb,
			Strength: 2, BaseRange: 37, MaxRange: 37, RemainingRange: 20,
		}
		_ = index
	}
	restore := projectGroundTaskIntoAircraft(&state, "ground-task")
	defer restore()
	state.RNGState = 11
	resolveGroundStrike(&state, "ground-task", airfield.Coord)
	report := state.AirCombats[len(state.AirCombats)-1]
	if report.AAHits != 0 {
		t.Fatalf("投影后的 B-17 仍被轻防空命中 %d 次", report.AAHits)
	}
}
