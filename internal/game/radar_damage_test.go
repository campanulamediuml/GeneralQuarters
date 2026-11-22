package game

import "testing"

// 火控雷达和搜索雷达拆开以后，两种损坏各扣各的东西：
// 火控雷达损坏只扣炮击命中率，搜索雷达损坏只取消突袭判定的对海雷达加成。
func TestFireControlAndSearchRadarDamageAreSeparate(t *testing.T) {
	build := func() (*Unit, *Unit, State) {
		attacker := &Unit{ID: "a", FleetID: "a", Type: "BB", Faction: "A", Coord: "0101", Card: ShipCard{Gunnery: 16, SurfaceRadar: true}}
		target := &Unit{ID: "b", FleetID: "b", Type: "CA", Faction: "B", Coord: "0101"}
		state := State{RNGState: 42, Doctrines: map[string]string{"A": "us"}, Map: MapDefinition{Cells: []MapCell{{Coord: "0101"}}}, Units: map[string]*Unit{"a": attacker, "b": target}}
		return attacker, target, state
	}

	// 基准：BB 打 CA，25 + 10（对海雷达）= 35。
	attacker, target, state := build()
	if chance := gunneryChance(state, attacker, target, []Unit{*target}); chance != 35 {
		t.Fatalf("基准命中率 %.1f，期望 35", chance)
	}

	// 火控雷达损坏：炮击加成消失。
	attacker, target, state = build()
	EnsureDamage(attacker).FireControlRadarDisabled = true
	if chance := gunneryChance(state, attacker, target, []Unit{*target}); chance != 25 {
		t.Fatalf("火控雷达损坏后命中率 %.1f，期望 25", chance)
	}

	// 搜索雷达损坏：命中率不变。
	attacker, target, state = build()
	EnsureDamage(attacker).SurfaceRadarDisabled = true
	if chance := gunneryChance(state, attacker, target, []Unit{*target}); chance != 35 {
		t.Fatalf("搜索雷达损坏后命中率 %.1f，期望 35（不该影响炮击）", chance)
	}

	// 突袭判定：搜索雷达损坏后舰队不再算"有对海雷达"。
	members := []Unit{{ID: "a", Card: ShipCard{SurfaceRadar: true}, Damage: &ShipDamage{}}}
	if !hasSurfaceRadar(members) {
		t.Fatal("完好的对海雷达应该计入突袭判定")
	}
	members[0].Damage.SurfaceRadarDisabled = true
	if hasSurfaceRadar(members) {
		t.Fatal("搜索雷达损坏后不该再计入突袭判定")
	}
	members[0].Damage.SurfaceRadarDisabled = false
	members[0].Damage.FireControlRadarDisabled = true
	if !hasSurfaceRadar(members) {
		t.Fatal("火控雷达损坏不该影响突袭判定")
	}
}

// 两种雷达损坏必须是严重伤害表里两条独立的条目，不能又合并回同一个效果。
func TestRadarDamageTableEntriesAreDistinct(t *testing.T) {
	unit := &Unit{ID: "b", FleetID: "b", Name: "雷达舰", Type: "CA", Faction: "A", Coord: "0101", Strength: 4, MaxStrength: 4, Card: ShipCard{DamageControl: 6, SurfaceRadar: true}}
	state := &State{RNGState: 7, Units: map[string]*Unit{"b": unit}, Doctrines: map[string]string{}}

	EnsureDamage(unit).FireControlRadarDisabled = false
	applySevereDamageResult(state, unit, "fire_control_radar_damage")
	if !unit.Damage.FireControlRadarDisabled || unit.Damage.SurfaceRadarDisabled {
		t.Fatal("火控雷达损坏只应该置火控标记")
	}
	unit.Damage.FireControlRadarDisabled = false
	applySevereDamageResult(state, unit, "surface_radar_damage")
	if !unit.Damage.SurfaceRadarDisabled || unit.Damage.FireControlRadarDisabled {
		t.Fatal("搜索雷达损坏只应该置搜索标记")
	}
}
