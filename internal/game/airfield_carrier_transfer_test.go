package game

import "testing"

// 陆基机场的舰载型飞机转场到航母：以前目标只能选机场，任务建了却永远设不上目标。
func TestGroundFighterTransfersOntoCarrier(t *testing.T) {
	state := State{
		Map:       MapDefinition{Rows: 12, Cols: 12},
		Airfields: map[string]*Airfield{},
		GroundAir: map[string]*AirfieldAirUnit{},
		Units:     map[string]*Unit{},
		Aircraft:  map[string]*AirUnit{},
	}
	state.Airfields["af"] = &Airfield{ID: "af", Faction: "A", Name: "凯恩斯机场", Coord: "0101", Type: AirfieldLarge, Runways: 3, Strength: 15, MaxStrength: 15, HangarCapacity: AirfieldHangarUnlimited}
	state.Units["cv"] = &Unit{ID: "cv", Faction: "A", Type: "CV", Name: "列克星敦", Coord: "0103", Strength: 4, MaxStrength: 4, Card: ShipCard{HangarCapacity: 24, Aircraft: 20}, Deck: &FlightDeck{Length: 8}}
	state.GroundAir["f1"] = &AirfieldAirUnit{ID: "f1", DefinitionID: "fighter", AirfieldID: "af", Faction: "A", Model: "F4F 野猫", Kind: "fighter", Status: "hangar", BaseRange: 9, MaxRange: 16, RemainingRange: 16}

	order := Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"f1"}, Coord: "0103"}
	if err := ValidateGroundAirOrder(state, order); err != nil {
		t.Fatalf("转场到有空位的航母应当被接受：%v", err)
	}

	plane := state.GroundAir["f1"]
	plane.Status = "transfer"
	plane.TaskID = "f1"
	plane.TaskKind = "transfer"
	plane.TargetCoord = "0103"
	plane.Coord = "0103"
	AdvanceGroundAir(&state)

	if state.Aircraft["f1"] == nil {
		t.Fatal("抵达航母后应当转成舰载机算子")
	}
	if state.GroundAir["f1"] != nil {
		t.Fatal("上舰后不该还留在机场算子表里")
	}
	if got := state.Aircraft["f1"]; got.CarrierID != "cv" || got.Status != "recovery" {
		t.Fatalf("上舰状态不对：carrier=%s status=%s", got.CarrierID, got.Status)
	}
	if state.Units["cv"].Card.Aircraft != 21 {
		t.Fatalf("母舰在舰机数应当 +1，实际 %d", state.Units["cv"].Card.Aircraft)
	}
}
