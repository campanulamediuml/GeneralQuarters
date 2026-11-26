package game

import (
	"fmt"
	"testing"
)

func fleetLimitFixture(count, maxShips int) (State, Order) {
	state := State{Units: map[string]*Unit{}, FleetMaxShips: maxShips}
	assignments := map[string]string{}
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("u%02d", i)
		state.Units[id] = &Unit{ID: id, Faction: "A", Coord: "0303", FleetID: "TF1"}
		assignments[id] = "TF1"
	}
	return state, Order{Type: "REORGANIZE_HEX", Coord: "0303", Assignments: assignments}
}

// TestFleetShipLimitFollowsScenario 校验舰队规模上限跟剧本走：默认 20，剧本配多少就是多少。
func TestFleetShipLimitFollowsScenario(t *testing.T) {
	state, order := fleetLimitFixture(30, 0)
	if limit := fleetShipLimit(state); limit != 20 {
		t.Fatalf("没配剧本时上限应为 20，实际 %d", limit)
	}
	if err := validateReorganization(state, "A", order); err == nil {
		t.Fatal("默认上限 20，30 艘的整编应当被拒")
	}

	state.FleetMaxShips = 40
	if limit := fleetShipLimit(state); limit != 40 {
		t.Fatalf("剧本配 40 时上限应为 40，实际 %d", limit)
	}
	if err := validateReorganization(state, "A", order); err != nil {
		t.Fatalf("放宽到 40 后 30 艘应当被接受：%v", err)
	}

	smallState, smallOrder := fleetLimitFixture(21, 0)
	if err := validateReorganization(smallState, "A", smallOrder); err == nil {
		t.Fatal("21 艘超过默认上限 20，应当被拒")
	}
}

// TestMergeFleetRespectsShipLimit 校验合并两支舰队也不能绕开规模上限。
func TestMergeFleetRespectsShipLimit(t *testing.T) {
	state := State{Units: map[string]*Unit{}}
	for i := 0; i < 12; i++ {
		aid := fmt.Sprintf("a%02d", i)
		bid := fmt.Sprintf("b%02d", i)
		state.Units[aid] = &Unit{ID: aid, Faction: "A", Coord: "0303", FleetID: "TF1"}
		state.Units[bid] = &Unit{ID: bid, Faction: "A", Coord: "0303", FleetID: "TF2"}
	}
	order := Order{Type: "MERGE_FLEET", UnitID: "a00", TargetUnitID: "b00"}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("合并后 24 艘超过默认上限 20，应当被拒")
	}
	state.FleetMaxShips = 40
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("放宽到 40 后 24 艘的合并应当被接受：%v", err)
	}
}
