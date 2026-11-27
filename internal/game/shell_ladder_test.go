package game

import "testing"

func shellLadderUnit(typ string, strength, damageControl int) *Unit {
	return &Unit{ID: "t", FleetID: "f", Name: typ, Type: typ, Faction: "A", Coord: "0101",
		Strength: strength, MaxStrength: strength,
		Card: ShipCard{DamageControl: damageControl}}
}

// 舰种决定损伤阶梯的起点：BB 1%、BC 2%、重巡/航母 4%、轻巡/轻母 8%、驱逐 10%，
// 护航航母和运输船 12%（没有能挡炮弹的结构）。
func TestShellDamageChanceByShipType(t *testing.T) {
	want := map[string]float64{
		"BB": 1, "BC": 2, "CA": 4, "CV": 4, "CL": 8, "CVL": 8, "DD": 10, "CVE": 12, "TRS": 12,
	}
	for typ, chance := range want {
		if got := shellDamageChance(shellLadderUnit(typ, 6, 5)); got != chance {
			t.Fatalf("%s 的直接扣血概率应为 %v%%，实际 %v%%", typ, chance, got)
		}
	}
}

// 每挨一发炮弹下移一级，DD 封顶。
func TestShellHitsDegradeTier(t *testing.T) {
	bb := shellLadderUnit("BB", 20, 10)
	for index, want := range []float64{1, 2, 4, 8, 10, 10} {
		if got := shellDamageChance(bb); got != want {
			t.Fatalf("挨了 %d 发的战列舰应为 %v%%，实际 %v%%", index, want, got)
		}
		EnsureDamage(bb).ShellHits++
	}
}

// 炮弹命中会推进阶梯，并且按当前概率直接扣血：1 点生命值的驱逐舰挨到必然沉没。
func TestShellHitDeductsStrengthAndSinks(t *testing.T) {
	state := &State{RNGState: 7, Doctrines: map[string]string{}}
	target := shellLadderUnit("DD", 1, 10)
	state.Units = map[string]*Unit{"t": target}
	for hit := 0; hit < 200 && state.Units["t"] != nil; hit++ {
		applyHits(state, []Hit{{AttackerID: "t", TargetID: "t", Cause: HitShell}})
	}
	if state.Units["t"] != nil {
		t.Fatalf("1 点生命的驱逐舰挨了 200 发还没沉：ShellHits=%d", target.Damage.ShellHits)
	}
}

// 炸弹和鱼雷不推炮弹那条阶梯：只有 HitShell 计数。
func TestNonShellHitsDoNotDegradeTier(t *testing.T) {
	state := &State{RNGState: 11, Doctrines: map[string]string{}}
	target := shellLadderUnit("BB", 40, 10)
	state.Units = map[string]*Unit{"t": target}
	applyHits(state, []Hit{{AttackerID: "t", TargetID: "t", Cause: HitBomb}})
	if got := EnsureDamage(target).ShellHits; got != 0 {
		t.Fatalf("炸弹不该推炮弹阶梯：%d", got)
	}
	applyHits(state, []Hit{{AttackerID: "t", TargetID: "t", Cause: HitShell}})
	if got := EnsureDamage(target).ShellHits; got != 1 {
		t.Fatalf("炮弹应推一级：%d", got)
	}
}
