package game

import (
	"strings"
	"testing"
)

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

// 返航中的成员要从打击编队里拆出去：编队卡只列还在执行任务的飞机。
func TestReturningMembersSplitOutOfStrikeTask(t *testing.T) {
	state := sandboxState(t)
	ids := []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.TaskID = "strike-split-out"
		aircraft.TaskKind = "attack"
		aircraft.Status = "attack"
		aircraft.Launched = true
		aircraft.TargetCoord = "0202"
	}
	state.Aircraft[ids[0]].Returning = true
	splitCarrierReturningMembers(&state)
	if got := state.Aircraft[ids[0]].TaskID; got == "strike-split-out" {
		t.Fatal("返航成员应单独成组，实际还挂在原任务上")
	}
	if got := state.Aircraft[ids[1]].TaskID; got != "strike-split-out" {
		t.Fatalf("还在任务的成员不该改任务号，实际 %s", got)
	}
	// 全员返航时整组就是返航编队，不用再拆。
	for _, id := range ids {
		state.Aircraft[id].Returning = true
	}
	splitCarrierReturningMembers(&state)
	if got := state.Aircraft[ids[1]].TaskID; got != "strike-split-out" {
		t.Fatalf("全员返航时不该拆组，实际 %s", got)
	}
}

// 机场侧同样：返航的机场算子不再挂在打击编队里。
func TestGroundReturningMembersSplitOutOfStrikeTask(t *testing.T) {
	state := coralCombatState(t)
	members := []string{"af-b-rabaul-g4m-1", "af-b-rabaul-g4m-2"}
	for _, id := range members {
		plane := state.GroundAir[id]
		plane.TaskID = "af-strike-split"
		plane.TaskKind = "attack"
		plane.Status = "attack"
		plane.TargetCoord = "1611"
	}
	state.GroundAir[members[0]].Returning = true
	splitGroundReturningMembers(&state)
	if got := state.GroundAir[members[0]].TaskID; got == "af-strike-split" {
		t.Fatal("返航的机场算子应单独成组，实际还挂在原任务上")
	}
	if got := state.GroundAir[members[1]].TaskID; got != "af-strike-split" {
		t.Fatalf("还在任务的机场算子不该改任务号，实际 %s", got)
	}
}

// 无线电改令不能把已经返航的飞机重新塞回打击编队。
func TestRadioRetaskSkipsReturningMembers(t *testing.T) {
	state := sandboxState(t)
	ids := []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.TaskID = "strike-radio"
		aircraft.TaskKind = "cap"
		aircraft.Status = "cap"
	}
	state.Aircraft[ids[0]].Returning = true
	order := Order{Type: "RETASK_AIR", TaskID: "strike-radio", TaskKind: "attack", Coord: "0202"}
	retaskRadio(&state, "B", order, []*AirUnit{state.Aircraft[ids[0]], state.Aircraft[ids[1]]}, nil, 0, 100)
	if !state.Aircraft[ids[0]].Returning {
		t.Fatal("返航中的成员不该被改令拉回来")
	}
	if state.Aircraft[ids[0]].TaskKind != "cap" {
		t.Fatalf("返航中的成员不该换任务类型：%s", state.Aircraft[ids[0]].TaskKind)
	}
	if state.Aircraft[ids[1]].TaskKind != "attack" {
		t.Fatalf("没返航的成员应该按改令转成打击：%s", state.Aircraft[ids[1]].TaskKind)
	}
}

// 任务号由只增不减的序号生成：同一批飞机第二次出击不会撞上一次的任务号。
func TestTaskIDsAreUniqueAcrossSorties(t *testing.T) {
	state := sandboxState(t)
	ids := []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}
	first := ""
	for round := 0; round < 2; round++ {
		for _, id := range ids {
			aircraft := state.Aircraft[id]
			aircraft.Status = "deck"
			aircraft.Launched = false
			aircraft.TaskID = ""
			aircraft.TaskKind = ""
		}
		ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids, Coord: "0202"})
		taskID := state.Aircraft[ids[0]].TaskID
		if taskID == "" {
			t.Fatalf("第 %d 轮没有生成任务号", round+1)
		}
		if round == 0 {
			first = taskID
			continue
		}
		if taskID == first {
			t.Fatalf("第二次出击复用了同一个任务号 %s", taskID)
		}
	}
}

// 落地即"不返航"：以前落地保留 Returning=true，整备到 ready 还挂着，
// 编队卡把地面整备的飞机混进返航队，设目标时又被"返航中不改目标"挡掉，永远打不出去。
func TestGroundAircraftClearsReturningWhenGrounded(t *testing.T) {
	state := coralCombatState(t)
	airfield := state.Airfields["af-b-rabaul"]
	plane := state.GroundAir["af-b-rabaul-g4m-1"]
	if plane == nil || airfield == nil {
		t.Fatal("缺少拉包尔机场或 G4M")
	}
	plane.Status = "ready"
	plane.Coord = airfield.Coord
	plane.Returning = true
	plane.TaskID = ""
	AdvanceGroundAir(&state)
	if plane.Returning {
		t.Fatal("地面上的算子不该还挂着返航中")
	}

	// 新任务的成员同样从"没在返航"开始。
	plane.Returning = true
	beginGroundMission(plane, "task-new", MissionStrike, 1)
	if plane.Returning {
		t.Fatal("新编队的成员不该继承上一轮的返航标记")
	}
}

// 返航的那一波拆出去之后，停在机场整备的同任务飞机留在原组：
// 编队卡不会再出现"整备完毕 + 返航中"混在一张卡里的情况。
func TestSplitKeepsGroundedMembersInOriginalTask(t *testing.T) {
	state := coralCombatState(t)
	taskID := "af-strike-mixed"
	airborne := state.GroundAir["af-b-rabaul-g4m-1"]
	grounded := state.GroundAir["af-b-rabaul-g4m-2"]
	for _, plane := range []*AirfieldAirUnit{airborne, grounded} {
		plane.TaskID = taskID
		plane.TaskKind = "attack"
		plane.TargetCoord = "1611"
	}
	airborne.Status = "attack"
	airborne.Returning = true
	airborne.Coord = "0910"
	grounded.Status = "ready"
	grounded.Returning = false
	grounded.Coord = state.Airfields["af-b-rabaul"].Coord

	splitGroundReturningMembers(&state)

	if airborne.TaskID == taskID {
		t.Fatal("返航中的空中算子应拆到新任务组")
	}
	if grounded.TaskID != taskID {
		t.Fatalf("停在机场整备的算子应留在原任务组，实际 %s", grounded.TaskID)
	}
}

// 集结逻辑：已经在出击的成员不会把还在等待的成员一起推出发。
// 分批出击（谁升空谁直接走）是另一套逻辑，两套混用时会出现"部分飞机单独飞向目标"。
func TestAttackFormationDoesNotDepartPartially(t *testing.T) {
	state := sandboxState(t)
	ids := []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}
	taskID := "task-partial"
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.Status = "attack_holding"
		aircraft.Launched = true
		aircraft.TargetCoord = "0202"
		aircraft.TaskID = taskID
		aircraft.TaskKind = "attack"
	}
	// 一号机已经出发，二号机还在母舰上空等。
	state.Aircraft[ids[0]].Status = "attack"
	updateAttackFormation(&state, taskID)
	if got := state.Aircraft[ids[1]].Status; got != "attack_holding" {
		t.Fatalf("等待中的成员不该被推出发（分批出击）：%s", got)
	}
	if got := state.Aircraft[ids[0]].Status; got != "attack" {
		t.Fatalf("已出发的成员不该被改回来：%s", got)
	}
}

// 机场编队同样：只有全员 attack_holding（升空等待）才算集结完毕。
func TestGroundFormationAssembleRequiresAllHolding(t *testing.T) {
	state := coralCombatState(t)
	taskID := "af-assemble"
	members := []string{"af-b-rabaul-g4m-1", "af-b-rabaul-g4m-2"}
	for _, id := range members {
		plane := state.GroundAir[id]
		plane.TaskID = taskID
		plane.TaskKind = "attack"
		plane.Coord = state.Airfields["af-b-rabaul"].Coord
		plane.TargetCoord = "1611"
	}
	state.GroundAir[members[0]].Status = "attack"
	state.GroundAir[members[1]].Status = "attack_holding"
	if groundTaskReadyToDepart(state, taskID) {
		t.Fatal("有成员已经出击时，剩下的不该判成集结完毕")
	}
	AdvanceGroundAir(&state)
	if got := state.GroundAir[members[1]].Status; got != "attack_holding" {
		t.Fatalf("等待中的机场算子不该被推出发：%s", got)
	}
}

// 打击编队出发后不能再加入飞机：要么让它单独飞过去（分批出击），要么永远等在地面。
func TestGroundStrikeJoinRejectedAfterDeparture(t *testing.T) {
	state := coralCombatState(t)
	// 校验第一步先看整备窗口：把时钟拨到白天，才能测到"编队已离场"这条。
	state.Turn = 1
	state.DayHours = 12
	state.Night = false
	taskID := "af-departed"
	flying := state.GroundAir["af-b-rabaul-g4m-1"]
	flying.TaskID = taskID
	flying.TaskKind = "attack"
	flying.Status = "attack"
	flying.Coord = "0910"
	flying.TargetCoord = "1611"

	extra := state.GroundAir["af-b-rabaul-g4m-2"]
	extra.Status = "ready"
	extra.TaskID = ""
	extra.TaskKind = ""

	order := Order{Type: "LAUNCH_STRIKE", AircraftID: extra.ID, TaskID: taskID}
	err := ValidateGroundAirOrder(state, order)
	if err == nil {
		t.Fatal("编队已出击时应该拒绝中途加入")
	}
	if !strings.Contains(err.Error(), "已经离场") {
		t.Fatalf("拒绝原因应说明编队已离场：%v", err)
	}
}

// 等集结的飞机也会烧油：剩的航程只够回家就自己脱队返航，不能烧到油尽坠毁。
func TestHoldingAttackersDetachWhenFuelLow(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-01-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	aircraft.Status = "attack_holding"
	aircraft.Launched = true
	aircraft.TaskID = "task-fuel"
	aircraft.TaskKind = "attack"
	aircraft.TargetCoord = "0202"
	aircraft.Coord = Neighbors(carrier.Coord)[0]
	aircraft.SetRange(1)
	events := AdvanceAviation(&state)
	if !aircraft.Returning {
		t.Fatalf("剩油只够回家时应脱队返航：%+v", aircraft)
	}
	found := false
	for _, event := range events {
		if event.Type == "attack_rtb" && strings.Contains(event.Text, "脱离攻击编队") {
			found = true
		}
	}
	if !found {
		t.Fatal("脱队返航应有战报")
	}
}
