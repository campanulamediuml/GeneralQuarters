package game

import "testing"

// TestEffectiveSpeedExposedInSnapshot 快照必须带"实际航速档位"：
// 兵牌上的白条画的就是它，而不是卡面印着的设计航速。
// 进水每层降一档，降档结果要和引擎算出来的一致（否则玩家看到的慢速只是美术效果）。
func TestEffectiveSpeedExposedInSnapshot(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["b-ca-01"]
	unit.Card.SpeedClass = "fast"

	read := func() string {
		for _, candidate := range VisibleUnits(state, "B") {
			if candidate.ID == unit.ID {
				return candidate.EffectiveSpeed
			}
		}
		t.Fatal("舰队没有出现在自己的可见单位里")
		return ""
	}

	if got := read(); got != "fast" {
		t.Fatalf("无进水时应为 fast，实际 %q", got)
	}
	EnsureDamage(unit).Flooding = []int{1}
	if got := read(); got != "normal" {
		t.Fatalf("1 层进水应降为 normal，实际 %q", got)
	}
	EnsureDamage(unit).Flooding = []int{1, 2}
	if got := read(); got != "slow" {
		t.Fatalf("2 层进水应降为 slow（白条），实际 %q", got)
	}
	EnsureDamage(unit).Flooding = []int{1, 2, 3}
	if got := read(); got != "very_slow" {
		t.Fatalf("3 层进水应降为 very_slow，实际 %q", got)
	}
	EnsureDamage(unit).Disabled = true
	if got := read(); got != "disabled" {
		t.Fatalf("失去动力时应为 disabled（橙条而不是白条），实际 %q", got)
	}
}

// TestNativeSlowShipReportsSlow 天生慢速舰同样通过实际档位暴露为 slow：
// 白条是"慢速"标志，不区分先天慢还是被打慢。
func TestNativeSlowShipReportsSlow(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["b-cl-01"]
	unit.Card.SpeedClass = "slow"
	unit.Card.DamageControl = 5
	found := false
	for _, candidate := range VisibleUnits(state, "B") {
		if candidate.ID == unit.ID {
			found = true
			if candidate.EffectiveSpeed != "slow" {
				t.Fatalf("天生慢速舰应报 slow，实际 %q", candidate.EffectiveSpeed)
			}
		}
	}
	if !found {
		t.Fatal("舰队没有出现在自己的可见单位里")
	}
}
