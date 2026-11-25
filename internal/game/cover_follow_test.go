package game

import "testing"

// 空中掩护跟着被掩护的舰队走，不能钉在起飞时的旧 hex。
func TestCoverFollowsGuardedFleet(t *testing.T) {
	state := State{
		Map: MapDefinition{Rows: 20, Cols: 20},
		Units: map[string]*Unit{
			"cv":      {ID: "cv", Faction: "B", Type: "CV", Coord: "1010", Card: ShipCard{DeckCapacity: 8}},
			"guarded": {ID: "guarded", Faction: "B", Type: "CA", Coord: "1111"},
		},
		Aircraft: map[string]*AirUnit{
			"cover": {ID: "cover", Faction: "B", CarrierID: "cv", Kind: "fighter", Status: "cover", Coord: "1111", TargetCoord: "1010", CoverUnitID: "guarded"},
		},
	}
	cover := state.Aircraft["cover"]
	cover.SetRange(20)
	advanceCover(&state, cover)
	if cover.TargetCoord != "1111" {
		t.Fatalf("掩护点没有跟随被掩护舰队：%s", cover.TargetCoord)
	}
}
