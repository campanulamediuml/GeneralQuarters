package game

import (
	"reflect"
	"strings"
	"testing"
)

func TestGunneryWeightsAndModifiers(t *testing.T) {
	if targetWeight("BB", "CV") != 9 || targetWeight("DD", "CL") != 7 || targetWeight("CA", "BB") != 3 {
		t.Fatal("交战目标权重表错误")
	}
	attacker := &Unit{ID: "a", FleetID: "a", Type: "BB", Faction: "A", Coord: "0101", Card: ShipCard{SurfaceRadar: true}}
	target := &Unit{ID: "b", FleetID: "b", Type: "CA", Faction: "B", Coord: "0101"}
	EnsureDamage(target).Fire = "burning"
	state := State{Map: MapDefinition{Cells: []MapCell{{Coord: "0101", Weather: "rain"}}}, Units: map[string]*Unit{"a": attacker, "b": target}, Aircraft: map[string]*AirUnit{
		"scout": {ID: "scout", CarrierID: "a", Seaplane: true, Status: "recon"},
	}}
	if chance := gunneryChance(state, attacker, target, []Unit{*target}); chance != 50 {
		t.Fatalf("雷达、水机、燃烧和天气修正错误：%.1f", chance)
	}
	attacker.Type = "DD"
	attacker.Card.SurfaceRadar = false
	if chance := gunneryChance(state, attacker, target, []Unit{*target}); chance != 30 {
		t.Fatalf("舰种修正错误：%.1f", chance)
	}
}

func TestSurfaceCombatDeterministic(t *testing.T) {
	newState := func() State {
		return State{RNGState: 42, Year: 1942, Doctrines: map[string]string{"A": "us", "B": "japan"}, Map: MapDefinition{Rows: 2, Cols: 2}, Contacts: map[string]map[string]*FleetContact{}, Units: map[string]*Unit{
			"a": {ID: "a", FleetID: "a", Name: "A", Type: "BB", Faction: "A", Coord: "0101", Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 16, DamageControl: 7, SurfaceRadar: true}},
			"b": {ID: "b", FleetID: "b", Name: "B", Type: "CA", Faction: "B", Coord: "0101", Strength: 4, MaxStrength: 4, Card: ShipCard{Gunnery: 7, DamageControl: 6, TorpedoSalvos: 2}},
		}}
	}
	first, second := newState(), newState()
	firstEvents := resolveContacts(&first)
	secondEvents := resolveContacts(&second)
	if !reflect.DeepEqual(firstEvents, secondEvents) || !reflect.DeepEqual(first, second) {
		t.Fatal("相同随机种子的交战结果不一致")
	}
	hasVolley := false
	for _, event := range firstEvents {
		if event.Type == "gunnery_volley" {
			hasVolley = true
		}
	}
	if !hasVolley {
		t.Fatal("交战没有执行火炮独立判定")
	}
}

func TestTorpedoUsesTargetSpeedAndConsumesSalvo(t *testing.T) {
	attacker := &Unit{ID: "a", FleetID: "a", Name: "攻击舰", Type: "DD", Faction: "A", Coord: "0101", Card: ShipCard{TorpedoSalvos: 2}}
	target := &Unit{ID: "b", FleetID: "b", Name: "目标舰", Type: "BB", Faction: "B", Coord: "0101", Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "slow", DamageControl: 7}}
	state := State{RNGState: 42, Units: map[string]*Unit{"a": attacker, "b": target}, Doctrines: map[string]string{}}
	_, events := torpedoVolley(&state, []Unit{*attacker}, []Unit{*target})
	if attacker.Card.TorpedoSalvos != 1 {
		t.Fatal("鱼雷齐射没有消耗库存")
	}
	if len(events) != 1 || !strings.Contains(events[0].Text, "35.0%") {
		t.Fatalf("鱼雷没有按目标慢速修正命中率：%+v", events)
	}
}

func TestSinkingCarrierRemovesAircraft(t *testing.T) {
	carrier := &Unit{ID: "cv", FleetID: "cv", Name: "航母", Type: "CV", Faction: "A", Strength: 1, MaxStrength: 5, Card: ShipCard{DamageControl: 6}}
	state := State{RNGState: 42, Units: map[string]*Unit{"cv": carrier}, Aircraft: map[string]*AirUnit{
		"air-1": {ID: "air-1", CarrierID: "cv"},
		"air-2": {ID: "air-2", CarrierID: "cv"},
	}}
	applyTorpedoHit(&state, ShellHit{TargetID: "cv", Torpedo: true})
	if state.Units["cv"] != nil || len(state.Aircraft) != 0 {
		t.Fatal("航母沉没后仍残留舰船或航空算子")
	}
}

func TestDamageRepairsAndOrganizationConfusion(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	damage := EnsureDamage(unit)
	damage.Fire = "small"
	damage.DeckDamaged = true
	damage.Flooding = []int{1}
	unit.Confusion = 2
	order := Order{Type: "REORGANIZE_HEX", Coord: unit.Coord, Assignments: map[string]string{}}
	for _, member := range state.Units {
		if member.Faction == "A" && member.Coord == unit.Coord {
			order.Assignments[member.ID] = FleetKey(member)
		}
	}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("混乱舰队被允许整编")
	}
	AdvanceShipConditions(&state)
	AdvanceMovementCooldown(&state)
	if unit.Damage.Fire != "" || unit.Damage.DeckDamaged || len(unit.Damage.Flooding) != 0 || unit.Confusion != 1 {
		t.Fatalf("回合状态修复错误：%+v", unit)
	}
}
