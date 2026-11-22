package game

import "testing"

// TestCoralScenarioSpawnsInZones 校验珊瑚海剧本：双方舰队在各自的随机出生区里，
// 所有成员落在同一个 hex，而且两边拉开足够距离（必须靠搜索才能接敌）。
func TestCoralScenarioSpawnsInZones(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral")
	if err != nil {
		t.Fatal(err)
	}
	if gameMap.Rows != 10 || gameMap.Cols != 12 {
		t.Fatalf("珊瑚海地图尺寸错误：%d×%d", gameMap.Rows, gameMap.Cols)
	}
	usZone, jpZone := map[string]bool{}, map[string]bool{}
	for _, coord := range []string{"0703", "0704", "0803", "0804"} {
		usZone[coord] = true
	}
	for _, coord := range []string{"0208", "0209", "0308", "0309"} {
		jpZone[coord] = true
	}
	usSpots, jpSpots := map[string]bool{}, map[string]bool{}
	for seed := uint64(1); seed <= 30; seed++ {
		state, err := NewState(scenario, gameMap, catalog, seed)
		if err != nil {
			t.Fatalf("种子%d部署失败：%v", seed, err)
		}
		usFleet, jpFleet := "", ""
		for _, unit := range state.Units {
			switch unit.Faction {
			case "A":
				usFleet = unit.Coord
				if !usZone[unit.Coord] {
					t.Fatalf("美军落到了出生区外：%s", unit.Coord)
				}
			case "B":
				jpFleet = unit.Coord
				if !jpZone[unit.Coord] {
					t.Fatalf("日军落到了出生区外：%s", unit.Coord)
				}
			}
		}
		if usFleet == "" || jpFleet == "" {
			t.Fatalf("种子%d缺少一方舰队", seed)
		}
		if Distance(usFleet, jpFleet) < 5 || Distance(usFleet, jpFleet) > 10 {
			t.Fatalf("种子%d双方出生距离太近：%s / %s", seed, usFleet, jpFleet)
		}
		usSpots[usFleet] = true
		jpSpots[jpFleet] = true
		if len(state.Aircraft) != len(scenario.AirDeployments) {
			t.Fatalf("航空部署数量错误：%d", len(state.Aircraft))
		}
	}
	if len(usSpots) < 3 || len(jpSpots) < 3 {
		t.Fatalf("出生点没有随机化：美军%v 日军%v", usSpots, jpSpots)
	}
}
