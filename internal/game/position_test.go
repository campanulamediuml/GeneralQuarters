package game

import (
	"strings"
	"testing"
)

// 接战阵位规则（取代旧的"原地停留 3 回合 = T 优"）：
// 停船本身不再带来任何好处，阵位只看机动和航速——至少一方本回合走了一格才判定，
// 航速差 ≥3 节时快的一方抢到阵位，慢的一方第一轮炮战只有一半火炮能开火。
//
// 测试用的周期换算：{3} = 25 节，{3,3,3,4} = 23 节（差 2 节，咬住），{4} = 18 节。

func positionState(attackerCycle, defenderCycle []int, attackerMoved, defenderMoved bool, defenderTypes []string) *State {
	units := map[string]*Unit{}
	for i, typ := range defenderTypes {
		id := "d" + string(rune('a'+i))
		units[id] = &Unit{ID: id, FleetID: "df", Name: "守方" + typ, Type: typ, Faction: "B",
			Coord: "0101", Strength: 6, MaxStrength: 6, Moved: defenderMoved,
			Card: ShipCard{Gunnery: 16, DamageControl: 7, SpeedClass: "normal", MoveCycle: defenderCycle}}
	}
	units["a1"] = &Unit{ID: "a1", FleetID: "af", Name: "突击舰", Type: "BB", Faction: "A",
		Coord: "0101", Strength: 6, MaxStrength: 6, Moved: attackerMoved,
		Card: ShipCard{Gunnery: 16, DamageControl: 7, SpeedClass: "normal", MoveCycle: attackerCycle}}
	return &State{
		RNGState:      4242,
		Map:           MapDefinition{Rows: 2, Cols: 2},
		Contacts:      map[string]map[string]*FleetContact{},
		Doctrines:     map[string]string{"A": "us", "B": "japan"},
		FleetSessions: map[string]int{},
		Units:         units,
	}
}

func factionsOf(state *State) map[string][]Unit {
	factions := map[string][]Unit{}
	for _, unit := range state.Units {
		factions[unit.Faction] = append(factions[unit.Faction], *unit)
	}
	return factions
}

// 快的一方机动进入，抢到阵位。
func TestPositionGoesToFasterMover(t *testing.T) {
	state := positionState([]int{3}, []int{4}, true, false, []string{"BB", "CA"})
	seized, crossed, reason := battlePositionState(state, factionsOf(state))
	if seized != "A" || crossed != "B" {
		t.Fatalf("25 节撞 18 节应该由 A 抢到阵位：seized=%q crossed=%q", seized, crossed)
	}
	if !strings.Contains(reason, "25") || !strings.Contains(reason, "18") {
		t.Fatalf("判定依据该带上双方航速：%q", reason)
	}
}

// 停船不再自动送阵位：双方都不动，哪怕已经摆了 3 回合、也早就发现了对方。
func TestStationaryFleetNoLongerSeizesPosition(t *testing.T) {
	state := positionState([]int{3}, []int{4}, false, false, []string{"BB", "CA"})
	state.FleetSessions["df"] = 3
	state.Contacts["B"] = map[string]*FleetContact{"af": {FleetID: "af", Faction: "A", Level: 2, Coord: "0101"}}
	if seized, crossed, _ := battlePositionState(state, factionsOf(state)); seized != "" || crossed != "" {
		t.Fatalf("没人机动时不该有阵位：seized=%q crossed=%q", seized, crossed)
	}
}

// 慢船一头撞进横队：自己吃压制，不是抢位的一方占便宜。
func TestSlowChargeGetsCrossed(t *testing.T) {
	state := positionState([]int{4}, []int{3}, true, false, []string{"BB", "CA"})
	seized, crossed, reason := battlePositionState(state, factionsOf(state))
	if seized != "B" || crossed != "A" {
		t.Fatalf("18 节撞 25 节应该由 B 抢到阵位：seized=%q crossed=%q", seized, crossed)
	}
	if !strings.Contains(reason, "撞进") {
		t.Fatalf("慢速抢入的判定依据该写明撞进横队：%q", reason)
	}
}

// 航速差不到 3 节：咬住了，谁也没占到位。
func TestCloseSpeedsCancelPosition(t *testing.T) {
	state := positionState([]int{3}, []int{3, 3, 3, 4}, true, false, []string{"BB", "CA"})
	if seized, crossed, _ := battlePositionState(state, factionsOf(state)); seized != "" || crossed != "" {
		t.Fatalf("25 节对 23 节不该有阵位：seized=%q crossed=%q", seized, crossed)
	}
}

// 编队里有航母或运输船：摆不出战列，双方都不判定。
func TestCarrierFormationSkipsPosition(t *testing.T) {
	for _, typ := range []string{"CV", "CVL", "CVE", "TRS"} {
		state := positionState([]int{3}, []int{4}, true, false, []string{"BB", typ})
		if seized, crossed, _ := battlePositionState(state, factionsOf(state)); seized != "" || crossed != "" {
			t.Fatalf("编队里有 %s 时不该有阵位：seized=%q crossed=%q", typ, seized, crossed)
		}
	}
}

// 抢到阵位的一方让对手第一轮只开一半火炮：16 点炮击值打成 8 次判定。
func TestPositionHalvesCrossedFirstVolley(t *testing.T) {
	state := positionState([]int{3}, []int{4}, true, false, []string{"BB", "CA"})
	events := resolveContacts(state)

	hasPosition := false
	halfVolleys := 0
	for _, event := range events {
		if event.Type == "position_seized" {
			hasPosition = true
		}
		if event.Type == "gunnery_volley" && strings.Contains(event.Text, "半数火炮") {
			halfVolleys++
			if !strings.Contains(event.Text, "8次判定") {
				t.Fatalf("半数火炮应按 16/2=8 次判定：%s", event.Text)
			}
		}
	}
	if !hasPosition {
		t.Fatal("没有发出 position_seized 事件")
	}
	// 守方两艘船（BB + CA），每艘都被压住一半火炮。
	if halfVolleys != 2 {
		t.Fatalf("被压制方两艘船该有两次半数火炮齐射，实际 %d 次", halfVolleys)
	}
}

// 炮击值 1 的船在阵位被压住那一轮打不出来（1/2 向下取整为 0）。
func TestHalfGunsSkipsSingleGunShips(t *testing.T) {
	state := &State{RNGState: 7, Map: MapDefinition{}, Doctrines: map[string]string{}}
	attacker := &Unit{ID: "a", FleetID: "af", Name: "轻护卫", Type: "DD", Faction: "A", Coord: "0101",
		Strength: 3, MaxStrength: 3, Card: ShipCard{Gunnery: 1, SpeedClass: "normal"}}
	target := &Unit{ID: "b", FleetID: "bf", Name: "目标", Type: "BB", Faction: "B", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", DamageControl: 7}}
	state.Units = map[string]*Unit{"a": attacker, "b": target}
	if hits, events := gunneryVolley(state, []Unit{*attacker}, []Unit{*target}, true); len(hits) != 0 || len(events) != 0 {
		t.Fatalf("炮击值 1 的船在压制轮不该开火：hits=%d events=%v", len(hits), events)
	}
	// 满火力时正常开火。
	if _, events := gunneryVolley(state, []Unit{*attacker}, []Unit{*target}, false); len(events) != 1 {
		t.Fatalf("满火力时应该有齐射事件：%v", events)
	}
}
