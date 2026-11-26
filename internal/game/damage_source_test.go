package game

import (
	"strings"
	"testing"
)

// 漏油只有鱼雷命中才可能抽到：别的来源抽到它等于白给一条结果。
func TestSevereDamageTableBySource(t *testing.T) {
	carrier := &Unit{ID: "cv", Type: "CV", Card: ShipCard{HangarCapacity: 2}}
	destroyer := &Unit{ID: "dd", Type: "DD", Card: ShipCard{TorpedoSalvos: 3}}
	// 漏油来自鱼雷和炮弹；炸弹、火灾不该抽到它。
	for _, cause := range []HitCause{HitBomb, HitFire} {
		for _, unit := range []*Unit{carrier, destroyer} {
			if table := severeDamageTable(cause, unit); containsDamage(table, "oil_leak") {
				t.Fatalf("%s 命中不该抽到漏油：%v", cause, table)
			}
		}
	}
	for _, cause := range []HitCause{HitTorpedo, HitShell} {
		if table := severeDamageTable(cause, destroyer); !containsDamage(table, "oil_leak") {
			t.Fatalf("%s 命中必须能抽到漏油：%v", cause, table)
		}
	}
	// 额外表仍然按船型和装备拼。
	if table := severeDamageTable(HitBomb, carrier); !containsDamage(table, "hangar_damage") || !containsDamage(table, "deck_destroyed") {
		t.Fatalf("航母额外表缺失：%v", table)
	}
	if table := severeDamageTable(HitBomb, destroyer); containsDamage(table, "hangar_damage") || !containsDamage(table, "sonar_damage") {
		t.Fatalf("驱逐舰的表不该有航母条目、必须有声呐：%v", table)
	}
}

func containsDamage(table []string, want string) bool {
	for _, item := range table {
		if item == want {
			return true
		}
	}
	return false
}

// 装甲航母只对炸弹硬：炸弹 25%，炮弹仍按 75%。
func TestArmoredCarrierDeckDamageChance(t *testing.T) {
	armored := &Unit{ID: "cv", Type: "CV", Card: ShipCard{ArmoredCarrier: true}}
	normal := &Unit{ID: "cv2", Type: "CV"}
	if got := deckDamageChance(HitBomb, armored); got != 25 {
		t.Fatalf("装甲航母吃炸弹应为 25%%，实际 %v", got)
	}
	if got := deckDamageChance(HitShell, armored); got != 75 {
		t.Fatalf("装甲航母吃炮弹仍应 75%%，实际 %v", got)
	}
	if got := deckDamageChance(HitBomb, normal); got != 75 {
		t.Fatalf("普通航母吃炸弹应 75%%，实际 %v", got)
	}
}

// 炮弹命中不结算整装待发飞机；炸弹命中才结算（规则书 421–423 行）。
func TestShellHitKeepsReadyAircraft(t *testing.T) {
	build := func() (*State, *Unit) {
		carrier := &Unit{ID: "cv", FleetID: "cv", Name: "航母", Type: "CV", Faction: "A", Strength: 5, MaxStrength: 5, Card: ShipCard{DamageControl: 10, HangarCapacity: 2, Aircraft: 2}}
		state := &State{RNGState: 7, Units: map[string]*Unit{"cv": carrier}, Aircraft: map[string]*AirUnit{
			"cv-f1": {ID: "cv-f1", CarrierID: "cv", Armed: true, Status: "hangar"},
			"cv-f2": {ID: "cv-f2", CarrierID: "cv", Armed: true, Status: "deck"},
		}}
		return state, carrier
	}
	shellState, shellCarrier := build()
	applyGunOrBombHit(shellState, Hit{TargetID: "cv", Cause: HitShell})
	if shellCarrier.Card.Aircraft != 2 || len(shellState.Aircraft) != 2 {
		t.Fatalf("炮弹命中不该摧毁整装待发飞机：%d 架 / %d 个算子", shellCarrier.Card.Aircraft, len(shellState.Aircraft))
	}
	if fire := EnsureDamage(shellCarrier).Fire; strings.Contains(fire, "small") {
		t.Fatalf("炮弹命中不该引起机库小型火灾：%q", fire)
	}
	bombState, bombCarrier := build()
	applyGunOrBombHit(bombState, Hit{TargetID: "cv", Cause: HitBomb})
	if bombCarrier.Card.Aircraft >= 2 {
		t.Fatalf("炸弹命中应当摧毁整装待发飞机，实际还剩 %d 架", bombCarrier.Card.Aircraft)
	}
}

// 机库的额外 DCT 只抽那张三选一小表：航空燃油爆炸、弹药爆炸、小型火灾。
func TestHangarExplosionTableIsThreeEntries(t *testing.T) {
	want := map[string]bool{"avgas_explosion": true, "ammo_explosion": true, "small_fire": true}
	if len(hangarExplosionTable) != len(want) {
		t.Fatalf("机库爆炸表应为 3 条，实际 %v", hangarExplosionTable)
	}
	for _, item := range hangarExplosionTable {
		if !want[item] {
			t.Fatalf("机库爆炸表混进了别的条目：%v", hangarExplosionTable)
		}
	}
}

// 有整装待发的机库飞机时，一发炸弹就可能炸出油气爆炸；甲板组只掉飞机和点小火，不掷额外 DCT。
// 这里直接测炸弹命中航母后的"整装待发飞机"结算，不带命中本身那一次 DCT，
// 免得随机抽到“机库损伤”先把飞机清空、把断言搅乱。
func TestBombAircraftGroupsSplitEffects(t *testing.T) {
	build := func(withHangar, withDeck bool) (*State, *Unit) {
		carrier := &Unit{ID: "cv", FleetID: "cv", Name: "航母", Type: "CV", Faction: "A", Strength: 6, MaxStrength: 6, Card: ShipCard{DamageControl: 0, HangarCapacity: 4, Aircraft: 4}}
		aircraft := map[string]*AirUnit{}
		if withHangar {
			aircraft["hangar-1"] = &AirUnit{ID: "hangar-1", CarrierID: "cv", Armed: true, Status: "hangar", Model: "SBD"}
			aircraft["hangar-2"] = &AirUnit{ID: "hangar-2", CarrierID: "cv", Armed: true, Status: "service", Model: "SBD"}
		}
		if withDeck {
			aircraft["deck-1"] = &AirUnit{ID: "deck-1", CarrierID: "cv", Armed: true, Status: "deck", Model: "F4F"}
			aircraft["deck-2"] = &AirUnit{ID: "deck-2", CarrierID: "cv", Armed: true, Status: "ready", Model: "F4F"}
		}
		return &State{RNGState: 11, Units: map[string]*Unit{"cv": carrier}, Aircraft: aircraft}, carrier
	}
	countSevere := func(events []EngineEvent) int {
		count := 0
		for _, event := range events {
			if strings.Contains(event.Text, "严重伤害判定") {
				count++
			}
		}
		return count
	}

	// 只打甲板：掉两个算子 + 小火，不掷额外 DCT。
	deckState, deckCarrier := build(false, true)
	events := bombHangarDeckEffect(deckState, deckCarrier, false)
	if got := countSevere(events); got != 0 {
		t.Fatalf("甲板组不该掷额外 DCT，实际 %d 次：%v", got, events)
	}
	if len(deckState.Aircraft) != 0 {
		t.Fatalf("甲板组应摧毁至多 2 个算子，剩 %d 个", len(deckState.Aircraft))
	}
	if EnsureDamage(deckCarrier).Fire != "small" {
		t.Fatalf("甲板飞机被摧毁只点一把小火，实际 %q", EnsureDamage(deckCarrier).Fire)
	}

	// 机库组：额外一次 DCT，必然落到三选一，且小火升级为大火。
	seen := map[string]bool{}
	for seed := uint64(1); seed <= 80; seed++ {
		state, carrier := build(true, true)
		state.RNGState = seed
		events := bombHangarDeckEffect(state, carrier, false)
		if got := countSevere(events); got != 1 {
			t.Fatalf("机库组应恰好掷一次 DCT，实际 %d 次：%v", got, events)
		}
		for _, event := range events {
			for _, name := range []string{"航空燃油爆炸", "弹药爆炸", "小型火灾"} {
				if strings.Contains(event.Text, name) {
					seen[name] = true
				}
			}
		}
		if EnsureDamage(carrier).Fire != "burning" {
			t.Fatalf("同回合两把小火应升级为大火，实际 %q", EnsureDamage(carrier).Fire)
		}
	}
	for _, name := range []string{"航空燃油爆炸", "弹药爆炸", "小型火灾"} {
		if !seen[name] {
			t.Fatalf("80 次里没抽到%s：%v", name, seen)
		}
	}
}

// 机库形式是卡面属性：开放机库取优势（两投取小），封闭机库取劣势（两投取大）。
// 同样一发炸弹、同样 50% 的失败线，开放机库 25% 会失败，封闭机库 75%。
func TestHangarDCTAdvantageByCard(t *testing.T) {
	trials := 400
	failRate := func(openHangar bool) float64 {
		state := &State{RNGState: 1}
		unit := &Unit{ID: "cv", Type: "CV", Faction: "A", Card: ShipCard{DamageControl: 5, OpenHangar: openHangar}}
		failures := 0
		for i := 0; i < trials; i++ {
			if hangarDCTRoll(state, unit) > unit.Card.DamageControl {
				failures++
			}
		}
		return float64(failures) / float64(trials)
	}
	open := failRate(true)
	closed := failRate(false)
	if open < 0.18 || open > 0.32 {
		t.Fatalf("开放机库的失败率应接近 25%%，实际 %.2f", open)
	}
	if closed < 0.68 || closed > 0.82 {
		t.Fatalf("封闭机库的失败率应接近 75%%，实际 %.2f", closed)
	}
	if !(closed > open) {
		t.Fatalf("封闭机库应该更容易被炸：open=%.2f closed=%.2f", open, closed)
	}
}

// 部署覆盖能按单舰改机库形式（剧本里微调不需要复制整张卡）。
func TestOpenHangarDeploymentOverride(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	openCount := 0
	for _, unit := range state.Units {
		if unit.Card.OpenHangar {
			openCount++
		}
	}
	if openCount == 0 {
		t.Fatal("珊瑚海剧本里应该有开放式机库的航母（翔鹤、瑞鹤）")
	}
}
