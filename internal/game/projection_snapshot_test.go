package game

import (
	"bytes"
	"encoding/json"
	"testing"
)

// 投影必须是值快照：取完投影之后再改房间状态，投影自己不能跟着变。
// 补丁是"和上一帧的投影比出来的差异"，上一帧要是还指着房间里同一块内存，
// 改动就会被算成"没变"，玩家那边就停在旧画面上了。
func TestProjectionsAreSnapshots(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	projections := map[string]any{
		"VisibleUnits":       VisibleUnits(state, "A"),
		"VisibleFleets":      VisibleFleets(state, "A"),
		"VisibleAviation":    VisibleAviation(state, "A"),
		"VisibleAirfields":   VisibleAirfields(state, "A"),
		"VisibleGroundAir":   VisibleGroundAir(state, "A"),
		"VisibleAirCombats":  VisibleAirCombats(state, "A"),
		"GroundAviationView": GroundAviationView(state, "A"),
		"GroundAircraftView": GroundAircraftView(state, "A"),
		"VisibleEnemyAir":    VisibleEnemyAir(state, "A"),
		"VisibleFleetStays":  VisibleFleetStays(state, "A"),
	}
	before := map[string][]byte{}
	for name, value := range projections {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		before[name] = data
	}
	// 引擎就是这么改状态的：原地改指针指向的对象、原地改切片元素。
	for _, unit := range state.Units {
		if unit.Damage == nil {
			unit.Damage = &ShipDamage{}
		}
		unit.Damage.Flooding = append(unit.Damage.Flooding, 1)
		unit.Damage.Fire = "fire"
		if unit.Deck != nil {
			unit.Deck.Length++
			unit.Deck.Operation = "changed"
			for _, elevator := range unit.Deck.Elevators {
				elevator.Status = "changed"
				elevator.AircraftIDs = append(elevator.AircraftIDs, "ghost")
			}
		}
	}
	for _, aircraft := range state.Aircraft {
		aircraft.Status = "changed"
	}
	for _, airfield := range state.Airfields {
		airfield.Strength--
	}
	for name, value := range projections {
		after, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before[name], after) {
			t.Errorf("%s 的投影跟着状态一起变了：投影必须是值快照，不能引用房间内存", name)
		}
	}
}
