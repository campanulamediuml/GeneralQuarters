package game

import "testing"

// 马里亚纳的日军水侦配属：除航母外，每艘 BB/BC/CA 都按卡面机库带满 E13A；
// 伊势、日向是航空战列舰（3 E13A + 3 F1M2）。这一批曾经漏配过（7 艘重巡 + 5 艘主力舰），
// 这条用例把整份清单钉死，改编成时别再把水侦落掉。
func TestMarianasJapaneseFloatplanes(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}

	names := map[string]string{}
	for _, unit := range state.Units {
		if unit.Faction == "B" {
			names[unit.ID] = unit.Name
		}
	}
	got := map[string]map[string]int{}
	for _, plane := range state.Aircraft {
		if plane.Faction != "B" {
			continue
		}
		name := names[plane.CarrierID]
		if name == "" {
			continue
		}
		if got[name] == nil {
			got[name] = map[string]int{}
		}
		got[name][plane.DefinitionID]++
	}

	want := map[string]map[string]int{
		"大和": {"e13a": 2}, "武藏": {"e13a": 2}, "长门": {"e13a": 2}, "陆奥": {"e13a": 2},
		"金刚": {"e13a": 2}, "榛名": {"e13a": 2}, "比叡": {"e13a": 2}, "雾岛": {"e13a": 2},
		"扶桑": {"e13a": 3}, "山城": {"e13a": 3},
		"妙高": {"e13a": 2}, "羽黑": {"e13a": 2}, "高雄": {"e13a": 2}, "爱宕": {"e13a": 2},
		"摩耶": {"e13a": 2}, "鸟海": {"e13a": 2}, "利根": {"e13a": 2}, "筑摩": {"e13a": 2},
		"那智": {"e13a": 2}, "足柄": {"e13a": 2}, "最上": {"e13a": 3}, "铃谷": {"e13a": 3}, "熊野": {"e13a": 3},
		"伊势": {"e13a": 3, "f1m": 3}, "日向": {"e13a": 3, "f1m": 3},
	}
	for name, expect := range want {
		if got[name] == nil {
			t.Fatalf("%s 没有配水侦", name)
		}
		for model, count := range expect {
			if got[name][model] != count {
				t.Fatalf("%s 的 %s 应是 %d 架，实际 %d（%v）", name, model, count, got[name][model], got[name])
			}
		}
	}

	// 总数：日军 292 个航空算子（航母 229 + 水侦/水战 63），全图 668。
	japanese := 0
	for _, plane := range state.Aircraft {
		if plane.Faction == "B" {
			japanese++
		}
	}
	if japanese != 292 {
		t.Fatalf("日军航空算子 %d 个，期望 292", japanese)
	}
	if len(state.Aircraft) != 668 {
		t.Fatalf("全图航空算子 %d 个，期望 668", len(state.Aircraft))
	}
}
