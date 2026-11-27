package game

import "testing"

// TestSplitFleetDetachesASWWithoutMovementLock 锁定"舰队拆分"：
// 反潜舰拆成新支队留在原格、追踪中的潜艇改挂到支队；不挂移动锁、不动冷却，
// 主队照常行动。
func TestSplitFleetDetachesASWWithoutMovementLock(t *testing.T) {
	state := submarineFixture()
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true,
			Discovered: true, DiscoveredBy: "A", TrackedFleetID: "TF-A"},
	}
	for _, unit := range state.Units {
		if unit.Faction == "A" {
			unit.MoveCooldown = 2
		}
	}
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-ca"}
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("有反潜舰也有主队时应允许拆分：%v", err)
	}
	ApplySplitFleet(&state, order)
	ca, dd := state.Units["a-ca"], state.Units["a-dd"]
	if dd.FleetID == ca.FleetID || dd.FleetID == "" {
		t.Fatalf("驱逐舰应拆到新舰队：dd=%s ca=%s", dd.FleetID, ca.FleetID)
	}
	if dd.Coord != "0505" || ca.Coord != "0505" {
		t.Fatal("拆分不应改变位置")
	}
	if dd.MoveCooldown != 2 || ca.MoveCooldown != 2 {
		t.Fatalf("拆分不应影响移动冷却：dd=%d ca=%d", dd.MoveCooldown, ca.MoveCooldown)
	}
	if sub := state.Submarines["sub-b-1"]; sub == nil || sub.TrackedFleetID != dd.FleetID {
		t.Fatalf("追踪中的潜艇应改挂到反潜支队：%+v", sub)
	}
	// 主队已经没有反潜舰了，再拆不报错、也没有可拆的船。
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("没有反潜舰时也应当接受命令（只是拆不出船）：%v", err)
	}
	if events := ApplySplitFleet(&state, order); len(events) == 0 || events[0].Type != "fleet_split_empty" {
		t.Fatalf("没有反潜舰时应给出空拆分说明，实际 %+v", events)
	}
}

// TestSplitFleetAllowsWholeFleet 整支舰队都是反潜舰时也可以拆走：
// 拆分后原舰队自然消失，新支队接管全部舰船。
func TestSplitFleetAllowsWholeFleet(t *testing.T) {
	state := submarineFixture()
	state.Units = map[string]*Unit{
		"a-dd":  {ID: "a-dd", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "弗莱彻", Type: "DD", Strength: 2, MaxStrength: 2, Card: ShipCard{DepthCharges: 2}},
		"a-dd2": {ID: "a-dd2", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "奥班农", Type: "DD", Strength: 2, MaxStrength: 2, Card: ShipCard{DepthCharges: 2}},
	}
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-dd"}
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("全是反潜舰时也应允许拆分：%v", err)
	}
	ApplySplitFleet(&state, order)
	newFleet := state.Units["a-dd"].FleetID
	if newFleet == "TF-A" || newFleet == "" {
		t.Fatalf("驱逐舰应进入新支队：%s", newFleet)
	}
	if state.Units["a-dd2"].FleetID != newFleet {
		t.Fatal("两艘驱逐舰都应拆到同一支队")
	}
}
