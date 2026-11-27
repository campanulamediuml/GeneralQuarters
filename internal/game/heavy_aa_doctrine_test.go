package game

import "testing"

// heavyAAProbe 跑一次真实的对舰空袭（三架俯冲轰炸机认领一支舰队），返回战报。
// setup 在开打前摆位：先把日军重防空全部清零，再把点数集中到指定的舰上。
func heavyAAProbe(t *testing.T, setup func(state *State, target, sponsor *Unit)) AirCombatReport {
	t.Helper()
	state := sandboxState(t)
	target := state.Units["b2-bb-01"]
	sponsor := state.Units["b2-ca-01"]
	if target == nil || sponsor == nil {
		t.Fatal("夹具里缺少 b2-bb-01 / b2-ca-01")
	}
	// 轻防空也清零：战报里的 AAHits 是重防空和轻防空共用计数，留着轻防空就分不清是谁打的。
	for _, unit := range state.Units {
		if unit.Faction == "B" {
			unit.Card.HeavyAA = 0
			unit.Card.LightAA = 0
		}
	}
	setup(&state, target, sponsor)

	coord := target.Coord
	planes := map[string]*AirUnit{}
	for _, model := range []string{"a2-cv-01-dive_bomber-x1", "a2-cv-01-dive_bomber-x2", "a2-cv-01-dive_bomber-x3"} {
		aircraft := state.Aircraft[model]
		if aircraft == nil {
			t.Fatalf("夹具里缺少 %s", model)
		}
		aircraft.Status = "attack"
		aircraft.Launched = true
		aircraft.TaskID = "strike-fleet-aa"
		aircraft.TaskKind = "attack"
		aircraft.Coord = coord
		aircraft.TargetCoord = coord
		aircraft.OriginCoord = coord
		aircraft.Strength = 2
		// 空袭认领的是"这支舰队"：同格的其他舰队不参与这次交战。
		aircraft.TargetFleetID = FleetKey(target)
		planes[model] = aircraft
	}
	state.Aircraft = planes
	state.RNGState = 7

	resolveAirStrike(&state, "strike-fleet-aa", coord)
	if len(state.AirCombats) == 0 {
		t.Fatal("没有生成空袭报告")
	}
	return state.AirCombats[len(state.AirCombats)-1]
}

// 重防空对双方都是"整队求和"：同一支舰队的所有重防空值加在一起，对整支来袭机队结算一次。
// 阵营差别只在命中率——美军后期 +2%（VT 引信与雷达火控），不在"能不能合并"。
func TestHeavyAAStacksFleetWide(t *testing.T) {
	combat := heavyAAProbe(t, func(_ *State, target, sponsor *Unit) {
		if sponsor.Coord != target.Coord {
			t.Fatalf("前提不成立：两舰应同格，实际 %s / %s", sponsor.Coord, target.Coord)
		}
		sponsor.Card.HeavyAA = 400
	})
	if combat.AAHits == 0 {
		t.Fatalf("目标舰自身重防空为 0，来袭击仍应吃到同队僚舰的重防空：%+v", combat)
	}
}

// 重防空只对自己这支舰队生效：同一格停着另一支舰队时，它的重防空不会被借过来，
// 它的舰船也不该出现在这次空袭的目标分配里。
func TestHeavyAADoesNotCrossFleetsInSameHex(t *testing.T) {
	combat := heavyAAProbe(t, func(state *State, target, _ *Unit) {
		// b-ca-01 属于另一支舰队（TG-B），把它挪到目标格并挂上全部重防空。
		other := state.Units["b-ca-01"]
		if other == nil {
			t.Fatal("夹具里缺少 b-ca-01")
		}
		if FleetKey(other) == FleetKey(target) {
			t.Fatalf("前提不成立：两舰应在不同舰队，实际同属 %s", FleetKey(other))
		}
		other.Coord = target.Coord
		other.Card.HeavyAA = 400
	})
	if len(combat.TargetAllocations) == 0 {
		t.Fatal("空袭没有分配目标，用例前提不成立")
	}
	for _, allocation := range combat.TargetAllocations {
		if allocation.ShipID == "b-ca-01" {
			t.Fatalf("同格另一支舰队的重巡不该被选作投弹目标：%+v", allocation)
		}
	}
	if combat.AAHits != 0 {
		t.Fatalf("另一支舰队的 400 点重防空不该替目标舰队开火，实际命中 %d 次：%+v", combat.AAHits, combat)
	}
}

// 合并的是点数，不是命中率：美军后期每点 5%、日军每点 3%，天气照样扣。
func TestHeavyAAHitChanceKeepsFactionBonus(t *testing.T) {
	state := sandboxState(t)
	state.Year = 1944 // 后期加成从 1943 年开始，夹具剧本是 1942 年
	coord := state.Units["b2-bb-01"].Coord
	if got := aaHitChance(state, coord, true, "B"); got != 3 {
		t.Fatalf("日军重防空每点应为 3%%，实际 %.1f", got)
	}
	if got := aaHitChance(state, coord, true, "A"); got != 5 {
		t.Fatalf("1944 年美军重防空应为 3%%+2%%，实际 %.1f", got)
	}
	if got := aaHitChance(state, coord, false, "A"); got != 3 {
		t.Fatalf("轻型防空不吃后期加成，应为 3%%，实际 %.1f", got)
	}
}
