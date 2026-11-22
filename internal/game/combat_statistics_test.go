package game

import (
	"math"
	"testing"
)

// —— 战斗结算的统计验证 ——
//
// 单元测试只能证明"代码走到了"，证明不了"概率算对了"。这里用大样本抽样，
// 把实测频率和规则书给出的公式对比，偏差超过置信区间就报错。

// sampled 返回 n 次采样中事件发生的频率。
func sampled(n int, trial func(*State) bool) float64 {
	state := State{RNGState: 20240401}
	hits := 0
	for i := 0; i < n; i++ {
		if trial(&state) {
			hits++
		}
	}
	return float64(hits) / float64(n)
}

// withinTolerance 判断实测频率是否落在期望值 ±5 个标准差的区间内。
func withinTolerance(t *testing.T, name string, got, want float64, n int) {
	t.Helper()
	if want <= 0 || want >= 1 {
		if math.Abs(got-want) > 0.01 {
			t.Fatalf("%s：期望 %.3f，实测 %.3f", name, want, got)
		}
		return
	}
	sigma := math.Sqrt(want * (1 - want) / float64(n))
	if math.Abs(got-want) > 5*sigma+0.01 {
		t.Fatalf("%s：期望 %.3f，实测 %.3f（%d 次，5σ=%.3f）", name, want, got, n, 5*sigma)
	}
}

// 空战命中率表（规则书 742–746 行）要逐档对上。
func TestAirCombatChanceTableStatistics(t *testing.T) {
	cases := []struct {
		diff int
		want float64
	}{
		{-4, 0.05}, {-3, 0.10}, {-2, 0.15}, {-1, 0.20}, {0, 0.25},
		{1, 0.35}, {2, 0.45}, {3, 0.55}, {4, 0.65}, {5, 0.75}, {6, 0.85}, {7, 0.95},
	}
	for _, c := range cases {
		state := State{RNGState: uint64(1000 + c.diff)}
		attacker := &AirUnit{ID: "a", Faction: "A", Kind: "fighter", AirCombat: 5}
		target := &AirUnit{ID: "d", Faction: "B", Kind: "fighter", AirCombat: 5 - c.diff}
		state.Aircraft = map[string]*AirUnit{"a": attacker, "d": target}
		n := 4000
		hit := 0
		for i := 0; i < n; i++ {
			state.Aircraft["d"] = &AirUnit{ID: "d", Faction: "B", Kind: "fighter", AirCombat: target.AirCombat}
			state.Aircraft["a"] = &AirUnit{ID: "a", Faction: "A", Kind: "fighter", AirCombat: 5}
			report := &AirCombatReport{}
			events := performAirAttack(&state, state.Aircraft["a"], []*AirUnit{state.Aircraft["d"]}, 0, map[string]bool{}, report, true)
			if len(events) > 0 {
				hit++
			}
		}
		withinTolerance(t, "空战差值 "+itoa(c.diff), float64(hit)/float64(n), c.want, n)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	if neg {
		return "-" + digits
	}
	return digits
}

// 防空每点 3% 命中（规则书 687 行）。
func TestAAHitRateStatistics(t *testing.T) {
	if got := aaHitChance(State{}, "0101", true, "B"); got != 3 {
		t.Fatalf("基础防空命中率应为 3%%，实际 %.1f", got)
	}
	state := State{RNGState: 777, Map: MapDefinition{}, Year: 1944, Doctrines: map[string]string{"A": "us"}}
	if got := aaHitChance(state, "0101", true, "A"); got != 5 {
		t.Fatalf("1943 年后的美军重型防空应为 3+2=5%%，实际 %.1f", got)
	}
	state.Map = MapDefinition{Cells: []MapCell{{Coord: "0101", Weather: "storm"}}}
	if got := aaHitChance(state, "0101", true, "A"); got != 3 {
		t.Fatalf("风暴下重型防空应为 5-2=3%%，实际 %.1f", got)
	}
	if got := aaHitChance(state, "0101", false, "A"); got != 1 {
		t.Fatalf("风暴下轻型防空应为 3-2=1%%，实际 %.1f", got)
	}
}

// 对舰投弹命中率：基础 25%，逐项修正要真的生效。
func TestStrikeHitChanceModifiers(t *testing.T) {
	state := State{Map: MapDefinition{}}
	target := &Unit{ID: "t", Type: "BB", FleetID: "f", Coord: "0101", Card: ShipCard{SpeedClass: "normal", LightAA: 14}}
	bomber := &AirUnit{ID: "b", Kind: "dive_bomber"}
	// 25% - 轻防空(14/7=2 档 → -10) = 15
	if got := strikeHitChance(state, bomber, target, false, true); math.Abs(got-15) > 0.01 {
		t.Fatalf("轻防空两档应 -10%%：期望 15，实际 %.1f", got)
	}
	// 无 CAP +5 → 20
	if got := strikeHitChance(state, bomber, target, false, false); math.Abs(got-20) > 0.01 {
		t.Fatalf("无 CAP 应 +5%%：期望 20，实际 %.1f", got)
	}
	// 混编 +10 → 30
	if got := strikeHitChance(state, bomber, target, true, false); math.Abs(got-30) > 0.01 {
		t.Fatalf("混编应 +10%%：期望 30，实际 %.1f", got)
	}
	// 慢速 +5 → 25，驱逐舰 -5 → 20
	target.Card.SpeedClass = "slow"
	if got := strikeHitChance(state, bomber, target, false, false); math.Abs(got-25) > 0.01 {
		t.Fatalf("慢速应 +5%%：期望 25，实际 %.1f", got)
	}
	target.Card.SpeedClass = "normal"
	target.Type = "DD"
	if got := strikeHitChance(state, bomber, target, false, false); math.Abs(got-15) > 0.01 {
		t.Fatalf("驱逐舰应 -5%%：期望 15，实际 %.1f", got)
	}
}

// 炮战修正（规则书 305–315 行）：基础 25%，逐条核对。
// 注意规则书没有"快船更难命中"，所以不加这个修正——正是这条测试帮我确认了这点。
func TestGunneryChanceModifiers(t *testing.T) {
	state := State{Map: MapDefinition{}, Doctrines: map[string]string{"A": "us", "B": "japan"}}
	attacker := &Unit{ID: "a", Faction: "A", Type: "BB", FleetID: "fa", Coord: "0101", Card: ShipCard{Gunnery: 17}}
	bb := &Unit{ID: "b", Faction: "B", Type: "BB", FleetID: "fb", Coord: "0101", Card: ShipCard{SpeedClass: "normal"}}
	dd := &Unit{ID: "d", Faction: "B", Type: "DD", FleetID: "fb", Coord: "0101", Card: ShipCard{SpeedClass: "normal"}}
	if got := gunneryChance(state, attacker, bb, []Unit{*bb}); math.Abs(got-25) > 0.01 {
		t.Fatalf("BB 打 BB 基础 25%%，实际 %.1f", got)
	}
	// 对海雷达 +10
	attacker.Card.SurfaceRadar = true
	if got := gunneryChance(state, attacker, bb, []Unit{*bb}); math.Abs(got-35) > 0.01 {
		t.Fatalf("对海雷达应 +10%%：期望 35，实际 %.1f", got)
	}
	attacker.Card.SurfaceRadar = false
	// 目标燃烧 +10
	burning := *bb
	burning.Damage = &ShipDamage{Fire: "burning"}
	if got := gunneryChance(state, attacker, &burning, []Unit{burning}); math.Abs(got-35) > 0.01 {
		t.Fatalf("目标燃烧应 +10%%：期望 35，实际 %.1f", got)
	}
	// 表里 BB=行0 在最上、DD=行5 在最下。规则书惩罚的是"靠下的单位攻击上方"，
	// 所以 BB 打 DD（目标在下方）没有惩罚，仍是 25%。
	if got := gunneryChance(state, attacker, dd, []Unit{*dd}); math.Abs(got-25) > 0.01 {
		t.Fatalf("以大打小不应有惩罚：期望 25，实际 %.1f", got)
	}
	// 反过来：DD(行5) 打 BB(行0)，目标在攻击者上方 5 行 → 螳臂当车 -15。
	ddAttacker := &Unit{ID: "d2", Faction: "A", Type: "DD", FleetID: "fa2", Coord: "0101", Card: ShipCard{Gunnery: 7}}
	if got := gunneryChance(state, ddAttacker, bb, []Unit{*bb}); math.Abs(got-10) > 0.01 {
		t.Fatalf("DD 打 BB 应 -15%%：期望 10，实际 %.1f", got)
	}
	// CA(行3) 打 BB(行0)：上方 3 行 → 力不从心 -10。
	caAttacker := &Unit{ID: "c2", Faction: "A", Type: "CA", FleetID: "fa3", Coord: "0101", Card: ShipCard{Gunnery: 6}}
	if got := gunneryChance(state, caAttacker, bb, []Unit{*bb}); math.Abs(got-15) > 0.01 {
		t.Fatalf("CA 打 BB 应 -10%%：期望 15，实际 %.1f", got)
	}
	// CL(行4) 打 CA(行3)：上方 1 行 → 以小博大 -5。
	ca := &Unit{ID: "ca", Faction: "B", Type: "CA", FleetID: "fb2", Coord: "0101", Card: ShipCard{SpeedClass: "normal"}}
	clAttacker := &Unit{ID: "l2", Faction: "A", Type: "CL", FleetID: "fa4", Coord: "0101", Card: ShipCard{Gunnery: 5}}
	if got := gunneryChance(state, clAttacker, ca, []Unit{*ca}); math.Abs(got-20) > 0.01 {
		t.Fatalf("CL 打 CA 应 -5%%：期望 20，实际 %.1f", got)
	}
	// 降雨 -5
	state.Map = MapDefinition{Cells: []MapCell{{Coord: "0101", Weather: "rain"}}}
	if got := gunneryChance(state, attacker, bb, []Unit{*bb}); math.Abs(got-20) > 0.01 {
		t.Fatalf("降雨应 -5%%：期望 20，实际 %.1f", got)
	}
}

// 鱼雷基础 25%，快艇 -10、慢船 +10。
func TestTorpedoChanceModifiers(t *testing.T) {
	state := State{Map: MapDefinition{}, Year: 1944, Doctrines: map[string]string{"A": "us"}}
	attacker := &Unit{ID: "a", Faction: "A", Type: "DD", Coord: "0101"}
	slow := &Unit{ID: "s", Type: "BB", FleetID: "f", Coord: "0101", Card: ShipCard{SpeedClass: "slow"}}
	fast := &Unit{ID: "f", Type: "BB", FleetID: "f", Coord: "0101", Card: ShipCard{SpeedClass: "fast"}}
	slowChance := torpedoChance(state, attacker, slow)
	fastChance := torpedoChance(state, attacker, fast)
	if slowChance <= fastChance {
		t.Fatalf("慢船应更好打：慢 %.1f 快 %.1f", slowChance, fastChance)
	}
}
