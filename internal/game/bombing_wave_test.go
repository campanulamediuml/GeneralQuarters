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
