package game

import (
	"strings"
	"testing"
)

// 回归：珊瑚海真实剧本的陆基航空队（拉包尔 / 莫尔兹比港）。
// 拉包尔：0428，6 架零战 + 6 架 G4M + 3 架 G3M（陆攻挂鱼雷）。
// 莫尔兹比港：1612，2 架 SBD + 4 架 P-40 + 3 架 B-25。
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
	if rabaul == nil || rabaul.Coord != "0428" || rabaul.Faction != "B" {
		t.Fatalf("拉包尔机场部署错误：%+v", rabaul)
	}
	moresby := state.Airfields["af-a-moresby"]
	if moresby == nil || moresby.Coord != "1612" || moresby.Faction != "A" {
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
		"af-b-rabaul":  {"fighter": 6, "g4m": 6, "g3m": 3},
		"af-a-moresby": {"dive_bomber": 2, "p40": 4, "b25": 3},
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
	applyGroundLoadoutChoice(g4m, DefaultLoadout(g4m.Kind, false, "attack", StrikeTargetShip))
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
