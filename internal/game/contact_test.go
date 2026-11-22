package game

import "testing"

// TestContactProjectionLevelsAndPrivacy 情报等级决定投影内容：
// 1 级只给一个算子、2 级只给编队大小、3 级给船只类型、4 级给完整舰船卡（规则书 142–147 行）。
func TestContactProjectionLevelsAndPrivacy(t *testing.T) {
	members := []Unit{{ID: "bb", FleetID: "enemy", Type: "BB", Faction: "B", Coord: "0208", Heading: "SW", Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 16}}}
	for level := 1; level <= 4; level++ {
		state := State{Units: map[string]*Unit{}, Contacts: map[string]map[string]*FleetContact{
			"A": {"enemy": {FleetID: "enemy", Faction: "B", Level: level, Coord: "0208", Heading: "SW", Count: 12, Members: members}},
		}}
		units := projectedContactUnits(*state.Contacts["A"]["enemy"])
		wantCount := 1
		if level == 3 {
			wantCount = 12
		}
		if len(units) != wantCount {
			t.Fatalf("情报%d的算子数量错误：%d，应为%d", level, len(units), wantCount)
		}
		for _, unit := range units {
			if unit.ContactLevel != level || unit.Heading != "SW" {
				t.Fatalf("情报%d没有保留等级和最后航向：%+v", level, unit)
			}
			if level <= 2 && unit.Type != "" {
				t.Fatalf("情报%d不应暴露舰种：%+v", level, unit)
			}
			if level < 4 && (unit.Card.Gunnery != 0 || unit.Strength != 0) {
				t.Fatalf("情报%d泄露具体属性：%+v", level, unit)
			}
			if level == 4 && (unit.Type != "BB" || unit.Card.Gunnery != 16) {
				t.Fatalf("四级情报没有恢复详细卡片：%d %+v", level, unit)
			}
		}
		fleets := VisibleFleets(state, "A")
		if len(fleets) != 1 || fleets[0].ContactLevel != level {
			t.Fatalf("情报%d卡片投影错误：%+v", level, fleets)
		}
		wantSize := "large"
		if level == 1 {
			wantSize = ""
		}
		if level == 4 {
			wantSize = "small"
		}
		if fleets[0].ContactSize != wantSize {
			t.Fatalf("情报%d的规模显示应为%q，实际%q", level, wantSize, fleets[0].ContactSize)
		}
	}
}

// TestContactProjectionHidesShipTypesBelowLevel3 3 级才允许显示舰种：0-2 级都是未知。
func TestContactProjectionHidesShipTypesBelowLevel3(t *testing.T) {
	members := []Unit{
		{ID: "cv", FleetID: "enemy", Type: "CV", Faction: "B", Coord: "0208"},
		{ID: "dd", FleetID: "enemy", Type: "DD", Faction: "B", Coord: "0208"},
	}
	contact := FleetContact{FleetID: "enemy", Faction: "B", Level: 3, Coord: "0208", Count: 2, Members: members}
	units := projectedContactUnits(contact)
	if len(units) != 2 || units[0].Type != "CV" || units[1].Type != "DD" {
		t.Fatalf("3 级情报应显示船只类型：%+v", units)
	}
	contact.Level = 2
	units = projectedContactUnits(contact)
	if len(units) != 1 || units[0].Type != "" {
		t.Fatalf("2 级情报不应显示舰种和数量：%+v", units)
	}
	state := State{Units: map[string]*Unit{}, Contacts: map[string]map[string]*FleetContact{"A": {"enemy": &contact}}}
	fleets := VisibleFleets(state, "A")
	if len(fleets) != 1 || fleets[0].ContactSize != "small" {
		t.Fatalf("2 级情报应显示编队大小：%+v", fleets)
	}
}

func TestContactSizeCategories(t *testing.T) {
	for _, test := range []struct {
		count int
		size  string
	}{{1, "small"}, {2, "small"}, {3, "medium"}, {5, "medium"}, {6, "large"}, {10, "large"}} {
		// 编队大小是 2 级情报才显示的内容，1 级只有算子。
		state := State{Units: map[string]*Unit{}, Contacts: map[string]map[string]*FleetContact{
			"A": {"enemy": {FleetID: "enemy", Faction: "B", Level: 2, Coord: "0208", Count: test.count}},
		}}
		fleets := VisibleFleets(state, "A")
		if len(fleets) != 1 || fleets[0].ContactSize != test.size {
			t.Fatalf("规模%d应显示%s：%+v", test.count, test.size, fleets)
		}
	}
}
