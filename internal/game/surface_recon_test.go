package game

import "testing"

// 舰队侦查的成功率修正（规则书 578–585 行）。
func TestSurfaceReconChanceModifiers(t *testing.T) {
	state := State{Map: MapDefinition{Cells: []MapCell{{Coord: "0102", Weather: "rain"}}}, Night: true}
	observer := []Unit{{ID: "a", Type: "DD", Coord: "0101"}}
	target := []Unit{{ID: "b", Type: "DD", Coord: "0102"}}
	// 50 - 15(降水) - 10(夜间) + 1(己方 1 艘) + 1(敌方普通 1 艘) = 27
	if chance := surfaceReconChance(state, observer, target); chance != 27 {
		t.Fatalf("基础修正错误：%.1f，期望 27", chance)
	}
	// 侦查方装备对海搜索雷达 +20
	observer[0].Card.SurfaceRadar = true
	if chance := surfaceReconChance(state, observer, target); chance != 47 {
		t.Fatalf("对海雷达加成错误：%.1f，期望 47", chance)
	}
	// 雷达被打坏以后不再给加成
	observer[0].Damage = &ShipDamage{SurfaceRadarDisabled: true}
	if chance := surfaceReconChance(state, observer, target); chance != 27 {
		t.Fatalf("搜索雷达损坏后仍有加成：%.1f，期望 27", chance)
	}
	// 目标换成战列舰：+2 替代 +1；漏油 +10；起火 +20
	target = []Unit{{
		ID: "b", Type: "BB", Coord: "0102",
		Damage: &ShipDamage{OilLeak: true, Fire: "burning"},
	}}
	observer[0].Damage = nil
	// 50 - 15 - 10 + 20(雷达) + 1(己方 1 艘) + 2(战列舰) + 10(漏油) + 20(起火) = 78
	if chance := surfaceReconChance(state, observer, target); chance != 78 {
		t.Fatalf("目标修正错误：%.1f，期望 78", chance)
	}
}

func surfaceReconState(dist string, seed uint64) State {
	return State{
		RNGState: seed,
		Map:      MapDefinition{Rows: 3, Cols: 3, Cells: []MapCell{}},
		Contacts: map[string]map[string]*FleetContact{},
		Units: map[string]*Unit{
			"a1": {ID: "a1", FleetID: "A1", FleetName: "第17特混舰队", Name: "法拉格特", Type: "DD", Faction: "A", Coord: "0101", Strength: 3, MaxStrength: 3},
			"b1": {ID: "b1", FleetID: "B1", FleetName: "珊瑚海机动部队", Name: "白露", Type: "DD", Faction: "B", Coord: dist, Strength: 3, MaxStrength: 3},
		},
	}
}

// 相邻 1 hex 会记情报，而且只加一级。
func TestSurfaceReconRaisesIntelForAdjacentFleet(t *testing.T) {
	hit := false
	for seed := uint64(1); seed <= 40 && !hit; seed++ {
		state := surfaceReconState("0102", seed)
		events := resolveSurfaceRecon(&state)
		if len(events) == 0 {
			t.Fatalf("相邻舰队没有产生侦查判定事件")
		}
		contact := state.Contacts["A"]["B1"]
		if contact == nil {
			continue
		}
		hit = true
		if contact.Level != 1 {
			t.Fatalf("发现后情报等级为 %d，期望 1", contact.Level)
		}
		found := false
		for _, event := range events {
			if event.Type == "surface_recon" && event.Audience == "A" && contact.Level == 1 {
				found = true
			}
		}
		if !found {
			t.Fatal("没有面向侦查方的 surface_recon 事件")
		}
	}
	if !hit {
		t.Fatal("40 个种子里一次都没掷中，判定概率明显不对")
	}
}

// 同格不重复掷（接触已经处理），2 hex 内外都超出水面侦查圈。
func TestSurfaceReconIgnoresSameHexAndDistantFleets(t *testing.T) {
	for _, coord := range []string{"0101", "0103", "0203"} {
		for seed := uint64(1); seed <= 20; seed++ {
			state := surfaceReconState(coord, seed)
			events := resolveSurfaceRecon(&state)
			if len(events) != 0 {
				t.Fatalf("%s（距离 %d）不该产生水面侦查：%v", coord, Distance("0101", coord), events[0].Text)
			}
			if state.Contacts["A"]["B1"] != nil {
				t.Fatalf("%s（距离 %d）不该记情报", coord, Distance("0101", coord))
			}
		}
	}
}
