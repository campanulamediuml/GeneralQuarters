package game

import "testing"

// 端到端：下午放出去的打击编队如果算下来回不来，必须自己掉头，
// 不能等 19:40 直接"天黑未能返航"整队报销。
func TestLateStrikeGroupReturnsBeforeNightfall(t *testing.T) {
	state := sandboxState(t)
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"}
	state.TurnsPerDay = 72
	state.DayHours = 0
	// 沙盒地图 7×9：坐标必须落在图内，否则"飞不回去"是地图的错。
	carrier := state.Units["b-cv-01"]
	carrier.Coord = "0303"
	carrier.FleetID = "MO"

	var strikers []*AirUnit
	for index := 1; index <= 3; index++ {
		id := "b-cv-01-dive_bomber-x" + string(rune('0'+index))
		plane := state.Aircraft[id]
		if plane == nil {
			t.Fatalf("缺少舰爆算子 %s", id)
		}
		plane.Status = "attack"
		plane.TaskKind = "attack"
		plane.TaskID = "strike-1"
		plane.Coord = "0506"
		plane.OriginCoord = carrier.Coord
		plane.TargetCoord = "0708"
		plane.Returning = false
		plane.SetRange(20)
		strikers = append(strikers, plane)
	}

	state.Turn = 50 // 16:20
	for turn := 50; turn <= 62; turn++ {
		state.Turn = turn
		for _, event := range ResolveOrders(&state, nil) {
			if event.Type == "air_destroyed" {
				t.Fatalf("第 %d 回合出现航空损失：%s", turn, event.Text)
			}
		}
	}
	for _, striker := range strikers {
		// 降不下来的编队会备降到机场（从 Aircraft 挪进 GroundAir），那也算安全落地。
		if grounded := state.GroundAir[striker.ID]; grounded != nil {
			continue
		}
		live := state.Aircraft[striker.ID]
		if live == nil {
			t.Fatalf("%s 被损失了", striker.ID)
		}
		if airborne(live) {
			t.Fatalf("%s 天黑时仍在天上", striker.ID)
		}
	}
}
