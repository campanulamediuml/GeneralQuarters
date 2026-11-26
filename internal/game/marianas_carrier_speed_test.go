package game

import "testing"

// TestMarianasJapaneseCarrierSpeeds 校验日军六艘舰队航母的速度档。
//
// 隼鹰/飞鹰（橿原丸级改建，25.5 节）和龙凤（大鲸改建，26.5 节）都不是慢速船：
// 游戏里 slow = 每 6 回合走一格 ≈ 12.5 节，把 25 节级的船扔进 slow 会让乙部队
// 比甲部队慢一倍，还白吃空袭 +10% 命中修正。大凤 33.3 节则按 fast 记，
// fast/normal 移动间隔相同，差别只在空袭命中 -10%。
func TestMarianasJapaneseCarrierSpeeds(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	want := map[string]string{
		"大凤": "fast",
		"翔鹤": "fast",
		"瑞鹤": "fast",
		"隼鹰": "normal",
		"飞鹰": "normal",
		"龙凤": "normal",
	}
	seen := map[string]bool{}
	for _, unit := range state.Units {
		if unit.Type != "CV" {
			continue
		}
		speed, ok := want[unit.Name]
		if !ok {
			continue
		}
		seen[unit.Name] = true
		if unit.Card.SpeedClass != speed {
			t.Fatalf("%s 速度档为 %s，期望 %s", unit.Name, unit.Card.SpeedClass, speed)
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("只找到 %d 艘目标航母，期望 %d：%v", len(seen), len(want), seen)
	}

	// 三个机动部队现在同速：每 3 回合走一格，不会再出现乙部队 6 回合、甲部队 3 回合的落差。
	for _, fleet := range []string{"TG1", "TG3", "TG4"} {
		id := firstUnitOfFleet(state, fleet)
		if id == "" {
			t.Fatalf("找不到舰队 %s", fleet)
		}
		if interval := FleetMoveInterval(state, state.Units[id]); interval != 3 {
			t.Fatalf("%s 每 %d 回合移动一格，期望 3", fleet, interval)
		}
	}
}
