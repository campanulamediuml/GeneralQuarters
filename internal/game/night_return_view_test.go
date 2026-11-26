package game

import "testing"

// 快照里要直接给出"这波回不回得来"，前端才能在天黑前拦住玩家。
func TestAviationViewExposesNightSafety(t *testing.T) {
	state := sandboxState(t)
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"}
	state.TurnsPerDay = 72
	state.DayHours = 0
	carrier := state.Units["b-cv-04"]
	carrier.Coord = "1030"

	makeRecon := func(id string, coord string, radius, distance int) *AirUnit {
		// 水上侦察机速度 1 hex/回合，返航时间才拉得开差距。
		plane := &AirUnit{ID: id, CarrierID: carrier.ID, Faction: "B", Kind: "recon", Model: "E13A", Seaplane: true,
			Status: "recon", Strength: 1, Heading: "E", SectorWidth: 60, ReconRadius: radius, ReconDistance: distance,
			Coord: coord, OriginCoord: carrier.Coord, OnCycle: true, CycleKind: "recon", TaskID: id, TaskKind: "recon"}
		plane.SetRange(20)
		return plane
	}
	// 17:00：离家 6 hex 的侦察机还要 6+1 回合，赶得上；离家 12 hex 的赶不上。
	safe := makeRecon("safe", "1036", 6, 6)
	doomed := makeRecon("doomed", "1042", 12, 12)
	state.Aircraft = map[string]*AirUnit{"safe": safe, "doomed": doomed}
	state.Turn = 52

	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 2 {
		t.Fatalf("应有两个任务：%+v", view.Tasks)
	}
	for _, task := range view.Tasks {
		switch task.ID {
		case "safe":
			if task.ReturnTurns == 0 || !task.NightSafe {
				t.Fatalf("6 hex 外的侦察机应该判定为能回来：%+v", task)
			}
		case "doomed":
			if task.NightSafe {
				t.Fatalf("12 hex 外的侦察机应判定为赶不上：%+v", task)
			}
		}
	}
	if clock := Clock(state); clock == nil || clock.NightInTurns != 8 {
		t.Fatalf("时钟应给出距天黑回合数：%+v", clock)
	}
}
