package game

import "testing"

// TestAttackDepartsImmediatelyWhenAllLaunched 回归：最后一批算子起飞的同一回合，
// 只要编队全员到齐且已设目标，就该立刻离场，而不是等玩家再发一次 SET_STRIKE_TARGET。
func TestAttackDepartsImmediatelyWhenAllLaunched(t *testing.T) {
	state := sandboxState(t)
	ids := []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: ids, Coord: "0202"}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft[ids[0]].TaskID
	for _, id := range ids {
		state.Aircraft[id].Status = "attack_holding"
		state.Aircraft[id].Launched = true
		state.Aircraft[id].TargetCoord = "0202"
		state.Aircraft[id].TaskID = taskID
		state.Aircraft[id].TaskKind = "attack"
	}
	// 只推进"编队集结"这一步，不重新下发设目标命令。
	updateAttackFormation(&state, taskID)
	departed := state.Aircraft[ids[0]].Status == "attack" && state.Aircraft[ids[1]].Status == "attack"
	if !departed {
		// 允许由 departReadyAttacks 兜底。
		events := departReadyAttacks(&state)
		found := false
		for _, event := range events {
			if event.Type == "attack_departed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("全员升空且已设目标时应自动离场：%+v", state.Aircraft[ids[0]])
		}
	}
}
