package game

import "testing"

// 编队里有一架燃油不足掉头返航，剩下的飞机必须照常出发：
// 实测 12 架 B-17 因为 4 架 B-26 掉头，在莱城上空"等待编队"烧了十几回合油。
func TestAttackDepartsWithReturningMember(t *testing.T) {
	state := sandboxState(t)
	ids := []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}
	taskID := "strike-returning"
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.Status = "attack_holding"
		aircraft.Launched = true
		aircraft.TargetCoord = "0202"
		aircraft.TaskID = taskID
		aircraft.TaskKind = "attack"
	}
	// 一号机剩油只够回家：它自己返航，二号机继续出击。
	state.Aircraft[ids[0]].Returning = true
	events := departReadyAttacks(&state)
	departed := false
	for _, event := range events {
		if event.Type == "attack_departed" {
			departed = true
		}
	}
	if !departed {
		t.Fatal("编队里有人返航时，其余飞机应该照常出发")
	}
	if state.Aircraft[ids[1]].Status != "attack" {
		t.Fatalf("没返航的飞机应该进入出击状态：%s", state.Aircraft[ids[1]].Status)
	}
	if !state.Aircraft[ids[0]].Returning {
		t.Fatal("返航的飞机不该被拉回编队")
	}
	if state.Aircraft[ids[0]].Status == "attack" {
		t.Fatal("返航中的飞机不该被改成出击状态")
	}
}

// 机场编队同样不能被返航成员卡住：剩下的飞机要继续出击。
func TestGroundAttackDepartsWithReturningMember(t *testing.T) {
	state := coralCombatState(t)
	airfield := state.Airfields["af-b-rabaul"]
	if airfield == nil {
		t.Fatal("找不到拉包尔机场")
	}
	taskID := "af-strike-split"
	members := []string{"af-b-rabaul-g4m-1", "af-b-rabaul-g4m-2"}
	for _, id := range members {
		plane := state.GroundAir[id]
		if plane == nil {
			t.Fatalf("找不到 %s", id)
		}
		plane.TaskID = taskID
		plane.TaskKind = "attack"
		plane.Status = "attack_holding"
		plane.Coord = airfield.Coord
		plane.TargetCoord = "1611"
	}
	state.GroundAir[members[0]].Returning = true
	if !groundTaskReadyToDepart(state, taskID) {
		t.Fatal("有成员返航时，机场编队应该仍然可以离场")
	}
	AdvanceGroundAir(&state)
	if got := state.GroundAir[members[1]].Status; got != "attack" {
		t.Fatalf("没返航的机场算子应该进入出击状态：%s", got)
	}
	if state.GroundAir[members[1]].Returning {
		t.Fatal("没返航的机场算子不该被标成返航")
	}
}

// 混在编队里的 attack_holding 成员不能被出击的队友拖着飞到目标上空：
// 以前它们会被 advanceGroundAttack 一起推进，停在目标上却还是"等待编队"。
func TestHoldingMemberIsNotDraggedToTarget(t *testing.T) {
	state := coralCombatState(t)
	airfield := state.Airfields["af-b-rabaul"]
	taskID := "af-strike-drag"
	lead := state.GroundAir["af-b-rabaul-g4m-1"]
	hold := state.GroundAir["af-b-rabaul-g4m-2"]
	for _, plane := range []*AirfieldAirUnit{lead, hold} {
		plane.TaskID = taskID
		plane.TaskKind = "attack"
		plane.TargetCoord = "1611"
	}
	lead.Status = "attack"
	lead.Coord = "0224"
	hold.Status = "attack_holding"
	hold.Coord = airfield.Coord
	before := hold.Coord

	AdvanceGroundAir(&state)

	if got := state.GroundAir["af-b-rabaul-g4m-2"].Coord; got != before {
		t.Fatalf("等待编队的成员不该被拖走：%s -> %s", before, got)
	}
}
