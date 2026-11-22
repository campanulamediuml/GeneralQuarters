package game

import "testing"

// TestEnemyAircraftLossesHiddenUntilEnd 对局进行中，敌方飞机损失不下发：
// 沉船是公开事件，但飞机损失（随舰沉没、机库被炸、燃油耗尽）对方看不见，
// 累计台账会暴露其剩余航空力量。终局后才公开。
func TestEnemyAircraftLossesHiddenUntilEnd(t *testing.T) {
	state := sandboxState(t)
	ship := state.Units["a2-cv-01"]
	RecordShipLoss(&state, ship)
	ledger := ledgerFor(&state, "A")
	ledger.Aircraft = append(ledger.Aircraft, AircraftLoss{Model: "F6F 地狱猫", Kind: "fighter", Count: 3})
	ledger.AircraftTotal = 3

	// 日军视角：能看到美军的沉船，但看不到飞机损失。
	view := VisibleLosses(state, "B")
	if len(view["A"].Ships) != 1 {
		t.Fatalf("敌方沉船是公开信息，应当下发：%+v", view["A"])
	}
	if len(view["A"].Aircraft) != 0 || view["A"].AircraftTotal != 0 {
		t.Fatalf("对局未结束时不应下发敌方飞机损失：%+v", view["A"])
	}
	// 美军视角：自己的飞机损失照常可见。
	own := VisibleLosses(state, "A")
	if len(own["A"].Aircraft) != 1 || own["A"].AircraftTotal != 3 {
		t.Fatalf("己方损失应完整下发：%+v", own["A"])
	}

	// 终局：公开双方。
	state.Outcome = &GameOutcome{Reason: "test"}
	final := VisibleLosses(state, "B")
	if len(final["A"].Aircraft) != 1 || final["A"].AircraftTotal != 3 {
		t.Fatalf("终局后应公开敌方飞机损失：%+v", final["A"])
	}
}