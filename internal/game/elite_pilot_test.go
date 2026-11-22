package game

import "testing"

// 开战初期日军精锐飞行员：D3A/B5N 对舰 +1、零战空战 +1、侦察基础发现率 +10%。
func TestElitePilotBonus1942(t *testing.T) {
	state := sandboxState(t)
	if state.Year != 1942 {
		t.Fatalf("沙盒剧本年份应为 1942：%d", state.Year)
	}
	zero := state.Aircraft["b-cv-01-fighter-x1"]
	dive := state.Aircraft["b-cv-01-dive_bomber-x1"]
	torpedo := state.Aircraft["b-cv-01-torpedo_bomber-x1"]

	if zero.AirCombat != 4 { // A6M2 基础 3 + 精锐 1
		t.Fatalf("零战空战应为 4：%d", zero.AirCombat)
	}
	if dive.AntiShip != 3 { // D3A1 基础 2 + 精锐 1
		t.Fatalf("九九舰爆对舰应为 3：%d", dive.AntiShip)
	}
	if torpedo.AntiShip != 3 { // B5N2 基础 2 + 精锐 1
		t.Fatalf("九七舰攻对舰应为 3：%d", torpedo.AntiShip)
	}
	// 美军没有这层加成。
	wildcat := state.Aircraft["a2-cv-01-fighter-x1"]
	if wildcat.AirCombat != 2 {
		t.Fatalf("F4F 空战不应被加成：%d", wildcat.AirCombat)
	}
}

// 侦察发现率：同样的一支小舰队，日军比美军高 10 个点（取未触顶的场景）。
func TestElitePilotReconBonus(t *testing.T) {
	state := sandboxState(t)
	// 只用一艘驱逐舰做目标，避免发现率撞到 100 的上限。
	members := []Unit{{ID: "target-dd", Faction: "A", Type: "DD", Coord: "0303"}}
	origin := &AirUnit{ID: "mother", CarrierID: "b-ca-01", Faction: "B", Coord: "0303", SectorWidth: 60, OriginCoord: "0303"}
	state.Units["b-ca-01"].Coord = "0303"
	state.Contacts = map[string]map[string]*FleetContact{}

	japanese := *origin
	japanese.Faction = "B"
	american := *origin
	american.Faction = "A"

	diff := reconDetectionChance(state, &japanese, members) - reconDetectionChance(state, &american, members)
	if diff != 10 {
		t.Fatalf("日军侦察发现率应高 10 点，实际差 %v", diff)
	}
}

// 1943 年以后精锐加成失效（同一机型不带这层 +1）。
func TestElitePilotBonusExpiresAfter1942(t *testing.T) {
	airCombat, antiShip := elitePilotBonus(State{Year: 1944, Doctrines: map[string]string{"B": "japan"}}, "B", "dive_bomber", 2, 2)
	if airCombat != 2 || antiShip != 2 {
		t.Fatalf("1944 年不应再加成：combat=%d ship=%d", airCombat, antiShip)
	}
	airCombat, antiShip = elitePilotBonus(State{Year: 1942, Doctrines: map[string]string{"B": "japan"}}, "B", "dive_bomber", 2, 2)
	if airCombat != 2 || antiShip != 3 {
		t.Fatalf("1942 年日军舰爆对舰应 +1：combat=%d ship=%d", airCombat, antiShip)
	}
}
