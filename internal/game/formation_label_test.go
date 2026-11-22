package game

import "testing"

// TestFormationLabelListsEveryModel 战报里的编队描述要按机型分开写，
// 不能统一用领队机型（"放飞7个A6M2 零战"而实际是 5 舰攻 + 2 零战）。
func TestFormationLabelListsEveryModel(t *testing.T) {
	members := []*AirUnit{
		{Model: "B5N2 九七舰攻"}, {Model: "B5N2 九七舰攻"}, {Model: "B5N2 九七舰攻"},
		{Model: "A6M2 零战"}, {Model: "A6M2 零战"},
	}
	label := formationLabel(members)
	if !containsText(label, "B5N2 九七舰攻×3") || !containsText(label, "A6M2 零战×2") {
		t.Fatalf("混合编队应按机型分别计数：%s", label)
	}
	single := formationLabel([]*AirUnit{{Model: "SB2C 地狱俯冲者"}, {Model: "SB2C 地狱俯冲者"}})
	if single != "SB2C 地狱俯冲者" {
		t.Fatalf("单一机型应直接给机型名：%s", single)
	}
}
