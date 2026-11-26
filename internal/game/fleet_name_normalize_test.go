package game

import "testing"

// 老存档里拆分出来的编队名字就是 split-<uuid>；读档时要修成能认的名字。
func TestNormalizeFleetNamesFixesSplitIDs(t *testing.T) {
	state := State{Units: map[string]*Unit{
		"a1": {ID: "a1", Name: "惠普尔", FleetID: "split-abc", FleetName: "split-abc"},
		"a2": {ID: "a2", Name: "莫里斯", FleetID: "split-abc", FleetName: "split-abc"},
		"a3": {ID: "a3", Name: "法拉格特", FleetID: "TF17", FleetName: "第17特混舰队"},
	}}
	NormalizeFleetNames(&state)
	if state.Units["a1"].FleetName != "惠普尔·分队" || state.Units["a2"].FleetName != "惠普尔·分队" {
		t.Fatalf("split 编队名未修好：%q / %q", state.Units["a1"].FleetName, state.Units["a2"].FleetName)
	}
	if state.Units["a3"].FleetName != "第17特混舰队" {
		t.Fatalf("不该动的舰队名被改了：%q", state.Units["a3"].FleetName)
	}
	// 幂等：再跑一次不会变成"惠普尔·分队·分队"。
	NormalizeFleetNames(&state)
	if state.Units["a1"].FleetName != "惠普尔·分队" {
		t.Fatalf("重复归一化改变了名字：%q", state.Units["a1"].FleetName)
	}
}
