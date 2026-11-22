package game

import (
	"fmt"
	"testing"
)

// 回归：雷达按"每个编队每回合一次"判定，不是按机逐架判定。
// 一支 12 机的编队在 2 hex 处（日军其他雷达舰 40%）如果按机判定，
// 实际 discovery 率会被放大到约 99.8%；按编队判定应该仍然接近 40%。
func TestRadarRollsOncePerFormation(t *testing.T) {
	const trials = 400
	hits := 0
	for i := 0; i < trials; i++ {
		state := State{
			Turn:      1,
			Doctrines: map[string]string{"A": "us", "B": "japan"},
			Units: map[string]*Unit{
				"ca-b": {ID: "ca-b", Faction: "B", Type: "CA", Coord: "0710", Card: ShipCard{AirRadar: true}},
			},
			Aircraft: map[string]*AirUnit{},
			Map:      MapDefinition{Rows: 20, Cols: 20},
			// 每条种子都不同，避免单次运气影响结论。
			RNGState:  uint64(i+1) * 2654435761,
			Airfields: map[string]*Airfield{},
		}
		for n := 0; n < 12; n++ {
			id := fmt.Sprintf("a%02d", n)
			state.Aircraft[id] = &AirUnit{
				ID: id, Faction: "A", Kind: "fighter", Model: "F4F",
				TaskID: "task-a", TaskKind: "cap", Status: "cap", Coord: "0510",
			}
		}
		resolveAirVisibility(&state)
		if len(VisibleEnemyAir(state, "B")) > 0 {
			hits++
		}
	}
	rate := float64(hits) / float64(trials)
	if rate < 0.25 || rate > 0.55 {
		t.Fatalf("雷达应按编队判定一次（2 hex 处约 40%%），实际 %.0f%%", rate*100)
	}
}
