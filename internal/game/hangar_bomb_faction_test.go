package game

import (
	"fmt"
	"testing"
)

// 舰船卡取值来自 content/units/marianas.json（1944）：
//
//	埃塞克斯：DCT 4 + 3 骰机库优势（三投取小，机库油气更难点着）
//	翔鹤：    DCT 3、没有机库优势（两投取大）
//	大凤：    DCT 4，装甲航母：机库要先过一次击穿 DCT，打穿后机库爆炸强制劣势
//	信浓：    DCT 7，装甲航母（装甲更厚，更难被击穿）
func factionProbeCard(dc int, hangarAdvantage HangarAdvantageLevel, armored bool, hangar int) ShipCard {
	return ShipCard{
		DamageControl:    dc,
		HangarAdvantage:  hangarAdvantage,
		ArmoredCarrier:   armored,
		HangarCapacity:   hangar,
		DeckCapacity:     8,
		SpeedClass:       "fast",
		AirRadar:         true,
		SurfaceRadar:     true,
		FireControlRadar: boolPtr(true),
		MainGunCount:     0,
	}
}

func boolPtr(value bool) *bool { return &value }

// factionHangarHit 模拟一次航弹命中，返回这一发的毁机数、结构损失、是否燃烧。
func factionHangarHit(card ShipCard, faction string, doctrines map[string]string, aircraft int, armed bool, seed uint64) (int, int, bool) {
	state := &State{
		Units:     map[string]*Unit{},
		Aircraft:  map[string]*AirUnit{},
		RNGState:  seed,
		Doctrines: doctrines,
		Year:      1944,
	}
	carrier := &Unit{ID: "cv", Faction: faction, Name: "carrier", Type: "CV", Strength: 12, MaxStrength: 12, Card: card}
	state.Units[carrier.ID] = carrier
	for index := 0; index < aircraft; index++ {
		id := fmt.Sprintf("a%02d", index)
		plane := &AirUnit{ID: id, CarrierID: carrier.ID, Faction: faction, Model: "B5N2", Kind: "torpedo_bomber", Status: "hangar", Strength: 1}
		if armed {
			plane.Loadout = LoadoutTorpedoTank
			plane.LoadoutManual = true
			plane.ServiceTurns = 1
		}
		state.Aircraft[id] = plane
	}
	syncArmedAircraft(state, nil)
	carrier.Card.Aircraft = aircraft
	before := carrier.Card.Aircraft
	applyGunOrBombHit(state, Hit{TargetID: carrier.ID, Cause: HitBomb})
	return before - carrier.Card.Aircraft, 12 - carrier.Strength, EnsureDamage(carrier).Fire == "burning"
}

// 美日两边的机库挨航弹差多少：飞机损失一样（都是整装待发 2 架/发），
// 但损管和机库优势让美军的机库爆炸/火灾明显少，日军的航母更容易被烧穿。
func TestFactionHangarBombDifference(t *testing.T) {
	doctrines := map[string]string{"A": DoctrineUS, "B": DoctrineJapan}
	const runs = 20000
	type stat struct {
		aircraft float64
		strength float64
		burning  float64
	}
	measure := func(card ShipCard, faction string, aircraft int, armed bool) stat {
		lost, strength, burning := 0, 0, 0
		for run := 0; run < runs; run++ {
			destroyed, delta, burned := factionHangarHit(card, faction, doctrines, aircraft, armed, uint64(run)*2654435761+13)
			lost += destroyed
			strength += delta
			if burned {
				burning++
			}
		}
		return stat{
			aircraft: float64(lost) / float64(runs),
			strength: float64(strength) / float64(runs),
			burning:  float64(burning) * 100 / float64(runs),
		}
	}
	us := factionProbeCard(4, 3, false, 24)
	jp := factionProbeCard(3, 0, false, 18)
	armored := factionProbeCard(4, 0, true, 20) // 大凤：装甲航母，DCT 4
	shinano := factionProbeCard(7, 0, true, 15) // 信浓：装甲航母，DCT 7

	usArmed := measure(us, "A", 18, true)
	usEmpty := measure(us, "A", 18, false)
	jpArmed := measure(jp, "B", 18, true)
	jpEmpty := measure(jp, "B", 18, false)
	taihoArmed := measure(armored, "B", 18, true)
	shinanoArmed := measure(shinano, "B", 15, true)

	t.Logf("整装待发：美军 毁机 %.2f 结构 -%.2f 燃烧 %.1f%%；日军 毁机 %.2f 结构 -%.2f 燃烧 %.1f%%",
		usArmed.aircraft, usArmed.strength, usArmed.burning, jpArmed.aircraft, jpArmed.strength, jpArmed.burning)
	t.Logf("空载未整备：美军 毁机 %.2f 结构 -%.2f 燃烧 %.1f%%；日军 毁机 %.2f 结构 -%.2f 燃烧 %.1f%%",
		usEmpty.aircraft, usEmpty.strength, usEmpty.burning, jpEmpty.aircraft, jpEmpty.strength, jpEmpty.burning)
	t.Logf("装甲航母（大凤 DCT4）整装待发：毁机 %.2f 结构 -%.2f 燃烧 %.1f%%", taihoArmed.aircraft, taihoArmed.strength, taihoArmed.burning)
	t.Logf("装甲航母（信浓 DCT7）整装待发：毁机 %.2f 结构 -%.2f 燃烧 %.1f%%", shinanoArmed.aircraft, shinanoArmed.strength, shinanoArmed.burning)

	// 飞机损失只由"整装待发"决定，和阵营无关：两边都是每发 2 架。
	for _, item := range []struct {
		name string
		got  stat
	}{{"美军", usArmed}, {"日军", jpArmed}} {
		if item.got.aircraft < 1.5 || item.got.aircraft > 2.05 {
			t.Fatalf("%s整装待发机库每发应毁 2 架左右，实际 %.2f", item.name, item.got.aircraft)
		}
	}
	// 没整备的机库基本不吃"整装待发被摧毁"，只剩严重伤害表里"机库损伤"的小概率。
	for _, item := range []struct {
		name string
		got  stat
	}{{"美军空载", usEmpty}, {"日军空载", jpEmpty}} {
		if item.got.aircraft > 0.3 {
			t.Fatalf("%s不该跟着吃整装待发那一套，实际每发毁 %.2f 架", item.name, item.got.aircraft)
		}
	}
	// 阵营差异出在损管：DCT 4 + 机库优势 vs DCT 3 + 两投取大。
	if jpArmed.strength < usArmed.strength+0.2 {
		t.Fatalf("日军的机库爆炸应该明显更重：%.2f vs %.2f", jpArmed.strength, usArmed.strength)
	}
	if jpArmed.burning < usArmed.burning+20 {
		t.Fatalf("日军整装待发的机库应该更容易烧穿：%.1f%% vs %.1f%%", jpArmed.burning, usArmed.burning)
	}
	// 装甲航母：机库不是免单，而是要先过一道"击穿装甲甲板"的 DCT。
	// 损管失败才打穿，所以大凤（DCT4）大约是普通航母的六成，信浓（DCT7）只有三成。
	if taihoArmed.aircraft < 1.0 || taihoArmed.aircraft > 1.45 {
		t.Fatalf("大凤（DCT4）每发应毁约 1.2 架（60%% 击穿 × 2 架），实际 %.2f", taihoArmed.aircraft)
	}
	if shinanoArmed.aircraft < 0.4 || shinanoArmed.aircraft > 0.85 {
		t.Fatalf("信浓（DCT7）每发应毁约 0.6 架（30%% 击穿 × 2 架），实际 %.2f", shinanoArmed.aircraft)
	}
	if !(taihoArmed.aircraft < jpArmed.aircraft && shinanoArmed.aircraft < taihoArmed.aircraft) {
		t.Fatalf("装甲越厚越难击穿：翔鹤 %.2f / 大凤 %.2f / 信浓 %.2f", jpArmed.aircraft, taihoArmed.aircraft, shinanoArmed.aircraft)
	}
}
