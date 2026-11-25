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

// 脱离接触后情报衰减：夜间每回合 -1，白天每 2 回合 -1（规则书 591–592 行）。
func TestContactIntelDecaysWhenStale(t *testing.T) {
	newState := func(night bool) State {
		state := sandboxState(t)
		state.Night = night
		state.Contacts["B"] = map[string]*FleetContact{
			"TG-A": {FleetID: "TG-A", Faction: "A", Level: 3, Coord: "0202", Count: 14},
		}
		return state
	}
	night := newState(true)
	decayContactIntel(&night)
	if night.Contacts["B"]["TG-A"].Level != 2 {
		t.Fatalf("夜间一回合应降一级：%d", night.Contacts["B"]["TG-A"].Level)
	}
	day := newState(false)
	decayContactIntel(&day)
	if day.Contacts["B"]["TG-A"].Level != 3 {
		t.Fatalf("白天第一回合不应衰减：%d", day.Contacts["B"]["TG-A"].Level)
	}
	decayContactIntel(&day)
	if day.Contacts["B"]["TG-A"].Level != 2 {
		t.Fatalf("白天第二回合应降一级：%d", day.Contacts["B"]["TG-A"].Level)
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

// 衰减到 0 级时情报消失，但显示位置停在最后发现处（规则书 593 行）。
func TestContactDisappearsAtZero(t *testing.T) {
	state := sandboxState(t)
	state.Night = true
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14},
	}
	decayContactIntel(&state)
	if _, ok := state.Contacts["B"]["TG-A"]; ok {
		t.Fatal("情报降到0后应当消失")
	}
}
