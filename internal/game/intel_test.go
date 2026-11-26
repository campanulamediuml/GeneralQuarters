package game

import "testing"

// 情报 1/2 级的位置误差分别是 2/1 hex，3、4 级位置准确。
// （规则书原表是 5/3/1，实测下来 5 hex 的误差让打击链路根本无从下手。）
func TestIntelErrorRadius(t *testing.T) {
	cases := map[int]int{0: 2, 1: 2, 2: 1, 3: 0, 4: 0}
	for level, want := range cases {
		if got := intelErrorRadius(level); got != want {
			t.Fatalf("情报%d级误差上限错误：%d（期望%d）", level, got, want)
		}
	}
}

// 显示位置在误差范围内，且同一份情报每次投影一致（不能刷新一次跳一次）。
func TestIntelDisplayCoordIsStableInRange(t *testing.T) {
	contact := FleetContact{FleetID: "TG-X", Level: 1, Coord: "0404"}
	first := intelDisplayCoord(contact)
	if offset := Distance(first, "0404"); offset != 2 {
		t.Fatalf("情报1级误差应为2hex：%d", offset)
	}
	for index := 0; index < 5; index++ {
		if intelDisplayCoord(contact) != first {
			t.Fatal("同一份情报的显示位置应当稳定")
		}
	}
	// 情报等级提升后误差范围收紧
	contact.Level = 2
	if offset := Distance(intelDisplayCoord(contact), "0404"); offset != 1 {
		t.Fatalf("情报2级误差应为1hex：%d", offset)
	}
	contact.Level = 3
	if got := intelDisplayCoord(contact); got != "0404" {
		t.Fatalf("情报3级位置必须准确：%s", got)
	}
	contact.Level = 4
	if intelDisplayCoord(contact) != "0404" {
		t.Fatal("情报4级位置必须准确")
	}
}

// 脱离接触后逐级衰减：4 级撑 6 回合掉到 3，之后每 3 回合掉一级（3→2→1→消失）。
func TestContactIntelDecaysByLevel(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Faction: "A", Level: 4, Coord: "0202", DisplayCoord: "0202", Count: 14},
	}
	// 期望值按回合号给：第 1–5 回合还是 4 级，第 6 回合掉到 3，之后每 3 回合掉一级。
	want := map[int]int{1: 4, 2: 4, 3: 4, 4: 4, 5: 4, 6: 3, 7: 3, 8: 3, 9: 2, 10: 2, 11: 2, 12: 1, 13: 1, 14: 1}
	stale := 0
	decayed := 0
	for turn := 1; turn <= 14; turn++ {
		state.IntelFresh = map[string]bool{}
		for _, event := range decayContactIntel(&state) {
			switch event.Type {
			case "contact_stale":
				stale++
			case "contact_decayed":
				decayed++
			case "contact_lost":
				t.Fatalf("第 %d 回合不该已经失去接触", turn)
			}
		}
		contact := state.Contacts["B"]["TG-A"]
		if contact == nil {
			t.Fatalf("第 %d 回合不该丢失接触", turn)
		}
		if contact.Level != want[turn] {
			t.Fatalf("第 %d 回合情报等级应为 %d，实际 %d", turn, want[turn], contact.Level)
		}
		if contact.StaleTurns != turn {
			t.Fatalf("第 %d 回合 StaleTurns 应累计到 %d，实际 %d", turn, turn, contact.StaleTurns)
		}
	}
	if decayed != 3 {
		t.Fatalf("14 回合内应降级 3 次（4→3→2→1），实际 %d", decayed)
	}
	// 降级前各一次（第 5/8/11 回合），外加第 14 回合"下一回合失去接触"的预警。
	if stale != 4 {
		t.Fatalf("每次降级与消失前一回合都应预警，实际 %d 次", stale)
	}
	// 1 级再撑 3 回合（第 15 回合）就归零，接触消失。
	state.IntelFresh = map[string]bool{}
	var lost bool
	for _, event := range decayContactIntel(&state) {
		if event.Type == "contact_lost" {
			lost = true
		}
	}
	if _, ok := state.Contacts["B"]["TG-A"]; ok || !lost {
		t.Fatal("满级情报脱离接触 15 回合后应归零并移除")
	}
}

// 获得新情报时不衰减；侦察机持续跟踪时也不衰减。
func TestContactIntelKeepsWhenFreshOrTracked(t *testing.T) {
	state := sandboxState(t)
	state.Night = true
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A":    {FleetID: "TG-A", Faction: "A", Level: 3, Coord: "0202", Count: 14},
		"TG-A-CV": {FleetID: "TG-A-CV", Faction: "A", Level: 3, Coord: "0602", Count: 17},
	}
	state.IntelFresh = map[string]bool{"B|TG-A": true}
	tracker := state.Aircraft["b-cv-04-dive_bomber-x1"]
	tracker.Status = "recon"
	tracker.TrackingFleetID = "TG-A-CV"
	decayContactIntel(&state)
	if state.Contacts["B"]["TG-A"].Level != 3 {
		t.Fatal("本回合有新情报不应衰减")
	}
	if state.Contacts["B"]["TG-A-CV"].Level != 3 {
		t.Fatal("侦察机持续跟踪时不应衰减")
	}
}

// 到期归零时情报消失，但在到期前一直停在最后发现位置。
func TestContactDisappearsAtZero(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", DisplayCoord: "0303", Count: 14},
	}
	for turn := 1; turn <= 2; turn++ {
		state.IntelFresh = map[string]bool{}
		decayContactIntel(&state)
		contact := state.Contacts["B"]["TG-A"]
		if contact == nil {
			t.Fatalf("第 %d 回合 1 级情报不该立刻消失", turn)
		}
		if contact.Level != 1 || contact.DisplayCoord != "0303" {
			t.Fatalf("衰减过程不该改动等级/显示位置：%+v", contact)
		}
	}
	state.IntelFresh = map[string]bool{}
	decayContactIntel(&state)
	if _, ok := state.Contacts["B"]["TG-A"]; ok {
		t.Fatal("1 级情报脱离接触 3 回合后应当消失")
	}
}
