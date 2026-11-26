package game

import (
	"math"
	"testing"
)

// —— 航空对轰验证 ——
//
// 跑真实的 resolveBombing / resolveHeavyAA / performAirAttack，统计实际结果是否落在规则书概率附近。
// 注意：命中后要走航空损伤表，结果可能是"无效果"，所以命中率不能用"有没有产生事件"来统计，
// 必须看 hist（report.AttackerHits / DefenderLosses）这类结构化计数。

func buildStrikeState() *State {
	return &State{
		Map:       MapDefinition{Rows: 7, Cols: 9},
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units:     map[string]*Unit{},
		Aircraft:  map[string]*AirUnit{},
		Airfields: map[string]*Airfield{},
		Contacts:  map[string]map[string]*FleetContact{},
		RNGState:  20241001,
	}
}

func addDefenderFleet(state *State, id, typ, coord string, heavyAA, lightAA float64) *Unit {
	state.Units[id] = &Unit{
		ID: id, Faction: "B", Type: typ, FleetID: "fleet-b", Coord: coord,
		Strength: 6, MaxStrength: 6,
		Card: ShipCard{SpeedClass: "normal", HeavyAA: heavyAA, LightAA: lightAA, DamageControl: 10},
	}
	return state.Units[id]
}

func addAttacker(state *State, id, kind string, airCombat, antiShip int, loadout Loadout) *AirUnit {
	state.Aircraft[id] = &AirUnit{
		ID: id, Faction: "A", Kind: kind, Model: "TEST", Status: "attack", TaskID: "task-a", TaskKind: "attack",
		Coord: "0202", TargetCoord: "0202", Strength: 3,
		AirCombat: airCombat, AntiShip: antiShip, Loadout: loadout,
		MaxRange: 10, Fuel: Fuel{Internal: 10}, RemainingRange: 10, BaseRange: 10,
	}
	return state.Aircraft[id]
}

// 规则书 634 行：每点对舰攻击值一次基础 25% 判定。
func TestAirStrikeBaseRate(t *testing.T) {
	state := buildStrikeState()
	addDefenderFleet(state, "bb-a", "BB", "0202", 0, 0)
	totalShots, totalHits := 0, 0
	const runs = 2000
	for i := 0; i < runs; i++ {
		delete(state.Aircraft, "a1")
		attacker := addAttacker(state, "a1", "dive_bomber", 1, 4, LoadoutBombTank)
		report := &AirCombatReport{}
		resolveBombingWave(state, []*AirUnit{attacker}, []Unit{*state.Units["bb-a"]}, false, true, report)
		totalShots += 4
		totalHits += report.StrikeHits
	}
	if rate := float64(totalHits) / float64(totalShots); math.Abs(rate-0.25) > 0.04 {
		t.Fatalf("对舰基础命中率应接近 25%%，实测 %.3f", rate)
	}
}

// 规则书 636–642 行。
func TestAirStrikeChanceModifiers(t *testing.T) {
	state := buildStrikeState()
	bb := addDefenderFleet(state, "bb", "BB", "0202", 0, 0)
	attacker := addAttacker(state, "a", "dive_bomber", 1, 4, LoadoutBombTank)
	if got := strikeHitChance(*state, attacker, bb, false, true); math.Abs(got-25) > 0.01 {
		t.Fatalf("基础应为 25%%，实际 %.1f", got)
	}
	if got := strikeHitChance(*state, attacker, bb, false, false); math.Abs(got-30) > 0.01 {
		t.Fatalf("无 CAP 应 +5%%：期望 30，实际 %.1f", got)
	}
	bb.Card.SpeedClass = "slow"
	if got := strikeHitChance(*state, attacker, bb, false, true); math.Abs(got-30) > 0.01 {
		t.Fatalf("慢速应 +5%%：期望 30，实际 %.1f", got)
	}
	bb.Card.SpeedClass = "normal"
	bb.Card.LightAA = 14
	if got := strikeHitChance(*state, attacker, bb, false, true); math.Abs(got-15) > 0.01 {
		t.Fatalf("轻防空两档应 -10%%：期望 15，实际 %.1f", got)
	}
}

// 规则书 687–688 行：每点防空 3% 命中；命中后按航空损伤表结算。
func TestAirDefenseHitRate(t *testing.T) {
	state := buildStrikeState()
	state.Units["bb"] = &Unit{ID: "bb", Faction: "B", Type: "BB", FleetID: "fleet-b", Coord: "0202",
		Strength: 6, MaxStrength: 6, Card: ShipCard{HeavyAA: 100, DamageControl: 10}}
	ships := []Unit{*state.Units["bb"]}
	const runs = 1000
	hits := 0
	for i := 0; i < runs; i++ {
		attackers := []*AirUnit{}
		for j := 0; j < 20; j++ {
			attackers = append(attackers, addAttacker(state, "atk-"+string(rune('A'+j)), "dive_bomber", 1, 4, LoadoutBombTank))
		}
		report := &AirCombatReport{}
		resolveHeavyAA(state, attackers, ships, "0202", report)
		hits += report.AAHits
		for j := 0; j < 20; j++ {
			delete(state.Aircraft, "atk-"+string(rune('A'+j)))
		}
	}
	// 100 点 × 3% = 每轮期望 3 次命中（20 架足够吸收，不会因为打光而截断）
	perRound := float64(hits) / float64(runs)
	if math.Abs(perRound-3.0) > 0.6 {
		t.Fatalf("100 点防空每轮命中数应接近 3，实测 %.2f", perRound)
	}
}

// 规则书 742–746 行：空战命中率按差值查表。用 report 计数而不是事件数。
func TestDogfightChanceStatistics(t *testing.T) {
	cases := []struct {
		a, d int
		want float64
	}{{3, 3, 0.25}, {4, 3, 0.35}, {2, 3, 0.20}, {0, 3, 0.05}}
	for _, c := range cases {
		state := buildStrikeState()
		hit, runs := 0, 3000
		for i := 0; i < runs; i++ {
			att := &AirUnit{ID: "x", Faction: "A", Kind: "fighter", Status: "attack", TaskID: "ta", Strength: 4, AirCombat: c.a}
			def := &AirUnit{ID: "y", Faction: "B", Kind: "fighter", Status: "cap", TaskID: "tb", Strength: 4, AirCombat: c.d}
			state.Aircraft = map[string]*AirUnit{"x": att, "y": def}
			report := &AirCombatReport{}
			// hitUnits 是"命中判定成功"的唯一可靠信号：
			// report.AttackerHits 只在航空损伤表打出撤退/损伤/摧毁时才记录，"无效果"不记录。
			hitUnits := map[string]bool{}
			performAirAttack(state, att, []*AirUnit{def}, 0, nil, hitUnits, report, true)
			if hitUnits[def.ID] {
				hit++
			}
		}
		if rate := float64(hit) / float64(runs); math.Abs(rate-c.want) > 0.06 {
			t.Fatalf("空战 %d vs %d 命中率应接近 %.2f，实测 %.3f", c.a, c.d, c.want, rate)
		}
	}
}

// 规则书 378–383 行：航空损伤表。
func TestAircraftDamageTableDistribution(t *testing.T) {
	state := buildStrikeState()
	counts := map[string]int{}
	const runs = 4000
	for i := 0; i < runs; i++ {
		aircraft := &AirUnit{ID: "d", Faction: "A", Kind: "dive_bomber", Strength: 2}
		state.Aircraft = map[string]*AirUnit{"d": aircraft}
		outcome, _ := applyAircraftHit(state, aircraft)
		counts[outcome]++
	}
	for outcome, want := range map[string]float64{"撤退": 0.30, "损伤": 0.30, "摧毁": 0.40} {
		if got := float64(counts[outcome]) / float64(runs); math.Abs(got-want) > 0.05 {
			t.Fatalf("强度 2 的%s概率应约 %.0f%%，实测 %.3f（%v）", outcome, want*100, got, counts)
		}
	}
}

// 规则书 692、696、698 行：轻防空对鱼雷机翻倍、混合编队 ×0.5。
func TestLightAAValueModifiers(t *testing.T) {
	state := buildStrikeState()
	ship := addDefenderFleet(state, "s", "CA", "0202", 0, 10)
	bomber := &AirUnit{Kind: "dive_bomber"}
	torpedo := &AirUnit{Kind: "torpedo_bomber"}
	if got := lightAAValue(*state, bomber, ship); math.Abs(got-10) > 0.01 {
		t.Fatalf("基础轻防空应为 10，实际 %.1f", got)
	}
	if got := lightAAValue(*state, torpedo, ship); math.Abs(got-20) > 0.01 {
		t.Fatalf("对鱼雷机应翻倍为 20，实际 %.1f", got)
	}
	// 复合编队减半落在命中率上（骰子数不变）。
	base := lightAAHitChance(*state, ship, bomber, false)
	if got := lightAAHitChance(*state, ship, bomber, true); math.Abs(got-base/2) > 0.01 {
		t.Fatalf("混编轻防空命中率应减半为 %.1f，实际 %.1f", base/2, got)
	}
}
