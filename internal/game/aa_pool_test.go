package game

import (
	"fmt"
	"testing"
)

// 轻防空是一副骰子：本舰防空值多少点就掷多少次，摊在"正在攻击本舰"的整队飞机上。
// 旧写法是每架攻击机各掷一轮本舰全值，五架飞机打一艘船会白掷五倍的骰子。
func TestLightAAPoolRollsShipValueNotPerAircraft(t *testing.T) {
	const runs = 400
	total := 0
	for i := 0; i < runs; i++ {
		state := State{
			RNGState: uint64(i + 1),
			Units: map[string]*Unit{
				"bb": {ID: "bb", Faction: "B", Type: "BB", Coord: "0202", Strength: 6, MaxStrength: 6, Card: ShipCard{LightAA: 10, DamageControl: 10}},
			},
			Aircraft: map[string]*AirUnit{},
		}
		planes := []*AirUnit{}
		for j := 0; j < 5; j++ {
			id := fmt.Sprintf("a%d", j)
			state.Aircraft[id] = &AirUnit{ID: id, Faction: "A", Kind: "torpedo_bomber", Strength: 3, Status: "attack", Coord: "0202"}
			planes = append(planes, state.Aircraft[id])
		}
		report := &AirCombatReport{}
		resolveLightAAPool(&state, planes, state.Units["bb"], false, report)
		total += report.AAHits
	}
	mean := float64(total) / runs
	// 10 点防空、对鱼雷机每颗骰子 6%：期望 0.6 次命中；
	// 按"每架飞机各掷一轮"的旧写法会变成 3.0 次。
	if mean > 1.2 {
		t.Fatalf("轻防空按机掷骰了：平均 %.2f 次命中，期望 ≈0.6", mean)
	}
	if mean < 0.2 {
		t.Fatalf("轻防空骰子太少：平均 %.2f 次命中，期望 ≈0.6", mean)
	}
}

// 重防空也是同一副骰子：命中一架就把它移出目标池，飞机打光后剩下的骰子没有目标。
func TestHeavyAAPoolStopsWhenAircraftRemoved(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		state := State{
			RNGState: seed,
			Units: map[string]*Unit{
				"bb": {ID: "bb", Faction: "B", Type: "BB", Coord: "0202", Strength: 6, MaxStrength: 6, Card: ShipCard{HeavyAA: 500, DamageControl: 10}},
			},
			Aircraft: map[string]*AirUnit{},
		}
		planes := []*AirUnit{}
		for j := 0; j < 3; j++ {
			id := fmt.Sprintf("a%d", j)
			// 强度 3：撤退 40% / 损伤 30% / 摧毁 30%，没有"无效果"，每命中一次必定出池。
			state.Aircraft[id] = &AirUnit{ID: id, Faction: "A", Kind: "dive_bomber", Strength: 3, Status: "attack", Coord: "0202"}
			planes = append(planes, state.Aircraft[id])
		}
		report := &AirCombatReport{}
		resolveHeavyAA(&state, planes, []Unit{*state.Units["bb"]}, "0202", report)
		if report.AAHits > 3 {
			t.Fatalf("命中 %d 次超过飞机总数 3：被打掉的算子没有被移出目标池", report.AAHits)
		}
	}
}

// 舰队对轰的目标池只剔除被彻底击沉的船：已经沉掉的不再吃炮弹，
// 但受损没沉的仍然留在池里。
func TestSunkShipLeavesGunneryTargetPool(t *testing.T) {
	total := 0
	for seed := uint64(1); seed <= 40; seed++ {
		state := State{RNGState: seed, Doctrines: map[string]string{}, Units: map[string]*Unit{
			"a": {ID: "a", FleetID: "a", Type: "BB", Faction: "A", Coord: "0101", Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 16, DamageControl: 7}},
			"b": {ID: "b", FleetID: "b", Type: "CA", Faction: "B", Coord: "0101", Strength: 4, MaxStrength: 4, Card: ShipCard{DamageControl: 6}},
		}}
		// "dead" 不在 state.Units 里 = 已经沉没
		dead := Unit{ID: "dead", FleetID: "b", Type: "DD", Faction: "B", Coord: "0101"}
		damaged := state.Units["b"]
		damaged.Strength = 1 // 受损但没沉：必须继续留在目标池
		hits, _ := gunneryVolley(&state, []Unit{*state.Units["a"]}, []Unit{dead, *damaged}, false)
		for _, hit := range hits {
			if hit.TargetID == "dead" {
				t.Fatalf("沉没的舰还在吃炮弹")
			}
		}
		total += len(hits)
	}
	if total == 0 {
		t.Fatal("40 个种子一次都没命中，判定概率明显不对")
	}
}
