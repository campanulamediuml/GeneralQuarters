package game

import (
	"slices"
	"testing"
)

func TestCoastalAirfieldFleetEntry(t *testing.T) {
	const target = "0202"
	for _, tc := range []struct {
		name, terrain, neighbor, escort string
		want                            bool
	}{
		{name: "coastal island", terrain: "island", neighbor: "water", want: true},
		{name: "coastal shoal", terrain: "shoal", neighbor: "water", want: true},
		{name: "inland airport", terrain: "island", neighbor: "shoal"},
		{name: "reef neighbors", terrain: "shoal", neighbor: "reef"},
		{name: "fleet with CV", terrain: "island", neighbor: "water", escort: "CV"},
		{name: "fleet with CVL", terrain: "shoal", neighbor: "water", escort: "CVL"},
		{name: "fleet with CVE at seaplane base", terrain: "water", neighbor: "water", escort: "CVE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unit := &Unit{ID: "dd", Type: "DD", Faction: "A", FleetID: "fleet", Coord: "0201"}
			state := State{
				Map:       MapDefinition{Rows: 3, Cols: 3, Cells: []MapCell{{Coord: target, Terrain: tc.terrain}}},
				Airfields: map[string]*Airfield{"base": {ID: "base", Coord: target, Faction: "B", Type: AirfieldSeaplane}},
				Units:     map[string]*Unit{unit.ID: unit},
			}
			for _, coord := range Neighbors(target) {
				if coord != unit.Coord || tc.neighbor != "water" {
					terrain := tc.neighbor
					if terrain == "water" {
						terrain = "island"
					}
					state.Map.Cells = append(state.Map.Cells, MapCell{Coord: coord, Terrain: terrain})
				}
			}
			if tc.escort != "" {
				state.Units["carrier"] = &Unit{ID: "carrier", Type: tc.escort, Faction: "A", FleetID: unit.FleetID, Coord: unit.Coord}
			}
			if got := slices.Contains(LegalMoves(state, unit), target); got != tc.want {
				t.Errorf("legal moves include airport = %v, want %v", got, tc.want)
			}
			for _, orderType := range []string{"SET_MOVE", "SET_AUTO_NAV"} {
				if err := ValidateOrder(state, "A", Order{Type: orderType, UnitID: unit.ID, Coord: target}); (err == nil) != tc.want {
					t.Errorf("%s: %v, want legal = %v", orderType, err, tc.want)
				}
			}
			// Runtime auto-navigation must also respect the rule for an already stored destination.
			unit.Destination = target
			if next, reason := autoNavStep(&state, unit); (next == target) != tc.want {
				t.Errorf("auto-nav next = %q (%s), want airport = %v", next, reason, tc.want)
			}
		})
	}
}

func TestMapEdgeDoesNotCountAsCoastalWater(t *testing.T) {
	gameMap := MapDefinition{Rows: 2, Cols: 2, Cells: []MapCell{
		{Coord: "0202", Terrain: "island"}, {Coord: "0201", Terrain: "shoal"},
		{Coord: "0102", Terrain: "reef"}, {Coord: "0101", Terrain: "island"},
	}}
	if fleetTerrainAllowsEntry(gameMap, "0202", false, true) {
		t.Fatal("out-of-bounds hex must not be treated as ocean")
	}
}

// 水面舰队不能直接开上岛屿：只有机场/港口 hex 有靠泊例外，
// 普通 island 格必须绕行。之前非航母舰队被放行到任何非礁石地形，
// 结果运输队从科科达小径横穿新几内亚。
func TestSurfaceFleetCannotCrossIsland(t *testing.T) {
	gameMap := MapDefinition{Rows: 3, Cols: 3, Cells: []MapCell{
		{Coord: "0202", Terrain: "island"},
		{Coord: "0201", Terrain: "shoal"},
	}}
	if fleetTerrainAllowsEntry(gameMap, "0202", false, false) {
		t.Fatal("非航母舰队不应进入普通 island 格")
	}
	if !fleetTerrainAllowsEntry(gameMap, "0201", false, false) {
		t.Fatal("非航母舰队应能进入 shoal")
	}
	if fleetTerrainAllowsEntry(gameMap, "0201", true, false) {
		t.Fatal("航母舰队不应进入 shoal")
	}
	if fleetTerrainAllowsEntry(gameMap, "0202", true, false) {
		t.Fatal("航母舰队不应进入 island")
	}
}
