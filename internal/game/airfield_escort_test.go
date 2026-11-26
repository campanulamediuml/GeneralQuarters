package game

import "testing"

// TestGroundStrikeAcceptsEscortFighter 回归：给已有攻击编队补一架战斗机是"护航"，
// 不是"用一架战斗机建新攻击编队"。以前机型校验不看 task_id，玩家在机场上点
// "加入攻击编队"选一架 P-40 就会被 400 拒绝（攻击编队只能由轰炸机和护航战斗机组成）。
func TestGroundStrikeAcceptsEscortFighter(t *testing.T) {
	state := coralCombatState(t)
	bomber := state.GroundAir["af-a-moresby-b25-1"]
	fighter := state.GroundAir["af-a-moresby-p40-1"]
	if bomber == nil || fighter == nil {
		t.Fatal("莫尔兹比缺少 B-25 或 P-40")
	}
	for _, plane := range []*AirfieldAirUnit{bomber, fighter} {
		plane.Status = "hangar"
		plane.TaskID = ""
		plane.TaskKind = ""
		plane.Damaged = false
		plane.Maintenance = 0
	}
	// 轰炸机已经在攻击编队里（界面上的顺序：先建编队，再把护航拉进来）。
	bomber.TaskID = bomber.ID
	bomber.TaskKind = "attack"
	state.Night = false
	state.Turn = 30 // 白天整备窗口

	join := Order{Type: "LAUNCH_STRIKE", AircraftID: fighter.ID, TaskID: bomber.TaskID}
	if err := ValidateOrder(state, "A", join); err != nil {
		t.Fatalf("战斗机应能加入已有攻击编队当护航：%v", err)
	}
}

// TestGroundStrikeRejectsLoneFighter 单架战斗机（编队里没有轰炸机）仍然不能自己建攻击编队。
// 这条和上面的回归项是一对：放宽"加入已有编队"不能顺手把"单架战斗机攻击"也放过去。
func TestGroundStrikeRejectsLoneFighter(t *testing.T) {
	state := coralCombatState(t)
	fighter := state.GroundAir["af-a-moresby-p40-1"]
	if fighter == nil {
		t.Fatal("莫尔兹比缺少 P-40")
	}
	fighter.Status = "hangar"
	fighter.TaskID = ""
	fighter.TaskKind = ""
	fighter.Damaged = false
	fighter.Maintenance = 0
	state.Night = false
	state.Turn = 30
	order := Order{Type: "LAUNCH_STRIKE", AircraftID: fighter.ID}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("单架战斗机不该能组建攻击编队")
	}
}
