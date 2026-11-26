package game

import "testing"

// 倾斜使航速降一档；没修好又中一次时不再叠加倾斜，而是变成一层进水 + 1 点生命损失。
func TestTiltSlowsShipAndEscalatesToFlooding(t *testing.T) {
	state := &State{RNGState: 9, Units: map[string]*Unit{}}
	unit := &Unit{ID: "ca", FleetID: "ca", Name: "重巡", Type: "CA", Faction: "A", Strength: 5, MaxStrength: 5, Card: ShipCard{SpeedClass: "normal", DamageControl: 5}}
	state.Units[unit.ID] = unit
	if got := EffectiveSpeedClass(unit); got != "normal" {
		t.Fatalf("初始应为 normal，实际 %s", got)
	}
	applyTilt(state, unit)
	if !EnsureDamage(unit).Tilt || unit.Strength != 5 {
		t.Fatalf("第一次倾斜只记录倾斜、不掉血：%+v", unit.Damage)
	}
	if got := EffectiveSpeedClass(unit); got != "slow" {
		t.Fatalf("倾斜应让航速降一档（normal→slow），实际 %s", got)
	}
	applyTilt(state, unit)
	if unit.Strength != 4 {
		t.Fatalf("倾斜未修复又中一次应损失 1 点生命值，实际 %d", unit.Strength)
	}
	if len(unit.Damage.Flooding) != 1 {
		t.Fatalf("重复倾斜应转成一层进水，实际 %v", unit.Damage.Flooding)
	}
	if got := EffectiveSpeedClass(unit); got != "very_slow" {
		t.Fatalf("倾斜 + 一层进水应降两档（normal→very_slow），实际 %s", got)
	}
	// 倾斜修好之后档位回来一档。
	unit.Damage.Tilt = false
	if got := EffectiveSpeedClass(unit); got != "slow" {
		t.Fatalf("倾斜修复后只剩一层进水的降档，实际 %s", got)
	}
}
