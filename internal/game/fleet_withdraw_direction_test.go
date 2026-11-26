package game

import "testing"

// TestMarianasWithdrawDirections 马里亚纳的撤退走廊是分阵营的：美军（含登陆运输队）往地图最东边走，
// 日军往西边走；而且退避队按自己的航速走，不是每回合无条件挪一格——慢速运输队会被追上。
func TestMarianasWithdrawDirections(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	// 挑离边缘还有一段距离的舰队：出生点是部署区内随机的，运气不好会刷在地图边上。
	pickMidMap := func(wantFaction string) string {
		for _, id := range sortedUnitIDs(state.Units) {
			unit := state.Units[id]
			if unit == nil || unit.Faction != wantFaction || unit.Withdrawing {
				continue
			}
			_, col, err := ParseCoord(unit.Coord)
			if err != nil || col < 5 || col > state.Map.Cols-5 {
				continue
			}
			return id
		}
		return ""
	}
	usFleet := pickMidMap("A")
	jpFleet := pickMidMap("B")
	if usFleet == "" || jpFleet == "" {
		t.Fatalf("找不到合适的美军/日军舰队：%q / %q", usFleet, jpFleet)
	}

	assertDirection := func(unitID, wantHeading string, wantColDelta int) {
		unit := state.Units[unitID]
		if unit == nil {
			t.Fatalf("找不到 %s", unitID)
		}
		_, col, err := ParseCoord(unit.Coord)
		if err != nil {
			t.Fatalf("%s 坐标无效：%v", unit.Name, err)
		}
		unit.Withdrawing = true
		advanceTowardEdge(&state)
		moved := state.Units[unitID]
		if moved == nil {
			t.Fatalf("%s 在第一格就撤出地图了，测试选的出生点不对", unit.Name)
		}
		_, nextCol, err := ParseCoord(moved.Coord)
		if err != nil {
			t.Fatalf("%s 撤离后坐标无效：%v", unit.Name, err)
		}
		if delta := nextCol - col; delta*wantColDelta <= 0 {
			t.Fatalf("%s 撤离方向不对：col %d → %d（期望朝%s）", unit.Name, col, nextCol, wantHeading)
		}
		// 冷却没走完之前不能再动：退避队也得按每 N 回合 1 hex 的航速。
		if moved.MoveCooldown <= 0 {
			t.Fatalf("%s 撤离后没有挂冷却，慢速舰队会被当成飞机用", unit.Name)
		}
		before := moved.Coord
		advanceTowardEdge(&state)
		if unit := state.Units[unitID]; unit != nil && unit.Coord != before {
			t.Fatalf("%s 冷却期内又移动了一格", unit.Name)
		}
	}

	assertDirection(usFleet, "E", 1)
	assertDirection(jpFleet, "W", -1)

	// 一直撤下去应该真的走到地图边缘并被移除，而不是在地图中间打转。
	if state.Units[usFleet] == nil {
		t.Fatal("美军退避队不见了")
	}
	for turn := 0; turn < state.Map.Cols*10 && state.Units[usFleet] != nil; turn++ {
		AdvanceMovementCooldown(&state)
		advanceTowardEdge(&state)
	}
	if state.Units[usFleet] != nil {
		t.Fatalf("美军退避队没有撤出地图：%s", state.Units[usFleet].Coord)
	}
}
