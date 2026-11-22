package game

import (
	"testing"
)

// 一轮投弹里所有炸弹都会砸在同一艘舰上：半路打沉不打断这一轮。
func TestBombingRoundLandsEveryBombEvenAfterSinking(t *testing.T) {
	const runs = 1500
	measure := func(shipStrength int) (float64, float64) {
		totalHits := 0
		sunk := 0
		for i := 0; i < runs; i++ {
			state := buildStrikeState()
			state.RNGState = uint64(7919*i+13)*2654435761 + 1
			state.Units["bb"] = &Unit{ID: "bb", Faction: "B", Type: "BB", FleetID: "fleet-b", Coord: "0202", Name: "bb",
				Strength: shipStrength, MaxStrength: 40, Card: ShipCard{SpeedClass: "normal", DamageControl: 1}}
			attacker := &AirUnit{ID: "a1", Faction: "A", Kind: "dive_bomber", Model: "SBD", Status: "attack",
				Coord: "0202", Strength: 4, AntiShip: 10, Loadout: LoadoutBomb}
			state.Aircraft["a1"] = attacker
			report := &AirCombatReport{}
			resolveBombingWave(state, []*AirUnit{attacker}, []Unit{*state.Units["bb"]}, false, false, report)
			if len(state.PendingSinks) != 0 {
				t.Fatalf("沉没没有在这一轮结束时结算掉：pending=%d", len(state.PendingSinks))
			}
			totalHits += report.StrikeHits
			if state.Units["bb"] == nil {
				sunk++
			}
		}
		return float64(totalHits) / runs, float64(sunk) / runs
	}
	frail, frailSunk := measure(1)
	t.Logf("1 强度舰：平均命中 %.2f、沉没率 %.2f", frail, frailSunk)
	if frailSunk < 0.15 {
		t.Fatalf("这组参数本该经常把它打沉，实际沉没率 %.2f，测不出半路沉没的路径", frailSunk)
	}
	// 10 点对舰攻击值、基础 25% 命中（起火后还会 +10%）：不打折的话在 3.5 上下。
	// 半路沉没若截断本轮，命中数会掉到 1.5 以下。
	if frail < 2.8 {
		t.Fatalf("沉没打断了这一轮投弹：平均命中 %.2f", frail)
	}
}

// 沉没要先挂起、等这一轮投完再移除：挂起期间舰还在场上但强度为 0，不产生 unit_destroyed。
func TestDeferredSinkKeepsShipUntilRoundEnds(t *testing.T) {
	state := buildStrikeState()
	unit := &Unit{ID: "bb", Faction: "B", Type: "BB", FleetID: "fleet-b", Coord: "0202", Name: "bb",
		Strength: 1, MaxStrength: 6, Card: ShipCard{DamageControl: 1}}
	state.Units["bb"] = unit
	state.DeferredSink = true
	events := damageStrength(state, unit, 1)
	if len(events) != 0 {
		t.Fatalf("延后沉没不该当场发事件：%v", events)
	}
	if state.Units["bb"] == nil {
		t.Fatal("延后沉没期间舰船不该立刻移除")
	}
	if unit.Strength != 0 {
		t.Fatalf("延后沉没的舰强度应归零，实际 %d", unit.Strength)
	}
	if len(state.PendingSinks) != 1 {
		t.Fatalf("应挂起 1 艘待沉船，实际 %d", len(state.PendingSinks))
	}
	// 同一艘舰再挨打，不重复挂起。
	damageStrength(state, unit, 1)
	if len(state.PendingSinks) != 1 {
		t.Fatalf("重复挂起：%d", len(state.PendingSinks))
	}
	flushed := flushDeferredSinks(state)
	if state.Units["bb"] != nil {
		t.Fatal("flush 之后舰船应被移除")
	}
	if state.DeferredSink {
		t.Fatal("flush 之后开关应复位")
	}
	found := false
	for _, event := range flushed {
		if event.Type == "unit_destroyed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("flush 应发出 unit_destroyed 事件：%v", flushed)
	}
}
