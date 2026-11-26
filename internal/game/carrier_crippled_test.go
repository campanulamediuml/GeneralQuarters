package game

import "testing"

// crippledCarrierState 造一个只有一艘 B 方航母的状态，方便逐个条件验证。
func crippledCarrierState(shipType string, strength, maxStrength, hangar int) *State {
	unit := &Unit{
		ID: "b-cv-01", Name: "翔鹤", Type: shipType, Faction: "B", Coord: "1010",
		FleetID: "MO", FleetName: "MO机动部队",
		Strength: strength, MaxStrength: maxStrength,
		Card: ShipCard{HangarCapacity: hangar},
	}
	return &State{Turn: 12, Units: map[string]*Unit{unit.ID: unit}}
}

// TestCrippledCarrierVictoryPointsTable 锁死"机库一半"的折算，奇数机库保留 .5。
func TestCrippledCarrierVictoryPointsTable(t *testing.T) {
	cases := []struct {
		hangar int
		want   float64
	}{
		{24, 12},
		{18, 9},
		{15, 7.5},
		{1, 0.5},
		{0, 1}, // 老存档没写机库：兜底 1 分，和击沉的兜底一致
	}
	for _, testCase := range cases {
		if got := CrippledCarrierVictoryPoints(ShipCard{HangarCapacity: testCase.hangar}); got != testCase.want {
			t.Errorf("机库%d → %v 分，期望 %v", testCase.hangar, got, testCase.want)
		}
	}
}

// TestCrippledCarrierTriggers 三个条件各自都能触发一次计分：甲板摧毁 / 结构损失超过 80% / 失去动力。
func TestCrippledCarrierTriggers(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Unit)
		want   float64
	}{
		{"飞行甲板摧毁", func(unit *Unit) { EnsureDamage(unit).DeckDestroyed = true }, 9},
		{"结构损失超过80%", nil, 9}, // 3/18 剩 16.7%，损失 83.3%
		{"失去动力", func(unit *Unit) { EnsureDamage(unit).Disabled = true }, 9},
	}
	for _, testCase := range cases {
		state := crippledCarrierState("CV", 3, 18, 18)
		if testCase.mutate != nil {
			// 单独验证甲板/动力时把结构修满，避免两个条件叠在一起说不清是谁触发的。
			state.Units["b-cv-01"].Strength = 18
			testCase.mutate(state.Units["b-cv-01"])
		}
		events := AwardCrippledCarriers(state)
		if len(events) != 1 || events[0].Type != "carrier_crippled" {
			t.Fatalf("%s：应产生一条 carrier_crippled 事件，实际 %+v", testCase.name, events)
		}
		if events[0].Audience != "all" {
			t.Fatalf("%s：事件应发给全场（比分本来就不是秘密），实际 %q", testCase.name, events[0].Audience)
		}
		if scores := VictoryPoints(*state); scores["A"] != testCase.want || scores["B"] != 0 {
			t.Fatalf("%s：对手应得 %v 分、瘫痪方 0 分，实际 %v", testCase.name, testCase.want, scores)
		}
	}
}

// TestCrippledCarrierNeedsMoreThan80Percent 锁死"超过 80%"是严格大于：刚好 80% 不算。
func TestCrippledCarrierNeedsMoreThan80Percent(t *testing.T) {
	state := crippledCarrierState("CV", 4, 20, 18) // 剩 20%，损失正好 80%
	if events := AwardCrippledCarriers(state); len(events) != 0 {
		t.Fatalf("刚好损失 80%% 不该计分，实际 %+v", events)
	}
	state.Units["b-cv-01"].Strength = 3 // 损失 85%
	if events := AwardCrippledCarriers(state); len(events) != 1 {
		t.Fatalf("损失超过 80%% 应该计分，实际 %+v", events)
	}
}

// TestCrippledCarrierAwardsOnce 只记第一次：舰况修回来或者一直坏着，都不能重复计分。
func TestCrippledCarrierAwardsOnce(t *testing.T) {
	state := crippledCarrierState("CV", 18, 18, 18)
	EnsureDamage(state.Units["b-cv-01"]).DeckDestroyed = true
	if events := AwardCrippledCarriers(state); len(events) != 1 {
		t.Fatalf("首次瘫痪应产生一条事件，实际 %+v", events)
	}
	// 甲板摧毁后又被"修复"（测试里模拟一下）、然后再炸一次：仍然只有一条记录。
	state.Units["b-cv-01"].Damage.DeckDestroyed = false
	if events := AwardCrippledCarriers(state); len(events) != 0 {
		t.Fatalf("已经记过的航母不该再次计分，实际 %+v", events)
	}
	state.Units["b-cv-01"].Damage.DeckDestroyed = true
	if events := AwardCrippledCarriers(state); len(events) != 0 {
		t.Fatalf("同一艘航母重复瘫痪只计一次，实际 %+v", events)
	}
	if ledger := state.Losses["B"]; ledger == nil || len(ledger.Crippled) != 1 {
		t.Fatalf("台账里应有 1 条瘫痪记录，实际 %+v", ledger)
	}
}

// TestCrippledCarrierThenSunkCountsOnce 先瘫后沉只领一次分：沉没改成按整艘算。
func TestCrippledCarrierThenSunkCountsOnce(t *testing.T) {
	state := crippledCarrierState("CV", 18, 18, 18)
	unit := state.Units["b-cv-01"]
	EnsureDamage(unit).DeckDestroyed = true
	AwardCrippledCarriers(state)
	if scores := VictoryPoints(*state); scores["A"] != 9 {
		t.Fatalf("瘫痪时应给 9 分，实际 %v", scores)
	}
	// 之后被打沉：沉船台账按整艘 18 分算，瘫痪那条不再重复计。
	RecordShipLoss(state, unit)
	delete(state.Units, unit.ID)
	if scores := VictoryPoints(*state); scores["A"] != 18 {
		t.Fatalf("先瘫后沉应总共 18 分（不是 27），实际 %v", scores)
	}
}

// TestCrippledCarrierIgnoresSunkAndNonCarriers 沉没的航母和别的舰种都不走这条规则。
func TestCrippledCarrierIgnoresSunkAndNonCarriers(t *testing.T) {
	sunk := crippledCarrierState("CV", 0, 18, 18)
	EnsureDamage(sunk.Units["b-cv-01"]).DeckDestroyed = true
	if events := AwardCrippledCarriers(sunk); len(events) != 0 {
		t.Fatalf("已经沉没的航母不记瘫痪，实际 %+v", events)
	}

	// 巡洋舰就算甲板被打烂（没有甲板）也不该拿瘫痪分：只有航母三型适用。
	cruiser := crippledCarrierState("CA", 1, 8, 18)
	EnsureDamage(cruiser.Units["b-cv-01"]).DeckDestroyed = true
	if events := AwardCrippledCarriers(cruiser); len(events) != 0 {
		t.Fatalf("巡洋舰不该计瘫痪分，实际 %+v", events)
	}
}

// TestCrippledCarrierVisibleToBothSides 瘫痪记录和沉船一样是公开战果：敌方也能看到。
func TestCrippledCarrierVisibleToBothSides(t *testing.T) {
	state := crippledCarrierState("CV", 18, 18, 18)
	EnsureDamage(state.Units["b-cv-01"]).DeckDestroyed = true
	AwardCrippledCarriers(state)
	for _, faction := range []string{"A", "B"} {
		losses := VisibleLosses(*state, faction)
		if len(losses["B"].Crippled) != 1 {
			t.Fatalf("%s 视角应看到 B 方 1 条瘫痪记录，实际 %+v", faction, losses["B"])
		}
	}
}
