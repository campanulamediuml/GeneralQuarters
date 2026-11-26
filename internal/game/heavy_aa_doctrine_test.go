package game

import "testing"

// 日军没有统一火控：重防空只能打正在攻击自己这艘舰的机群。
func TestJapaneseHeavyAAOnlyDefendsItself(t *testing.T) {
	state := State{Map: MapDefinition{Rows: 12, Cols: 12}, Doctrines: map[string]string{"A": DoctrineUS, "B": DoctrineJapan}}
	if rulesFor(state, "A").FleetHeavyAA != true {
		t.Fatal("美军应当整队合并重防空")
	}
	if rulesFor(state, "B").FleetHeavyAA {
		t.Fatal("日军不该整队合并重防空")
	}

	ship := &Unit{ID: "s", Name: "大和", Faction: "B", Type: "BB", Coord: "0303",
		Card: ShipCard{HeavyAA: 5000}}
	attackers := []*AirUnit{
		{ID: "a1", Faction: "A", Kind: "dive_bomber", Status: "attack", Coord: "0303", Strength: 1},
		{ID: "a2", Faction: "A", Kind: "dive_bomber", Status: "attack", Coord: "0303", Strength: 1},
	}
	state.Aircraft = map[string]*AirUnit{"a1": attackers[0], "a2": attackers[1]}
	report := AirCombatReport{}
	events := resolveShipHeavyAA(&state, attackers, ship, &report)
	if report.AAHits == 0 {
		t.Fatalf("这艘舰 5000 点重防空打自己头上的机群，一次都没命中：%+v", events)
	}
	if len(resolveShipHeavyAA(&state, nil, ship, &report)) != 0 {
		t.Fatal("没有攻击机时不该开火")
	}
}
