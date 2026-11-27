package game

import "testing"

// 开战初期日军精锐机组：卡面数值不动，临场发挥的掷骰档位升一档（普通按老兵，2 骰优势）。
func TestElitePilotSkill1942(t *testing.T) {
	state := sandboxState(t)
	if state.Year != 1942 {
		t.Fatalf("沙盒剧本年份应为 1942：%d", state.Year)
	}
	zero := state.Aircraft["b-cv-01-fighter-x1"]
	dive := state.Aircraft["b-cv-01-dive_bomber-x1"]
	torpedo := state.Aircraft["b-cv-01-torpedo_bomber-x1"]

	// 卡面就是机型值：精锐机组不往上加 1。
	if zero.AirCombat != 3 {
		t.Fatalf("零战空战应为机型值 3：%d", zero.AirCombat)
	}
	if dive.AntiShip != 2 {
		t.Fatalf("九九舰爆对舰应为机型值 2：%d", dive.AntiShip)
	}
	if torpedo.AntiShip != 2 {
		t.Fatalf("九七舰攻对舰应为机型值 2：%d", torpedo.AntiShip)
	}
	// 精锐体现在掷骰档位：普通机组升到 2 骰优势（75%）。
	for _, plane := range []*AirUnit{zero, dive, torpedo} {
		if plane.Skill != PilotVeteran || PilotDiceAdvantage(plane.Skill) != 2 {
			t.Fatalf("%s 的精锐机组应按老兵档掷骰，实际 %s", plane.Model, plane.Skill)
		}
	}
	// 美军没有这层加成。
	wildcat := state.Aircraft["a2-cv-01-fighter-x1"]
	if wildcat.AirCombat != 2 || wildcat.Skill != PilotRegular {
		t.Fatalf("F4F 不该吃精锐加成：%d/%s", wildcat.AirCombat, wildcat.Skill)
	}
}

// 侦察发现率：1942 年日军精锐机组 +10% 正好抵掉美军侦察基础 +10%（净差 0）；
// 1944 年精锐机组失效，美军靠基础值领先 10 点（取未触顶的场景）。
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
	if diff != 0 {
		t.Fatalf("1942 年双方侦察应持平（精锐 +10 对基础 +10），实际差 %v", diff)
	}
	state.Year = 1944
	diff = reconDetectionChance(state, &japanese, members) - reconDetectionChance(state, &american, members)
	if diff != -10 {
		t.Fatalf("1944 年美军靠侦察基础值应领先 10 点，实际差 %v", diff)
	}
}

// 1943 年以后精锐机组失效；升档最多停在王牌（3 骰），不会叠出 4 骰。
func TestElitePilotSkillExpiresAfter1942(t *testing.T) {
	japan1944 := State{Year: 1944, Doctrines: map[string]string{"B": "japan"}}
	if got := elitePilotSkill(japan1944, "B", PilotRegular); got != PilotRegular {
		t.Fatalf("1944 年不该升档：%s", got)
	}
	japan1942 := State{Year: 1942, Doctrines: map[string]string{"B": "japan"}}
	upgrades := map[string]string{
		PilotRookie:  PilotRegular,
		PilotRegular: PilotVeteran,
		PilotVeteran: PilotAce,
		PilotAce:     PilotAce,
	}
	for from, want := range upgrades {
		if got := elitePilotSkill(japan1942, "B", from); got != want {
			t.Fatalf("1942 年 %s 应升到 %s，实际 %s", from, want, got)
		}
	}
	// 美军不吃这条规则。
	if got := elitePilotSkill(japan1942, "A", PilotRegular); got != PilotRegular {
		t.Fatalf("美军不该升档：%s", got)
	}
}
