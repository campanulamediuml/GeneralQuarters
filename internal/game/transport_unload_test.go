package game

import "testing"

// 运输船停在敌方机场 hex：停留计数含到达当回合，满 3 回合后（第 4 回合）自动开始卸货，
// 每回合卸 1 点，卸空后强制撤离。
func TestTransportUnloadsAutomaticallyAfterThreeTurns(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	trs := &Unit{
		ID: "trs-test", FleetID: "TRS-A", FleetName: "运输队", Name: "运输船", Type: "TRS", Faction: "A",
		Coord: airfield.Coord, Strength: 2, MaxStrength: 2, Heading: "E",
		Card: ShipCard{Cargo: 3, SpeedClass: "normal", DamageControl: 3},
	}
	state.Units[trs.ID] = trs
	state.FleetStays = map[string]string{}
	state.FleetSessions = map[string]int{}
	state.ShoreSupplies = map[string]int{}
	state.RNGState = 52

	unloadTurns := []int{}
	for turn := 1; turn <= 7; turn++ {
		events := ResolveOrders(&state, nil)
		for _, event := range events {
			if event.Type == "troop_unloading" {
				unloadTurns = append(unloadTurns, turn)
			}
		}
		t.Logf("turn %d session=%d cargo=%d supplies=%d withdrawing=%v", turn, SessionOf(state, "TRS-A"), trs.Card.Cargo, shoreSupplies(state, "A", airfield.Coord), trs.Withdrawing)
	}
	if len(unloadTurns) != 3 {
		t.Fatalf("3 点物资应分 3 回合卸完，实际卸货回合 %v", unloadTurns)
	}
	if unloadTurns[0] != 4 {
		t.Fatalf("到达当回合算第 1 回合，应在第 4 回合开始卸货，实际第 %d 回合", unloadTurns[0])
	}
	if trs.Card.Cargo != 0 || shoreSupplies(state, "A", airfield.Coord) != 3 || !trs.Withdrawing {
		t.Fatalf("卸货结果错误：cargo=%d supplies=%d withdrawing=%v", trs.Card.Cargo, shoreSupplies(state, "A", airfield.Coord), trs.Withdrawing)
	}
}
