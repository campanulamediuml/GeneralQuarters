package game

import "testing"

// 回归：被召回的"等待编队"（attack_holding）必须真的往母舰飞，
// 而不是原地扣油——以前它会一直悬着，直到燃油耗尽才消失，看起来就是"卡在空中"。
func TestHoldingAircraftReturnsWhenRecalled(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	carrier.Coord = "0303"
	plane := state.Aircraft["b-cv-01-dive_bomber-x1"]
	plane.Status = "attack_holding"
	plane.TaskKind = "attack"
	plane.TaskID = "strike-x"
	plane.Coord = "0506"
	plane.Returning = true
	plane.SetRange(20)
	startDistance := Distance(plane.Coord, carrier.Coord)

	AdvanceAviation(&state)
	if plane.Status == "attack_holding" && Distance(plane.Coord, carrier.Coord) >= startDistance {
		t.Fatalf("被召回的等待编队没有往回飞：coord=%s 距离 %d", plane.Coord, Distance(plane.Coord, carrier.Coord))
	}
	for turn := 0; turn < 8 && airborne(plane); turn++ {
		AdvanceAviation(&state)
	}
	if airborne(plane) {
		t.Fatalf("等待编队应该已经落舰：status=%s coord=%s", plane.Status, plane.Coord)
	}
}
