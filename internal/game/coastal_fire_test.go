package game

import "testing"

// 岸炮是每回合的常备火力，不是"只有挨岸轰才还击":舰队停在机场格上,
// 哪怕这一回合什么都没打（比如刚打完一轮、正在混乱里等冷却），岸炮照样开火。
func TestCoastalFireFiresEveryTurn(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Coastal = 40
	state.Units["a-dd-coastal"] = &Unit{
		ID: "a-dd-coastal", FleetID: "TRS-A", FleetName: "运输队", Name: "驱逐舰", Type: "DD",
		Faction: "A", Coord: airfield.Coord, Strength: 200, MaxStrength: 200,
		Card: ShipCard{DamageControl: 20},
	}
	state.RNGState = 11
	for turn := 1; turn <= 3; turn++ {
		events := resolveCoastalFire(&state)
		found := false
		for _, event := range events {
			if event.Type == "coastal_fire" {
				found = true
			}
		}
		if !found {
			t.Fatalf("第 %d 回合岸炮没有开火", turn)
		}
	}
}

// 岸炮只打敌人：自家舰队停在自家机场格上不会被自己的岸炮轰。
func TestCoastalFireSparesOwnShips(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Coastal = 40
	state.Units["b-cl-owned"] = &Unit{
		ID: "b-cl-owned", FleetID: "B-OWN", FleetName: "自家编队", Name: "轻巡", Type: "CL",
		Faction: airfield.Faction, Coord: airfield.Coord, Strength: 5, MaxStrength: 5,
		Card: ShipCard{DamageControl: 5},
	}
	state.RNGState = 11
	for _, event := range resolveCoastalFire(&state) {
		if event.Type == "coastal_fire" {
			t.Fatalf("自家舰船不该被自家岸炮轰击：%s", event.Text)
		}
	}
}

// 岸炮打空以后就不再开火。
func TestCoastalFireStopsAtZero(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Coastal = 0
	state.Units["a-ca-coastal"] = &Unit{
		ID: "a-ca-coastal", FleetID: "TRS-A", FleetName: "运输队", Name: "重巡", Type: "CA",
		Faction: "A", Coord: airfield.Coord, Strength: 200, MaxStrength: 200,
		Card: ShipCard{DamageControl: 20},
	}
	state.RNGState = 11
	if events := resolveCoastalFire(&state); len(events) != 0 {
		t.Fatalf("岸炮清零后不该再有射击事件：%+v", events)
	}
}

// 同一格里有两支敌方舰队时，岸炮对每一支各打一遍（两条事件），
// 不是把整格的舰船混在一起只开一轮。
func TestCoastalFireHitsEachFleetOnce(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	airfield.Coastal = 40
	state.Units["a-fleet-one"] = &Unit{
		ID: "a-fleet-one", FleetID: "TF-ONE", FleetName: "第一编队", Name: "驱逐舰", Type: "DD",
		Faction: "A", Coord: airfield.Coord, Strength: 200, MaxStrength: 200,
		Card: ShipCard{DamageControl: 20},
	}
	state.Units["a-fleet-two"] = &Unit{
		ID: "a-fleet-two", FleetID: "TF-TWO", FleetName: "第二编队", Name: "驱逐舰", Type: "DD",
		Faction: "A", Coord: airfield.Coord, Strength: 200, MaxStrength: 200,
		Card: ShipCard{DamageControl: 20},
	}
	state.RNGState = 11
	volleys := 0
	for _, event := range resolveCoastalFire(&state) {
		if event.Type == "coastal_fire" {
			volleys++
		}
	}
	if volleys != 2 {
		t.Fatalf("两支舰队应当各打一轮，实际 %d 轮", volleys)
	}
}
