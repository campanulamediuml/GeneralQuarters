package game

import "testing"

// TestSplitFleetMovesSelectedShipsWithoutMovementLock 舰队拆分是通用操作：
// 把选中的船拆成新舰队，留在原格；不挂移动锁、不动冷却。
// 拆出去的正好是反潜舰时，正在追踪的潜艇跟着改挂到新舰队。
func TestSplitFleetMovesSelectedShipsWithoutMovementLock(t *testing.T) {
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
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-dd"}}
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("拆出驱逐舰应被允许：%v", err)
	}
	ApplySplitFleet(&state, order)
	ca, dd := state.Units["a-ca"], state.Units["a-dd"]
	if dd.FleetID == ca.FleetID || dd.FleetID == "" {
		t.Fatalf("驱逐舰应拆到新舰队：dd=%s ca=%s", dd.FleetID, ca.FleetID)
	}
	if ca.FleetID != "TF-A" {
		t.Fatalf("主队不应改 ID：%s", ca.FleetID)
	}
	if dd.Coord != "0505" || ca.Coord != "0505" {
		t.Fatal("拆分不应改变位置")
	}
	if dd.MoveCooldown != 2 || ca.MoveCooldown != 2 {
		t.Fatalf("拆分不应影响移动冷却：dd=%d ca=%d", dd.MoveCooldown, ca.MoveCooldown)
	}
	if sub := state.Submarines["sub-b-1"]; sub == nil || sub.TrackedFleetID != dd.FleetID {
		t.Fatalf("反潜舰拆走后追踪应交給新舰队：%+v", sub)
	}
}

// TestSplitFleetLeavesTrackingWhenASWStays 主队还留着反潜舰时，追踪不交给新分队。
func TestSplitFleetLeavesTrackingWhenASWStays(t *testing.T) {
	state := submarineFixture()
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true,
			Discovered: true, DiscoveredBy: "A", TrackedFleetID: "TF-A"},
	}
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-ca"}}
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("拆分非反潜舰也应被允许：%v", err)
	}
	ApplySplitFleet(&state, order)
	if sub := state.Submarines["sub-b-1"]; sub == nil || sub.TrackedFleetID != "TF-A" {
		t.Fatalf("主队仍有反潜舰时追踪应留在主队：%+v", sub)
	}
	if state.Units["a-ca"].FleetID == "TF-A" {
		t.Fatal("被选中的舰船应挪到新舰队")
	}
}

// TestSplitFleetMixedDetachmentKeepsHunting 拆出"重巡 + 驱逐舰"这类混编分队：
// 只要分队里有反潜舰，被追踪的潜艇就交给分队继续反潜，主队可以放心走。
func TestSplitFleetMixedDetachmentKeepsHunting(t *testing.T) {
	state := submarineFixture()
	state.Units["a-dd2"] = &Unit{ID: "a-dd2", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "奥班农", Type: "DD", Strength: 2, MaxStrength: 2, Card: ShipCard{DepthCharges: 2}}
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true,
			Discovered: true, DiscoveredBy: "A", TrackedFleetID: "TF-A"},
	}
	// 拆出一艘重巡 + 一艘驱逐舰，主队还留着另一艘驱逐舰。
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-ca", "a-dd"}}
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("混编拆分应被允许：%v", err)
	}
	ApplySplitFleet(&state, order)
	detachment := state.Units["a-dd"].FleetID
	if detachment == "TF-A" {
		t.Fatal("拆出的舰船应在新分队里")
	}
	if sub := state.Submarines["sub-b-1"]; sub == nil || sub.TrackedFleetID != detachment {
		t.Fatalf("分队里有反潜舰时应由分队接替反潜：%+v", sub)
	}
	if state.Units["a-dd2"].FleetID != "TF-A" {
		t.Fatal("没被选中的驱逐舰应留在主队")
	}
}

// TestSplitFleetWholeFleet 全选等于整体改成新舰队：旧 ID 自然消失，追踪交给新舰队。
func TestSplitFleetWholeFleet(t *testing.T) {
	state := submarineFixture()
	state.Units = map[string]*Unit{
		"a-dd":  {ID: "a-dd", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "弗莱彻", Type: "DD", Strength: 2, MaxStrength: 2, Card: ShipCard{DepthCharges: 2}},
		"a-dd2": {ID: "a-dd2", FleetID: "TF-A", FleetName: "TF-A", Faction: "A", Coord: "0505", Name: "奥班农", Type: "DD", Strength: 2, MaxStrength: 2, Card: ShipCard{DepthCharges: 2}},
	}
	state.Submarines = map[string]*Submarine{
		"sub-b-1": {ID: "sub-b-1", Faction: "B", Name: "伊号", Coord: "0505", Deployed: true,
			Discovered: true, DiscoveredBy: "A", TrackedFleetID: "TF-A"},
	}
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-dd", SplitIDs: []string{"a-dd", "a-dd2"}}
	if err := validateSplitFleet(state, "A", order); err != nil {
		t.Fatalf("全选也应允许：%v", err)
	}
	ApplySplitFleet(&state, order)
	newFleet := state.Units["a-dd"].FleetID
	if newFleet == "TF-A" || newFleet == "" {
		t.Fatalf("两艘驱逐舰应进入新舰队：%s", newFleet)
	}
	if state.Units["a-dd2"].FleetID != newFleet {
		t.Fatal("两艘驱逐舰都应在同一支新舰队里")
	}
	if sub := state.Submarines["sub-b-1"]; sub == nil || sub.TrackedFleetID != newFleet {
		t.Fatalf("旧舰队消失后追踪应交给新舰队：%+v", sub)
	}
}

func TestSplitFleetDuringDeploy(t *testing.T) {
	state := submarineFixture()
	state.Phase = PhaseDeploy
	order := Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-dd"}}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("部署阶段应允许拆分舰队：%v", err)
	}
	ApplySplitFleet(&state, order)
	if state.Units["a-dd"].FleetID == state.Units["a-ca"].FleetID {
		t.Fatal("部署阶段拆分后，所选舰船应进入新舰队")
	}
	if state.Units["a-dd"].MoveCooldown != 0 || state.Units["a-ca"].MoveCooldown != 0 {
		t.Fatal("部署阶段拆分不应附加移动冷却")
	}
}

// TestSplitFleetValidation 只拦"没选船"和"选了别家的船"。
func TestSplitFleetValidation(t *testing.T) {
	state := submarineFixture()
	if err := validateSplitFleet(state, "A", Order{Type: "SPLIT_FLEET", UnitID: "a-ca"}); err == nil {
		t.Fatal("没选舰船时应拒绝")
	}
	if err := validateSplitFleet(state, "A", Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"b-cv"}}); err == nil {
		t.Fatal("不能把敌方舰船拆进自己的新舰队")
	}
	if err := validateSplitFleet(state, "B", Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-dd"}}); err == nil {
		t.Fatal("不能拆别人的舰队")
	}
}
