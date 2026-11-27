package game

import "testing"

// 马里亚纳日军的两个部署区：甲/乙/丙三支航母群在地图西北角（1023 周围 6 hex），
// 丁部队和栗田的第一游击部队在西南、帕劳以北（4221 周围 6 hex）。
// 出生点必须是舰队能进去的水域：不能是岛屿/暗礁，航母编队也不能落在浅滩。
func TestMarianasJapaneseSpawnInZones(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	for _, seed := range []uint64{1, 42, 777, 20261127} {
		checkJapaneseSpawns(t, scenario, gameMap, catalog, seed)
	}
}

func checkJapaneseSpawns(t *testing.T, scenario ScenarioDefinition, gameMap MapDefinition, catalog UnitCatalog, seed uint64) {
	t.Helper()
	state, err := NewState(scenario, gameMap, catalog, seed)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}
	want := map[string]string{
		"TG1": "1023",
		"TG2": "1023",
		"TG3": "1023",
		"TG4": "4221",
		"TG5": "4221",
	}
	carriers := map[string]bool{"TG1": true, "TG2": true, "TG3": true, "TG4": true}
	byFleet := map[string][]*Unit{}
	for _, unit := range state.Units {
		byFleet[unit.FleetID] = append(byFleet[unit.FleetID], unit)
	}
	occupied := map[string]string{}
	for fleet, anchor := range want {
		units := byFleet[fleet]
		if len(units) == 0 {
			t.Fatalf("找不到舰队 %s", fleet)
		}
		coord := units[0].Coord
		if Distance(coord, anchor) > 6 {
			t.Fatalf("%s 出生在 %s，离锚点 %s 超过 6 hex", fleet, coord, anchor)
		}
		for _, unit := range units {
			if unit.Coord != coord {
				t.Fatalf("%s 的成员没有挤在同一格：%s vs %s", fleet, unit.Coord, coord)
			}
		}
		switch terrainAt(gameMap, coord) {
		case TerrainIsland, TerrainReef:
			t.Fatalf("%s 出生在不能进入的 %s（%s）", fleet, coord, terrainAt(gameMap, coord))
		}
		if carriers[fleet] && terrainAt(gameMap, coord) == TerrainShoal {
			t.Fatalf("%s 是航母编队，不能出生在浅滩 %s", fleet, coord)
		}
		if other, ok := occupied[coord]; ok {
			t.Fatalf("%s 和 %s 挤在同一个出生格 %s", fleet, other, coord)
		}
		occupied[coord] = fleet
	}
}
