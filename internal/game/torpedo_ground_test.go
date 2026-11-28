package game

import (
	"strings"
	"testing"
)

// 只有炸弹能对地；鱼雷只对舰，其他挂载的两种攻击值都为 0。
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
		{LoadoutAirTank, 4, 0},
		{LoadoutReconTank, 4, 0},
		{LoadoutTransfer, 4, 0},
		{LoadoutDepthCharge, 4, 0},
		{LoadoutNone, 4, 0},
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

	shipCases := []struct {
		loadout Loadout
		want    int
	}{
		{LoadoutBombTank, 5},
		{LoadoutBomb, 7},
		{LoadoutTorpedo, 5},
		{LoadoutTorpedoTank, 5},
		{LoadoutReconTank, 0},
		{LoadoutTransfer, 0},
		{LoadoutAirTank, 0},
		{LoadoutDepthCharge, 0},
		{LoadoutNone, 0},
	}
	for _, test := range shipCases {
		if got := EffectiveAntiShip(test.loadout, 5); got != test.want {
			t.Fatalf("%s 对舰值应为 %d，实际 %d", test.loadout.Label(), test.want, got)
		}
		if got := withLoadoutAttackGround(AirfieldAirUnit{Loadout: test.loadout, AntiShip: 5}).AntiShip; got != test.want {
			t.Fatalf("机场视图 %s 对舰值应为 %d，实际 %d", test.loadout.Label(), test.want, got)
		}
	}
}

// 熟练度加成也不能把鱼雷机的对地值救回来：临场发挥的 +1 只加在有效攻击值上，
// 挂鱼雷时对地仍然是 0——否则"精英机组带鱼雷炸机场"就成了规则漏洞。
func TestPilotSkillCannotRestoreTorpedoGroundAttack(t *testing.T) {
	unit := AirUnit{Model: "九七舰攻", Skill: PilotAce, AirCombat: 6, AntiShip: 5,
		AntiGround: 4, Loadout: LoadoutTorpedo}
	if view := withLoadoutAttack(unit); view.AntiGround != 0 {
		t.Fatalf("王牌挂鱼雷时对地仍应为 0，实际 %d", view.AntiGround)
	}
	// 熟练度本身不改对地值：挂炸弹时对地就是卡片原值。
	unit.Loadout = LoadoutBombTank
	if view := withLoadoutAttack(unit); view.AntiGround != 4 {
		t.Fatalf("熟练度不该给对地加成，挂炸弹应为 4，实际 %d", view.AntiGround)
	}
}

func TestZeroAttackValueSkipsPilotBonus(t *testing.T) {
	previous := dcCheckRate
	dcCheckRate = 100
	t.Cleanup(func() { dcCheckRate = previous })

	for _, loadout := range []Loadout{LoadoutNone, LoadoutReconTank, LoadoutAirTank, LoadoutTransfer, LoadoutDepthCharge} {
		state := sandboxState(t)
		target := firstEnemyShip(&state, "B")
		attacker := &AirUnit{ID: "zero-ship", Faction: "A", Kind: "dive_bomber", Status: "attack", Skill: PilotAce,
			AntiShip: 5, AntiGround: 4, Loadout: loadout}
		state.Aircraft[attacker.ID] = attacker
		report := &AirCombatReport{}
		resolveBombingOn(&state, attacker, target, false, true, report)
		if report.BombAttempts != 0 {
			t.Fatalf("%s 对舰不应获得临场加成，投弹数为 %d", loadout.Label(), report.BombAttempts)
		}

		groundState := sandboxState(t)
		airfield := groundState.Airfields["af-b-sentry"]
		groundState.Aircraft = map[string]*AirUnit{
			"zero-ground": {ID: "zero-ground", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C",
				Status: "attack", Skill: PilotAce, Strength: 2, AntiGround: 4, TaskID: "ground-task", TaskKind: "attack", Loadout: loadout,
				Coord: airfield.Coord, TargetCoord: airfield.Coord, RemainingRange: 8, MaxRange: 12},
		}
		resolveGroundStrike(&groundState, "ground-task", airfield.Coord)
		groundReport := groundState.AirCombats[len(groundState.AirCombats)-1]
		if groundReport.BombAttempts != 0 {
			t.Fatalf("%s 对地不应获得临场加成，投弹数为 %d", loadout.Label(), groundReport.BombAttempts)
		}
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
