package game

import "testing"

// 接敌失败后的"沿航向继续搜索"也要吃掉本回合的移动预算（规则书 196 行：每回合 2 hex）。
// 以前赶路 2 格之后还会再搜索移动 1 格，编队一回合能飞出 3 格——实测珊瑚海第 123 回合
// 有 6 架 B 方攻击机在同一个回合里从 1931 跳到 2029。
func TestFailedEngageSearchDoesNotExceedTwoHexes(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	carrier := state.Units["b-cv-01"]
	if carrier == nil {
		t.Fatal("找不到翔鹤")
	}
	start := carrier.Coord
	// 找一条两格的水域航线：start → mid → target。
	mid, target := "", ""
	for _, first := range Neighbors(start) {
		if terrainAt(state.Map, first) != "water" {
			continue
		}
		for _, second := range Neighbors(first) {
			if second == start || terrainAt(state.Map, second) != "water" {
				continue
			}
			mid, target = first, second
			break
		}
		if mid != "" {
			break
		}
	}
	if target == "" {
		t.Fatal("找不到两格的水域航线")
	}

	plane := &AirUnit{ID: "budget-strike", CarrierID: carrier.ID, Faction: "B", Kind: "dive_bomber", Model: "D3A1",
		Status: "attack", Strength: 1, AirCombat: 1, AntiShip: 3,
		TaskID: "budget-strike", TaskKind: "attack",
		Coord: start, OriginCoord: start, TargetCoord: target}
	plane.SetRange(20)
	state.Aircraft = map[string]*AirUnit{plane.ID: plane}
	state.Contacts = nil

	AdvanceAviation(&state)
	if moved := Distance(start, plane.Coord); moved > 2 {
		t.Fatalf("一回合移动了 %d 格（上限 2）：%s → %s（中途 %s）", moved, start, plane.Coord, mid)
	}
}
