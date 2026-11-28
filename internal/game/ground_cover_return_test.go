package game

import "testing"

// TestGroundCoverReturnMovesWholeFormation 回归：被召回的多机空中掩护编队必须整队往回飞。
// 以前 cover 分支被 processedAttack 整队挡住，每回合只有排在第一的那架在动，
// 另外几架原地等着——傍晚召回 3 架、每架要 5 回合才到家，最后两架必然在天上过夜。
func TestGroundCoverReturnMovesWholeFormation(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	if airfield == nil {
		t.Fatal("找不到莫尔兹比港机场")
	}
	planes := []*AirfieldAirUnit{}
	for _, id := range []string{"af-a-moresby-p39-1", "af-a-moresby-p39-2", "af-a-moresby-p39-3"} {
		plane := state.GroundAir[id]
		if plane == nil {
			t.Fatalf("缺少 %s", id)
		}
		plane.Status = "cover"
		plane.TaskID = "cover-return"
		plane.TaskKind = "cover"
		plane.OriginCoord = airfield.Coord
		plane.TargetCoord = "2119"
		plane.CoverUnitID = ""
		plane.Returning = true
		plane.SetRange(12)
		plane.Coord = "2120"
		planes = append(planes, plane)
	}
	state.Night = false

	before := make([]int, len(planes))
	for index, plane := range planes {
		before[index] = Distance(plane.Coord, airfield.Coord)
	}
	AdvanceGroundAir(&state)
	for index, plane := range planes {
		if got := Distance(plane.Coord, airfield.Coord); got >= before[index] {
			t.Fatalf("第 %d 架被召回的掩护机没有往回飞：%d -> %d（同回合其他架次是否也在动？）", index+1, before[index], got)
		}
	}
}

// TestGroundCoverReturnLandsBeforeDark 回归：整队返航的回合数要按单架算，不能被队形拖长。
// 3 架 9 格外（2 格/回合）应当在 5 回合内全部落地。
func TestGroundCoverReturnLandsBeforeDark(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	if airfield == nil {
		t.Fatal("找不到莫尔兹比港机场")
	}
	planes := []*AirfieldAirUnit{}
	for _, id := range []string{"af-a-moresby-p39-1", "af-a-moresby-p39-2", "af-a-moresby-p39-3"} {
		plane := state.GroundAir[id]
		if plane == nil {
			t.Fatalf("缺少 %s", id)
		}
		plane.Status = "cover"
		plane.TaskID = "cover-return"
		plane.TaskKind = "cover"
		plane.OriginCoord = airfield.Coord
		plane.TargetCoord = "2119"
		plane.CoverUnitID = ""
		plane.Returning = true
		plane.SetRange(12)
		plane.Coord = "2120"
		planes = append(planes, plane)
	}
	state.Night = false
	turns := 0
	for ; turns < 8; turns++ {
		AdvanceGroundAir(&state)
		allHome := true
		for _, plane := range planes {
			if airfieldAirborne(plane) {
				allHome = false
				break
			}
		}
		if allHome {
			break
		}
	}
	// 距离 9 格、每回合 2 格：5 回合足够；以前每回合只飞一架，要 15 回合。
	if turns > 5 {
		t.Fatalf("3 架掩护队返航用了 %d 回合，应当 ≤5（2 格/回合）", turns)
	}
}

// TestGroundHoldingAttackReturnsWhenRecalled 被召回的等待编队不能继续原地烧油。
func TestGroundHoldingAttackReturnsWhenRecalled(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	if airfield == nil {
		t.Fatal("找不到莫尔兹比港机场")
	}
	plane := state.GroundAir["af-a-moresby-b25-1"]
	if plane == nil {
		t.Fatal("找不到 B-25")
	}
	plane.Status = "attack_holding"
	plane.TaskID = "holding-return"
	plane.TaskKind = "attack"
	plane.Coord = "1813"
	plane.OriginCoord = airfield.Coord
	plane.TargetCoord = "1418"
	plane.Returning = true
	plane.SetRange(10)
	state.Night = false
	before := Distance(plane.Coord, airfield.Coord)
	AdvanceGroundAir(&state)
	if got := Distance(plane.Coord, airfield.Coord); got >= before {
		t.Fatalf("被召回的等待编队应该往回飞：%d -> %d", before, got)
	}
}
