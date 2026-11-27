package game

import "testing"

// TestDiveAfterAttackExposesSubmarine "下潜逃避"不是隐身：潜艇一下潜就暴露，
// 从下回合起被触发舰队锁定，进入每回合深弹 + 撤退判定的流程。
func TestDiveAfterAttackExposesSubmarine(t *testing.T) {
	state := submarineFixture()
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true},
	}
	report := &SubmarineBattle{Turn: state.Turn, Coord: "0505", FleetID: "TF-A", FleetFaction: "A", SubmarineFaction: "B"}
	buySubmarineEffect(&state, state.Submarines["sub-b-1"], "TF-A", 2, report)
	sub := state.Submarines["sub-b-1"]
	if sub == nil {
		t.Fatal("潜艇不应被击沉")
	}
	if !sub.Discovered || sub.DiscoveredBy != "A" || sub.TrackedFleetID != "TF-A" {
		t.Fatalf("下潜逃避应让潜艇暴露给触发舰队：%+v", sub)
	}
	if !report.Discovered {
		t.Fatal("战报应记下潜艇暴露")
	}
}
