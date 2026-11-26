package game

import "testing"

// TestMarianasAircraftStats 锁住马里亚纳的机型数值。
//
// units JSON 里的 range 是【机内油】基础航程，也就是不挂副油箱时的航程；
// 出击默认挂副油箱（×1.3），所以机库牌面上看到的是 22/18/27 这一档。
func TestMarianasAircraftStats(t *testing.T) {
	_, _, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	want := map[string]struct{ strength, baseRange, airCombat, antiShip, antiGround int }{
		"f6f":  {3, 17, 5, 0, 2},
		"sb2c": {2, 14, 2, 3, 1},
		"tbf":  {3, 14, 1, 2, 2},
		"fm2":  {3, 16, 2, 0, 1},
		"a6m5": {2, 21, 3, 0, 1},
		"d4y":  {1, 23, 2, 2, 1},
		"b6n":  {1, 23, 1, 2, 1},
		"h8k":  {2, 49, 0, 0, 1},
		"os2u": {1, 12, 1, 0, 1},
	}
	seen := map[string]bool{}
	for _, aircraft := range catalog.Air {
		w, ok := want[aircraft.ID]
		if !ok {
			continue
		}
		seen[aircraft.ID] = true
		got := struct{ strength, baseRange, airCombat, antiShip, antiGround int }{
			aircraft.Strength, aircraft.Range, aircraft.AirCombat, aircraft.AntiShip, aircraft.AntiGround,
		}
		if got != w {
			t.Fatalf("%s 数值为 %+v，期望 %+v", aircraft.ID, got, w)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("只找到 %d 种机型，期望 %d：%v", len(seen), len(want), seen)
	}

	// 副油箱航程：基础 ×1.3，四舍五入。零战 21→27，地狱猫 17→22，二式大艇 49→64。
	for id, wantRange := range map[string]int{"a6m5": 27, "f6f": 22, "h8k": 64, "tbf": 18, "os2u": 16} {
		def := catalogAirByID(t, catalog.Air, id)
		if got := MountedFuel(def.Range, LoadoutForTask(def.Kind, false, def.AntiGround, def.AntiShip, "cap", StrikeTargetShip)).Available(); got != wantRange {
			t.Fatalf("%s 挂副油箱后的航程=%d，期望 %d", id, got, wantRange)
		}
	}
}

func catalogAirByID(t *testing.T, defs []AircraftDefinition, id string) AircraftDefinition {
	t.Helper()
	for _, def := range defs {
		if def.ID == id {
			return def
		}
	}
	t.Fatalf("找不到机型 %s", id)
	return AircraftDefinition{}
}
