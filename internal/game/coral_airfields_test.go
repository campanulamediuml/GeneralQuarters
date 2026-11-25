package game

import (
	"strings"
	"testing"
)

// 回归：珊瑚海真实剧本的陆基航空队（拉包尔 / 莫尔兹比港）。
// 拉包尔：0323，6 架零战 + 6 架 G4M + 3 架 G3M（陆攻挂鱼雷）。
// 莫尔兹比港：1611，2 架 SBD + 4 架 P-40 + 3 架 B-25。
func TestCoralRealLandBasedAirGroups(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 5)
	if err != nil {
		t.Fatal(err)
	}
	rabaul := state.Airfields["af-b-rabaul"]
	if rabaul == nil || rabaul.Coord != "0323" || rabaul.Faction != "B" {
		t.Fatalf("拉包尔机场部署错误：%+v", rabaul)
	}
	moresby := state.Airfields["af-a-moresby"]
	if moresby == nil || moresby.Coord != "1611" || moresby.Faction != "A" {
		t.Fatalf("莫尔兹比港机场部署错误：%+v", moresby)
	}
	groups := map[string]map[string]int{}
	for _, aircraft := range state.GroundAir {
		if groups[aircraft.AirfieldID] == nil {
			groups[aircraft.AirfieldID] = map[string]int{}
		}
		groups[aircraft.AirfieldID][aircraft.DefinitionID]++
	}
	want := map[string]map[string]int{
		"af-b-rabaul":     {"fighter": 6, "g4m": 6, "g3m": 3},
		"af-a-moresby":    {"dive_bomber": 2, "p40": 4, "b25": 3, "b17": 3},
		"af-a-townsville": {"b17": 4, "b26": 4, "p40": 4},
	}
	for airfieldID, expected := range want {
		for kind, count := range expected {
			if got := groups[airfieldID][kind]; got != count {
				t.Fatalf("%s 的 %s 应有 %d 架，实际 %d", airfieldID, kind, count, got)
			}
		}
	}

	// 机型数据：G4M 航程 40、G3M 38、都是强度 1 / 对空 0 / 对舰 2（1942 精锐 +1 后对舰 3）。
	for _, id := range []string{"af-b-rabaul-g4m-1", "af-b-rabaul-g3m-1"} {
		aircraft := state.GroundAir[id]
		wantRange := 40
		if strings.Contains(id, "g3m") {
			wantRange = 38
		}
		if aircraft == nil || aircraft.BaseRange != wantRange || aircraft.Strength != 1 || aircraft.AntiShip != 3 {
			t.Fatalf("%s 数据不对：%+v", id, aircraft)
		}
	}
	// 陆攻对舰出击要挂鱼雷。
	g4m := state.GroundAir["af-b-rabaul-g4m-1"]
	applyGroundLoadoutChoice(g4m, DefaultLoadout(g4m.Kind, false, g4m.AntiGround, g4m.AntiShip, "attack", StrikeTargetShip))
	if !strings.Contains(string(g4m.Loadout), "torpedo") {
		t.Fatalf("G4M 对舰出击没有挂鱼雷：%s", g4m.Loadout)
	}
	// P-40 / B-25 的数值。陆基机型要带 airfield_only，前端据此不显示舰载机的蓝色航程框。
	p40 := state.GroundAir["af-a-moresby-p40-1"]
	if p40 == nil || p40.Strength != 2 || p40.BaseRange != 17 || p40.AirCombat != 2 || !p40.AirfieldOnly {
		t.Fatalf("P-40 数据不对：%+v", p40)
	}
	b25 := state.GroundAir["af-a-moresby-b25-1"]
	if b25 == nil || b25.Strength != 3 || b25.AirCombat != 2 || b25.AntiShip != 1 || b25.AntiGround != 2 || !b25.AirfieldOnly {
		t.Fatalf("B-25 数据不对：%+v", b25)
	}
	// 舰载机（SBD / A6M）不能被标记成陆基。
	if sbd := state.GroundAir["af-a-moresby-dive_bomber-1"]; sbd == nil || sbd.AirfieldOnly {
		t.Fatalf("SBD 不该是陆基机型：%+v", sbd)
	}
}

// 图拉吉、德伯因重新开放水机基地；日军还有肖特兰、莱卡塔，美军有鲍恩。
func TestCoralRealSeaplaneBases(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 5)
	if err != nil {
		t.Fatal(err)
	}
	wantBases := map[string]string{
		"af-b-tulagi":    "1541",
		"af-b-deboyne":   "1023",
		"af-b-shortland": "0831",
		"af-b-rekata":    "1238",
		"af-a-bowen":     "4613",
	}
	for id, coord := range wantBases {
		base := state.Airfields[id]
		if base == nil || base.Coord != coord || base.Type != AirfieldSeaplane {
			t.Fatalf("%s 水机基地部署错误：%+v", id, base)
		}
	}
	counts := map[string]map[string]int{}
	for _, aircraft := range state.GroundAir {
		if counts[aircraft.AirfieldID] == nil {
			counts[aircraft.AirfieldID] = map[string]int{}
		}
		counts[aircraft.AirfieldID][aircraft.DefinitionID]++
	}
	if counts["af-b-tulagi"]["h6k"] != 4 || counts["af-b-tulagi"]["seaplane"] != 3 {
		t.Fatalf("图拉吉机型数量错误：%v", counts["af-b-tulagi"])
	}
	if counts["af-b-deboyne"]["seaplane"] != 2 {
		t.Fatalf("德伯因机型数量错误：%v", counts["af-b-deboyne"])
	}
	if counts["af-b-shortland"]["h6k"] != 6 {
		t.Fatalf("肖特兰 H6K 数量错误：%v", counts["af-b-shortland"])
	}
	if counts["af-b-rekata"]["h6k"] != 3 || counts["af-b-rekata"]["seaplane"] != 2 {
		t.Fatalf("莱卡塔机型数量错误：%v", counts["af-b-rekata"])
	}
	if counts["af-a-bowen"]["pby"] != 2 {
		t.Fatalf("鲍恩 PBY 数量错误：%v", counts["af-a-bowen"])
	}
}

// 莱城（0910）：3 架 A5M + 3 架 G4M。A5M 基础对空 1、航程 13、强度 1、无对地对舰。
func TestCoralRealLaeAirGroup(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 5)
	if err != nil {
		t.Fatal(err)
	}
	lae := state.Airfields["af-b-lae"]
	if lae == nil || lae.Coord != "0910" || lae.Type != AirfieldMedium || lae.Faction != "B" {
		t.Fatalf("莱城机场部署错误：%+v", lae)
	}
	counts := map[string]int{}
	for _, aircraft := range state.GroundAir {
		if aircraft.AirfieldID == "af-b-lae" {
			counts[aircraft.DefinitionID]++
		}
	}
	if counts["a5m"] != 3 || counts["g4m"] != 3 {
		t.Fatalf("莱城机群数量错误：%v", counts)
	}
	a5m := state.GroundAir["af-b-lae-a5m-1"]
	// 1942 年日军精锐飞行员规则给战斗机对空 +1，所以这里读到的是基础 1 + 1 = 2。
	if a5m == nil || a5m.Model != "A5M 九六舰战" || a5m.BaseRange != 13 || a5m.Strength != 1 || a5m.AirCombat != 2 || a5m.AntiShip != 0 || a5m.AntiGround != 0 {
		t.Fatalf("A5M 数值错误：%+v", a5m)
	}
}
