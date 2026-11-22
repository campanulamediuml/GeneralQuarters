package game

import "testing"

// TestSmallFireDoesNotCrippleShip 小型火灾只是本回合封锁作业，下一回合自动扑灭，
// 不构成重创（规则书 432 行）；只有起火燃烧才强制脱离（规则书 398 行）。
func TestSmallFireDoesNotCrippleShip(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	EnsureDamage(unit).Fire = "small"
	if unitCrippled(unit) {
		t.Fatal("满血舰只因小型火灾就被判定重创")
	}
	resolveDetachments(&state)
	if unit.Withdrawing {
		t.Fatal("小型火灾不应触发重创脱离")
	}
	// 回合开始时自动扑灭。
	AdvanceShipConditions(&state)
	if unit.Damage.Fire != "" {
		t.Fatalf("小型火灾应在下一回合开始扑灭：%+v", unit.Damage)
	}
}

// TestBurningFireCripplesShip 起火燃烧每回合损管判定、可能弃船，属于重创。
func TestBurningFireCripplesShip(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	EnsureDamage(unit).Fire = "burning"
	if !unitCrippled(unit) {
		t.Fatal("起火燃烧应判定为重创")
	}
	resolveDetachments(&state)
	if !unit.Withdrawing {
		t.Fatal("起火燃烧的舰船应强制脱离")
	}
}
