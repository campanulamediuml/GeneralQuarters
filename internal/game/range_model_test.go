package game

import "testing"

// 航程模型：
//
//	基础航程 = 空载
//	挂副油箱 -> 基础 x 1.3，不带惩罚
//	不挂副油箱 -> 在基础上施加惩罚（重型炸弹 -30%；鱼雷只是没有加成）
//
// 备用航程（4 hex）单独记账。
func TestSortieRangeModel(t *testing.T) {
	const base = 9
	cases := []struct {
		loadout Loadout
		primary int
		reserve int
		note    string
	}{
		{LoadoutBombTank, 12, 4, "炸弹+副油箱 = 9x1.3"},
		{LoadoutTorpedoTank, 12, 4, "鱼雷+副油箱 = 9x1.3"},
		{LoadoutBomb, 6, 4, "重型炸弹 = 9x0.7"},
		{LoadoutTorpedo, 9, 4, "鱼雷无油箱 = 基础，无惩罚"},
	}
	for _, c := range cases {
		fuel := MountedFuel(base, c.loadout)
		if got := fuel.Internal + fuel.Tank; got != c.primary {
			t.Fatalf("%s：主燃油应为 %d，实际 %d", c.note, c.primary, got)
		}
		if fuel.Reserve != c.reserve {
			t.Fatalf("%s：备用航程应为 %d，实际 %d", c.note, c.reserve, fuel.Reserve)
		}
		if got := SortieBaseRange(base, c.loadout); got != c.primary {
			t.Fatalf("%s：SortieBaseRange 应为 %d，实际 %d", c.note, c.primary, got)
		}
	}
	if SortieBaseRange(base, LoadoutBombTank) <= base {
		t.Fatal("挂副油箱应当增加航程")
	}
	if SortieBaseRange(base, LoadoutBomb) >= base {
		t.Fatal("重型炸弹应当减少航程")
	}
	if SortieBaseRange(base, LoadoutTorpedo) != base {
		t.Fatal("鱼雷无油箱不应有惩罚")
	}
}
