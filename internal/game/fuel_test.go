package game

import "testing"

// 规则书 47 行：副油箱提供额外 30% 航程（总航程 = base × 1.3，四舍五入）。
func TestNewFuelSplitsTankAndInternal(t *testing.T) {
	cases := []struct {
		base, tank, internal int
	}{
		{10, 3, 10}, // round(13) - 10 = 3
		{9, 3, 9},   // round(11.7)=12 - 9 = 3
		{12, 4, 12}, // round(15.6)=16 - 12 = 4
	}
	for _, item := range cases {
		fuel := NewFuel(item.base, true)
		if fuel.Tank != item.tank || fuel.Internal != item.internal || fuel.Reserve != ReserveRange {
			t.Fatalf("基础航程%d拆分为 %+v，期望 Tank=%d Internal=%d", item.base, fuel, item.tank, item.internal)
		}
	}
	// 不挂副油箱时没有副油箱里程
	plain := NewFuel(10, false)
	if plain.Tank != 0 || plain.Internal != 10 {
		t.Fatalf("不挂副油箱时航程拆分错误：%+v", plain)
	}
}

// 规则书 49 行：副油箱航程优先消耗。
func TestFuelConsumesTankFirst(t *testing.T) {
	fuel := Fuel{Tank: 4, Internal: 12, Reserve: 4}
	fuel.Consume(3)
	if fuel.Tank != 1 || fuel.Internal != 12 {
		t.Fatalf("副油箱没有被优先消耗：%+v", fuel)
	}
	fuel.Consume(3) // 1 点来自副油箱，2 点来自机内
	if fuel.Tank != 0 || fuel.Internal != 10 {
		t.Fatalf("跨桶消耗错误：%+v", fuel)
	}
}

// 主油耗尽后才动用备用油，并让剩余航程变负（规则书 69、71 行）。
func TestFuelFallsBackToReserve(t *testing.T) {
	fuel := Fuel{Internal: 1, Reserve: 4}
	fuel.Consume(3) // 1 点内油 + 2 点备用
	if fuel.Reserve != 2 || fuel.Internal != 0 {
		t.Fatalf("备用油消耗错误：%+v", fuel)
	}
	fuel.Consume(4) // 2 点备用 + 2 点透支
	if fuel.Reserve != -2 {
		t.Fatalf("备用油透支后应为负数：%+v", fuel)
	}
}

// 抛弃副油箱只损失还没烧掉的那部分。
func TestDropTankOnlyLosesRemaining(t *testing.T) {
	fuel := Fuel{Tank: 3, Internal: 12, Reserve: 4}
	fuel.Consume(1) // 用掉 1 点副油箱
	wasted := fuel.DropTank()
	if wasted != 2 {
		t.Fatalf("投掉副油箱浪费的里程应当只剩 2，实际%d", wasted)
	}
	if fuel.Tank != 0 || fuel.Internal != 12 {
		t.Fatalf("投掉副油箱后机内油不应受影响：%+v", fuel)
	}
}

// Range() 不含备用油；备用油单独查询。
func TestAirUnitRangeExcludesReserve(t *testing.T) {
	aircraft := &AirUnit{Fuel: Fuel{Tank: 3, Internal: 10, Reserve: 4}}
	if got := aircraft.Range(); got != 13 {
		t.Fatalf("Range() 应为副油箱+机内=13，实际%d", got)
	}
	if got := aircraft.ReserveLeft(); got != 4 {
		t.Fatalf("备用油应为4，实际%d", got)
	}
	// 消耗 14 点：13 点主油 + 1 点备用。主油耗尽后，剩余航程由备用油余额表示。
	aircraft.Spend(14)
	if got := aircraft.Range(); got != 3 {
		t.Fatalf("主油耗尽后应继续用备用油，剩余3；实际%d", got)
	}
	if got := aircraft.ReserveLeft(); got != 3 {
		t.Fatalf("备用油应剩3，实际%d", got)
	}
	// 备用油也烧穿：剩余航程变负（规则书 72-76 行的 -1、-2…）
	aircraft.Spend(4)
	if got := aircraft.Range(); got != -1 {
		t.Fatalf("备用油透支后 Range() 应为 -1，实际%d", got)
	}
}

// SetRange 与 Spend 保持 RemainingRange 旧字段同步，兼容老存档。
func TestLegacyRemainingRangeStaysInSync(t *testing.T) {
	aircraft := &AirUnit{}
	aircraft.SetRange(12)
	if aircraft.Range() != 12 || aircraft.RemainingRange != 12 {
		t.Fatalf("SetRange 后不同步：%+v", aircraft)
	}
	aircraft.Spend(2)
	if aircraft.Range() != 10 || aircraft.RemainingRange != 10 {
		t.Fatalf("Spend 后不同步：%+v", aircraft)
	}
	// 只有旧字段（没迁移过的存档）
	legacy := &AirUnit{RemainingRange: 7}
	if legacy.Range() != 7 {
		t.Fatalf("老存档应当回退到旧字段：%d", legacy.Range())
	}
}
