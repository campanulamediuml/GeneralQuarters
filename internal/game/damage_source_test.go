package game

import (
	"encoding/json"
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
	// 漏油会让火灾条目各多一份。
	leaking := &Unit{ID: "dd2", Type: "DD", Card: destroyer.Card, Damage: &ShipDamage{OilLeak: true}}
	dry := severeDamageTable(HitShell, destroyer)
	wet := severeDamageTable(HitShell, leaking)
	if len(wet) != len(dry)+2 {
		t.Fatalf("漏油应让表多两条火灾条目：dry=%d wet=%d", len(dry), len(wet))
	}
	countFire := func(table []string) int {
		count := 0
		for _, item := range table {
			if item == "small_fire" || item == "burning" {
				count++
			}
		}
		return count
	}
	if got := countFire(wet); got != countFire(dry)+2 {
		t.Fatalf("火灾条目应各多一份：dry=%d wet=%d", countFire(dry), got)
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

// 机库形式是卡面属性：0 = 劣势（两投取大）、2 = 2 骰优势（两投取小）、3 = 3 骰优势（三投取小）。
// 同样 DCT 5 的失败线：劣势 75%、2 骰 25%、3 骰 12.5%。
func TestHangarDCTAdvantageByCard(t *testing.T) {
	trials := 2000
	failRate := func(level HangarAdvantageLevel, sealed bool) float64 {
		state := &State{RNGState: 1}
		unit := &Unit{ID: "cv", Type: "CV", Faction: "A", Card: ShipCard{DamageControl: 5, HangarAdvantage: level}}
		failures := 0
		for i := 0; i < trials; i++ {
			if hangarDCTRoll(state, unit, sealed) > unit.Card.DamageControl {
				failures++
			}
		}
		return float64(failures) / float64(trials)
	}
	disadvantage := failRate(0, false)
	advantage2 := failRate(2, false)
	advantage3 := failRate(3, false)
	if disadvantage < 0.70 || disadvantage > 0.80 {
		t.Fatalf("劣势（两投取大）的失败率应接近 75%%，实际 %.2f", disadvantage)
	}
	if advantage2 < 0.21 || advantage2 > 0.29 {
		t.Fatalf("2 骰优势的失败率应接近 25%%，实际 %.2f", advantage2)
	}
	if advantage3 < 0.09 || advantage3 > 0.16 {
		t.Fatalf("3 骰优势的失败率应接近 12.5%%，实际 %.2f", advantage3)
	}
	if !(disadvantage > advantage2 && advantage2 > advantage3) {
		t.Fatalf("优势档位越高越不容易被炸：劣势 %.2f / 2 骰 %.2f / 3 骰 %.2f", disadvantage, advantage2, advantage3)
	}
	// 装甲航母被打穿后机库封闭：就算卡面是 3 骰优势，也按劣势结算。
	sealed := failRate(3, true)
	if sealed < 0.70 || sealed > 0.80 {
		t.Fatalf("封闭机库应强制劣势（75%%），实际 %.2f", sealed)
	}
}

// 部署覆盖能按单舰改机库形式（剧本里微调不需要复制整张卡）。
func TestHangarAdvantageInScenario(t *testing.T) {
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
		if unit.Card.HangarAdvantage > 0 {
			openCount++
		}
	}
	if openCount == 0 {
		t.Fatal("珊瑚海剧本里应该有取优势的航母（约克城）")
	}
	for _, unit := range state.Units {
		// 1942 珊瑚海：美军航母是 2 骰优势。
		if unit.DefinitionID == "yorktown" && unit.Card.HangarAdvantage != 2 {
			t.Fatalf("1942 珊瑚海的约克城应是 2 骰优势，实际 %d", unit.Card.HangarAdvantage)
		}
		if unit.DefinitionID == "lexington" && unit.Card.HangarAdvantage > 0 {
			t.Fatal("列克星敦级没有机库优势")
		}
		if unit.DefinitionID == "shokaku" && unit.Card.HangarAdvantage > 0 {
			t.Fatal("翔鹤级没有机库优势")
		}
	}
}

// 老存档里 hangar_advantage 是布尔值：true 按 2 骰、false 按劣势兼容。
func TestHangarAdvantageLegacyBoolDecode(t *testing.T) {
	cases := []struct {
		input string
		want  HangarAdvantageLevel
	}{
		{"{\"hangar_advantage\": true}", 2},
		{"{\"hangar_advantage\": false}", 0},
		{"{\"hangar_advantage\": 3}", 3},
		{"{}", 0},
	}
	for _, item := range cases {
		var card ShipCard
		if err := json.Unmarshal([]byte(item.input), &card); err != nil {
			t.Fatalf("%s 解码失败：%v", item.input, err)
		}
		if card.HangarAdvantage != item.want {
			t.Fatalf("%s 解出 %d，期望 %d", item.input, card.HangarAdvantage, item.want)
		}
	}
}

// 漏油每 3 回合掷一次 d10：≤ 当前损管值即修复，失败再等 3 回合。
func TestOilLeakRepairCycle(t *testing.T) {
	build := func(damageControl int) (*State, *Unit) {
		unit := &Unit{ID: "ca", FleetID: "ca", Name: "重巡", Type: "CA", Faction: "A", Strength: 5, MaxStrength: 5, Card: ShipCard{DamageControl: damageControl}}
		unit.Damage = &ShipDamage{OilLeak: true, OilLeakTurns: 3}
		state := &State{RNGState: 7, Units: map[string]*Unit{"ca": unit}}
		return state, unit
	}

	// 损管 10：必过。第 3 个损管阶段才修复。
	state, unit := build(10)
	AdvanceShipConditions(state)
	AdvanceShipConditions(state)
	if !unit.Damage.OilLeak {
		t.Fatal("漏油不满 3 回合不该修复")
	}
	AdvanceShipConditions(state)
	if unit.Damage.OilLeak {
		t.Fatal("第 3 回合损管判定成功应修复漏油")
	}

	// 损管 0：永远过不了，失败后重置为 3 再等一轮。
	state, unit = build(0)
	for turn := 0; turn < 6; turn++ {
		AdvanceShipConditions(state)
	}
	if !unit.Damage.OilLeak {
		t.Fatal("损管 0 时漏油不可能修好")
	}
	if unit.Damage.OilLeakTurns <= 0 || unit.Damage.OilLeakTurns > 3 {
		t.Fatalf("失败后倒计时应重置到 1..3，实际 %d", unit.Damage.OilLeakTurns)
	}
}
