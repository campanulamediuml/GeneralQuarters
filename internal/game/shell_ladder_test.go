package game

import "testing"

func shellTestShip(id, typ string, strength, damageControl int) *Unit {
	return &Unit{ID: id, FleetID: "f", Name: typ, Type: typ, Faction: "A", Coord: "0101",
		Strength: strength, MaxStrength: strength,
		Card: ShipCard{DamageControl: damageControl}}
}

// 炮弹直击要害的概率只看双方装甲档次差：
// 同级 1%、低一档 2%、低两档 4%、低三档 8%、低四档 10%、低五档 12%；
// 打比自己大的目标 0%（只能去摇 DCT 伤害表）。
func TestShellDamageChanceByCaliberDifference(t *testing.T) {
	cases := []struct {
		attacker, target string
		want             float64
	}{
		{"BB", "BB", 1},
		{"BB", "BC", 2},
		{"BB", "CA", 4},
		{"BB", "CV", 4},
		{"BB", "CL", 8},
		{"BB", "CVL", 8},
		{"BB", "DD", 10},
		{"BB", "CVE", 12},
		{"BB", "TRS", 12},
		{"BC", "BB", 0},
		{"CA", "BB", 0},
		{"CA", "CA", 1},
		{"CA", "CL", 2},
		{"CA", "DD", 4},
		{"DD", "BB", 0},
		{"DD", "DD", 1},
		{"DD", "TRS", 2},
		{"CVE", "TRS", 1},
		{"CVE", "BB", 0},
	}
	for _, c := range cases {
		attacker := shellTestShip("a", c.attacker, 6, 5)
		target := shellTestShip("t", c.target, 6, 5)
		if got := shellDamageChance(attacker, target); got != c.want {
			t.Fatalf("%s 打 %s 应为 %v%%，实际 %v%%", c.attacker, c.target, c.want, got)
		}
	}
}

// 战列舰打 1 点生命值的驱逐舰：一发 10%，打到底必然沉没。
func TestShellHitSinksWeakHull(t *testing.T) {
	state := &State{RNGState: 7, Doctrines: map[string]string{}}
	attacker := shellTestShip("a", "BB", 8, 10)
	target := shellTestShip("t", "DD", 1, 10)
	state.Units = map[string]*Unit{"a": attacker, "t": target}
	for hit := 0; hit < 200 && state.Units["t"] != nil; hit++ {
		applyHits(state, []Hit{{AttackerID: "a", TargetID: "t", Cause: HitShell}})
	}
	if state.Units["t"] != nil {
		t.Fatalf("驱逐舰挨了 200 发战列舰炮弹还没沉")
	}
}

// 驱逐舰还击战列舰：口径不够，打不掉生命值，只能靠 DCT。
func TestSmallCaliberCannotChipLargeHull(t *testing.T) {
	state := &State{RNGState: 13, Doctrines: map[string]string{}}
	attacker := shellTestShip("a", "DD", 2, 10)
	target := shellTestShip("t", "BB", 6, 10)
	state.Units = map[string]*Unit{"a": attacker, "t": target}
	for hit := 0; hit < 300; hit++ {
		applyHits(state, []Hit{{AttackerID: "a", TargetID: "t", Cause: HitShell}})
	}
	if state.Units["t"] == nil {
		t.Fatal("驱逐舰炮弹不该直接打沉战列舰")
	}
	if got := state.Units["t"].Strength; got != 6 {
		t.Fatalf("驱逐舰炮弹不该直接扣血：生命 %d", got)
	}
}

// 炸弹和鱼雷不走这条判定。
func TestNonShellHitsSkipCaliberCheck(t *testing.T) {
	state := &State{RNGState: 11, Doctrines: map[string]string{}}
	attacker := shellTestShip("a", "BB", 8, 10)
	target := shellTestShip("t", "TRS", 4, 10)
	state.Units = map[string]*Unit{"a": attacker, "t": target}
	for hit := 0; hit < 300; hit++ {
		applyHits(state, []Hit{{AttackerID: "a", TargetID: "t", Cause: HitBomb}})
	}
	if state.Units["t"] == nil || state.Units["t"].Strength != 4 {
		t.Fatal("炸弹不该吃炮弹的口径判定")
	}
}
// 小口径打不穿大船的船体：CA 的炮弹打不出战列舰的严重进水，CL/DD 连船体损伤都打不出。
func TestSmallCaliberCannotRollHeavyHullDamage(t *testing.T) {
	bb := shellTestShip("t", "BB", 8, 6)
	bc := shellTestShip("bc", "BC", 6, 5)
	ca := shellTestShip("ca", "CA", 6, 5)
	cl := shellTestShip("cl", "CL", 4, 5)
	dd := shellTestShip("dd", "DD", 2, 5)

	if table := severeDamageTableFrom(HitShell, bb, bc); !containsDamage(table, "severe_flooding") ||
		!containsDamage(table, "hull_damage") || !containsDamage(table, "ammo_explosion") {
		t.Fatal("战巡的炮弹该能打出战列舰的严重进水、船体损伤和弹药爆炸")
	}
	// 差 2 档：能打船体损伤，但打不出致命进水和弹药爆炸。
	if table := severeDamageTableFrom(HitShell, bb, ca); containsDamage(table, "severe_flooding") ||
		containsDamage(table, "ammo_explosion") || !containsDamage(table, "hull_damage") {
		t.Fatal("重巡能打船体损伤，但打不出严重进水和弹药爆炸")
	}
	// 差 3 档以上：连船体损伤都没有。
	for _, attacker := range []*Unit{cl, dd} {
		if table := severeDamageTableFrom(HitShell, bb, attacker); containsDamage(table, "severe_flooding") ||
			containsDamage(table, "hull_damage") || containsDamage(table, "ammo_explosion") {
			t.Fatalf("%s 的炮弹不该打出战列舰的严重进水/船体损伤/弹药爆炸", attacker.Type)
		}
	}
	// 鱼雷不设限：没有攻击方，也不看口径。
	if table := severeDamageTable(HitTorpedo, bb); !containsDamage(table, "severe_flooding") || !containsDamage(table, "hull_damage") {
		t.Fatal("鱼雷该能打出战列舰的严重进水和船体损伤")
	}
}
