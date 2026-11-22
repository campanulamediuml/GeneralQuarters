package game

import (
	"fmt"
	"testing"
)

// setupStrikeAircraft 把一组打击算子摆到目标 hex 上（直接进入空袭结算）。
func setupStrikeAircraft(t *testing.T, state *State, taskID, target string, ids []string) {
	t.Helper()
	for _, id := range ids {
		a := state.Aircraft[id]
		if a == nil {
			t.Fatalf("缺少%s", id)
		}
		a.Status = "attack"
		a.TaskID = taskID
		a.TaskKind = "attack"
		a.TargetCoord = target
		a.Coord = target
		a.Engaged = true
		a.Launched = true
		switch a.Kind {
		case "dive_bomber":
			applyLoadoutChoice(a, LoadoutBombTank)
		case "torpedo_bomber":
			applyLoadoutChoice(a, LoadoutTorpedoTank)
		default:
			applyLoadoutChoice(a, LoadoutAirTank)
		}
	}
}

// setupCAP 把指定战斗机放到自家舰队上空执行 CAP。
func setupCAP(state *State, taskID string, ids []string) {
	for _, id := range ids {
		a := state.Aircraft[id]
		if a == nil {
			continue
		}
		fleetCoord := state.Units[a.CarrierID].Coord
		a.Status = "cap"
		a.TaskID = taskID
		a.TaskKind = "cap"
		a.Coord = fleetCoord
		a.Launched = true
		applyLoadoutChoice(a, LoadoutAirTank)
	}
}

// capFighters 取没被派去护航的战斗机做 CAP：每舰取 count 架，从编号 start 开始。
func capFighters(prefix string, start, count int) []string {
	ids := []string{}
	for _, carrier := range carrierIDs(prefix) {
		for n := start; n < start+count; n++ {
			ids = append(ids, fmt.Sprintf("%s-fighter-x%d", carrier, n))
		}
	}
	return ids
}

func fleetStrength(state *State, prefix string) int {
	total := 0
	for _, id := range carrierIDs(prefix) {
		if unit := state.Units[id]; unit != nil {
			total += unit.Strength
		}
	}
	return total
}

// carrierDamage 返回沉没艘数与"存活但掉强度"的艘数。
func carrierDamage(state *State, prefix string) (sunk, hurt int) {
	for _, id := range carrierIDs(prefix) {
		unit := state.Units[id]
		if unit == nil {
			sunk++
			continue
		}
		if unit.Strength < unit.MaxStrength {
			hurt++
		}
	}
	return sunk, hurt
}

func aliveAircraft(state *State, ids []string) int {
	count := 0
	for _, id := range ids {
		if state.Aircraft[id] != nil {
			count++
		}
	}
	return count
}

// TestCarrierDuelProbe 是"对轰"体检：双方同回合各出 32 架全甲板打击
// （6 护航战斗机 + 14 俯冲 + 12 鱼雷），各自舰队上空还有 CAP，看互相打成什么样。
func TestCarrierDuelProbe(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_sandbox")
	if err != nil {
		t.Fatal(err)
	}
	const trials = 50
	var usDmg, jpDmg, usOut, jpOut, usLost, jpLost float64
	var usSunk, usHurt, jpSunk, jpHurt float64
	var usCAPCount, jpCAPCount float64
	for seed := uint64(1); seed <= trials; seed++ {
		state, err := NewState(scenario, gameMap, catalog, seed)
		if err != nil {
			t.Fatal(err)
		}
		aTarget := state.Units["b-cv-01"].Coord
		bTarget := state.Units["a2-cv-01"].Coord
		usIDs := deckStrike32("a2-cv")
		jpIDs := deckStrike32("b-cv")
		setupStrikeAircraft(t, &state, "T-A", aTarget, usIDs)
		setupStrikeAircraft(t, &state, "T-B", bTarget, jpIDs)
		// 美军每舰留 4 架 CAP（编号 x3 起，避开护航的 x1/x2），日军每舰留 2 架。
		setupCAP(&state, "cap-A", []string{
			"a2-cv-01-fighter-x3", "a2-cv-01-fighter-x4", "a2-cv-01-fighter-x5", "a2-cv-01-fighter-x6",
			"a2-cv-02-fighter-x3", "a2-cv-02-fighter-x4", "a2-cv-02-fighter-x5", "a2-cv-02-fighter-x6",
			"a2-cv-03-fighter-x2", "a2-cv-03-fighter-x3", "a2-cv-03-fighter-x4", "a2-cv-03-fighter-x5",
			"a2-cv-04-fighter-x2", "a2-cv-04-fighter-x3", "a2-cv-04-fighter-x4", "a2-cv-04-fighter-x5",
		})
		setupCAP(&state, "cap-B", []string{
			"b-cv-01-fighter-x3", "b-cv-01-fighter-x4",
			"b-cv-02-fighter-x3", "b-cv-02-fighter-x4",
			"b-cv-03-fighter-x2", "b-cv-03-fighter-x3",
			"b-cv-04-fighter-x2", "b-cv-04-fighter-x3",
		})

		beforeUS := fleetStrength(&state, "a2-cv")
		beforeJP := fleetStrength(&state, "b-cv")
		if seed%2 == 0 {
			resolveAirStrike(&state, "T-A", aTarget)
			resolveAirStrike(&state, "T-B", bTarget)
		} else {
			resolveAirStrike(&state, "T-B", bTarget)
			resolveAirStrike(&state, "T-A", aTarget)
		}

		usDmg += float64(beforeJP - fleetStrength(&state, "b-cv"))
		jpDmg += float64(beforeUS - fleetStrength(&state, "a2-cv"))
		sunk, hurt := carrierDamage(&state, "b-cv")
		jpSunk += float64(sunk)
		jpHurt += float64(hurt)
		sunk, hurt = carrierDamage(&state, "a2-cv")
		usSunk += float64(sunk)
		usHurt += float64(hurt)
		usLost += float64(len(usIDs) - aliveAircraft(&state, usIDs))
		jpLost += float64(len(jpIDs) - aliveAircraft(&state, jpIDs))
		for _, report := range state.AirCombats {
			if report.AttackerFaction == "A" {
				usOut += float64(report.AttackerLosses)
				jpCAPCount += float64(report.DefenderCount)
			} else {
				jpOut += float64(report.AttackerLosses)
				usCAPCount += float64(report.DefenderCount)
			}
		}
	}
	f := func(v float64) float64 { return v / float64(trials) }
	t.Logf("对轰（%d次平均，双方各 32 架：6 护航 + 14 俯冲 + 12 鱼雷）：美军打掉日军 %.1f 点强度（沉 %.1f 艘、伤 %.1f 艘）；日军打掉美军 %.1f 点强度（沉 %.1f 艘、伤 %.1f 艘）",
		trials, f(usDmg), f(jpSunk), f(jpHurt), f(jpDmg), f(usSunk), f(usHurt))
	t.Logf("             攻击机：美军 32 中退出 %.1f、被击落 %.1f；日军 32 中退出 %.1f、被击落 %.1f",
		f(usOut), f(usLost), f(jpOut), f(jpLost))
	t.Logf("             拦截端 CAP：面对美军空袭的日军 CAP %.1f 架，面对日军空袭的美军 CAP %.1f 架",
		f(jpCAPCount), f(usCAPCount))
}
