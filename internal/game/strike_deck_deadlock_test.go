package game

import "testing"

// TestStrikeWithoutTargetKeepsDeckClear 回归：没有目标的攻击编队不该被提升到甲板。
// 以前它们照样占升降机、占满甲板又不放飞，回收被堵死，整条航空作业链卡在那里，
// 玩家只能看着飞机堆在甲板上什么也做不了。
func TestStrikeWithoutTargetKeepsDeckClear(t *testing.T) {
	state := sandboxState(t)
	freezeFleets(&state)
	carrier := state.Units["b-cv-01"]
	carrier.Deck.Operation = ""

	strike := []*AirUnit{}
	for _, id := range []string{
		"b-cv-01-dive_bomber-x1", "b-cv-01-dive_bomber-x2", "b-cv-01-dive_bomber-x3",
		"b-cv-01-dive_bomber-x4", "b-cv-01-dive_bomber-x5", "b-cv-01-dive_bomber-x6",
	} {
		plane := state.Aircraft[id]
		if plane == nil {
			t.Fatalf("缺少算子 %s", id)
		}
		plane.Status = "service"
		plane.TaskID = "strike-no-target"
		plane.TaskKind = "attack"
		plane.TargetCoord = ""
		plane.Coord = carrier.Coord
		plane.ServiceTurns = 0
		plane.Maintenance = 0
		plane.Returning = false
		plane.SetRange(12)
		strike = append(strike, plane)
	}

	for turn := 0; turn < 8; turn++ {
		AdvanceAviation(&state)
		for _, plane := range strike {
			switch plane.Status {
			case "deck", "elevator", "attack_holding", "attack":
				t.Fatalf("第 %d 回合无目标编队升到了甲板：%s status=%s", turn, plane.ID, plane.Status)
			}
		}
		if used := deckUsed(state, carrier.ID); used != 0 {
			t.Fatalf("第 %d 回合甲板被无目标编队占用：%d", turn, used)
		}
	}

	// 一旦补上目标，同一批飞机应该能正常整备、提升、放飞。
	for _, plane := range strike {
		plane.TargetCoord = "0402"
	}
	launched := false
	for turn := 0; turn < 10; turn++ {
		for _, event := range AdvanceAviation(&state) {
			if event.Type == "air_launched" {
				launched = true
			}
		}
		if launched {
			break
		}
	}
	if !launched {
		seen := []string{}
		for _, plane := range strike {
			seen = append(seen, plane.ID+":"+plane.Status)
		}
		t.Fatalf("补上目标之后编队仍然没能起飞：%v", seen)
	}
}
