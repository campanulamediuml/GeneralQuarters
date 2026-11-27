package game

import (
	"strings"
	"testing"
)

// 挂鱼雷的飞机对地值按 0 算：鱼雷只能打船。
func TestTorpedoLoadoutZeroesAntiGround(t *testing.T) {
	cases := []struct {
		loadout Loadout
		base    int
		want    int
	}{
		{LoadoutTorpedo, 4, 0},
		{LoadoutTorpedoTank, 4, 0},
		{LoadoutBombTank, 4, 4},
		{LoadoutBomb, 4, 6}, // 重型炸弹 +2
		{LoadoutAirTank, 4, 4},
	}
	for _, c := range cases {
		if got := EffectiveAntiGround(c.loadout, c.base); got != c.want {
			t.Fatalf("%s 对地值应为 %d，实际 %d", c.loadout.Label(), c.want, got)
		}
	}
	// 视图也要跟着变：玩家看到的对地值不能还是原值。
	unit := AirUnit{Model: "九七舰攻", Loadout: LoadoutTorpedo, AntiShip: 5, AntiGround: 4}
	view := withLoadoutAttack(unit)
	if view.AntiGround != 0 || view.AntiShip != 5 {
		t.Fatalf("视图对地应为 0、对舰保持 5：%+v", view)
	}
	unit.Loadout = LoadoutBomb
	if view := withLoadoutAttack(unit); view.AntiGround != 6 {
		t.Fatalf("重型炸弹视图对地应为 6，实际 %d", view.AntiGround)
	}
}

// 挂鱼雷的舰攻编队飞到机场上空打不出任何伤害，并且要有一条明确的战报。
func TestTorpedoPlaneCannotBombAirfield(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	before := airfield.Strength
	state.Aircraft = map[string]*AirUnit{
		"torp-1": {ID: "torp-1", CarrierID: "a2-cv-01", Faction: "A", Kind: "torpedo_bomber", Model: "B5N2",
			Status: "attack", Strength: 2, AirCombat: 1, AntiGround: 4, AntiShip: 5,
			TaskID: "ground-task", TaskKind: "attack", Loadout: LoadoutTorpedo,
			Coord: airfield.Coord, TargetCoord: airfield.Coord, RemainingRange: 8, MaxRange: 12},
	}
	state.RNGState = 7
	events := resolveGroundStrike(&state, "ground-task", airfield.Coord)
	if airfield.Strength != before {
		t.Fatalf("挂鱼雷的飞机不该打掉机场生命值：%d -> %d", before, airfield.Strength)
	}
	blocked := false
	for _, event := range events {
		if event.Type == "ground_strike_blocked" && strings.Contains(event.Text, "B5N2") {
			blocked = true
		}
		if event.Type == "ground_strike" {
			t.Fatalf("挂鱼雷的飞机不该有对地命中：%s", event.Text)
		}
	}
	if !blocked {
		t.Fatalf("应给出挂鱼雷无法对地的战报：%+v", events)
	}

	// 同一架飞机换成炸弹就能炸跑道。
	for seed := uint64(1); seed <= 120; seed++ {
		state := sandboxState(t)
		airfield := state.Airfields["af-b-sentry"]
		before := airfield.Strength
		state.Aircraft = map[string]*AirUnit{
			"bomb-1": {ID: "bomb-1", CarrierID: "a2-cv-01", Faction: "A", Kind: "torpedo_bomber", Model: "B5N2",
				Status: "attack", Strength: 2, AirCombat: 1, AntiGround: 4, AntiShip: 5,
				TaskID: "ground-task", TaskKind: "attack", Loadout: LoadoutBombTank,
				Coord: airfield.Coord, TargetCoord: airfield.Coord, RemainingRange: 8, MaxRange: 12},
		}
		state.RNGState = seed
		resolveGroundStrike(&state, "ground-task", airfield.Coord)
		if airfield.Strength < before {
			return
		}
	}
	t.Fatal("换成炸弹后 120 个种子都没打中机场，对地判定可能坏了")
}
