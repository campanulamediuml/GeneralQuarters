package game

import "testing"

// disableDCCheck 把 DC 判定固定成"必不过"：老回归测试依赖固定的随机数序列，
// 新增的 DC 掷骰会打乱它。测试结束自动恢复原始通过率。
func disableDCCheck(t *testing.T) {
	t.Helper()
	previous := dcCheckRate
	dcCheckRate = 0
	t.Cleanup(func() { dcCheckRate = previous })
}

// DC 判定：空战开始/投弹前各掷一次，通过率就是 50%，两边都不应该因为机种不同而偏移。
func TestDCCheckPassRate(t *testing.T) {
	state := State{RNGState: 20240619}
	passed := 0
	const rolls = 2000
	for i := 0; i < rolls; i++ {
		if dcCheck(&state) {
			passed++
		}
	}
	rate := float64(passed) / rolls * 100
	if rate < 45 || rate > 55 {
		t.Fatalf("DC 判定通过率应接近 50%%，实际 %.1f%%", rate)
	}
}

// 美军侦察基础值 +10%：同样的目标、同样的机型，只有阵营不同。
func TestUSReconBaseBonus(t *testing.T) {
	state := State{RNGState: 1, Year: 1944, Doctrines: map[string]string{"A": "us", "B": "japan"},
		Map: MapDefinition{Rows: 3, Cols: 5}}
	members := []Unit{{ID: "t1", FleetID: "target", Faction: "B", Type: "DD", Coord: "0104"}}
	us := &AirUnit{OriginCoord: "0101", SectorWidth: 60, Faction: "A"}
	jp := &AirUnit{OriginCoord: "0101", SectorWidth: 60, Faction: "B"}
	diff := reconDetectionChance(state, us, members) - reconDetectionChance(state, jp, members)
	if diff != 10 {
		t.Fatalf("美军侦察基础值应比日军高 10 点，实际差 %v", diff)
	}
	// 地面机场侦察走同一个字段。
	groundDiff := groundReconChance(state, &AirfieldAirUnit{Faction: "A", SectorWidth: 60, OriginCoord: "0101"}, members) -
		groundReconChance(state, &AirfieldAirUnit{Faction: "B", SectorWidth: 60, OriginCoord: "0101"}, members)
	if groundDiff != 10 {
		t.Fatalf("陆基侦察也应带美军 +10，实际差 %v", groundDiff)
	}
}
