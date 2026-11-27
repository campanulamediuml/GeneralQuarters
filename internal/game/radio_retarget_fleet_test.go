package game

import "testing"

// 无线电改目标要认"玩家点名的那支舰队"：同格停着两支敌舰队时，
// 只按情报顺序认领第一支等于点谁都没用，玩家点 B2 却打到 B1。
func TestRetargetAirPicksNamedFleet(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	aircraft.Status = "attack"
	aircraft.TaskID = "strike-1"
	aircraft.TaskKind = "attack"
	// 编队就压在自家母舰头顶：无线电距离 0，成功率 100%。
	coord := state.Units["a2-cv-01"].Coord
	aircraft.Coord = coord
	state.Contacts["A"] = map[string]*FleetContact{
		"B1": {FleetID: "B1", Faction: "B", Level: 3, Coord: coord, DisplayCoord: coord, Count: 4},
		"B2": {FleetID: "B2", Faction: "B", Level: 3, Coord: coord, DisplayCoord: coord, Count: 6},
	}
	order := Order{Type: "RETARGET_AIR", TaskID: "strike-1", Coord: coord, TargetFleetID: "B2"}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("点名已侦测舰队改目标应当合法：%v", err)
	}
	ResolveOrders(&state, []Order{order})
	if aircraft.TargetFleetID != "B2" {
		t.Fatalf("改目标没有认领点名的舰队：%q", aircraft.TargetFleetID)
	}
	if aircraft.TargetCoord != coord {
		t.Fatalf("目标坐标应跟着被点名的舰队：%q", aircraft.TargetCoord)
	}

	// 点名一支没情报的舰队要被拦下，不能借改目标白嫖目标信息。
	state.RadioUsed = map[string]string{}
	bad := Order{Type: "RETARGET_AIR", TaskID: "strike-1", Coord: coord, TargetFleetID: "B9"}
	if err := ValidateOrder(state, "A", bad); err == nil {
		t.Fatal("没有情报的舰队不应能被点名为打击目标")
	}
}
