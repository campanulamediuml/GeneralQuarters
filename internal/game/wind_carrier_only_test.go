package game

import "testing"

// 规则：风力只影响航母甲板。巡洋舰（水上飞机弹射/回收）、驱逐舰、机场和
// 水上飞机基地的起降都不吃风向修正，所以舰队投影里没有航母时 deck_modifier 必须是 0。
func TestWindDeckModifierOnlyForCarrierFleets(t *testing.T) {
	state := State{
		Wind: "SE",
		Units: map[string]*Unit{
			"ca1": {ID: "ca1", Faction: "A", Type: "CA", FleetID: "TFCA", Heading: "NE"},
			"dd1": {ID: "dd1", Faction: "A", Type: "DD", FleetID: "TFCA", Heading: "NE"},
			"cv1": {ID: "cv1", Faction: "A", Type: "CV", FleetID: "TFCV", Heading: "NE"},
		},
	}
	view := VisibleWind(state, "A")
	seen := map[string]FleetWind{}
	for _, fleet := range view.Fleets {
		seen[fleet.FleetID] = fleet
	}
	ca, ok := seen["TFCA"]
	if !ok {
		t.Fatalf("巡洋舰队没有出现在风向投影里：%+v", view.Fleets)
	}
	if ca.HasCarrier {
		t.Fatal("巡洋舰队不该被标记成有航母")
	}
	if ca.DeckModifier != 0 {
		t.Fatalf("没有航母的舰队不该有甲板修正，实际 %+d", ca.DeckModifier)
	}
	cv, ok := seen["TFCV"]
	if !ok {
		t.Fatal("航母舰队没有出现在风向投影里")
	}
	if !cv.HasCarrier || cv.DeckModifier == 0 {
		t.Fatalf("航母舰队应该带甲板修正：%+v", cv)
	}
	// 非航母的甲板长度也不随风向变化。
	ca1 := state.Units["ca1"]
	ca1.Deck = &FlightDeck{Length: 1, BaseLength: 1}
	if got := deckLengthFor(state, ca1); got != 1 {
		t.Fatalf("巡洋舰甲板长度不该被风向修正，实际 %d", got)
	}
}
