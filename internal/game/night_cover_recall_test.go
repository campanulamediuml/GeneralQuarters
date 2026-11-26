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

// 返航是按"距离 + 油量"算的，不是"天黑就统一掉头"：
// 入夜后召回规划不再运行（那一刻已经没有"赶不赶得上"可算），还挂在空中的 CAP/掩护
// 由各自的油量判定决定什么时候撤——航程远的长腿飞机熬到天亮再落地，摔毁率是 0。
func TestNightDoesNotForceLoiteringAircraftHome(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 60 // 19:40 之后
	state.Night = true

	carrier := firstFactionUnit(&state, "A", func(unit *Unit) bool { return IsCarrier(unit) })
	if carrier == nil {
		t.Fatal("找不到美军航母")
	}
	capUnit := &AirUnit{ID: "cap-night", CarrierID: carrier.ID, Faction: "A", Model: "F4F", Kind: "fighter",
		Status: "cap", Coord: carrier.Coord, PatrolCoord: carrier.Coord, TaskID: "cap-night-task", TaskKind: "cap"}
	capUnit.SetRange(9)
	state.Aircraft["cap-night"] = capUnit

	events := recallLoopingAircraftBeforeNight(&state)

	if capUnit.Returning || hasEvent(events, "night_recall") {
		t.Fatalf("入夜后不该凭天色强行召回（要不要现在落地该由油量决定）：%+v", events)
	}
	// 油量见底时它仍然会自己撤：CAP 每回合烧 1 点，烧到只够回程那一点就该掉头。
	capUnit.SetRange(0)
	AdvanceAviation(&state)
	if !capUnit.Returning {
		t.Fatal("航程只够回家时 CAP 应该自己返航")
	}
}

// 目视范围内（2 hex）的敌方舰队，下发的 contact_level 必须是满级：
// 以前这里抄的是真实算子的 0——接口一边说"没有情报"，一边把完整舰种、强度、卡面全发出去，
// 观战页和外部脚本都只能按"有舰名就当满级"去猜。
func TestVisualRangeFleetsReportFullContactLevel(t *testing.T) {
	state := coralCombatState(t)
	enemy := firstFactionUnit(&state, "B", func(unit *Unit) bool { return unit.Type == "CV" })
	friendly := firstFactionUnit(&state, "A", func(unit *Unit) bool { return unit.Type == "CV" })
	if enemy == nil || friendly == nil {
		t.Fatal("剧本里应当有双方航母")
	}
	enemy.Coord = friendly.Coord
	// 只留目视这一条路径：情报表清空，投影不许借侦察接触的等级。
	state.Contacts = map[string]map[string]*FleetContact{}
	for _, fleet := range VisibleFleets(state, "A") {
		if fleet.Faction != "B" {
			continue
		}
		if fleet.ContactLevel != 4 {
			t.Fatalf("目视到的敌方舰队情报等级应为 4，实际 %d", fleet.ContactLevel)
		}
		return
	}
	t.Fatal("目视范围内的敌方舰队应当出现在视图里")
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
