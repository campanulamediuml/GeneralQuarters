package game

import "testing"

// 珊瑚海 1919/1920/1924/1925 暗礁、1921 浅滩、1922 开阔水面的狭窄水道：
// 暗礁谁都进不去，浅滩只挡含航母的编队，所以运输船有 1921、1922 两条路，
// 航母编队只剩 1922 一个口子。这条规则完全靠地形表实现，不需要额外代码。
func TestCoralStraitPassage(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	wantTerrain := map[string]Terrain{
		"1919": TerrainReef,
		"1920": TerrainReef,
		"1921": TerrainShoal,
		"1922": TerrainWater,
		"1924": TerrainReef,
		"1925": TerrainReef,
	}
	for coord, want := range wantTerrain {
		if got := terrainAt(state.Map, coord); got != want {
			t.Fatalf("%s 地形是 %s，期望 %s", coord, got, want)
		}
	}

	// 运输船（不含航母的编队）：1921、1922 都能过。
	for _, coord := range []string{"1921", "1922"} {
		if !fleetTerrainAllowsEntry(state.Map, coord, false, false) {
			t.Fatalf("运输队进不了 %s", coord)
		}
	}
	// 航母编队：只有 1922 能过，浅滩 1921 挡下。
	if !fleetTerrainAllowsEntry(state.Map, "1922", true, false) {
		t.Fatal("航母编队进不了 1922，水道断头了")
	}
	if fleetTerrainAllowsEntry(state.Map, "1921", true, false) {
		t.Fatal("航母编队可以进浅滩 1921，浅滩没拦住航母")
	}
	// 暗礁对谁都是死路。
	for _, coord := range []string{"1919", "1920", "1924", "1925"} {
		if fleetTerrainAllowsEntry(state.Map, coord, false, false) {
			t.Fatalf("运输队能进暗礁 %s", coord)
		}
		if fleetTerrainAllowsEntry(state.Map, coord, true, false) {
			t.Fatalf("航母编队能进暗礁 %s", coord)
		}
	}
}
