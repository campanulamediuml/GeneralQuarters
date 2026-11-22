package game

import "testing"

// TestProbabilityRollMeasuresPercent probabilityRoll 返回的是百分数（0–100），
// 不能再乘 100。回归：岸轰/轰炸/防空曾写成 probabilityRoll()*100 < 25，
// 实际命中率约 0.28% 而不是 25%，导致整支舰队岸轰全空。
func TestProbabilityRollMeasuresPercent(t *testing.T) {
	rng := uint64(12345)
	const n = 20000
	hits := 0
	for index := 0; index < n; index++ {
		if probabilityRoll(&rng) < 25 {
			hits++
		}
	}
	rate := float64(hits) / n * 100
	if rate < 24 || rate > 26 {
		t.Fatalf("probabilityRoll(x) < 25 的命中率应在 25%% 附近，实际 %.2f%%", rate)
	}
}

// TestShoreBombardmentHitRate 整支第一战列舰队（102 点火炮）对机场岸轰，500 次里绝大多数回合应有命中。
func TestShoreBombardmentHitRate(t *testing.T) {
	zeroRuns := 0
	totalHits := 0
	const runs = 200
	for seed := uint64(1); seed <= runs; seed++ {
		state := sandboxState(t)
		airfield := state.Airfields["af-b-sentry"]
		for _, unit := range state.Units {
			if FleetKey(unit) == "TG-A" {
				unit.Coord = airfield.Coord
			}
		}
		state.FleetStays = map[string]string{"TG-A": airfield.Coord}
		state.RNGState = seed
		before := airfield.Strength
		resolveShoreBombardment(&state)
		hits := before - airfield.Strength
		totalHits += hits
		if hits == 0 {
			zeroRuns++
		}
	}
	if zeroRuns > runs/10 {
		t.Fatalf("%d 点火炮的整支舰队岸轰全空比例过高：%d/%d", 102, zeroRuns, runs)
	}
	// 注意：机场生命值只有 15，命中数按"生命值扣减"统计，上限就是 15；
	// 25% × BB(16点×2)/BC(15×2) 两艘就足以打空，因此这里检查的是"打到上限"而不是数学期望 25。
	if avg := float64(totalHits) / runs; avg < 10 {
		t.Fatalf("平均命中数过低：%.1f（应受限于机场 15 点生命值，接近上限）", avg)
	}
}

// TestShoreBombardmentTargetMustBeEnemy 回归：岸轰只能打敌方机场，选错目标会静默不结算。
func TestShoreBombardmentTargetMustBeEnemy(t *testing.T) {
	state := sandboxState(t)
	enemy := state.Airfields["af-b-sentry"]
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = enemy.Coord
		}
	}
	state.FleetStays = map[string]string{"TG-A": enemy.Coord}
	if portFor(state, enemy.Coord) == nil || portFor(state, enemy.Coord).Faction == "A" {
		t.Fatal("ports 应返回该 hex 上的机场设施")
	}
	if canBombard(state, state.Units["a-bb-01"]) != enemy {
		t.Fatalf("水面舰队应能岸轰敌方机场：%v", canBombard(state, state.Units["a-bb-01"]))
	}
}

// TestPortSupplyClearsConfusion 舰队在友方机场 hex 停留满 1 回合后，港口补给清除岸轰积累的混乱。
func TestPortSupplyClearsConfusion(t *testing.T) {
	state := sandboxState(t)
	port := state.Airfields["af-a-lighthouse"]
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = port.Coord
			unit.Confusion = 4
		}
	}
	state.FleetStays = map[string]string{"TG-A": port.Coord}
	events := resolvePortSupply(&state)
	if state.Units["a-bb-01"].Confusion != 0 {
		t.Fatalf("港口补给应清除混乱，实际 %d", state.Units["a-bb-01"].Confusion)
	}
	if len(events) == 0 {
		t.Fatal("港口补给应产生战报")
	}
}
