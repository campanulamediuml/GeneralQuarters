package game

import (
	"fmt"
	"testing"
)

func coordAtDistance(t *testing.T, state State, from string, want int) string {
	t.Helper()
	for row := 1; row <= state.Map.Rows; row++ {
		for col := 1; col <= state.Map.Cols; col++ {
			coord := fmt.Sprintf("%02d%02d", row, col)
			if Distance(from, coord) == want {
				return coord
			}
		}
	}
	t.Fatalf("地图里找不到距离 %s 恰好 %d hex 的格子", from, want)
	return ""
}

// 舰载机转场：手动挂载要让位给副油箱，目的地航程只在还没起飞时查一次。
func TestCarrierTransferRangeCheckedAtTakeoff(t *testing.T) {
	state := sandboxState(t)
	// 换一张更大的空地图，方便凑出"刚好够"和"差两格"的目的地。
	state.Map.Rows, state.Map.Cols = 24, 24
	carrier := state.Units["a2-cv-01"]
	plane := state.Aircraft["a2-cv-01-fighter-x3"]
	plane.Status = "hangar"
	plane.Launched = false
	plane.LoadoutManual = true
	applyLoadoutChoice(plane, LoadoutAirTank)

	create := Order{Type: "LAUNCH_TRANSFER", AircraftID: plane.ID}
	if err := ValidateAirOrder(state, "A", create); err != nil {
		t.Fatalf("建立转场任务失败：%v", err)
	}
	ApplyAirOrder(&state, create)

	// 手动挂载的飞机进转场必须换成副油箱：否则校验按 1.69× 放行、实际只有 1.3×。
	if plane.Loadout != LoadoutTransfer {
		t.Fatalf("转场任务没有换成副油箱：loadout=%s", plane.Loadout)
	}
	if plane.LoadoutManual {
		t.Fatal("转场卸掉武器后，手动挂载标记没有清掉")
	}
	limit := SortieBaseRange(plane.BaseRange, LoadoutTransfer)
	if plane.MaxRange != limit {
		t.Fatalf("转场满油航程 = %d，应为 %d", plane.MaxRange, limit)
	}

	near := coordAtDistance(t, state, carrier.Coord, limit-1)
	far := coordAtDistance(t, state, carrier.Coord, limit+1)
	target := Order{Type: "SET_STRIKE_TARGET", AircraftID: plane.TaskID, Coord: far}
	if err := ValidateAirOrder(state, "A", target); err == nil {
		t.Fatal("超出转场航程的目的地应当被拒绝")
	}
	target.Coord = near
	if err := ValidateAirOrder(state, "A", target); err != nil {
		t.Fatalf("航程内的目的地被拒绝：%v", err)
	}

	// 已经升空的转场不再复检：目标舰队跑远了照样按原目标飞。
	plane.Status = "transfer"
	plane.Launched = true
	target.Coord = far
	if err := ValidateAirOrder(state, "A", target); err != nil {
		t.Fatalf("空中的转场改目标不该再查航程：%v", err)
	}
}
