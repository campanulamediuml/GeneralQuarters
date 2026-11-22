package game

import (
	"fmt"
	"testing"
)

// 复合编队缩的是命中率，不是骰子数（规则书 712 行）。
func TestMixedStrikeHalvesLightAAHitChance(t *testing.T) {
	const runs = 4000
	hitRate := func(kind string, mixed bool) float64 {
		total := 0
		dice := 0
		for i := 0; i < runs; i++ {
			state := State{
				RNGState: uint64(7919*i+13)*2654435761 + 1,
				Units: map[string]*Unit{
					"bb": {ID: "bb", Faction: "B", Type: "BB", Coord: "0202", Strength: 6, MaxStrength: 6, Card: ShipCard{LightAA: 10, DamageControl: 10}},
				},
				Aircraft: map[string]*AirUnit{},
			}
			planes := []*AirUnit{}
			for j := 0; j < 20; j++ {
				id := fmt.Sprintf("a%d", j)
				state.Aircraft[id] = &AirUnit{ID: id, Faction: "A", Kind: kind, Strength: 4, Status: "attack", Coord: "0202"}
				planes = append(planes, state.Aircraft[id])
			}
			report := &AirCombatReport{}
			resolveLightAAPool(&state, planes, state.Units["bb"], mixed, report)
			total += report.AAHits
			dice += 10
		}
		return float64(total) / float64(dice)
	}
	single := hitRate("dive_bomber", false)
	if single < 0.02 || single > 0.04 {
		t.Fatalf("俯冲轰炸机单机种命中率应约 3%%，实测 %.4f", single)
	}
	mixed := hitRate("dive_bomber", true)
	if diff := single - mixed*2; diff > 0.008 || diff < -0.008 {
		t.Fatalf("复合编队命中率应为单机种的一半：单机种 %.4f，复合 %.4f", single, mixed)
	}
	// 纯鱼雷机编队：整副骰子命中率翻倍（规则书 673 行）。
	torpedo := hitRate("torpedo_bomber", false)
	if diff := torpedo - single*2; diff > 0.012 || diff < -0.012 {
		t.Fatalf("纯鱼雷机编队命中率应翻倍：俯冲 %.4f，鱼雷 %.4f", single, torpedo)
	}
}

// 复合编队里鱼雷机的翻倍与减半互相抵消，净效果等于原值。
func TestMixedStrikeTorpedoDiceCancelOut(t *testing.T) {
	state := State{
		Doctrines: map[string]string{"B": "japan"},
		Map:       MapDefinition{Rows: 7, Cols: 9},
		Units:     map[string]*Unit{"bb": {ID: "bb", Faction: "B", Type: "BB", Coord: "0202", Card: ShipCard{LightAA: 10}}},
	}
	ship := state.Units["bb"]
	base := lightAAHitChance(state, ship, &AirUnit{Kind: "dive_bomber"}, false)
	if got := lightAAHitChance(state, ship, &AirUnit{Kind: "torpedo_bomber"}, true); got != base {
		t.Fatalf("混编鱼雷机命中率应等于基础值 %.1f%%，实际 %.1f%%", base, got)
	}
	if got := lightAAHitChance(state, ship, &AirUnit{Kind: "torpedo_bomber"}, false); got != base*2 {
		t.Fatalf("纯鱼雷机命中率应为 %.1f%%，实际 %.1f%%", base*2, got)
	}
}
