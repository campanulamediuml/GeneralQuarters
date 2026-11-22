package game

import "testing"

// 入夜：舰上没有待发任务，甲板/整备中的算子全部解除任务收回机库。
// 否则会留下"甲板上有任务、不能放飞、又拖不回机库"的死状态。
func TestNightParksDockedAircraft(t *testing.T) {
	state := sandboxState(t)
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40", NightLoss: true}
	state.TurnsPerDay = 72
	state.DayHours = 0
	state.Turn = 60 // 19:40 入夜

	deck := state.Aircraft["b-cv-01-fighter-x1"]
	deck.Status = "deck"
	deck.TaskID = "cap-MO"
	deck.TaskKind = "cap"
	service := state.Aircraft["b-cv-01-dive_bomber-x1"]
	service.Status = "service"
	service.TaskID = "strike-1"
	service.TaskKind = "attack"

	UpdateDayNight(&state)
	for _, id := range []string{"b-cv-01-fighter-x1", "b-cv-01-dive_bomber-x1"} {
		aircraft := state.Aircraft[id]
		if aircraft.Status != "hangar" || aircraft.TaskID != "" || aircraft.TaskKind != "" {
			t.Fatalf("%s 入夜后应收回机库并解除任务：status=%s task=%s/%s", id, aircraft.Status, aircraft.TaskID, aircraft.TaskKind)
		}
	}
}
