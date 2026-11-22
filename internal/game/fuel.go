package game

// —— 航程构成（规则书 43–83 行）——
//
// 一次出击的航程由三部分叠成，消耗顺序固定：
//   1. 副油箱（External）：默认挂载，额外 +30% 航程；空战后抛弃，未消耗的里程直接作废。
//   2. 机内油（Internal）：机身自带航程。
//   3. 备用油（Reserve）：内油烧尽后还能飞 4 hex / 在空中停留 4 回合。
//
// 早期实现只有一个 RemainingRange 整数，导致"投掉副油箱后还剩多少"这类问题
// 只能靠调用方自己推演。把燃料拆成三部分后，这些判断变成结构自带的性质。

// ReserveRange 是内油耗尽后的备用航程（规则书 69 行：4hex 或空中停留 4 回合）。
const ReserveRange = 4

// Fuel 是一架飞机的燃料账本。零值表示没有燃料。
type Fuel struct {
	Tank     int `json:"tank,omitempty"`     // 副油箱里程（优先消耗）
	Internal int `json:"internal,omitempty"` // 机内油里程
	Reserve  int `json:"reserve,omitempty"`  // 备用航程
}

// Total 返回当前还能飞的里程（含备用油）。
func (f Fuel) Total() int {
	return f.Tank + f.Internal + f.Reserve
}

// Available 返回不含备用油的可用里程：这是正常规划航线的上限。
func (f Fuel) Available() int {
	return f.Tank + f.Internal
}

// Consume 按副油箱 → 机内 → 备用的顺序扣掉 n 点里程，返回实际扣除量。
// 剩余里程可以为负（规则书 71 行"使用备用航程后剩余航程记为负数"）。
func (f *Fuel) Consume(n int) int {
	if n <= 0 {
		return 0
	}
	remaining := n
	// 副油箱优先（规则书 49 行：副油箱航程优先消耗），然后机内油。
	for _, bucket := range []*int{&f.Tank, &f.Internal} {
		if remaining <= 0 {
			break
		}
		if *bucket <= 0 {
			continue
		}
		take := remaining
		if take > *bucket {
			take = *bucket
		}
		*bucket -= take
		remaining -= take
	}
	if remaining <= 0 {
		return n
	}
	// 主油耗尽：动用备用油，备用油扣穿后变负数表示透支
	// （规则书 72-76 行的"剩余航程 -1 ~ -5"就是这么来的）。
	f.Reserve -= remaining
	return n
}

// DropTank 抛弃副油箱，返回被浪费掉的里程。已经投过则返回 0。
func (f *Fuel) DropTank() int {
	wasted := f.Tank
	f.Tank = 0
	return wasted
}

// NewFuel 按基础航程建立一个满油账本。
//
// 规则书 47 行："副油箱提供额外 30% 航程，例如默认航程 9hex 的飞机，悬挂副油箱后航程为 12hex"。
// 这里的语义是"总航程 = base × 1.3"，多出来的部分由副油箱承担，而不是在 base 之上再加 30%。
// 所以：Tank = round(base×1.3) - base，Internal = base。
// 往回加的副油箱里程单独记账，抛弃副油箱时只损失"还没烧掉的那部分"。
func NewFuel(baseRange int, external bool) Fuel {
	if baseRange < 0 {
		baseRange = 0
	}
	fuel := Fuel{Internal: baseRange, Reserve: ReserveRange}
	if external {
		total := (baseRange*13 + 5) / 10 // round(base × 1.3)
		if extra := total - baseRange; extra > 0 {
			fuel.Tank = extra
		}
	}
	return fuel
}

// Range 返回编队当前的剩余航程（不含备用油）。
//
// 规则书 69 行："航程降至 0 时，飞机还能依靠备用燃料飞行 4hex"，
// 所以备用油不属于正常航程，单独用 ReserveLeft() 查询。
// 主油耗尽后会开始动用备用油，此时 Range() 变负（规则书 71-76 行）。
func (a *AirUnit) Range() int {
	if a == nil {
		return 0
	}
	if a.Fuel == (Fuel{}) {
		return a.RemainingRange
	}
	main := a.Fuel.Tank + a.Fuel.Internal
	if main > 0 {
		return main
	}
	// 主油已尽：剩余航程由备用油的余额（可能为负）表示。
	return a.Fuel.Reserve
}

// ReserveLeft 返回还剩多少备用油（规则书 69 行）。
func (a *AirUnit) ReserveLeft() int {
	if a == nil || a.Fuel == (Fuel{}) {
		return 0
	}
	return a.Fuel.Reserve
}

// SetRange 直接设定剩余航程，同步旧字段。
func (a *AirUnit) SetRange(value int) {
	if a == nil {
		return
	}
	a.Fuel = Fuel{Internal: value}
	a.RemainingRange = value
}

// Spend 扣掉行程里程并同步旧字段，返回实际消耗。
func (a *AirUnit) Spend(hexes int) int {
	if a == nil || hexes <= 0 {
		return 0
	}
	if a.Fuel == (Fuel{}) {
		a.Fuel = Fuel{Internal: a.RemainingRange}
	}
	a.Fuel.Consume(hexes)
	a.RemainingRange = a.Range()
	return hexes
}

// CanFly 判断剩余航程（不含备用油）是否还能飞满 hexes 格。
func (a *AirUnit) CanFly(hexes int) bool {
	return a != nil && a.Range() >= hexes
}

// —— 机场飞机的燃料账本：与舰载机共享 Fuel 结构，接口保持一致 ——

// Range 返回机场飞机当前的剩余航程（不含备用油）。
func (a *AirfieldAirUnit) Range() int {
	if a == nil {
		return 0
	}
	if a.Fuel == (Fuel{}) {
		return a.RemainingRange
	}
	main := a.Fuel.Tank + a.Fuel.Internal
	if main > 0 {
		return main
	}
	return a.Fuel.Reserve
}

// ReserveLeft 返回还剩多少备用油。
func (a *AirfieldAirUnit) ReserveLeft() int {
	if a == nil || a.Fuel == (Fuel{}) {
		return 0
	}
	return a.Fuel.Reserve
}

// SetRange 设定剩余航程，同步旧字段。
func (a *AirfieldAirUnit) SetRange(value int) {
	if a == nil {
		return
	}
	a.Fuel = Fuel{Internal: value}
	a.RemainingRange = value
}

// Spend 扣掉行程里程，返回实际消耗。
func (a *AirfieldAirUnit) Spend(hexes int) int {
	if a == nil || hexes <= 0 {
		return 0
	}
	if a.Fuel == (Fuel{}) {
		a.Fuel = Fuel{Internal: a.RemainingRange}
	}
	a.Fuel.Consume(hexes)
	a.RemainingRange = a.Range()
	return hexes
}

// CanFly 判断是否还能飞满 hexes 格。
func (a *AirfieldAirUnit) CanFly(hexes int) bool {
	return a != nil && a.Range() >= hexes
}
