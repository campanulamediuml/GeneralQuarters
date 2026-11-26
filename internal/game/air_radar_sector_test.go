package game

import "testing"

// 回归：侦察机对"敌方飞行编队"的发现范围必须是航向扇面，不是以顶点为圆心的整圆。
// 以前那段判定是 Distance(origin, coord) <= ReconDistance，一架向东飞 6 格的侦察机
// 会把西边 6 格内的空域也算成"搜过了"——敌方编队隔着十几个 hex 就暴露位置。
func TestReconAirCoverageRespectsSector(t *testing.T) {
	gameMap := MapDefinition{ID: "t", Rows: 20, Cols: 20}
	state := State{
		Map: gameMap,
		Aircraft: map[string]*AirUnit{
			"recon": {
				ID: "recon", Faction: "A", Kind: "recon", Model: "SBD", Status: "recon",
				OriginCoord: "1010", Coord: "1016", Heading: "E", SectorWidth: 60,
				ReconRadius: 8, ReconDistance: 6,
			},
		},
	}
	covered := reconCoveredCells(state, "A")
	if !covered["1013"] {
		t.Fatalf("正前方扇面内的 1013 应该被覆盖")
	}
	for _, behind := range []string{"1005", "1006", "1007"} {
		if covered[behind] {
			t.Fatalf("%s 在侦察机背后，不该被算成已搜索", behind)
		}
	}
}

// 回归：机场雷达用机场自己的表，不能套舰艇表。
func TestAirfieldRadarTable(t *testing.T) {
	state := State{Doctrines: map[string]string{"A": "us", "B": "japan"}}
	if got := airfieldRadarChance(state, "A", 1); got != 100 {
		t.Fatalf("美军机场 1 hex 发现率应为 100%%，实际 %.0f", got)
	}
	if got := airfieldRadarChance(state, "A", 5); got != 20 {
		t.Fatalf("美军机场 5 hex 发现率应为 20%%，实际 %.0f", got)
	}
	if got := airfieldRadarChance(state, "B", 5); got != 0 {
		t.Fatalf("日军机场 5 hex 发现率应为 0%%，实际 %.0f", got)
	}
	if got := airfieldRadarChance(state, "B", 1); got != 80 {
		t.Fatalf("日军机场 1 hex 发现率应为 80%%，实际 %.0f", got)
	}
}
