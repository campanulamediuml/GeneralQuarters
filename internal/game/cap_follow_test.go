package game

import "testing"

// CAP 必须跟着母舰走：母舰移动后，CAP 算子的 Coord / PatrolCoord 要同步到母舰当前位置。
// 以前只更新了判定用的 capCoord()，算子自己的坐标停在放飞那一格，地图上看起来没跟舰队。
func TestCAPFollowsCarrierPosition(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	carrier.Coord = "0303"
	cap := state.Aircraft["b-cv-01-fighter-x1"]
	cap.Status = "cap"
	cap.TaskKind = "cap"
	cap.TaskID = "cap-MO"
	cap.Coord = "0303"
	cap.PatrolCoord = "0303"
	cap.SetRange(20)

	// 母舰开走两格，CAP 应该立刻跟过去。
	carrier.Coord = "0505"
	AdvanceAviation(&state)
	if cap.Coord != carrier.Coord || cap.PatrolCoord != carrier.Coord {
		t.Fatalf("CAP 未跟随母舰：coord=%s patrol=%s carrier=%s", cap.Coord, cap.PatrolCoord, carrier.Coord)
	}
}
