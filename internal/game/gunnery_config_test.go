package game

import "testing"

// 炮击值以单位库 JSON 为准：写了 gunnery 就照用，没写才按口径×炮塔数折算。
func TestGunneryComesFromUnitJSON(t *testing.T) {
	_, gameMap, catalog, err := LoadScenario("../../content", "_coral")
	if err != nil {
		t.Fatal(err)
	}
	// 找一条卡：口径×炮塔数折算出来的值一定和显式 gunnery 不同，用来分辨"读 JSON"还是"算出来的"。
	index := -1
	for i := range catalog.Units {
		u := catalog.Units[i]
		derived := int(u.Card.MainGunCaliber * float64(u.Card.MainGunCount) / 9)
		if u.Card.Gunnery > 0 && u.Card.Gunnery != derived {
			index = i
			break
		}
	}
	if index < 0 {
		t.Skip("单位库里没有 gunnery 与折算值不同的卡，改脚本改数据后此测试才有意义")
	}
	definition := catalog.Units[index]
	scenario := ScenarioDefinition{
		ID: "gunnery-test", Name: "炮击值测试", Year: 1942,
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Deployments: []Deployment{{
			ID: "ship1", Unit: definition.ID, Faction: definition.Faction, Name: "测试舰", Coord: "0602",
		}},
	}
	state, err := NewState(scenario, gameMap, catalog, 1)
	if err != nil {
		t.Fatal(err)
	}
	unit := state.Units["ship1"]
	if unit == nil {
		t.Fatal("部署没有生成舰船")
	}
	if unit.Card.Gunnery != definition.Card.Gunnery {
		t.Fatalf("炮击值没有按单位库读取：得到 %d，JSON 是 %d", unit.Card.Gunnery, definition.Card.Gunnery)
	}
}
