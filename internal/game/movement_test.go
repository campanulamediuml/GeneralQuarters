package game

import (
	"encoding/json"
	"testing"
)

func TestFleetMovementIntervalsAndCooldown(t *testing.T) {
	state := sandboxState(t)
	leader := state.Units["a-bb-01"]
	if interval := FleetMoveInterval(state, leader); interval != 3 {
		t.Fatalf("普通舰队移动间隔错误：%d", interval)
	}
	order := Order{Type: "SET_MOVE", UnitID: leader.ID, Coord: "0203"}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatal(err)
	}
	ApplyImmediateMove(&state, order)
	for _, unit := range state.Units {
		if unit.Faction == "A" && FleetKey(unit) == "TG-A" && unit.MoveCooldown != 3 {
			t.Fatalf("移动后没有给整队设置3回合冷却：%+v", unit)
		}
	}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("移动后允许无视冷却再次移动")
	}
	for turn := 1; turn <= 2; turn++ {
		ResolveOrders(&state, nil)
		if state.Units[leader.ID].MoveCooldown != 3-turn {
			t.Fatalf("第%d回合冷却递减错误：%d", turn, state.Units[leader.ID].MoveCooldown)
		}
		if err := ValidateOrder(state, "A", order); err == nil {
			t.Fatalf("第%d回合仍允许提前移动", turn)
		}
	}
	ResolveOrders(&state, nil)
	if state.Units[leader.ID].MoveCooldown != 0 {
		t.Fatal("第三回合后仍未恢复移动")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: leader.ID, Coord: "0202"}); err != nil {
		t.Fatalf("冷却结束后仍禁止移动：%v", err)
	}
}

func TestSpecialFleetSpeedsAndFlooding(t *testing.T) {
	destroyer := &Unit{ID: "dd", FleetID: "dd", Type: "DD", Card: ShipCard{SpeedClass: "fast", DamageControl: 5}}
	state := State{Units: map[string]*Unit{"dd": destroyer}}
	if interval := FleetMoveInterval(state, destroyer); interval != 2 {
		t.Fatalf("纯高速驱逐舰间隔应为2：%d", interval)
	}
	battleship := &Unit{ID: "bb", FleetID: "dd", Type: "BB", Card: ShipCard{SpeedClass: "normal", DamageControl: 7}}
	state.Units["bb"] = battleship
	if interval := FleetMoveInterval(state, destroyer); interval != 3 {
		t.Fatalf("混入普通舰后间隔应为3：%d", interval)
	}
	battleship.Card.SpeedClass = "slow"
	if interval := FleetMoveInterval(state, destroyer); interval != 5 {
		t.Fatalf("慢速舰队间隔应为5：%d", interval)
	}
	battleship.Card.SpeedClass = "very_slow"
	if interval := FleetMoveInterval(state, destroyer); interval != 10 {
		t.Fatalf("极慢舰队间隔应为10：%d", interval)
	}
	EnsureDamage(battleship).Disabled = true
	if interval := FleetMoveInterval(state, destroyer); interval != 24 {
		t.Fatalf("失去动力间隔应为24：%d", interval)
	}
	battleship.Damage.Disabled = false
	battleship.Card.SpeedClass = "normal"
	battleship.Damage.Flooding = []int{1}
	if speed := EffectiveSpeedClass(battleship); speed != "slow" {
		t.Fatalf("进水没有降低航速档位：%s", speed)
	}
}

func TestMovementAndDamagePersistInStateJSON(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["a-bb-01"]
	unit.MoveCooldown = 3
	unit.Confusion = 2
	damage := EnsureDamage(unit)
	damage.Fire = "burning"
	damage.Flooding = []int{1}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	unit = restored.Units["a-bb-01"]
	if unit.MoveCooldown != 3 || unit.Confusion != 2 || unit.Damage == nil || unit.Damage.Fire != "burning" || len(unit.Damage.Flooding) != 1 {
		t.Fatalf("移动和损伤状态没有完整持久化：%+v", unit)
	}
}

func TestFloodingBeyondDamageControlDisablesPermanently(t *testing.T) {
	unit := &Unit{ID: "bb", Type: "BB", Card: ShipCard{SpeedClass: "normal", DamageControl: 1}}
	state := State{RNGState: 42, Units: map[string]*Unit{"bb": unit}}
	unit.Damage = &ShipDamage{Flooding: []int{1, 1}}
	AdvanceShipConditions(&state)
	unit.Damage.Flooding = []int{}
	if !unit.Damage.Disabled || EffectiveSpeedClass(unit) != "disabled" {
		t.Fatal("进水超过损管后没有永久失去动力")
	}
}
