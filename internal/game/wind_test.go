package game

import "testing"

// 规则书 817–822 行：顶风 +1、侧逆风 0、侧顺风 -1、顺风 -2。
// 风向按"来向"记：NE 表示风从东北吹来，舰艏朝 NE 就是顶着风跑（顶风 +1）。
func TestWindRelationAndDeckModifier(t *testing.T) {
	cases := []struct {
		wind, heading, relation string
		modifier                int
	}{
		{"NE", "NE", "para", 1},
		{"NE", "SW", "same", -2},
		{"NE", "E", "ortho", 0},
		{"NE", "NW", "ortho", 0},
		{"NE", "SE", "meta", -1},
		{"NE", "W", "meta", -1},
		{"E", "SE", "ortho", 0},
	}
	for _, item := range cases {
		if got := windRelation(item.wind, item.heading); got != item.relation {
			t.Fatalf("风向%s / 朝向%s 关系错误：%s（期望%s）", item.wind, item.heading, got, item.relation)
		}
		if got := windDeckModifier(item.wind, item.heading); got != item.modifier {
			t.Fatalf("风向%s / 朝向%s 修正错误：%d（期望%d）", item.wind, item.heading, got, item.modifier)
		}
	}
}

// 卡面 DeckCapacity 是印刷的基础值，风向只改本回合可用长度 Deck.Length。
func TestWindKeepsPrintedDeckCapacity(t *testing.T) {
	state := sandboxState(t)
	// 这两条用例测的是"没有风场时退回剧本单一风向"的老路径；
	// 每 hex 风场由 windfield_test.go 单独覆盖。
	state.WindField = nil
	state.Wind = "NE"
	carrier := state.Units["a2-cv-01"]
	carrier.Card.DeckCapacity = 8
	carrier.Deck.BaseLength = 8
	carrier.Heading = "SE" // 侧顺风 -1
	RefreshDeckLengths(&state)
	if carrier.Deck.Length != 7 {
		t.Fatalf("本回合甲板长度错误：%d（期望7）", carrier.Deck.Length)
	}
	if carrier.Card.DeckCapacity != 8 {
		t.Fatalf("卡面基础甲板容量被风向改写：%d（期望8）", carrier.Card.DeckCapacity)
	}
}

// 修正后的甲板长度最低为 0（规则书 824 行）。
func TestDeckLengthClampedToZero(t *testing.T) {
	state := sandboxState(t)
	state.WindField = nil
	state.Wind = "NE"
	carrier := state.Units["a2-cv-01"]
	// 风向 NE（风从东北来）：舰艏朝 NE 是顶风 +1，朝 SW 才是顺风 -2。
	carrier.Heading = "SW" // 顺风 -2
	carrier.Deck.BaseLength = 1
	RefreshDeckLengths(&state)
	if carrier.Deck.Length != 0 {
		t.Fatalf("甲板长度应被夹到 0：%d", carrier.Deck.Length)
	}
	carrier.Heading = "NE" // 顶风 +1
	RefreshDeckLengths(&state)
	if carrier.Deck.Length != 2 {
		t.Fatalf("顶风甲板长度错误：%d（1+1=2）", carrier.Deck.Length)
	}
}

// 逆风起降容量更大，顺风更小。
func TestWindAffectsLaunchCapacity(t *testing.T) {
	withWind := func(wind, heading string) int {
		state := sandboxState(t)
		state.Wind = wind
		carrier := state.Units["a2-cv-01"]
		carrier.Heading = heading
		RefreshDeckLengths(&state)
		return carrier.Deck.Length
	}
	// 风从 NE 来：朝 NE 是逆风，朝 SW 是顺风。
	base := withWind("NE", "NE")
	if base <= withWind("NE", "SW") {
		t.Fatal("逆风时的甲板容量应当大于顺风")
	}
}

// 每回合每支舰队只能调整一次朝向。
func TestFleetHeadingOncePerTurn(t *testing.T) {
	state := sandboxState(t)
	state.Wind = "NE"
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "SW"}); err != nil {
		t.Fatalf("第一次调整朝向应当合法：%v", err)
	}
	events := SetFleetHeading(&state, FleetKey(state.Units["a2-cv-01"]), "SW")
	if len(events) != 1 {
		t.Fatal("调整朝向应当产生战报")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "NE"}); err == nil {
		t.Fatal("同一舰队本回合不应能再次调整朝向")
	}
	// 整队一起转向
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A-CV" && unit.Heading != "SW" {
			t.Fatalf("整队朝向没有统一：%+v", unit)
		}
	}
	// 新回合可以再调整
	ResolveOrders(&state, nil)
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "NE"}); err != nil {
		t.Fatalf("新回合应当可以再次调整朝向：%v", err)
	}
}

// 非法的朝向被拒绝。
func TestInvalidHeadingRejected(t *testing.T) {
	state := sandboxState(t)
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "NORTH"}); err == nil {
		t.Fatal("非法朝向未被拒绝")
	}
	if err := ValidateOrder(state, "B", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "NE"}); err == nil {
		t.Fatal("不应能调整敌方舰队朝向")
	}
}

// 朝向不能免费改：本回合移动过的舰队，朝向由移动方向决定，不能再单独调。
func TestFleetHeadingLockedAfterMove(t *testing.T) {
	state := sandboxState(t)
	state.Wind = "NE"
	mover := state.Units["a2-cv-01"]
	before := mover.Heading
	mover.Moved = true
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "SW"}); err == nil {
		t.Fatal("移动过的舰队还能免费调整朝向")
	}
	// 绕过校验直接调用也不行，而且朝向不能被改动。
	if events := SetFleetHeading(&state, FleetKey(mover), "SW"); len(events) != 0 {
		t.Fatalf("移动过的舰队直接改朝向应当无效，却产生了 %d 条战报", len(events))
	}
	if mover.Heading != before {
		t.Fatalf("移动过的舰队朝向被改成了 %s，原本 %s", mover.Heading, before)
	}
	// 没移动的舰队照常可以调。
	mover.Moved = false
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: "a2-cv-01", Heading: "SW"}); err != nil {
		t.Fatalf("未移动的舰队应当可以调整朝向：%v", err)
	}
}

// 移动即朝向：本回合移动过的舰队，朝向必须等于实际移动方向（手动移动/自动航行/撤离
// 三条路都写 headingBetween），这样玩家没法"往 NE 走、朝向却挂 SW"两头占便宜。
func TestFleetHeadingFollowsMoveDirection(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a2-cv-01"]
	moves := LegalMoves(state, unit)
	if len(moves) == 0 {
		t.Skip("沙盒单位本回合没有可走格")
	}
	from := unit.Coord
	target := moves[0]
	events := ResolveOrders(&state, []Order{{Type: "SET_MOVE", UnitID: unit.ID, Coord: target}})
	if len(events) == 0 {
		t.Fatal("移动没有产生战报")
	}
	for _, member := range state.Units {
		if FleetKey(member) != FleetKey(unit) {
			continue
		}
		if member.Coord != target {
			t.Fatalf("%s 没有移动到 %s", member.Name, target)
		}
		if want := headingBetween(from, target); member.Heading != want {
			t.Fatalf("%s 朝 %s，移动方向是 %s，朝向没有跟着移动", member.Name, member.Heading, want)
		}
		if !member.Moved {
			t.Fatalf("%s 移动了但没标记 Moved", member.Name)
		}
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_HEADING", UnitID: unit.ID, Heading: "NE"}); err == nil {
		t.Fatal("移动过的舰队还能再单独调朝向")
	}
}
