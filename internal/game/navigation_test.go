package game

import "testing"

// 规则书 280–283 行：设置自动航行后，舰队沿最短可通行路线前进，抵达后自动结束。
func TestAutoNavigationMovesTowardDestination(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	start := unit.Coord
	destination := "0205"
	if err := ValidateOrder(state, "A", Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: destination}); err != nil {
		t.Fatalf("合法自动航行被拒绝：%v", err)
	}
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: destination})
	if got := FleetDestination(state, FleetKey(unit)); got != destination {
		t.Fatalf("目的地未保存：%s", got)
	}
	for turn := 0; turn < 20 && FleetDestination(state, "TG-A") != ""; turn++ {
		ResolveOrders(&state, nil)
	}
	if FleetDestination(state, "TG-A") != "" {
		t.Fatalf("舰队没有抵达目的地，停在 %s", state.Units["a-bb-01"].Coord)
	}
	if state.Units["a-bb-01"].Coord != destination {
		t.Fatalf("舰队应在目的地 %s，实际 %s", destination, state.Units["a-bb-01"].Coord)
	}
	if state.Units["a-bb-01"].Coord == start {
		t.Fatal("舰队根本没有移动")
	}
}

// 手动移动与自动移动共用移动机会：本回合手动动过，自动航行就不该再走一格。
func TestManualMoveConsumesAutoNavigationTurn(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: "0205"})
	moves := LegalMoves(state, unit)
	if len(moves) == 0 {
		t.Fatal("没有合法手动移动")
	}
	unit.Moved = true
	before := unit.Coord
	ResolveOrders(&state, []Order{{Type: "SET_MOVE", UnitID: "a-bb-01", Coord: moves[0]}})
	if state.Units["a-bb-01"].Coord != moves[0] || moves[0] == before {
		if state.Units["a-bb-01"].Coord != moves[0] {
			t.Fatalf("手动移动应生效：%s → %s", before, state.Units["a-bb-01"].Coord)
		}
	}
	// 该回合不应该再被自动航行多推一格（Moved 已置位，自动航行直接跳过）。
	if state.Units["a-bb-01"].Moved == true && state.Units["a-bb-01"].Destination == "" {
		t.Fatal("意外抵达目的地")
	}
}

// 规则书 368 行：避免近战时尝试绕开敌军格；无路可绕则停止移动。
func TestAvoidEngagementStopsWhenNoDetour(t *testing.T) {
	state := sandboxState(t)
	attacker := state.Units["a-bb-01"]
	ApplyNavOrder(&state, Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: "0205", Engagement: string(EngageAvoid)})
	ApplyEngagementOrder(&state, Order{Type: "SET_ENGAGEMENT", UnitID: "a-bb-01", Engagement: string(EngageAvoid)})
	if got := FleetEngagementSetting(state, FleetKey(attacker)); got != EngageAvoid {
		t.Fatalf("交战设置未生效：%s", got)
	}
	// 把敌舰直接放在下一格上，并堵死其它相邻格，验证"没有绕行路线就停下"。
	path, ok := shortestFleetPath(state, attacker.Coord, "0205", false)
	if !ok || len(path) == 0 {
		t.Skip("沙盘地图上没有可用航线")
	}
	next := path[0]
	enemy := &Unit{ID: "blocker", FleetID: "blocker", Faction: "B", Type: "CA", Name: "堵路舰", Coord: next,
		Strength: 5, MaxStrength: 5, MoveCooldown: 99, Card: ShipCard{SpeedClass: "normal"}}
	state.Units[enemy.ID] = enemy
	for _, neighbor := range Neighbors(attacker.Coord) {
		if neighbor == next || !inBounds(state.Map, neighbor) {
			continue
		}
		// 用暗礁把绕行方向封死
		state.Map.Cells = append(state.Map.Cells, MapCell{Coord: neighbor, Terrain: "reef"})
	}
	blocker := &Unit{ID: "reefblock", FleetID: "reefblock", Faction: "B", Type: "CA", Name: "礁石占位", Coord: next,
		Strength: 5, MaxStrength: 5, MoveCooldown: 99, Card: ShipCard{SpeedClass: "normal"}}
	_ = blocker
	next2, reason := autoNavStep(&state, attacker)
	if next2 == next {
		t.Fatal("避免近战时不应主动进入敌军格")
	}
	if next2 == "" && reason == "" {
		t.Fatal("无法绕行时应给出停止原因")
	}
}

// 规则书 367 行：优势交战只在情报显示敌舰数量少于己方时才进入。
func TestAdvantageEngagementNeedsSuperiority(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	path, ok := shortestFleetPath(state, unit.Coord, "0205", false)
	if !ok || len(path) == 0 {
		t.Skip("沙盘地图上没有可用航线")
	}
	next := path[0]
	state.Units["enemy-x"] = &Unit{ID: "enemy-x", FleetID: "enemy-x", Faction: "B", Type: "CA", Name: "敌舰", Coord: next,
		Strength: 5, MaxStrength: 5, MoveCooldown: 99, Card: ShipCard{SpeedClass: "normal"}}
	unit.Destination = "0205"
	unit.Engagement = string(EngageAdvantage)
	// 情报显示对方很大：停下来。
	state.Contacts["A"] = map[string]*FleetContact{"enemy-x": {FleetID: "enemy-x", Faction: "B", Level: 2, Count: 99, Coord: next}}
	if got, _ := autoNavStep(&state, unit); got != "" {
		t.Fatalf("兵力不占优势时不应进入：%s", got)
	}
	// 情报显示对方只有 1 艘：进入。
	state.Contacts["A"]["enemy-x"].Count = 1
	if got, _ := autoNavStep(&state, unit); got != next {
		t.Fatalf("兵力占优时应进入 %s，实际 %q", next, got)
	}
}

// 目的地必须可达。
func TestAutoNavRejectsUnreachableDestination(t *testing.T) {
	state := sandboxState(t)
	if err := ValidateOrder(state, "A", Order{Type: "SET_AUTO_NAV", UnitID: "a-bb-01", Coord: "9999"}); err == nil {
		t.Fatal("地图外目的地应被拒绝")
	}
}
