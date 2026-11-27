package game

import "testing"

// 交替节奏的意义：固定整数周期只能落在 37.5/25/12.5/8.3 节几档，
// 交替之后 25 海里/hex、20 分钟/回合的换算能把航速填到 30、20、12 节这种中间值。
func TestMoveCycleKnots(t *testing.T) {
	cases := []struct {
		cycle []int
		knots int
	}{
		{[]int{3}, 25},
		{[]int{2, 3}, 30},
		{[]int{3, 3, 2}, 28},
		{[]int{4, 4, 4, 3}, 20},
		{[]int{6, 6, 6, 7}, 12},
	}
	for _, c := range cases {
		if got := CycleKnots(c.cycle); got != c.knots {
			t.Fatalf("%v 应折合 %d 节，实际 %d", c.cycle, c.knots, got)
		}
	}
}

// moveGaps 在假想的连续回合里走一遍，返回相邻两次移动之间隔了几个回合。
// 顺序和引擎一致：规划移动的那一回合结算不递减（engine.DeferCooldown），
// 之后每回合结算递减 1，减到 0 才能再动。
func moveGaps(state *State, unit *Unit, turns int) []int {
	gaps := []int{}
	last := -1
	for turn := 0; turn < turns; turn++ {
		if unit.MoveCooldown > 0 {
			AdvanceMovementCooldown(state)
			continue
		}
		if last >= 0 {
			gaps = append(gaps, turn-last)
		}
		last = turn
		ApplyMoveCooldown(state, unit)
	}
	return gaps
}

func TestAlternatingMoveCycleRepeats(t *testing.T) {
	unit := &Unit{ID: "u1", FleetID: "f1", Type: "CA", Card: ShipCard{MoveCycle: []int{2, 3}, DamageControl: 5}}
	state := State{Units: map[string]*Unit{"u1": unit}}
	gaps := moveGaps(&state, unit, 21)
	if len(gaps) != 8 {
		t.Fatalf("21 回合里应走 9 格，实际 %v", gaps)
	}
	for i, gap := range gaps {
		want := 2
		if i%2 == 1 {
			want = 3
		}
		if gap != want {
			t.Fatalf("30 节舰的间隔应是 2/3 交替，实际 %v", gaps)
		}
	}

	convoy := &Unit{ID: "c1", FleetID: "f2", Type: "TRS", Card: ShipCard{MoveCycle: []int{6, 6, 6, 7}, DamageControl: 3}}
	convoyState := State{Units: map[string]*Unit{"c1": convoy}}
	if got := moveGaps(&convoyState, convoy, 30); len(got) < 4 || got[0] != 6 || got[3] != 7 || got[0] != got[1] {
		t.Fatalf("12 节船团的间隔应是 6,6,6,7，实际 %v", got)
	}
}

// 编队按最慢的那艘走：拖一条 25 节的战列舰，高速航母也得跟着 3 回合。
func TestFleetMoveCycleTakesSlowest(t *testing.T) {
	carrier := &Unit{ID: "cv", FleetID: "f1", Type: "CV", Card: ShipCard{MoveCycle: []int{2, 3}, DamageControl: 4}}
	bb := &Unit{ID: "bb", FleetID: "f1", Type: "BB", Card: ShipCard{MoveCycle: []int{3}, DamageControl: 7}}
	state := State{Units: map[string]*Unit{"cv": carrier, "bb": bb}}
	if got := FleetCycleKnots(state, carrier); got != 25 {
		t.Fatalf("整队航速应取最慢的 25 节，实际 %d", got)
	}
	if got := moveGaps(&state, carrier, 10); got[0] != 3 || got[1] != 3 {
		t.Fatalf("整队应按 3 回合走一格，实际 %v", got)
	}
}

// 进水每层、倾斜各 +1 回合；动力全失直接 24。
func TestMoveCycleDamagePenalty(t *testing.T) {
	unit := &Unit{ID: "u1", FleetID: "f1", Type: "CA", Card: ShipCard{MoveCycle: []int{3}, DamageControl: 4}}
	state := State{Units: map[string]*Unit{"u1": unit}}
	damage := EnsureDamage(unit)
	damage.Flooding = []int{1}
	if got := FleetMoveInterval(state, unit); got != 4 {
		t.Fatalf("一层进水应把 3 回合拖成 4 回合，实际 %d", got)
	}
	damage.Tilt = true
	if got := FleetMoveInterval(state, unit); got != 5 {
		t.Fatalf("倾斜再 +1 回合，实际 %d", got)
	}
	damage.Flooding = []int{1, 2, 3, 4, 5}
	if got := FleetMoveInterval(state, unit); got != 24 {
		t.Fatalf("丧失动力后应停在 24，实际 %d", got)
	}
}

// 没配 move_cycle 的老卡继续按四档走，旧存档行为不变。
func TestLegacySpeedClassFallback(t *testing.T) {
	unit := &Unit{ID: "u1", FleetID: "f1", Type: "CA", Card: ShipCard{SpeedClass: "slow", DamageControl: 5}}
	state := State{Units: map[string]*Unit{"u1": unit}}
	if got := FleetMoveInterval(state, unit); got != 6 {
		t.Fatalf("慢速档仍是 6 回合，实际 %d", got)
	}
}
