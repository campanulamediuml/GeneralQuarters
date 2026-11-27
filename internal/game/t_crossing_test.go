package game

import (
	"strings"
	"testing"
)

// 战列线（T 优 / T 劣）规则：
// 守方原地停留 ≥3 回合、编队没有航母/运输船、交战前已发现对方，而对方本回合主动
// 撞进来——守方拿到 T 优，抢入的一方第一轮炮战只有一半火炮能开火。

func tCrossingState(defenderTypes []string, defenderMoved bool, sessions int, prior bool, attackerMoved bool) *State {
	units := map[string]*Unit{}
	state := &State{
		RNGState:      4242,
		Map:           MapDefinition{Rows: 2, Cols: 2},
		Contacts:      map[string]map[string]*FleetContact{},
		Doctrines:     map[string]string{"A": "us", "B": "japan"},
		FleetSessions: map[string]int{},
	}
	for i, typ := range defenderTypes {
		id := "d" + string(rune('a'+i))
		units[id] = &Unit{ID: id, FleetID: "df", Name: "守方" + typ, Type: typ, Faction: "B",
			Coord: "0101", Strength: 6, MaxStrength: 6, Moved: defenderMoved,
			Card: ShipCard{Gunnery: 16, DamageControl: 7, SpeedClass: "normal"}}
	}
	units["a1"] = &Unit{ID: "a1", FleetID: "af", Name: "突击舰", Type: "BB", Faction: "A",
		Coord: "0101", Strength: 6, MaxStrength: 6, Moved: attackerMoved,
		Card: ShipCard{Gunnery: 16, DamageControl: 7, SpeedClass: "normal"}}
	state.Units = units
	state.FleetSessions["df"] = sessions
	if prior {
		// 守方在交战前就有对方舰队的情报（Level>0）。
		state.Contacts["B"] = map[string]*FleetContact{"af": {FleetID: "af", Faction: "A", Level: 2, Coord: "0101"}}
	}
	return state
}

func TestBattleLineAdvantageNeedsStationaryLineFleet(t *testing.T) {
	state := tCrossingState([]string{"BB", "CA"}, false, tCrossingStayTurns, true, true)
	factions := map[string][]Unit{}
	for _, unit := range state.Units {
		factions[unit.Faction] = append(factions[unit.Faction], *unit)
	}
	adv, dis, reason := battleLineTState(state, factions, map[string]bool{"A": false, "B": true})
	if adv != "B" || dis != "A" || !strings.Contains(reason, "停留") {
		t.Fatalf("守方应拿 T 优：adv=%q dis=%q reason=%q", adv, dis, reason)
	}

	// 守方本回合也动了：两边都在抢位，没有战列线。
	moved := tCrossingState([]string{"BB"}, true, tCrossingStayTurns, true, true)
	if adv, dis, _ := battleLineTState(moved, factionsOf(moved), map[string]bool{"A": false, "B": true}); adv != "" || dis != "" {
		t.Fatalf("双方都在移动时不该有 T 优：%q %q", adv, dis)
	}
	// 抢入的一方也没动（双方本来就贴在一起打第二回合）：不该有 T 优。
	static := tCrossingState([]string{"BB"}, false, tCrossingStayTurns, true, false)
	if adv, _, _ := battleLineTState(static, factionsOf(static), map[string]bool{"A": false, "B": true}); adv != "" {
		t.Fatalf("没人抢入时不该有 T 优：%q", adv)
	}
	// 停留不足 3 回合。
	short := tCrossingState([]string{"BB"}, false, tCrossingStayTurns-1, true, true)
	if adv, _, _ := battleLineTState(short, factionsOf(short), map[string]bool{"A": false, "B": true}); adv != "" {
		t.Fatalf("停留 %d 回合不该有 T 优：%q", tCrossingStayTurns-1, adv)
	}
	// 交战前没发现对方：是被人摸到脸上，摆不出战列线。
	blind := tCrossingState([]string{"BB"}, false, tCrossingStayTurns, false, true)
	if adv, _, _ := battleLineTState(blind, factionsOf(blind), map[string]bool{"A": false, "B": false}); adv != "" {
		t.Fatalf("没有提前发现时不该有 T 优：%q", adv)
	}
	// 编队里有航母或运输船：不摆战列线。
	for _, typ := range []string{"CV", "CVL", "CVE", "TRS"} {
		mixed := tCrossingState([]string{"BB", typ}, false, tCrossingStayTurns, true, true)
		if adv, _, _ := battleLineTState(mixed, factionsOf(mixed), map[string]bool{"A": false, "B": true}); adv != "" {
			t.Fatalf("编队里有 %s 时不该有 T 优：%q", typ, adv)
		}
	}
}

func factionsOf(state *State) map[string][]Unit {
	factions := map[string][]Unit{}
	for _, unit := range state.Units {
		factions[unit.Faction] = append(factions[unit.Faction], *unit)
	}
	return factions
}

// 抢入炮战的一方在第一轮只开一半火炮：16 点炮击值打成 8 次判定。
func TestTCrossingHalvesAttackerFirstVolley(t *testing.T) {
	state := tCrossingState([]string{"BB", "CA"}, false, tCrossingStayTurns, true, true)
	events := resolveContacts(state)

	hasT := false
	halfVolleys := 0
	for _, event := range events {
		if event.Type == "t_crossing" {
			hasT = true
		}
		if event.Type == "gunnery_volley" && strings.Contains(event.Text, "半数火炮") {
			halfVolleys++
			if !strings.Contains(event.Text, "8次判定") {
				t.Fatalf("半数火炮应按 16/2=8 次判定：%s", event.Text)
			}
		}
	}
	if !hasT {
		t.Fatal("没有发出 t_crossing 事件")
	}
	if halfVolleys != 1 {
		t.Fatalf("抢入方只该有一次半数火炮齐射，实际 %d 次", halfVolleys)
	}
}

// 炮击值 1 的船在 T 劣那一轮打不出来（1/2 向下取整为 0）。
func TestHalfGunsSkipsSingleGunShips(t *testing.T) {
	state := &State{RNGState: 7, Map: MapDefinition{}, Doctrines: map[string]string{}}
	attacker := &Unit{ID: "a", FleetID: "af", Name: "轻护卫", Type: "DD", Faction: "A", Coord: "0101",
		Strength: 3, MaxStrength: 3, Card: ShipCard{Gunnery: 1, SpeedClass: "normal"}}
	target := &Unit{ID: "b", FleetID: "bf", Name: "目标", Type: "BB", Faction: "B", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", DamageControl: 7}}
	state.Units = map[string]*Unit{"a": attacker, "b": target}
	if hits, events := gunneryVolley(state, []Unit{*attacker}, []Unit{*target}, true); len(hits) != 0 || len(events) != 0 {
		t.Fatalf("炮击值 1 的船在 T 劣轮不该开火：hits=%d events=%v", len(hits), events)
	}
	// 满火力时正常开火。
	if _, events := gunneryVolley(state, []Unit{*attacker}, []Unit{*target}, false); len(events) != 1 {
		t.Fatalf("满火力时应该有齐射事件：%v", events)
	}
}
