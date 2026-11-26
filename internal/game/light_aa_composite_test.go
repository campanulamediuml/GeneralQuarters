package game

import "testing"

// 复合编队不拆轻防空骰子：整队飞机进场只挨一轮、撤离只挨一轮，
// 波次只决定投弹顺序。旧写法是按波次各掷一副，混编会白掷两倍骰子。
func TestCompositePackageRollsLightAAOncePerSide(t *testing.T) {
	const runs = 2000
	measure := func(kinds []string) float64 {
		total := 0
		for i := 0; i < runs; i++ {
			state := &State{
				RNGState:  uint64(7919*i+13)*2654435761 + 1,
				Map:       MapDefinition{Rows: 7, Cols: 9},
				Doctrines: map[string]string{"A": "us", "B": "japan"},
				Units: map[string]*Unit{
					"bb": {ID: "bb", Faction: "B", FleetID: "b", Type: "BB", Coord: "0202", Name: "bb",
						Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", LightAA: 10, DamageControl: 10}},
				},
				Aircraft: map[string]*AirUnit{},
			}
			var planes []*AirUnit
			for j, kind := range kinds {
				id := "a" + itoa(j)
				loadout := LoadoutBomb
				if kind == "torpedo_bomber" {
					// 鱼雷机的翻倍只对真正挂鱼雷的出击生效：这里必须挂鱼雷。
					loadout = LoadoutTorpedoTank
				}
				state.Aircraft[id] = &AirUnit{ID: id, Faction: "A", Kind: kind, Model: "X", Status: "attack",
					Coord: "0202", Strength: 4, AntiShip: 4, Loadout: loadout}
				planes = append(planes, state.Aircraft[id])
			}
			report := &AirCombatReport{}
			resolveBombingWave(state, planes, []Unit{*state.Units["bb"]}, true, false, report)
			total += report.AAHits
		}
		return float64(total) / runs
	}
	// 两波各一副骰子会跑到 0.9 附近；整队两副应该是 2 × 10 × 2.25% ≈ 0.45。
	mixed := measure([]string{"dive_bomber", "dive_bomber", "torpedo_bomber", "torpedo_bomber"})
	if mixed > 0.62 {
		t.Fatalf("复合编队把轻防空按波次拆了：平均 %.2f 次命中，期望 ≈0.45", mixed)
	}
	if mixed < 0.30 {
		t.Fatalf("复合编队轻防空骰子太少：平均 %.2f 次命中，期望 ≈0.45", mixed)
	}
}
