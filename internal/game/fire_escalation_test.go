package game

import "testing"

// TestSecondSmallFireEscalatesToBurning 小型火灾在同一回合内再次触发就升级为起火燃烧
// （规则书 432 行"本回合内再次触发小型火灾时，转为起火燃烧"）。
func TestSecondSmallFireEscalatesToBurning(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	addFire(unit, "small")
	if unit.Damage.Fire != "small" || unit.Damage.SmallFireHits != 1 {
		t.Fatalf("第一次小型火灾应只点小火：%+v", unit.Damage)
	}
	addFire(unit, "small")
	if unit.Damage.Fire != "burning" {
		t.Fatalf("第二次小型火灾应升级为起火燃烧：%+v", unit.Damage)
	}
}

// TestSmallFireCounterResetsEachTurn 小火在回合开始自动扑灭，计数同时清零：
// 跨回合的两把火不能叠加成大火。
func TestSmallFireCounterResetsEachTurn(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	addFire(unit, "small")
	AdvanceShipConditions(&state)
	if unit.Damage.Fire != "" || unit.Damage.SmallFireHits != 0 {
		t.Fatalf("小火应在回合开始扑灭并清零计数：%+v", unit.Damage)
	}
	addFire(unit, "small")
	if unit.Damage.Fire != "small" {
		t.Fatalf("下一回合的第一把小火仍是小火：%+v", unit.Damage)
	}
}

// TestCarrierNeverWithdraws 航母不因重创自动脱离：掉 80% 强度、起火燃烧、失去动力都留在场上。
func TestCarrierNeverWithdraws(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	if carrier == nil || !IsCarrier(carrier) {
		t.Fatal("沙盒剧本应有美军航母 a2-cv-01")
	}
	carrier.Strength = 1
	damage := EnsureDamage(carrier)
	damage.Fire = "burning"
	damage.Disabled = true
	if unitCrippled(carrier) {
		t.Fatal("航母不应被判定为需要脱离的重创舰")
	}
	resolveDetachments(&state)
	if state.Units["a2-cv-01"].Withdrawing {
		t.Fatal("航母不应进入撤离状态")
	}
}

// TestNonCarrierStillWithdrawsWithBurning 非航母起火燃烧仍然按规则重创脱离。
func TestNonCarrierStillWithdrawsWithBurning(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	EnsureDamage(unit).Fire = "burning"
	resolveDetachments(&state)
	if !state.Units["a-bb-01"].Withdrawing {
		t.Fatal("战列舰起火燃烧应强制脱离")
	}
}
