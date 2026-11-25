package game

import (
	"strings"
	"testing"
)

// 投弹顺序：水平轰炸 → 俯冲轰炸 → 鱼雷。先到的波次打掉防空、点起火，后到的波次收人头。
func TestBombingWaveOrderLevelDiveTorpedo(t *testing.T) {
	build := func() (*State, []*AirUnit) {
		state := &State{
			RNGState: 7,
			Units:    map[string]*Unit{"bb": {ID: "bb", FleetID: "b", Faction: "B", Type: "BB", Coord: "0202", Strength: 6, MaxStrength: 6, Card: ShipCard{DamageControl: 10}}},
			Aircraft: map[string]*AirUnit{},
		}
		mk := func(id, kind, model string) *AirUnit {
			state.Aircraft[id] = &AirUnit{ID: id, Faction: "A", Kind: kind, Model: model, Status: "attack", Coord: "0202", Strength: 3, AntiShip: 1, Loadout: LoadoutBomb}
			return state.Aircraft[id]
		}
		// 故意按 鱼雷 → 俯冲 → 水平 的顺序塞进去，验证结算顺序由机种决定
		return state, []*AirUnit{
			mk("t1", "torpedo_bomber", "TBF"),
			mk("d1", "dive_bomber", "SBD"),
			mk("l1", "strategic_bomber", "B-17"),
		}
	}
	state, bombers := build()
	report := &AirCombatReport{}
	events := resolveBombingWave(state, bombers, []Unit{*state.Units["bb"]}, true, true, report)
	got := []string{}
	for _, event := range events {
		if event.Type != "strike_bombing" {
			continue
		}
		got = append(got, strings.SplitN(event.Text, "攻击", 2)[0])
	}
	want := []string{"B-17", "SBD", "TBF"}
	if len(got) != len(want) {
		t.Fatalf("投弹波次数量不对：%v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("投弹顺序错误：期望 %v，实际 %v", want, got)
		}
	}
}

// 挂弹战斗机对舰是投弹，不是扫射：按炸弹波结算，伤害吃对舰值 + 重型炸弹加成。
func TestBombArmedFighterBombsShips(t *testing.T) {
	state := buildStrikeState()
	addDefenderFleet(state, "bb-a", "BB", "0202", 0, 0)
	fighter := addAttacker(state, "f1", "fighter", 2, 3, LoadoutBomb)
	report := &AirCombatReport{}
	resolveBombingWave(state, []*AirUnit{fighter}, []Unit{*state.Units["bb-a"]}, false, false, report)
	if report.BombAttempts != 5 { // 3 点对舰 + 重型炸弹 2 点
		t.Fatalf("挂弹战斗机应按对舰值+重型炸弹结算，尝试次数 %d（期望5）", report.BombAttempts)
	}
	if len(report.TargetAllocations) != 1 {
		t.Fatalf("挂弹战斗机应进入目标分配，实际 %d 条", len(report.TargetAllocations))
	}
}

// 不挂弹的护航战斗机不参与对舰投弹。
func TestAirLoadoutFighterDoesNotBombShips(t *testing.T) {
	state := buildStrikeState()
	addDefenderFleet(state, "bb-a", "BB", "0202", 0, 0)
	fighter := addAttacker(state, "f1", "fighter", 2, 3, LoadoutAirTank)
	report := &AirCombatReport{}
	resolveBombingWave(state, []*AirUnit{fighter}, []Unit{*state.Units["bb-a"]}, false, false, report)
	if report.BombAttempts != 0 || len(report.TargetAllocations) != 0 {
		t.Fatalf("不挂弹的护航战斗机不该投弹：attempts=%d allocations=%d", report.BombAttempts, len(report.TargetAllocations))
	}
}

// 挂弹战斗机算炸弹机，不因为 kind=fighter 被排除出投弹名单。
func TestStrikeBomberIncludesBombArmedFighter(t *testing.T) {
	if !strikeBomber(&AirUnit{Kind: "fighter", Loadout: LoadoutBomb}) {
		t.Fatal("挂弹战斗机应算投弹机")
	}
	if strikeBomber(&AirUnit{Kind: "fighter", Loadout: LoadoutAirTank}) {
		t.Fatal("不挂弹的护航战斗机不该算投弹机")
	}
	if !strikeBomber(&AirUnit{Kind: "dive_bomber", Loadout: LoadoutBombTank}) {
		t.Fatal("俯冲轰炸机应算投弹机")
	}
}

// 挂弹战斗机按攻击机整备；不挂弹的护航战斗机不算。
func TestBombArmedFighterCanArmForAttack(t *testing.T) {
	if !armedTaskMatches("attack", &AirUnit{Kind: "fighter", Loadout: LoadoutBomb}) {
		t.Fatal("挂弹战斗机应能作为攻击机整备")
	}
	if armedTaskMatches("attack", &AirUnit{Kind: "fighter", Loadout: LoadoutAirTank}) {
		t.Fatal("不挂弹的护航战斗机不该按攻击机整备")
	}
}
