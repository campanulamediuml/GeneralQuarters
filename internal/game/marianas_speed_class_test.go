package game

import "testing"

// TestMarianasSpeedClasses 校验马里亚纳的速度分档：35 节以上 fast、25–35 节 normal、
// 25 节以下 slow。移动上 fast 和 normal 都是 3 回合走一格，fast 的差别只在
// 被鱼雷 -10% / 被空袭 -5% 的命中修正（外加纯高速驱逐舰队 2 回合）。
//
// 这条分档的直接后果：二战能跑 35 节以上基本只有驱逐舰和日本那批 35 节级巡洋舰，
// 所以航母、战列舰（包括依阿华 33 节、金刚级 30 节）全部是 normal，
// 扶桑/山城 24.7 节按 25 节取整，同样算 normal（3 回合走一格）。
func TestMarianasSpeedClasses(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	want := map[string]string{
		// 35 节以上：驱逐舰和日本轻/重巡
		"妙高":     "fast",
		"高雄":     "fast",
		"利根":     "fast",
		"最上":     "fast",
		"川内":     "fast",
		"矢矧":     "fast",
		"弗莱彻号01": "fast",
		// 25–35 节：全部航母、战列舰、美军巡洋舰
		"赤城":   "normal",
		"加贺":   "normal",
		"苍龙":   "normal",
		"翔鹤":   "normal",
		"大凤":   "normal",
		"隼鹰":   "normal",
		"龙凤":   "normal",
		"信浓":   "normal",
		"千岁":   "normal",
		"瑞凤":   "normal",
		"埃塞克斯": "normal",
		"依阿华":  "normal",
		"金刚":   "normal",
		"大和":   "normal",
		"陆奥":   "normal",
		"巴尔的摩": "normal",
		// 25 节以下：美军老式战列舰、护航航母、运输船
		"田纳西":   "slow",
		"卡萨布兰卡": "slow",
	}

	seen := map[string]bool{}
	for _, unit := range state.Units {
		want, ok := want[unit.Name]
		if !ok {
			continue
		}
		seen[unit.Name] = true
		if unit.Card.SpeedClass != want {
			t.Fatalf("%s 速度档为 %s，期望 %s", unit.Name, unit.Card.SpeedClass, want)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("没找到 %s", name)
		}
	}

	// 五支任务群按现实航速走、再按编队规模扣速，各自不同：
	// 甲 25（信浓拖的）、乙 26（加贺）、丙 24（龙凤）、丁 22（扶桑/山城）、栗田 23（伊势/日向）。
	wantFleetKnots := map[string]int{"TG1": 25, "TG2": 26, "TG3": 24, "TG4": 22, "TG5": 23}
	for fleet, want := range wantFleetKnots {
		id := firstUnitOfFleet(state, fleet)
		if id == "" {
			t.Fatalf("找不到舰队 %s", fleet)
		}
		if knots := FleetCycleKnots(state, state.Units[id]); knots != want {
			t.Fatalf("%s 航速 %d 节，期望 %d", fleet, knots, want)
		}
	}
}
