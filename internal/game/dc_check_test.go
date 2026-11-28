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

// 普通机组的 DC 判定：空战开始/投弹前各掷一次，2 骰取劣后通过率为 25%。
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
	if rate < 22 || rate > 28 {
		t.Fatalf("普通机组 DC 判定通过率应接近 25%%，实际 %.1f%%", rate)
	}
}

// 熟练度决定 DC 掷骰的档位：新手无加成、普通 25%、老兵单骰 50%、王牌 2 骰取优 75%。
func TestDCCheckByPilotSkill(t *testing.T) {
	cases := map[string]float64{
		PilotRookie:  0,
		PilotRegular: 25,
		PilotVeteran: 50,
		PilotAce:     75,
	}
	for skill, want := range cases {
		state := State{RNGState: 424242}
		passed := 0
		const rolls = 4000
		for i := 0; i < rolls; i++ {
			if dcCheckWithSkill(&state, skill) {
				passed++
			}
		}
		rate := float64(passed) / rolls * 100
		if rate < want-3 || rate > want+3 {
			t.Fatalf("%s 的 DC 通过率应接近 %.1f%%，实际 %.1f%%", PilotSkillLabel(skill), want, rate)
		}
	}
}

func TestRookieNeverReceivesDCBonus(t *testing.T) {
	previous := dcCheckRate
	dcCheckRate = 100
	t.Cleanup(func() { dcCheckRate = previous })

	state := State{RNGState: 7}
	before := state.RNGState
	if dcCheckWithSkill(&state, PilotRookie) {
		t.Fatal("即使其他 DC 判定必过，新手也不应获得临场加成")
	}
	if state.RNGState != before {
		t.Fatal("新手无加成不应消耗随机数")
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
