package game

import (
	"strings"
	"testing"
)

// 规则：19:40 回收截止之后到次日 04:00，任何"安排航空任务"的命令都要报错。
func TestNightRejectsAirOrders(t *testing.T) {
	state := State{
		Night:       true,
		TurnsPerDay: 72,
		// 珊瑚海时间线；Turn 1 = 00:00，整备窗口没开 = 夜间。
		Timeline: &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"},
		Aircraft: map[string]*AirUnit{
			"a1": {ID: "a1", Faction: "A", Kind: "fighter", Status: "hangar", CarrierID: "cv"},
		},
		Units: map[string]*Unit{"cv": {ID: "cv", Faction: "A", Type: "CV", Coord: "1010", Card: ShipCard{AirRadar: true}}},
	}
	for _, orderType := range []string{"LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "RETASK_AIR"} {
		err := ValidateAirOrder(state, "A", Order{Type: orderType, AircraftIDs: []string{"a1"}})
		if err == nil || !strings.Contains(err.Error(), "夜间无法执行航空任务") {
			t.Fatalf("%s 夜间应该被拒绝并提示夜间无法执行航空任务，实际 %v", orderType, err)
		}
	}
	// 白天（整备窗口已开）同一个命令不再是"夜间"错误，可能因为别的校验失败，但文案不同。
	state.Night = false
	state.Turn = 30 // 09:40，整备窗口内
	err := ValidateAirOrder(state, "A", Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"a1"}})
	if err != nil && strings.Contains(err.Error(), "夜间无法执行航空任务") {
		t.Fatalf("白天不该报夜间错误：%v", err)
	}
}

// 规则：入夜全员回机库，任务与自动循环一起清掉，第二天 04:00 重新分配。
func TestNightParkingClearsTasksAndCycles(t *testing.T) {
	state := State{
		Aircraft: map[string]*AirUnit{
			"deck": {ID: "deck", Faction: "A", Status: "deck", TaskID: "cap-1", TaskKind: "cap",
				OnCycle: true, CycleKind: "cap", CycleTaskID: "cap-1", PatrolCoord: "1010"},
			"hangar": {ID: "hangar", Faction: "A", Status: "hangar", TaskID: "recon-1", TaskKind: "recon",
				OnCycle: true, CycleKind: "recon", CycleTaskID: "recon-1", Heading: "E", SectorWidth: 60},
			"airborne": {ID: "airborne", Faction: "A", Status: "cap", TaskID: "cap-2", TaskKind: "cap", OnCycle: true},
		},
	}
	events := parkAircraftForNight(&state)
	if len(events) == 0 {
		t.Fatal("夜间清场应该产生事件")
	}
	for _, id := range []string{"deck", "hangar"} {
		aircraft := state.Aircraft[id]
		if aircraft.Status != "hangar" || aircraft.TaskID != "" || aircraft.TaskKind != "" || aircraft.OnCycle || aircraft.CycleKind != "" {
			t.Fatalf("%s 没有被完整清场：%+v", id, aircraft)
		}
	}
	// 在空中的算子不打断，交给摸黑返航处理。
	if state.Aircraft["airborne"].Status != "cap" || state.Aircraft["airborne"].TaskID != "cap-2" {
		t.Fatalf("空中算子不该被直接收走：%+v", state.Aircraft["airborne"])
	}
}
