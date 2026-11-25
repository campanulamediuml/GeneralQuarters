package game

import "testing"

// TestGroundStrikeSearchesEmptyTarget 机场打击编队抵达空 hex 之后必须继续搜索，
// 不能原地僵住：以前既不判定接敌也不返航，油耗也不再变化，
// 一队陆攻会在空中停几十个回合。
func TestGroundStrikeSearchesEmptyTarget(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	const id = "af-b-rabaul-g4m-1"
	plane := state.GroundAir[id]
	if plane == nil {
		t.Fatalf("缺少算子 %s", id)
	}
	empty := "0920"
	plane.Status = "attack"
	plane.TaskID = "ground-strike-search"
	plane.TaskKind = "attack"
	plane.Coord = empty
	plane.TargetCoord = empty
	plane.OriginCoord = "0428"
	plane.Heading = "E"
	plane.SetRange(30)
	plane.Returning = false

	start := plane.Coord
	for turn := 0; turn < 3; turn++ {
		advanceGroundAttack(&state, plane)
	}
	if plane.Coord == start && !plane.Returning {
		t.Fatalf("抵达空 hex 后编队原地不动也没有返航：coord=%s range=%d", plane.Coord, plane.Range())
	}
	if plane.Range() >= 30 && !plane.Returning {
		t.Fatalf("搜索应当消耗航程：%d", plane.Range())
	}
}
