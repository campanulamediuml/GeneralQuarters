package game

import (
	"math"
	"testing"
)

// 水上飞机飞得慢，但最终发现率 ×2：两条侦察链路都要按这个来。
func TestSeaplaneReconChanceDoubled(t *testing.T) {
	state := coralCombatState(t)
	members := []Unit{{Type: "DD", Coord: "0832", Faction: "A"}}
	plain := reconDetectionChance(state, &AirUnit{Faction: "B", OriginCoord: "0831", SectorWidth: 60}, members)
	seaplane := reconDetectionChance(state, &AirUnit{Faction: "B", OriginCoord: "0831", SectorWidth: 60, Seaplane: true}, members)
	if plain <= 0 || math.Abs(seaplane-2*plain) > 0.001 {
		t.Fatalf("水上飞机最终发现率应×2：普通 %.1f / 水机 %.1f", plain, seaplane)
	}
}

func TestGroundSeaplaneReconChanceDoubled(t *testing.T) {
	state := coralCombatState(t)
	members := []Unit{{Type: "DD", Coord: "0832", Faction: "A"}}
	plain := groundReconChance(state, &AirfieldAirUnit{OriginCoord: "0831", SectorWidth: 60}, members)
	seaplane := groundReconChance(state, &AirfieldAirUnit{OriginCoord: "0831", SectorWidth: 60, Seaplane: true}, members)
	if plain <= 0 || math.Abs(seaplane-2*plain) > 0.001 {
		t.Fatalf("陆基水机最终发现率应×2：普通 %.1f / 水机 %.1f", plain, seaplane)
	}
}
