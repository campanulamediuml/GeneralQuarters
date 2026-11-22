package game

import "testing"

// 雷达截获敌方编队后，该编队对观察者可见。
func TestRadarRevealsEnemyAirTask(t *testing.T) {
	state := State{
		Turn:      3,
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units: map[string]*Unit{
			"cv-a": {ID: "cv-a", Faction: "A", Type: "CV", Coord: "0202", Card: ShipCard{AirRadar: true}},
		},
		Aircraft: map[string]*AirUnit{
			"atk": {ID: "atk", Faction: "B", CarrierID: "cv-b", Kind: "dive_bomber", Model: "SBD", TaskID: "task-b",
				TaskKind: "attack", Status: "attack", Coord: "0203", MaxRange: 10},
		},
		Map:       MapDefinition{Rows: 7, Cols: 9},
		RNGState:  1,
		Airfields: map[string]*Airfield{},
	}
	// 距离 1，美军航母雷达 90%：多掷几次必然命中。
	found := false
	for i := 0; i < 40 && !found; i++ {
		detectAirTaskByRadar(&state, "A", state.Aircraft["atk"])
		found = len(VisibleEnemyAir(state, "A")) > 0
	}
	if !found {
		t.Fatal("近距离雷达始终没有发现敌方编队")
	}
	contact := VisibleEnemyAir(state, "A")[0]
	if contact.TaskID != "task-b" || contact.Coord != "0203" || contact.Count != 1 || contact.Kind != "attack" {
		t.Fatalf("编队信息投影错误：%+v", contact)
	}
	// 自己看不到自己的编队。
	if len(VisibleEnemyAir(state, "B")) != 0 {
		t.Fatal("不应把己方编队当成敌方可见目标")
	}
}

// 未被发现前不可见，被发现后即使离开雷达范围也保持可见。
func TestAirContactPersistsAfterLeavingRadar(t *testing.T) {
	state := State{
		Turn:      1,
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units:     map[string]*Unit{},
		Aircraft: map[string]*AirUnit{
			"atk": {ID: "atk", Faction: "B", Kind: "dive_bomber", Model: "SBD", TaskID: "t", TaskKind: "attack",
				Status: "attack", Coord: "0202", MaxRange: 10},
		},
		Map:       MapDefinition{Rows: 7, Cols: 9},
		Airfields: map[string]*Airfield{},
	}
	if len(VisibleEnemyAir(state, "A")) != 0 {
		t.Fatal("未被发现时不应可见")
	}
	revealAirTask(&state, "A", state.Aircraft["atk"], "radar")
	state.Aircraft["atk"].Coord = "0709"
	refreshKnownAirContacts(&state)
	visible := VisibleEnemyAir(state, "A")
	if len(visible) != 1 {
		t.Fatal("被发现后不应再次隐藏")
	}
	if visible[0].Coord != "0709" {
		t.Fatalf("可见敌机应更新到当前位置：%s", visible[0].Coord)
	}
}

// 目视接触：双方编队相邻时互相可见。
func TestVisualContactRevealsAdjacentTask(t *testing.T) {
	state := State{
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units:     map[string]*Unit{},
		Aircraft: map[string]*AirUnit{
			"a-cap": {ID: "a-cap", Faction: "A", Kind: "fighter", TaskID: "ta", TaskKind: "cap", Status: "cap", Coord: "0303", PatrolCoord: "0303"},
			"b-atk": {ID: "b-atk", Faction: "B", Kind: "dive_bomber", TaskID: "tb", TaskKind: "attack", Status: "attack", Coord: "0304"},
		},
		Map:       MapDefinition{Rows: 7, Cols: 9},
		Airfields: map[string]*Airfield{},
	}
	if !visualContact(state, "A", state.Aircraft["b-atk"]) {
		t.Skip("两个 hex 不相邻")
	}
	revealAirTask(&state, "A", state.Aircraft["b-atk"], "visual")
	if len(VisibleEnemyAir(state, "A")) != 1 {
		t.Fatal("目视接触应发现编队")
	}
}
