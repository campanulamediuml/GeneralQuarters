package game

import (
	"math"
	"testing"
)

// 鱼雷机挂炸弹时按轰炸机结算：不算鱼雷攻击、轻防空不翻倍、排进炸弹波；
// 但它仍是低空小轰炸机，轻防空照样打得到，也不享受高空免疫。
func TestTorpedoBomberWithBombsIsABomber(t *testing.T) {
	state := &State{
		RNGState:  5,
		Map:       MapDefinition{Rows: 7, Cols: 9},
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units: map[string]*Unit{
			"bb": {ID: "bb", Faction: "B", FleetID: "b", Type: "BB", Coord: "0202", Strength: 6, MaxStrength: 6,
				Card: ShipCard{SpeedClass: "normal", LightAA: 10, DamageControl: 10}},
		},
		Aircraft: map[string]*AirUnit{},
	}
	ship := state.Units["bb"]
	bombTBF := &AirUnit{ID: "tbf-bomb", Faction: "A", Kind: "torpedo_bomber", Model: "TBF 复仇者", Loadout: LoadoutBombTank}
	torpedoTBF := &AirUnit{ID: "tbf-torp", Faction: "A", Kind: "torpedo_bomber", Model: "TBF 复仇者", Loadout: LoadoutTorpedoTank}

	if isTorpedoAttack(bombTBF) {
		t.Fatal("挂炸弹的鱼雷机不应按鱼雷机结算")
	}
	if ignoresLightAA(bombTBF) {
		t.Fatal("挂炸弹的鱼雷机是低空小轰炸机，不该免疫轻防空")
	}
	if got := bombWavePriority(bombTBF); got != 1 {
		t.Fatalf("挂炸弹的鱼雷机应排进炸弹波（1），实际 %d", got)
	}

	base := aaHitChance(*state, ship.Coord, false, ship.Faction)
	if got := lightAAHitChance(*state, ship, bombTBF, false); math.Abs(got-base) > 0.01 {
		t.Fatalf("挂炸弹的鱼雷机不应吃轻防空翻倍：%.2f vs 基础 %.2f", got, base)
	}
	if got := lightAAHitChance(*state, ship, torpedoTBF, false); math.Abs(got-base*2) > 0.01 {
		t.Fatalf("挂鱼雷的鱼雷机轻防空应翻倍：%.2f vs 基础 %.2f", got, base*2)
	}
	if !isTorpedoAttack(torpedoTBF) || bombWavePriority(torpedoTBF) != 2 {
		t.Fatal("挂鱼雷的鱼雷机仍应排进鱼雷波")
	}
}

// B-26 挂鱼雷是低空突击：照常吃轻防空翻倍；挂炸弹才走高空免疫。
func TestB26LoadoutDecidesAltitude(t *testing.T) {
	bomb := &AirUnit{ID: "b26-bomb", Kind: "medium_bomber", Model: "B-26 劫掠者", TorpedoCapable: true, HighAltitudeBomber: true, Loadout: LoadoutBomb}
	torpedo := &AirUnit{ID: "b26-torp", Kind: "medium_bomber", Model: "B-26 劫掠者", TorpedoCapable: true, HighAltitudeBomber: true, Loadout: LoadoutTorpedoTank}

	if !isHighAltitudeBomber(bomb) {
		t.Fatal("B-26 挂炸弹应在高空投弹")
	}
	if isTorpedoAttack(bomb) {
		t.Fatal("B-26 挂炸弹不是鱼雷攻击")
	}
	if !isTorpedoAttack(torpedo) {
		t.Fatal("B-26 挂鱼雷应按鱼雷机结算")
	}
	if isHighAltitudeBomber(torpedo) {
		t.Fatal("B-26 挂鱼雷是低空突击，不算高空")
	}
	if ignoresLightAA(torpedo) {
		t.Fatal("B-26 挂鱼雷是低空突击，不该免疫轻防空")
	}
}

// 中型轰炸机挂炸弹打舰同样在高空投弹：100 点轻防空一颗骰子也落不到它头上，
// 只有重防空能打。命中率仍按中型机的水平轰炸折减（由 strikeHitChance 单独决定）。
func TestMediumBomberIgnoresShipLightAA(t *testing.T) {
	state := &State{
		RNGState:  9,
		Map:       MapDefinition{Rows: 7, Cols: 9},
		Doctrines: map[string]string{"A": "us", "B": "japan"},
		Units: map[string]*Unit{
			"bb": {ID: "bb", Faction: "B", FleetID: "b", Type: "BB", Coord: "0202", Name: "bb",
				Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", LightAA: 100, DamageControl: 10}},
		},
		Aircraft: map[string]*AirUnit{},
	}
	b25 := &AirUnit{ID: "b25", Faction: "A", Kind: "level_bomber", Model: "B-25 米切尔", Status: "attack",
		Coord: "0202", Strength: 4, AntiShip: 2, Loadout: LoadoutBomb}
	state.Aircraft[b25.ID] = b25
	report := &AirCombatReport{}
	resolveBombingWave(state, []*AirUnit{b25}, []Unit{*state.Units["bb"]}, false, true, report)
	if report.AAHits != 0 {
		t.Fatalf("中型轰炸机高空投弹不应吃轻防空，实际命中 %d 次", report.AAHits)
	}
}
