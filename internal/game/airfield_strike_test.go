package game

import (
	"strings"
	"testing"
)

// 陆基轰炸机空袭敌方机场：CAP 拦截 → 重防空 → 轻防空 → 逐点对地判定 → 扣生命值/摧毁地面飞机。
func TestLandBasedStrikeOnEnemyAirfield(t *testing.T) {
	state := coralCombatState(t)
	airfield := state.Airfields["af-a-moresby"]
	if airfield == nil {
		t.Fatal("找不到莫尔兹比机场")
	}
	before := airfield.Strength
	beforePlanes := len(sortedGroundAirIDs(state, airfield.ID))

	// 拉包尔的两架 G3M 挂炸弹去打莫尔兹比。
	taskID := "af-strike-moresby"
	for _, id := range []string{"af-b-rabaul-g3m-1", "af-b-rabaul-g3m-2"} {
		plane := state.GroundAir[id]
		if plane == nil {
			t.Fatalf("找不到 %s", id)
		}
		plane.TaskID = taskID
		plane.TaskKind = "attack"
		plane.Status = "attack"
		plane.Coord = airfield.Coord
		plane.TargetCoord = airfield.Coord
		plane.Loadout = LoadoutBomb
		plane.SetRange(10)
	}

	if err := ValidateStrikeTarget(state, "B", airfield.Coord); err != nil {
		t.Fatalf("机场应该可以直接选为打击目标：%v", err)
	}

	hit := false
	for attempt := 0; attempt < 40 && !hit; attempt++ {
		state.RNGState = uint64(attempt + 1)
		before = airfield.Strength
		beforePlanes = len(sortedGroundAirIDs(state, airfield.ID))
		// 真实流程里 advanceGroundAttack 会先把机场编队投影成舰载机算子再结算，
		// 这里照做，否则 taskAircraft 在 state.Aircraft 里找不到攻击者。
		restore := projectGroundTaskIntoAircraft(&state, taskID)
		events := resolveGroundStrike(&state, taskID, airfield.Coord)
		restore()
		if airfield.Strength < before || len(sortedGroundAirIDs(state, airfield.ID)) < beforePlanes {
			hit = true
		}
		if len(events) == 0 {
			t.Fatal("对机场空袭没有产生任何事件")
		}
		for _, plane := range state.GroundAir {
			if plane.TaskID == taskID {
				plane.Status = "attack"
				plane.Returning = false
				plane.Coord = airfield.Coord
			}
		}
	}
	if !hit {
		t.Fatal("40 次空袭都没有伤到机场或地面飞机")
	}
	if len(state.AirCombats) == 0 {
		t.Fatal("对机场空袭没有生成战报")
	}
	report := state.AirCombats[len(state.AirCombats)-1]
	if report.Target != string(StrikeTargetAir) || report.Airfield == nil {
		t.Fatalf("战报目标不是机场：%+v", report.Target)
	}
	if !strings.Contains(report.Summary, "空袭") {
		t.Fatalf("战报摘要不对：%s", report.Summary)
	}
}

// 敌方机场永远可以被选为打击目标（不依赖舰队情报）。
func TestEnemyAirfieldIsAlwaysAValidStrikeTarget(t *testing.T) {
	state := coralCombatState(t)
	for _, airfield := range state.Airfields {
		if airfield.Faction != "A" {
			continue
		}
		if err := ValidateStrikeTarget(state, "B", airfield.Coord); err != nil {
			t.Fatalf("%s 应该可以被打：%v", airfield.Name, err)
		}
	}
	// 自己的机场不能当目标。
	own := state.Airfields["af-b-rabaul"]
	if err := ValidateStrikeTarget(state, "B", own.Coord); err == nil {
		t.Fatal("自己的机场不应该能作为打击目标")
	}
}

// 不挂弹的护航战斗机扫射机场：固定 1 点强度，不按空战值，不减机场生命/岸防。
func TestFighterStrafesAirfieldWithOnePoint(t *testing.T) {
	const taskID = "af-strafe-moresby"
	var report *AirCombatReport
	for seed := uint64(1); seed <= 300 && report == nil; seed++ {
		state := coralCombatState(t)
		state.RNGState = seed
		airfield := state.Airfields["af-a-moresby"]
		if airfield == nil {
			t.Fatal("找不到莫尔兹比机场")
		}
		fighter := state.GroundAir["af-b-rabaul-fighter-1"]
		if fighter == nil {
			t.Fatal("找不到拉包尔的 A6M2")
		}
		fighter.TaskID = taskID
		fighter.TaskKind = "attack"
		fighter.Status = "attack"
		fighter.Coord = airfield.Coord
		fighter.TargetCoord = airfield.Coord
		fighter.Loadout = LoadoutAir
		fighter.SetRange(10)

		beforeStrength := airfield.Strength
		restore := projectGroundTaskIntoAircraft(&state, taskID)
		resolveGroundStrike(&state, taskID, airfield.Coord)
		restore()
		if len(state.AirCombats) == 0 {
			continue
		}
		candidate := &state.AirCombats[len(state.AirCombats)-1]
		if candidate.StrafeAttempts != 1 {
			// 被轻防空打掉/打退的种子没有扫射，换下一个。
			continue
		}
		if candidate.BombAttempts != 0 || candidate.StrikeHits != 0 {
			t.Fatalf("战斗机扫射不应按投弹结算：%+v", candidate)
		}
		if airfield.Strength != beforeStrength || candidate.AirfieldAfter != candidate.AirfieldBefore {
			t.Fatalf("战斗机扫射不应扣机场生命：%d -> %d", beforeStrength, airfield.Strength)
		}
		if candidate.StrafeHits > 1 || candidate.GroundLoss > 1 {
			t.Fatalf("1 点扫射最多摧毁 1 架地面飞机：%+v", candidate)
		}
		report = candidate
	}
	if report == nil {
		t.Fatal("300 个种子里没有一次完成 1 点扫射")
	}
}

// 机场出击允许混合机型：B-25 和 SBD 同机场、同状态可以进同一条 LAUNCH_STRIKE。
// 之前美军遇到的 400 实际是状态不同（机库/待命），不是机型限制。
func TestGroundStrikeAllowsMixedAircraftTypes(t *testing.T) {
	state := coralCombatState(t)
	sbd := state.GroundAir["af-a-moresby-dive_bomber-1"]
	b25 := state.GroundAir["af-a-moresby-b25-1"]
	if sbd == nil || b25 == nil {
		t.Fatal("莫尔兹比缺少 SBD 或 B-25")
	}
	for _, plane := range []*AirfieldAirUnit{sbd, b25} {
		plane.Status = "hangar"
		plane.TaskID = ""
		plane.TaskKind = ""
		plane.Damaged = false
		plane.Maintenance = 0
	}
	state.Night = false
	state.Turn = 30 // 白天整备窗口，避免夜间校验先拦下
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{sbd.ID, b25.ID}, Coord: "0224"}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("同机场同状态的混合机型应允许出击：%v", err)
	}
}
