package game

import "testing"

// 运输舰队离开原 hex 后，停留计数必须清零；否则会在移动途中“隔空卸货”。
func TestTransportStayResetsWhenFleetMoves(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	trs := &Unit{
		ID: "trs-test", FleetID: "TRS-A", FleetName: "运输队", Name: "运输船", Type: "TRS", Faction: "A",
		Coord: airfield.Coord, Strength: 2, MaxStrength: 2, Heading: "E",
		Card: ShipCard{Cargo: 3, SpeedClass: "normal", DamageControl: 3},
	}
	state.Units[trs.ID] = trs
	state.FleetStays = map[string]string{}
	state.FleetSessions = map[string]int{}
	state.RNGState = 52
	ResolveOrders(&state, nil)
	if got := SessionOf(state, "TRS-A"); got != 1 {
		t.Fatalf("到达当回合停留计数应为 1，实际 %d", got)
	}
	// 离开机场格（模拟移动后的 clearFleetStay + 新位置）：新 hex 从第 1 回合重新算。
	trs.Coord = "0101"
	clearFleetStay(&state, "TRS-A")
	ResolveOrders(&state, nil)
	if got := SessionOf(state, "TRS-A"); got != 1 {
		t.Fatalf("离开原 hex 后停留计数应从 1 重新算，实际 %d", got)
	}
}
