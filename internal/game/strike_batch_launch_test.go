package game

import "testing"

// TestStrikeLaunchesWithinDeckCapacity 回归：一次任务编队超过单舰甲板容量时，
// 只能放飞容量内的架数，其余顺延，而不是整组卡在甲板等玩家手动退回。
func TestStrikeLaunchesWithinDeckCapacity(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	// 把甲板压小到 4 架，制造"任务编队比甲板容量大"的场景（实战里风向修正会让甲板变短）。
	carrier.Deck.Length = 4
	carrier.Deck.BaseLength = 4
	carrier.Card.DeckCapacity = 4
	deck := carrier.Deck.Length
	if deck <= 0 {
		t.Fatalf("航母甲板长度异常：%d", deck)
	}
	ids := []string{}
	for id, aircraft := range state.Aircraft {
		if aircraft.CarrierID == carrier.ID && aircraft.Kind == "dive_bomber" {
			ids = append(ids, id)
		}
	}
	if len(ids) <= deck {
		t.Skipf("该航母俯冲轰炸机数量 %d 不超过甲板容量 %d，无需测试分批", len(ids), deck)
	}
	// 把整组塞到甲板上等待放飞，超过甲板容量。
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.Status = "deck"
		aircraft.TaskID = "strike-cap"
		aircraft.TaskKind = "attack"
		aircraft.TargetCoord = "0202"
		aircraft.Launched = false
	}
	events := launchReadyTasks(&state)
	launched := 0
	for _, id := range ids {
		if state.Aircraft[id].Launched || airborne(state.Aircraft[id]) {
			launched++
		}
	}
	if launched != deck {
		t.Fatalf("本回合应只放飞甲板容量 %d 架，实际 %d 架", deck, launched)
	}
	deferred := false
	for _, event := range events {
		if event.Type == "air_launch_delayed" {
			deferred = true
		}
	}
	if !deferred {
		t.Fatal("超出容量的部分应报顺延提示，而不是静默丢弃")
	}
	if len(state.Aircraft) == 0 {
		t.Fatal("未放飞的算子不应被移除")
	}
}
