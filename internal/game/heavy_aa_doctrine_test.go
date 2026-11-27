package game

import "testing"

// 重防空对双方都是"整队求和"：同一格舰队的所有重防空值加在一起，对整支来袭机队结算一次。
// 阵营差别只在命中率——美军后期 +2%（VT 引信与雷达火控），不在"能不能合并"。
// 这条用例用一艘自己不承担攻击目标的舰来验证：目标舰重防空为 0，来袭击仍要挨整队的弹幕。
func TestHeavyAAStacksFleetWide(t *testing.T) {
	state := sandboxState(t)
	target := state.Units["b2-bb-01"]
	sponsor := state.Units["b2-ca-01"]
	if target == nil || sponsor == nil {
		t.Fatal("夹具里缺少 b2-bb-01 / b2-ca-01")
	}
	if sponsor.Coord != target.Coord {
		t.Fatalf("前提不成立：两舰应同格，实际 %s / %s", sponsor.Coord, target.Coord)
	}
	// 全舰队重防空清零，只留同格里那艘不会被选作目标的轻巡。
	for _, unit := range state.Units {
		if unit.Faction == "B" {
			unit.Card.HeavyAA = 0
		}
	}
	sponsor.Card.HeavyAA = 400

	coord := target.Coord
	planes := map[string]*AirUnit{}
	for index, model := range []string{"a2-cv-01-dive_bomber-x1", "a2-cv-01-dive_bomber-x2", "a2-cv-01-dive_bomber-x3"} {
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
		planes[model] = aircraft
		_ = index
	}
	state.Aircraft = planes
	state.RNGState = 7

	resolveAirStrike(&state, "strike-fleet-aa", coord)
	if len(state.AirCombats) == 0 {
		t.Fatal("没有生成空袭报告")
	}
	combat := state.AirCombats[len(state.AirCombats)-1]
	for _, allocation := range combat.TargetAllocations {
		if allocation.ShipID == sponsor.ID {
			t.Fatalf("轻巡不该被选作投弹目标，否则这条用例证明不了重防空是整队求和：%+v", allocation)
		}
	}
	if combat.AAHits == 0 {
		t.Fatalf("目标舰自身重防空为 0，来袭击仍应吃到同格其他舰的重防空：%+v", combat)
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
