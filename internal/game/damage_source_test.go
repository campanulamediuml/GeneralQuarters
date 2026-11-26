package game

import (
	"strings"
	"testing"
)

// 漏油只有鱼雷命中才可能抽到：别的来源抽到它等于白给一条结果。
func TestSevereDamageTableBySource(t *testing.T) {
	carrier := &Unit{ID: "cv", Type: "CV", Card: ShipCard{HangarCapacity: 2}}
	destroyer := &Unit{ID: "dd", Type: "DD", Card: ShipCard{TorpedoSalvos: 3}}
	for _, cause := range []HitCause{HitShell, HitBomb, HitFire} {
		for _, unit := range []*Unit{carrier, destroyer} {
			if table := severeDamageTable(cause, unit); containsDamage(table, "oil_leak") {
				t.Fatalf("%s 命中不该抽到漏油：%v", cause, table)
			}
		}
	}
	if table := severeDamageTable(HitTorpedo, destroyer); !containsDamage(table, "oil_leak") {
		t.Fatalf("鱼雷命中必须能抽到漏油：%v", table)
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
