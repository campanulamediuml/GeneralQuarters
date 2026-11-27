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

// 改目标不自动换挂载：挂鱼雷的编队改成打机场后仍是鱼雷，
// 服务端只提示"这个机队有些挂载不适合这个任务"。
func TestRetargetToAirfieldKeepsLoadoutAndWarns(t *testing.T) {
	state := sandboxState(t)
	plane := &AirUnit{
		ID: "tb-1", CarrierID: "b-cv-01", Faction: "B", Model: "B5N2", Kind: "torpedo_bomber",
		TaskID: "task-tb", TaskKind: "attack", Status: "deck",
		Loadout: LoadoutTorpedo, AntiGround: 2, AntiShip: 4,
		BaseRange: 10, MaxRange: 10, RemainingRange: 10, TargetCoord: "0202",
	}
	state.Aircraft[plane.ID] = plane
	// af-a-lighthouse 是美方机场（0304），对 B 阵营就是敌方机场 → 目标类型改成对地。
	events := ApplyAirOrder(&state, Order{Type: "SET_STRIKE_TARGET", AircraftID: plane.ID, TaskID: plane.TaskID, Coord: "0304"})
	if plane.Loadout != LoadoutTorpedo {
		t.Fatalf("改目标不应自动换挂载，实际 %s", plane.Loadout)
	}
	warned := false
	for _, event := range events {
		if event.Type == "air_loadout_mismatch" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("应提示这个机队有些挂载不适合这个任务：%+v", events)
	}
}

// 挂载本来就对得上时不该弹这条提示：炸弹打机场是正常配置。
func TestRetargetMatchingLoadoutDoesNotWarn(t *testing.T) {
	state := sandboxState(t)
	plane := &AirUnit{
		ID: "db-1", CarrierID: "b-cv-01", Faction: "B", Model: "D3A", Kind: "dive_bomber",
		TaskID: "task-db", TaskKind: "attack", Status: "deck",
		Loadout: LoadoutBomb, AntiGround: 4, AntiShip: 2,
		BaseRange: 10, MaxRange: 10, RemainingRange: 10, TargetCoord: "0202",
	}
	state.Aircraft[plane.ID] = plane
	events := ApplyAirOrder(&state, Order{Type: "SET_STRIKE_TARGET", AircraftID: plane.ID, TaskID: plane.TaskID, Coord: "0304"})
	for _, event := range events {
		if event.Type == "air_loadout_mismatch" {
			t.Fatalf("挂炸弹打机场不该提示挂载不适合：%s", event.Text)
		}
	}
}
