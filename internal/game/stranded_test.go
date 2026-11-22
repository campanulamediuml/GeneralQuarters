package game

import "testing"

// 编队里有人永远上不了甲板（母舰甲板被摧毁）时，不能把整组拖死：
// 剩下的飞机应该照常离场，卡住的成员退出编队并给出战报。
func TestAttackGroupDropsStrandedMembers(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	carrier.Coord = "0303"
	EnsureDamage(carrier).DeckDestroyed = true

	ready := state.Aircraft["b-cv-02-dive_bomber-x1"]
	ready.Status = "attack_holding"
	ready.TaskKind = "attack"
	ready.TaskID = "strike-x"
	ready.TargetCoord = "0708"
	ready.Coord = "0303"
	ready.SetRange(20)

	stranded := state.Aircraft["b-cv-01-dive_bomber-x1"]
	stranded.Status = "service"
	stranded.TaskKind = "attack"
	stranded.TaskID = "strike-x"
	stranded.TargetCoord = "0708"
	stranded.ServiceTurns = 2
	stranded.Coord = "0303"

	events := AdvanceAviation(&state)
	dropped := false
	for _, event := range events {
		if event.Type == "attack_member_stranded" {
			dropped = true
		}
	}
	if !dropped {
		t.Fatalf("应有成员退出编队的战报：%+v", events)
	}
	if stranded.TaskID != "" || stranded.Status != "hangar" {
		t.Fatalf("卡住的成员应回到机库并退出编队：status=%s task=%s", stranded.Status, stranded.TaskID)
	}
	if ready.Status != "attack" {
		t.Fatalf("剩下的飞机应照常离场：status=%s", ready.Status)
	}
}
