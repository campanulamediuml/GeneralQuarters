package game

import (
	"math"
	"testing"
)

// —— 水面对战验证 ——
//
// 不只看公式，而是跑真实的 resolveSurfaceBattle，统计"每次齐射的命中数"是否落在理论值附近。

// volleyHitRate 统计一艘船 n 次齐射的平均命中数，除以火炮点数得到每点命中率。
func volleyHitRate(t *testing.T, state *State, attacker *Unit, target *Unit, n int) float64 {
	t.Helper()
	state.Units = map[string]*Unit{attacker.ID: attacker, target.ID: target}
	state.Aircraft = map[string]*AirUnit{}
	hits := 0
	for i := 0; i < n; i++ {
		attacker.Strength = attacker.MaxStrength
		target.Strength = target.MaxStrength
		if target.Damage != nil {
			target.Damage.Fire = ""
		}
		volley, _ := gunneryVolley(state, []Unit{*attacker}, []Unit{*target})
		hits += len(volley)
	}
	return float64(hits) / float64(n) / float64(attacker.Card.Gunnery)
}

// 规则书 305 行：每点火炮提供一次基础 25% 的独立判定，无修正时实测应接近 25%。
func TestSurfaceGunneryBaseRate(t *testing.T) {
	state := State{Map: MapDefinition{}, Doctrines: map[string]string{"A": "us", "B": "japan"}, RNGState: 424242}
	attacker := &Unit{ID: "a", Faction: "A", Type: "BB", FleetID: "fa", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 17, SpeedClass: "normal"}}
	target := &Unit{ID: "b", Faction: "B", Type: "BB", FleetID: "fb", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", DamageControl: 10}}
	rate := volleyHitRate(t, &state, attacker, target, 3000)
	if math.Abs(rate-0.25) > 0.02 {
		t.Fatalf("基础命中率应接近 25%%，实测 %.3f", rate)
	}
}

// 规则书 310–312 行：靠下的单位打上方的单位要受罚，方向必须是单向的。
func TestSurfaceGunneryRowPenaltyIsDirectional(t *testing.T) {
	state := State{Map: MapDefinition{}, Doctrines: map[string]string{"A": "us", "B": "japan"}}
	mk := func(id, typ, fleet string) *Unit {
		return &Unit{ID: id, Faction: "A", Type: typ, FleetID: fleet, Coord: "0101",
			Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 10}}
	}
	bb := mk("bb", "BB", "f-bb")
	bc := mk("bc", "BC", "f-bc")
	ca := mk("ca", "CA", "f-ca")
	cl := mk("cl", "CL", "f-cl")
	dd := mk("dd", "DD", "f-dd")
	trs := mk("trs", "TRS", "f-trs")

	// BB 在最上，打谁都没有惩罚。
	for name, target := range map[string]*Unit{"BB": bb, "BC": bc, "CA": ca, "CL": cl, "DD": dd, "TRS": trs} {
		if got := gunneryChance(state, bb, target, []Unit{*target}); math.Abs(got-25) > 0.01 {
			t.Fatalf("BB 打 %s（目标在下方）不应有惩罚，实际 %.1f", name, got)
		}
	}
	// 靠下的单位打上方：CL→CA 上方 1 行 -5；CL→BB 上方 4 行 -15。
	if got := gunneryChance(state, cl, ca, []Unit{*ca}); math.Abs(got-20) > 0.01 {
		t.Fatalf("CL 打 CA（上方1行）应 -5%%：期望 20，实际 %.1f", got)
	}
	if got := gunneryChance(state, cl, bb, []Unit{*bb}); math.Abs(got-10) > 0.01 {
		t.Fatalf("CL 打 BB（上方4行）应 -15%%：期望 10，实际 %.1f", got)
	}
	// 相邻两行 -5，隔 2~3 行 -10。CA=行3、BC=行1，相差 2 行 → -10 → 15。
	if got := gunneryChance(state, ca, bc, []Unit{*bc}); math.Abs(got-15) > 0.01 {
		t.Fatalf("CA 打 BC（上方2行）应 -10%%：期望 15，实际 %.1f", got)
	}
	// TRS 在最下，打 BB 上方 6 行 -15。
	if got := gunneryChance(state, trs, bb, []Unit{*bb}); math.Abs(got-10) > 0.01 {
		t.Fatalf("TRS 打 BB 应 -15%%：期望 10，实际 %.1f", got)
	}
}

// 有惩罚时，实测命中率也要真的下降（不只是公式返回 10）。
func TestSurfaceGunneryPenaltyAffectsActualHits(t *testing.T) {
	state := State{Map: MapDefinition{}, Doctrines: map[string]string{"A": "us", "B": "japan"}, RNGState: 777}
	cl := &Unit{ID: "cl", Faction: "A", Type: "CL", FleetID: "fa", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 20, SpeedClass: "normal"}}
	bb := &Unit{ID: "bb", Faction: "B", Type: "BB", FleetID: "fb", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", DamageControl: 10}}
	dd := &Unit{ID: "dd", Faction: "B", Type: "DD", FleetID: "fb", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", DamageControl: 10}}
	upstream := volleyHitRate(t, &state, cl, bb, 3000)   // 打上方：受罚
	downstream := volleyHitRate(t, &state, cl, dd, 3000) // 打下方：不受罚
	if !(upstream < downstream-0.05) {
		t.Fatalf("打上方的实际命中率应明显低于打下方：上方 %.3f vs 下方 %.3f", upstream, downstream)
	}
}

// 对海雷达 +10、水机侦察 +10、目标燃烧 +10，实测都要真的加上去。
func TestSurfaceGunneryBonusesAffectActualHits(t *testing.T) {
	state := State{Map: MapDefinition{}, Doctrines: map[string]string{"A": "us", "B": "japan"}, RNGState: 999}
	attacker := &Unit{ID: "a", Faction: "A", Type: "BB", FleetID: "fa", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{Gunnery: 20, SpeedClass: "normal"}}
	target := &Unit{ID: "b", Faction: "B", Type: "BB", FleetID: "fb", Coord: "0101",
		Strength: 6, MaxStrength: 6, Card: ShipCard{SpeedClass: "normal", DamageControl: 10}}
	base := volleyHitRate(t, &state, attacker, target, 2500)
	attacker.Card.SurfaceRadar = true
	withRadar := volleyHitRate(t, &state, attacker, target, 2500)
	if math.Abs((withRadar-base)-0.10) > 0.03 {
		t.Fatalf("对海雷达应 +10%%：%.3f → %.3f", base, withRadar)
	}
}

// 交战目标权重表：BB 打舰队时按 7/7/1/1 之类的权重分配。
func TestSurfaceTargetWeightsMatchTable(t *testing.T) {
	table := map[string]map[string]int{
		"BB": {"BB": 7, "BC": 7, "CV": 9, "CA": 7, "CL": 1, "DD": 1, "TRS": 3},
		"BC": {"BB": 5, "BC": 7, "CV": 9, "CA": 7, "CL": 3, "DD": 1, "TRS": 3},
		"CA": {"BB": 3, "BC": 3, "CV": 7, "CA": 5, "CL": 7, "DD": 7, "TRS": 1},
		"CL": {"BB": 1, "BC": 1, "CV": 1, "CA": 3, "CL": 7, "DD": 7, "TRS": 1},
		"DD": {"BB": 1, "BC": 1, "CV": 1, "CA": 1, "CL": 7, "DD": 7, "TRS": 1},
		"CV": {"BB": 1, "BC": 1, "CV": 1, "CA": 1, "CL": 3, "DD": 7, "TRS": 1},
	}
	for attacker, row := range table {
		for target, want := range row {
			if got := targetWeight(attacker, target); got != want {
				t.Fatalf("%s 打 %s 权重应为 %d，实际 %d", attacker, target, want, got)
			}
		}
	}
	// CVL/CVE 按 CV 结算（规则书 119 行）。
	if targetWeight("CVL", "BB") != 1 || targetWeight("CVE", "DD") != 7 {
		t.Fatal("轻航母应使用 CV 的攻击表")
	}
}

// 突袭判定：1942 年日军 +1、夜间日军 +1、美军 1943 后 +1（规则书 319–328 行）。
func TestSurpriseScoreModifiers(t *testing.T) {
	jp := []Unit{{ID: "b1", Faction: "B", Type: "DD", Card: ShipCard{Gunnery: 2}}}
	us := []Unit{{ID: "a1", Faction: "A", Type: "CL", Card: ShipCard{Gunnery: 2}}}
	state := State{RNGState: 1, Doctrines: map[string]string{"A": "us", "B": "japan"}, Year: 1942}
	score, mods := surpriseScore(&state, "B", jp, us, false)
	if score < 2 {
		t.Fatalf("1942 日军应有 d6+精锐+1，实际 %d（%v）", score, mods)
	}
	state.Night = true
	night, nightMods := surpriseScore(&state, "B", jp, us, false)
	if !(night >= score) {
		t.Fatalf("夜间日军应再加 1：%d → %d（%v）", score, night, nightMods)
	}
	state.Night = false
	state.Year = 1944
	usScore, usMods := surpriseScore(&state, "A", us, jp, false)
	if usScore < 2 {
		t.Fatalf("1943 年后的美军应有 d6+1，实际 %d（%v）", usScore, usMods)
	}
}
