package game

import "testing"

// TestCAPReturnsHomeWhenFuelRunsLow 回归：CAP 油量不够往返时必须自动返航。
// 之前 cap 分支无条件 Spend(1)，飞机一路烧到航程 -5 才被移除，事件只写"燃油耗尽被摧毁"。
func TestCAPReturnsHomeWhenFuelRunsLow(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-01-fighter-x1"]
	mother := state.Units[aircraft.CarrierID]
	aircraft.Status = "cap"
	aircraft.Launched = true
	aircraft.TaskID = "cap-test"
	aircraft.TaskKind = "cap"
	aircraft.Coord = mother.Coord
	aircraft.OriginCoord = mother.Coord
	aircraft.SetRange(rtbDistance(state, aircraft))

	AdvanceAviation(&state)

	if !aircraft.Returning {
		t.Fatalf("CAP 剩余航程刚好等于返航距离时应自动返航：%+v", aircraft)
	}
}

// TestCAPKeepsPatrollingWithFuel 油量充足时 CAP 不应提前返航。
func TestCAPKeepsPatrollingWithFuel(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-01-fighter-x1"]
	mother := state.Units[aircraft.CarrierID]
	aircraft.Status = "cap"
	aircraft.Launched = true
	aircraft.TaskID = "cap-test"
	aircraft.TaskKind = "cap"
	aircraft.Coord = mother.Coord
	aircraft.OriginCoord = mother.Coord
	aircraft.SetRange(rtbDistance(state, aircraft) + 5)

	AdvanceAviation(&state)

	if aircraft.Returning {
		t.Fatalf("油量充足时 CAP 应继续巡逻：%+v", aircraft)
	}
}
