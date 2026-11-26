package game

import (
	"encoding/json"
	"strings"
	"testing"
)

// 快照里的 night_safe 必须是显式字段：以前带 omitempty，false 会被整个省掉，
// 前端 if (task.night_safe === false) 永远不成立，AI 也读不到"这波回不来"。
func TestAirTaskNightFieldsAlwaysSerialized(t *testing.T) {
	task := AirTask{ID: "t", ReturnTurns: 6, NightSafe: false, TurnsToDark: 3, NightMargin: -3}
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	for _, want := range []string{`"night_safe":false`, `"turns_to_dark":3`, `"night_margin":-3`, `"return_turns":6`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("快照缺少 %s：%s", want, raw)
		}
	}
}

// 打击编队离母舰太远、又快天黑时，任务必须明确给出"回不来"。
func TestVisibleAviationNightMargin(t *testing.T) {
	state := State{
		Turn:        1,
		TurnsPerDay: 72,
		Timeline:    &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40"},
		Units:       map[string]*Unit{},
		Aircraft:    map[string]*AirUnit{},
	}
	found := 0
	for turn := 1; turn <= 400; turn++ {
		state.Turn = turn
		if turnsUntilNight(state) == 1 {
			found = turn
			break
		}
	}
	if found == 0 {
		t.Fatalf("找不到距天黑只剩 1 回合的时刻")
	}
	state.Units["cv"] = &Unit{ID: "cv", FleetID: "cv", Faction: "A", Type: "CV", Coord: "0101", Card: ShipCard{HangarCapacity: 20, DeckCapacity: 6}}
	state.Aircraft["strike"] = &AirUnit{ID: "strike", CarrierID: "cv", Faction: "A", Kind: "dive_bomber", Model: "SBD", Status: "attack", TaskID: "strike-task", TaskKind: "attack", Coord: "0110", TargetCoord: "0110", RemainingRange: 20, MaxRange: 20}
	view := VisibleAviation(state, "A")
	for _, task := range view.Tasks {
		if task.ID != "strike-task" {
			continue
		}
		if task.TurnsToDark != 1 {
			t.Fatalf("turns_to_dark = %d，应为 1", task.TurnsToDark)
		}
		if task.NightSafe {
			t.Fatalf("13 hex 外的打击编队被判为天黑前能回来（margin=%d）", task.NightMargin)
		}
		if task.NightMargin >= 0 {
			t.Fatalf("night_margin = %d，应为负数", task.NightMargin)
		}
		return
	}
	t.Fatalf("快照里找不到打击任务")
}
