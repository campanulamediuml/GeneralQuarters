package game

import "testing"

// 同一 hex 有敌方机场时，显式指定 target_fleet_id 应优先打舰队，
// 否则挂载判断仍会把目标当成机场。
func TestStrikeTargetKindPrefersExplicitFleet(t *testing.T) {
	state := State{Airfields: map[string]*Airfield{"af": {ID: "af", Coord: "1611", Faction: "A"}}}
	plane := &AirUnit{ID: "p", Kind: "torpedo_bomber", Faction: "B", TargetCoord: "1611", TargetFleetID: "TF44"}
	if got := strikeTargetKind(state, plane); got != StrikeTargetShip {
		t.Fatalf("显式指定舰队时目标类型应为对舰，实际 %s", got)
	}
}

// 从对舰切换到机场目标，挂载必须从鱼雷换成炸弹，并退回整备。
func TestRetargetToAirfieldRequiresRearm(t *testing.T) {
	state := State{Airfields: map[string]*Airfield{"af": {ID: "af", Coord: "1611", Faction: "A"}}}
	plane := &AirUnit{
		ID: "p", Kind: "torpedo_bomber", Faction: "B", TaskKind: "attack",
		TargetCoord: "1611", Loadout: LoadoutTorpedo, AntiGround: 2, AntiShip: 4,
		BaseRange: 10, MaxRange: 10, Status: "ready",
	}
	applyStrikeLoadoutChange(&state, plane)
	if plane.Loadout == LoadoutTorpedo {
		t.Fatal("目标改成机场后仍挂着鱼雷")
	}
	if plane.Status != "service" {
		t.Fatalf("换挂载后应退回整备，实际 %s", plane.Status)
	}
}

// 已经升空的飞机不能在半空中换挂载，必须返航重新整备。
func TestAirborneStrikeCannotChangeLoadout(t *testing.T) {
	state := State{Airfields: map[string]*Airfield{"af": {ID: "af", Coord: "1611", Faction: "A"}}}
	plane := &AirUnit{
		ID: "p", Kind: "torpedo_bomber", Faction: "B", TaskKind: "attack",
		TargetCoord: "1611", Loadout: LoadoutTorpedo, AntiGround: 2, AntiShip: 4,
		BaseRange: 10, MaxRange: 10, Status: "attack", Launched: true,
	}
	if err := validateStrikeLoadoutChange(state, []*AirUnit{plane}, "", "1611"); err == nil {
		t.Fatal("已升空的飞机换目标类型时应要求返航整备")
	}
}
