package game

import "testing"

// 卸载节奏：在敌方机场 hex 停满 3 回合才开始，之后每 3 回合卸 1 点（第 3、6、9…回合）。
// 每 1 点物资让机场生命值 -1，而不是卸一点就整座机场停摆。
func TestTransportUnloadsAutomaticallyAfterThreeTurns(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	before := airfield.Strength
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
	for turn := 1; turn <= 10; turn++ {
		events := ResolveOrders(&state, nil)
		for _, event := range events {
			if event.Type == "troop_unloading" {
				unloadTurns = append(unloadTurns, turn)
			}
		}
	}
	want := []int{4, 7, 10}
	if len(unloadTurns) != len(want) {
		t.Fatalf("3 点物资应按每 3 回合 1 点卸完，实际卸货回合 %v", unloadTurns)
	}
	for i, turn := range want {
		if unloadTurns[i] != turn {
			t.Fatalf("卸货回合应为 %v，实际 %v", want, unloadTurns)
		}
	}
	if trs.Card.Cargo != 0 || shoreSupplies(state, "A", airfield.Coord) != 3 || !trs.Withdrawing {
		t.Fatalf("卸货结果错误：cargo=%d supplies=%d withdrawing=%v", trs.Card.Cargo, shoreSupplies(state, "A", airfield.Coord), trs.Withdrawing)
	}
	// 每点物资只扣 1 点机场生命值。
	wantStrength := before - 3
	if wantStrength < 0 {
		wantStrength = 0
	}
	if airfield.Strength != wantStrength {
		t.Fatalf("机场生命值应为 %d，实际 %d", wantStrength, airfield.Strength)
	}
}

// 风暴天卸载间隔翻倍：每 6 回合才卸 1 点。
func TestUnloadIntervalDoublesInStorm(t *testing.T) {
	for sessions := 3; sessions <= 12; sessions++ {
		due := unloadDue(sessions, true)
		want := (sessions-3)%6 == 0
		if due != want {
			t.Fatalf("风暴天第 %d 回合卸货判定应为 %v，实际 %v", sessions, want, due)
		}
	}
	if !unloadDue(3, false) || unloadDue(4, false) || !unloadDue(6, false) || !unloadDue(9, false) {
		t.Fatal("正常天气应按每 3 回合卸 1 点")
	}
}
