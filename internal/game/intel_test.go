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

// 脱离接触后不逐级衰减：白天 12 回合（4 小时）到期直接归零，夜间 4 回合归零。
func TestContactIntelTimesOutDirectly(t *testing.T) {
	newState := func(night bool) State {
		state := sandboxState(t)
		state.Night = night
		state.Contacts["B"] = map[string]*FleetContact{
			"TG-A": {FleetID: "TG-A", Faction: "A", Level: 3, Coord: "0202", Count: 14},
		}
		return state
	}

	day := newState(false)
	for turn := 1; turn <= 12; turn++ {
		day.IntelFresh = map[string]bool{}
		decayContactIntel(&day)
		contact := day.Contacts["B"]["TG-A"]
		if turn < 12 {
			if contact == nil {
				t.Fatalf("白天第 %d 回合不应丢失情报", turn)
			}
			if contact.Level != 3 {
				t.Fatalf("白天不应逐级衰减：第 %d 回合等级变成 %d", turn, contact.Level)
			}
		}
	}
	if _, ok := day.Contacts["B"]["TG-A"]; ok {
		t.Fatal("白天 12 回合没有新情报，接触应直接归零")
	}

	night := newState(true)
	for turn := 1; turn <= 4; turn++ {
		night.IntelFresh = map[string]bool{}
		decayContactIntel(&night)
		contact := night.Contacts["B"]["TG-A"]
		if turn < 4 {
			if contact == nil {
				t.Fatalf("夜间第 %d 回合不应丢失情报", turn)
			}
			if contact.Level != 3 {
				t.Fatalf("夜间不应逐级衰减：第 %d 回合等级变成 %d", turn, contact.Level)
			}
		}
	}
	if _, ok := night.Contacts["B"]["TG-A"]; ok {
		t.Fatal("夜间 4 回合没有新情报，接触应直接归零")
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
	state.Night = true
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", DisplayCoord: "0303", Count: 14},
	}
	for turn := 0; turn < 4; turn++ {
		state.IntelFresh = map[string]bool{}
		decayContactIntel(&state)
	}
	if _, ok := state.Contacts["B"]["TG-A"]; ok {
		t.Fatal("夜间 4 回合到期后情报应当消失")
	}
}
