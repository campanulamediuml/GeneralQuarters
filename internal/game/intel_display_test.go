package game

import (
	"sort"
	"testing"
)

// TestIntelDisplayCoordDoesNotMoveWhenIntelDecays 回归：情报衰减只降等级，
// 地图上的"最后已知位置"必须留在原地（规则书 593 行）。
// 之前的散列把等级算进种子，每掉一级标记就跳到另一个偏移，打击编队会追着幻影来回折返。
func TestIntelDisplayCoordDoesNotMoveWhenIntelDecays(t *testing.T) {
	state := sandboxState(t)
	state.Contacts = map[string]map[string]*FleetContact{"A": {}}
	fleetID, members := enemyFleetForTest(&state)
	if fleetID == "" || len(members) == 0 {
		t.Fatal("沙盒剧本应有敌方舰队")
	}
	// 情报一次只升一级：连续四个回合发现才到 4 级（同回合多个侦查手段也只升一级）。
	for level := 0; level < 4; level++ {
		recordContact(&state, "A", fleetID, members)
		advanceTurnStart(&state)
	}
	contact := state.Contacts["A"][fleetID]
	if contact.Level != 4 {
		t.Fatalf("连续四次发现应升到 4 级情报：%+v", contact)
	}
	settled := intelDisplayCoord(*contact)
	if settled != contact.Coord {
		t.Fatalf("4 级情报必须给出准确位置：显示 %s，真实 %s", settled, contact.Coord)
	}
	for _, level := range []int{3, 2, 1} {
		contact.Level = level
		if got := intelDisplayCoord(*contact); got != settled {
			t.Fatalf("情报衰减到 %d 级后显示位置不应改变：%s → %s", level, settled, got)
		}
	}
}

// enemyFleetForTest 返回沙盒剧本里任意一支敌方舰队及其成员。
func enemyFleetForTest(state *State) (string, []Unit) {
	ids := make([]string, 0, len(state.Units))
	for id, unit := range state.Units {
		if unit.Faction != "A" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return "", nil
	}
	fleetID := FleetKey(state.Units[ids[0]])
	return fleetID, fleetMembers(state, fleetID)
}

// TestFreshLowIntelContactProjectsAnOffset 1 级情报必须有位置误差，且多次投影结果一致。
func TestFreshLowIntelContactProjectsAnOffset(t *testing.T) {
	state := sandboxState(t)
	state.Contacts = map[string]map[string]*FleetContact{"A": {}}
	fleetID, members := enemyFleetForTest(&state)
	recordContact(&state, "A", fleetID, members)
	contact := state.Contacts["A"][fleetID]
	if contact.Level != 1 {
		t.Fatalf("首次发现应为 1 级情报：%+v", contact)
	}
	first := intelDisplayCoord(*contact)
	if first == contact.Coord {
		t.Fatalf("1 级情报不应给出准确位置：%+v", contact)
	}
	if again := intelDisplayCoord(*contact); again != first {
		t.Fatalf("同一份情报的显示位置必须稳定：%s → %s", first, again)
	}
}
