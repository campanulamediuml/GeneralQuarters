package game

import "testing"

// 珊瑚海第 59 回合那 8 架 E13A 的复盘：
// 水上侦察机每回合只飞 1 格，召回又在回合结束时才登记，飞机下一回合才真的往回飞。
// 于是"再飞一格"这一回合会让返航所需回合 +1、可用回合 −1 —— 一来一回差两回合。
// 只留一回合余量时，召回那一刻已经注定赶不上，落地前一格被入夜判定判失联。
func TestSeaplaneReconRecalledWithTwoTurnMargin(t *testing.T) {
	build := func(coord string) (State, *AirUnit) {
		state := State{Turn: 16, DayHours: 12, TurnsPerDay: 72}
		carrier := &Unit{ID: "ca", Faction: "B", Coord: "2432"}
		state.Units = map[string]*Unit{"ca": carrier}
		plane := &AirUnit{ID: "e13a", Faction: "B", Model: "E13A 零式水侦", Kind: "recon", Status: "recon",
			Seaplane: true, CarrierID: "ca", Coord: coord, TaskID: "recon", TaskKind: "recon"}
		plane.SetRange(9)
		state.Aircraft = map[string]*AirUnit{"e13a": plane}
		if got := turnsUntilNight(state); got != 8 {
			t.Fatalf("前置条件变了：距天黑应为 8 回合，实际 %d", got)
		}
		return state, plane
	}

	// 距离 5 格、还需要 6 回合才能落地：按"只留一回合余量"会被判成还来得及，
	// 再往外飞一格就必然回不来——这正是漏掉的整整一个往返。
	far, farPlane := build("2929")
	events := recallLoopingAircraftBeforeNight(&far)
	if !farPlane.Returning {
		t.Fatalf("距天黑 8 回合、返航要 6 回合的水侦应当立刻召回：%+v", events)
	}

	// 距离 2 格的水侦还来得及，不要提前打断搜索。
	near, nearPlane := build("2631")
	recallLoopingAircraftBeforeNight(&near)
	if nearPlane.Returning {
		t.Fatal("近在咫尺的水侦不该被提前召回")
	}
}
