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

// TestFloodingAndTiltAreNotCrippled 进水与倾斜不单独构成重创：它们每回合都能修，
// 修完自动恢复航速档位，只要动力还在就继续留在编队里打。
func TestFloodingAndTiltAreNotCrippled(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	damage := EnsureDamage(unit)
	damage.Flooding = []int{1, 2, 3}
	damage.Tilt = true
	if unitCrippled(unit) {
		t.Fatal("满血舰只有进水/倾斜不应判定重创")
	}
	resolveDetachments(&state)
	if unit.Withdrawing {
		t.Fatal("进水、倾斜可修的舰船不该被强制脱离")
	}
}

// TestStrengthLossCripplesShip 生命值损失达到 80% 仍然算重创：打残的船留在场上只是靶子。
func TestStrengthLossCripplesShip(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	unit.Strength = unit.MaxStrength / 5 // 损失 4/5 ≥ 80%
	if unit.MaxStrength-unit.Strength < 2 {
		t.Fatalf("%s 的 MaxStrength=%d 太小，这个用例不成立", unit.Name, unit.MaxStrength)
	}
	if !unitCrippled(unit) {
		t.Fatalf("损失 %d/%d 生命值应判定重创", unit.MaxStrength-unit.Strength, unit.MaxStrength)
	}
	resolveDetachments(&state)
	if !unit.Withdrawing {
		t.Fatal("损失 80% 生命值的舰船应强制脱离")
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
