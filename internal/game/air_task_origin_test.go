package game

import "testing"

// 任务卡要能回答"这队飞机从哪儿起飞、要去哪儿"。
// 以前只有侦察和攻击记 OriginCoord，CAP/掩护/转场的任务卡只给一个当前坐标，
// 玩家看到的就是"坐标 = 母舰"，分不清出发点。
func TestAirTaskReportsLaunchOrigin(t *testing.T) {
	state := coralCombatState(t)

	// 机场侧掩护：起飞顶点就是机场坐标。
	cover := state.GroundAir["af-a-moresby-p40-1"]
	if cover == nil {
		t.Fatal("找不到莫尔斯比的 P-40")
	}
	airfield := airfieldFor(&state, cover.AirfieldID)
	if airfield == nil {
		t.Fatal("找不到所属机场")
	}
	cover.Status = "cover"
	cover.TaskID = "cover-origin"
	cover.TaskKind = "cover"
	cover.Coord = "1811"
	cover.OriginCoord = airfield.Coord
	cover.TargetCoord = "1811"

	found := false
	for _, task := range GroundAviationView(state, "A").Tasks {
		if task.ID != "cover-origin" {
			continue
		}
		found = true
		if task.OriginCoord != airfield.Coord {
			t.Fatalf("机场掩护任务的起飞位置应为 %s，实际 %q", airfield.Coord, task.OriginCoord)
		}
		if task.TargetCoord != "1811" {
			t.Fatalf("机场掩护任务的目标应为 1811，实际 %q", task.TargetCoord)
		}
	}
	if !found {
		t.Fatal("机场掩护任务没有出现在航空视图里")
	}

	// 舰载侧转场：同样是"从哪条船起飞"。
	carrier := firstFactionUnit(&state, "A", func(unit *Unit) bool { return IsCarrier(unit) })
	if carrier == nil {
		t.Fatal("找不到美军航母")
	}
	plane := &AirUnit{ID: "transfer-origin", CarrierID: carrier.ID, Faction: "A", Model: "SBD", Kind: "dive_bomber",
		Status: "transfer", Coord: carrier.Coord, OriginCoord: carrier.Coord, TargetCoord: "1611",
		TaskID: "transfer-origin-task", TaskKind: "transfer"}
	state.Aircraft[plane.ID] = plane

	for _, task := range VisibleAviation(state, "A").Tasks {
		if task.ID != "transfer-origin-task" {
			continue
		}
		if task.OriginCoord != carrier.Coord {
			t.Fatalf("舰载转场任务的起飞位置应为 %s，实际 %q", carrier.Coord, task.OriginCoord)
		}
		return
	}
	t.Fatal("舰载转场任务没有出现在航空视图里")
}
