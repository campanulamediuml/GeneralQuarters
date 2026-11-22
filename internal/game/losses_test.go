package game

import "testing"

// TestShipLossRecordedOnSink 舰船沉没必须进战果台账（含回合与舰种），
// 之前沉船只是从 state 里删掉，玩家再也看不到这一局打掉了什么。
func TestShipLossRecordedOnSink(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["b-ca-01"]
	state.Turn = 12
	damageStrength(&state, unit, 99)

	ledger := VisibleLosses(state, "A")["B"]
	if len(ledger.Ships) != 1 {
		t.Fatalf("应记录 1 艘沉船，实际 %d", len(ledger.Ships))
	}
	if ledger.Ships[0].Name != unit.Name || ledger.Ships[0].Turn != 12 {
		t.Fatalf("沉船记录不对：%+v", ledger.Ships[0])
	}
	if ledger.Ships[0].Type != "CA" {
		t.Fatalf("沉船应保留舰种，实际 %q", ledger.Ships[0].Type)
	}
}

// TestAircraftLossAggregatedByModel 回合结束对账：消失的算子按机型累加。
func TestAircraftLossAggregatedByModel(t *testing.T) {
	state := sandboxState(t)
	snapshotAircraftRoster(&state)

	ids := sortedAirUnitIDs(state)
	if len(ids) == 0 {
		t.Fatal("沙盒里应该有航空算子")
	}
	removedID := ids[0]
	victim := state.Aircraft[removedID]
	model, faction := victim.Model, victim.Faction
	delete(state.Aircraft, removedID)
	settleAircraftLosses(&state)

	ledger := VisibleLosses(state, faction)[faction]
	total := 0
	found := false
	for _, item := range ledger.Aircraft {
		total += item.Count
		if item.Model == model {
			found = true
		}
	}
	if total != 1 || !found {
		t.Fatalf("应按机型记录 1 架损失（%s），实际 %+v", model, ledger.Aircraft)
	}
	// 对账过一次就不该重复计数
	settleAircraftLosses(&state)
	if got := VisibleLosses(state, faction)[faction].AircraftTotal; got != 1 {
		t.Fatalf("重复对账不应重复计数，实际 %d", got)
	}
}

// TestLossesExposedForBothSides 快照里双方都要有战果账（空账也要有，前端不用判空）。
func TestLossesExposedForBothSides(t *testing.T) {
	state := sandboxState(t)
	losses := VisibleLosses(state, "A")
	for _, side := range []string{"A", "B"} {
		if _, ok := losses[side]; !ok {
			t.Fatalf("阵营 %s 缺少战果账", side)
		}
	}
}
