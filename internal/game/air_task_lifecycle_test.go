package game

import (
	"fmt"
	"testing"
)

// TestLandedAircraftCanBeRetasked 回归：任务完成降回机库后，算子必须能再次被分配任务。
//
// LandAircraft 为了保住任务分组只清任务参数、保留 TaskID，而 TaskKind 已经被清空；
// 分组关闭逻辑只认 TaskKind，于是一架打完一轮的飞机会永久卡在"已经属于其他任务"，
// 而 RETURN_TO_HANGAR 又不接受 hangar 状态，玩家没有任何办法把它重新拖进任务栏。
func TestLandedAircraftCanBeRetasked(t *testing.T) {
	state := sandboxState(t)
	id := "b-cv-01-dive_bomber-x1"
	aircraft := state.Aircraft[id]
	aircraft.Status = "attack"
	aircraft.TaskID = "strike-test"
	aircraft.TaskKind = "attack"
	aircraft.TargetCoord = "0505"

	carrier := state.Units["b-cv-01"]
	LandAircraft(&state, aircraft, carrier)
	if aircraft.TaskID != "strike-test" {
		t.Fatalf("着舰应保留 TaskID 以维持分组：%+v", aircraft)
	}
	// 升降机下降完成后的状态。
	aircraft.Status = "hangar"
	AdvanceAviation(&state)

	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: id, Heading: "W", SectorWidth: 60}); err != nil {
		t.Fatalf("降回机库的算子应能再次分配任务，实际被拒：%v（TaskID=%q TaskKind=%q）", err, aircraft.TaskID, aircraft.TaskKind)
	}
}

// TestEngagedStrikeDoesNotFreezeOnEmptyTarget 回归：接敌成功后敌舰队在移动阶段跑掉，
// 编队不能永久停在空海面——必须清掉接敌状态、回到搜索/返航流程。
func TestEngagedStrikeDoesNotFreezeOnEmptyTarget(t *testing.T) {
	state := sandboxState(t)
	empty := emptyHexForTest(t, state)
	for _, id := range []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"} {
		aircraft := state.Aircraft[id]
		aircraft.Status = "attack"
		aircraft.Launched = true
		aircraft.TaskID = "strike-freeze"
		aircraft.TaskKind = "attack"
		aircraft.Coord = empty
		aircraft.OriginCoord = empty
		aircraft.TargetCoord = empty
		aircraft.Engaged = true
	}
	AdvanceAviation(&state)

	aircraft := state.Aircraft["b-cv-01-dive_bomber-x1"]
	if aircraft == nil {
		t.Fatal("编队不应被移除")
	}
	if aircraft.Engaged {
		t.Fatalf("目标 hex 已无敌舰时必须作废接敌状态，否则编队永久冻结：%+v", aircraft)
	}
}

// TestCarrierStrikeEngagesAirfieldTarget 回归：机场不是水面舰队，接敌判定恒为 0%，
// 舰载攻击编队必须在抵达机场 hex 时直接接敌成功，否则只会在机场上空反复搜索。
func TestCarrierStrikeEngagesAirfieldTarget(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-a-lighthouse"]
	if airfield == nil {
		t.Fatal("沙盒剧本应部署美军灯塔岛机场")
	}
	aircraft := state.Aircraft["b-cv-01-dive_bomber-x1"]
	aircraft.Status = "attack"
	aircraft.Launched = true
	aircraft.TaskID = "strike-airfield"
	aircraft.TaskKind = "attack"
	aircraft.Coord = airfield.Coord
	aircraft.OriginCoord = airfield.Coord
	aircraft.TargetCoord = airfield.Coord
	aircraft.Engaged = false

	AdvanceAviation(&state)

	aircraft = state.Aircraft["b-cv-01-dive_bomber-x1"]
	if aircraft == nil {
		t.Fatal("攻击编队不应被移除")
	}
	if !aircraft.Engaged {
		t.Fatalf("抵达敌方机场上空应直接接敌成功，下一回合才能发起对地攻击：%+v", aircraft)
	}
}

// emptyHexForTest 找一个没有舰船、也没有机场的地图内 hex。
func emptyHexForTest(t *testing.T, state State) string {
	t.Helper()
	for row := 0; row < state.Map.Rows; row++ {
		for col := 0; col < state.Map.Cols; col++ {
			coord := fmt.Sprintf("%02d%02d", row, col)
			if !inBounds(state.Map, coord) {
				continue
			}
			if len(unitsAtFleet(state, coord)) > 0 {
				continue
			}
			if strikeAirfieldAt(state, "B", coord) != nil || strikeAirfieldAt(state, "A", coord) != nil {
				continue
			}
			return coord
		}
	}
	t.Fatal("地图上找不到空白 hex")
	return ""
}
