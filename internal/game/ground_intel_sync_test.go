package game

import "testing"

// 情报刷新时必须把陆基打击编队的目标一起更新：它们住在 GroundAir 里，
// 以前这段只扫 state.Aircraft，陆攻会一直对着出航时那一格 hex 飞。
func TestContactRefreshMovesGroundStrikeTarget(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	// 一支美军舰队，以及"我"（日军）对它的 4 级情报。
	fleet := firstEnemyShip(&state, "A")
	if fleet == nil {
		t.Fatal("找不到敌方舰队")
	}
	fleetID := FleetKey(fleet)
	state.Contacts = map[string]map[string]*FleetContact{
		"B": {fleetID: {FleetID: fleetID, Faction: "A", Level: 4, Coord: fleet.Coord}},
	}
	g4m := state.GroundAir["af-b-rabaul-g4m-1"]
	g4m.Status = "attack"
	g4m.TaskID = "g-strike"
	g4m.TaskKind = "attack"
	g4m.TargetFleetID = fleetID
	g4m.TargetCoord = "1111"

	refreshContactMembers(&state)
	if g4m.TargetCoord != fleet.Coord {
		t.Fatalf("陆攻目标应跟到情报显示位置 %s：%s", fleet.Coord, g4m.TargetCoord)
	}
}
