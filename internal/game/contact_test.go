package game

import "testing"

func TestContactProjectionLevelsAndPrivacy(t *testing.T) {
	members := []Unit{{ID: "bb", FleetID: "enemy", Type: "BB", Faction: "B", Coord: "0208", Heading: "SW", Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 16}}}
	for level := 1; level <= 4; level++ {
		state := State{Units: map[string]*Unit{}, Contacts: map[string]map[string]*FleetContact{
			"A": {"enemy": {FleetID: "enemy", Faction: "B", Level: level, Coord: "0208", Heading: "SW", Count: 12, Members: members}},
		}}
		units := projectedContactUnits(*state.Contacts["A"]["enemy"])
		expected := 12
		if level == 4 {
			expected = len(members)
		}
		if len(units) != expected {
			t.Fatalf("情报%d的舰队规模错误：%d，应为%d", level, len(units), expected)
		}
		for _, unit := range units {
			if unit.ContactLevel != level || unit.Heading != "SW" {
				t.Fatalf("情报%d没有保留等级和最后航向：%+v", level, unit)
			}
			if level < 4 && (unit.Type != "" || unit.Card.Gunnery != 0 || unit.Strength != 0) {
				t.Fatalf("低情报泄露具体舰船属性：%+v", unit)
			}
			if level == 4 && (unit.Type != "BB" || unit.Card.Gunnery != 16) {
				t.Fatalf("四级情报没有恢复详细卡片：%+v", unit)
			}
		}
		fleets := VisibleFleets(state, "A")
		expectedSize := "large"
		if level == 4 {
			expectedSize = "small"
		}
		if len(fleets) != 1 || fleets[0].ContactLevel != level || fleets[0].ContactSize != expectedSize {
			t.Fatalf("情报%d卡片投影错误：%+v", level, fleets)
		}
	}
}

func TestContactSizeCategories(t *testing.T) {
	for _, test := range []struct {
		count int
		size  string
	}{{4, "small"}, {5, "medium"}, {9, "medium"}, {10, "large"}} {
		state := State{Units: map[string]*Unit{}, Contacts: map[string]map[string]*FleetContact{
			"A": {"enemy": {FleetID: "enemy", Faction: "B", Level: 1, Coord: "0208", Count: test.count}},
		}}
		fleets := VisibleFleets(state, "A")
		if len(fleets) != 1 || fleets[0].ContactSize != test.size {
			t.Fatalf("规模%d应显示%s：%+v", test.count, test.size, fleets)
		}
	}
}
