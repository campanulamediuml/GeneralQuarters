package game

import "testing"

// 机场掩护编队被召回后必须真的往回飞。
// 以前 cover 分支不看 Returning，advanceGroundPatrol 又跳过 Returning 的成员：
// 日志写着"强制返航"，飞机却钉在掩护空域不动，天黑之后还在天上。
func TestGroundCoverForcedRecallFliesHome(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	cover := state.GroundAir["af-a-moresby-p40-1"]
	if cover == nil {
		t.Fatal("找不到莫尔斯比的 P-40")
	}
	airfield := airfieldFor(&state, cover.AirfieldID)
	if airfield == nil {
		t.Fatal("找不到所属机场")
	}
	// 停在离机场两格外的空域值班，召回标记已经挂上。
	cover.Status = "cover"
	cover.TaskID = "cover-moresby"
	cover.TaskKind = "cover"
	cover.Coord = "1811"
	cover.OriginCoord = airfield.Coord
	cover.TargetCoord = "1811"
	cover.Returning = true
	before := Distance(cover.Coord, airfield.Coord)
	if before < 2 {
		t.Fatalf("用例前提：掩护点应离机场至少 2 格，实际 %d", before)
	}

	AdvanceGroundAir(&state)

	if got := Distance(cover.Coord, airfield.Coord); got >= before {
		t.Fatalf("被召回的掩护编队应该往机场飞：%d -> %d", before, got)
	}
}

// 天黑之后还在盘旋的 CAP／掩护／侦察必须掉头：它们没有"打完就回家"的终点，
// 不召回就会一直烧油到被删除。打击编队和转场不在此列。
func TestNightRecallsLoiteringCoverAndCap(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 60 // 19:40 之后
	state.Night = true

	carrier := firstFactionUnit(&state, "A", func(unit *Unit) bool { return IsCarrier(unit) })
	if carrier == nil {
		t.Fatal("找不到美军航母")
	}
	capUnit := &AirUnit{ID: "cap-night", CarrierID: carrier.ID, Faction: "A", Model: "F4F", Kind: "fighter",
		Status: "cap", Coord: carrier.Coord, PatrolCoord: carrier.Coord, TaskID: "cap-night-task", TaskKind: "cap"}
	state.Aircraft["cap-night"] = capUnit

	cover := state.GroundAir["af-a-moresby-p40-2"]
	if cover == nil {
		t.Fatal("找不到莫尔斯比的第二架 P-40")
	}
	airfield := airfieldFor(&state, cover.AirfieldID)
	cover.Status = "cover"
	cover.TaskKind = "cover"
	cover.TaskID = "cover-night"
	cover.Coord = "1811"
	cover.OriginCoord = airfield.Coord
	cover.TargetCoord = "1811"

	events := recallLoopingAircraftBeforeNight(&state)

	if !capUnit.Returning {
		t.Fatal("天黑后还在巡逻的 CAP 应该掉头")
	}
	if !cover.Returning {
		t.Fatal("天黑后还在掩护空域的陆基编队应该掉头")
	}
	if !hasEvent(events, "night_recall") {
		t.Fatalf("应当产生夜间召回事件：%+v", events)
	}
}

// 打击编队是玩家的赌注：天黑后不替玩家掉头（入夜前只警告一次）。
func TestNightDoesNotForceAttackFormationsHome(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 60
	state.Night = true
	carrier := firstFactionUnit(&state, "A", func(unit *Unit) bool { return IsCarrier(unit) })
	attack := &AirUnit{ID: "attack-night", CarrierID: carrier.ID, Faction: "A", Model: "SBD", Kind: "dive_bomber",
		Status: "attack", Coord: "2011", TargetCoord: "1611", TaskID: "strike-night", TaskKind: "attack"}
	state.Aircraft["attack-night"] = attack
	recallLoopingAircraftBeforeNight(&state)
	if attack.Returning {
		t.Fatal("打击编队不该被服务器强制掉头")
	}
}

func firstFactionUnit(state *State, faction string, match func(*Unit) bool) *Unit {
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit != nil && unit.Faction == faction && match(unit) {
			return unit
		}
	}
	return nil
}
