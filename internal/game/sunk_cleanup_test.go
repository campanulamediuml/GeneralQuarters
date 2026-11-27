package game

import "testing"

// 幽灵舰：0 生命值的舰船还留在 state.Units 里（延迟沉没没被 flush 的残局）。
// 表现是观战/玩家面板里挂着"结构值 0/1 + 沉没"，编队人数、重防空合计、舰船上限还把它算进去。
func ghostFleetState() *State {
	return &State{
		Turn: 3,
		Units: map[string]*Unit{
			"a-cv": {ID: "a-cv", Faction: "A", FleetID: "TG1", FleetName: "TF58.1", Name: "黄蜂", Type: "CV", Coord: "0202", Strength: 4, MaxStrength: 4},
			"a-dd": {ID: "a-dd", Faction: "A", FleetID: "TG1", FleetName: "TF58.1", Name: "弗莱彻", Type: "DD", Coord: "0202", Strength: 0, MaxStrength: 1},
		},
		Aircraft:  map[string]*AirUnit{},
		Airfields: map[string]*Airfield{},
		Losses:    map[string]*LossLedger{},
	}
}

func TestSweepSunkUnitsClearsGhosts(t *testing.T) {
	state := ghostFleetState()
	events := sweepSunkUnits(state)
	if state.Units["a-dd"] != nil {
		t.Fatal("0 生命值的舰船应当按沉没移除")
	}
	if state.Units["a-cv"] == nil {
		t.Fatal("还有生命值的舰船不该被清掉")
	}
	if len(events) == 0 {
		t.Fatal("清理幽灵舰应当发出沉没事件")
	}
	ledger := state.Losses["A"]
	if ledger == nil || ledger.ShipTotal != 1 || ledger.Ships[0].ID != "a-dd" {
		t.Fatalf("幽灵舰要记进战果台账，实际 %+v", ledger)
	}
}

// 视图层兜底：就算没来得及清理，也不能把 0 生命值的船画出来。
func TestVisibleUnitsSkipsSunkShips(t *testing.T) {
	state := ghostFleetState()
	units := VisibleUnits(*state, "A")
	for _, unit := range units {
		if unit.ID == "a-dd" {
			t.Fatal("0 生命值的舰船不该出现在视图里")
		}
	}
	fleets := VisibleFleets(*state, "A")
	if len(fleets) != 1 {
		t.Fatalf("应当只剩一支舰队，实际 %d", len(fleets))
	}
	if fleets[0].Count != 1 {
		t.Fatalf("编队人数只算还浮着的舰船，实际 %d", fleets[0].Count)
	}
}
