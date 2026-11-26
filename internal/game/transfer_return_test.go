package game

import "testing"

// 被召回/中止的转场必须真的往回飞，而不是每回合再走一次"目的地不再是我方机场"的判定。
func TestReturningGroundTransferFliesHome(t *testing.T) {
	state := State{
		Map:       MapDefinition{Rows: 12, Cols: 12},
		Airfields: map[string]*Airfield{},
		GroundAir: map[string]*AirfieldAirUnit{},
	}
	state.Airfields["home"] = &Airfield{ID: "home", Faction: "A", Name: "母基地", Coord: "0101", Type: AirfieldSeaplane, Runways: 4, Strength: 20, MaxStrength: 20, HangarCapacity: AirfieldHangarUnlimited}
	state.Airfields["dest"] = &Airfield{ID: "dest", Faction: "A", Name: "目标基地", Coord: "0109", Type: AirfieldSeaplane, Runways: 4, Strength: 20, MaxStrength: 20, HangarCapacity: AirfieldHangarUnlimited}
	state.GroundAir["p1"] = &AirfieldAirUnit{
		ID: "p1", DefinitionID: "pby", AirfieldID: "home", Faction: "A", Model: "PBY 卡特里娜",
		Kind: "recon", Seaplane: true, AirfieldOnly: true, Status: "transfer", TaskID: "t1", TaskKind: "transfer", Returning: true,
		Coord: "0105", OriginCoord: "0101", BaseRange: 40, MaxRange: 40, RemainingRange: 40,
	}
	aircraft := state.GroundAir["p1"]
	before := Distance(aircraft.Coord, "0101")

	events := AdvanceGroundAir(&state)
	for _, event := range events {
		if event.Type == "transfer_aborted" {
			t.Fatalf("已经在返航的转场不该再判一次中止：%+v", event)
		}
	}
	if aircraft.Coord == "0105" || Distance(aircraft.Coord, "0101") >= before {
		t.Fatalf("返航中的转场应当朝母基地飞，实际仍在 %s", aircraft.Coord)
	}
}
