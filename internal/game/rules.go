package game

import "math"

// —— 阵营规则（doctrine）——
//
// 舰船和飞机的差异是**数据**（content/units 里每张卡自带数值），但还有一类差异是
// "这个国家怎么打仗"：修复率、无线电衰减、机场雷达表、夜战加成、弹药消耗曲线、
// 雷击火控成熟度……这些不是某艘船或某种飞机的属性，而是国家的。
//
// 以前这类规则散在十几个文件里，各写一遍 `Doctrines[faction] == "japan"` 或者干脆
// `Faction == "B"`。两个国家时能跑，但加第三个国家就得翻遍代码；更糟的是
// 同一个规则在两条代码路径上各写一份（夜间 70% 摔毁只写在航母降落里，
// 机场降落那条漏了 20%）——抽象缺失就会这样漏。
//
// 现在统一解析成一份 FactionRules，调用点只读字段：规则怎么变都改这一个文件。
// 注意 doctrine 来自剧本（`doctrines: {"A":"us","B":"japan"}`），不能拿内部代号 A/B 当国家。
const (
	DoctrineUS    = "us"
	DoctrineJapan = "japan"
)

// FactionRules 是一个阵营在当前剧本、当前年份、当前天色下的规则参数。
//
// 和年份/天色有关的项在解析时就结算好（例如"1944 年前的雷达火控惩罚"），
// 调用点不需要再判一次年份。
type FactionRules struct {
	Doctrine string

	// RepairChance 是损伤算子回机库后每回合的修复概率（%）。
	RepairChance float64
	// RadioPenaltyPerHex 是无线电每 1hex 距离的成功率惩罚（%）。
	RadioPenaltyPerHex float64

	// AirfieldRadar 是机场对空雷达按距离 1..5 的探测率（%）。
	AirfieldRadar [5]float64
	// CapitalShipRadar 是航母/战列舰对空雷达按距离 1..5 的探测率（%）。
	CapitalShipRadar [5]float64
	// OtherRadar 是其余带雷达舰船的表；NoRadar 是没装雷达的单位 1hex 内的发现率。
	OtherRadar [5]float64
	NoRadar    [5]float64

	// AmmunitionBands 是战斗机弹药消耗曲线的三个兵力比拐点：
	// 超过 high 10%、超过 mid 25%、超过/达到 low 50%，否则 75%。
	AmmunitionBands [3]float64
	// AmmunitionLowInclusive 表示 50% 那一档的边界是"≥"还是">"：
	// 规则书里日军写成"兵力比≥1"，美军写成">1/2"，边界上差一档，这里照抄不合并。
	AmmunitionLowInclusive bool

	// ElitePilotBonus：开战初期精锐机组，1942 年及以前战斗机对空 +1、俯冲/鱼雷机对舰 +1。
	ElitePilotBonus int
	// EarlyWarReconBonus：同一批精锐机组在侦察上的发现率加成（%）。
	EarlyWarReconBonus float64
	// ReconBaseBonus：侦查看家的基础发现率加成（%）。美军无线电/雷达与情报体系更好，
	// 规则书给侦察基础值 +10%；日军为 0。
	ReconBaseBonus float64
	// AirRadarReconDetectBonus：本方对空雷达让敌方侦察机更容易被发现的加成（%）。
	AirRadarReconDetectBonus float64
	// ReconInCAPBonus：本方侦察机被敌方 CAP 盯上时的额外暴露概率（%）。
	ReconInCAPBonus float64
	// ShortRangeEngagePenalty：攻击队剩余航程不足以返航时的突袭成功率惩罚（%）。
	ShortRangeEngagePenalty float64

	// DeckDestroyChance：航母甲板被炸弹命中后彻底报废的概率（%）。
	DeckDestroyChance float64
	// EarlyRadarPenalty：雷达火控还不成熟的年份里，雷击命中率的惩罚（%）。
	EarlyRadarPenalty float64
	// HeavyAALateBonus：后期重型防空命中率加成（%）。
	HeavyAALateBonus float64
	// FleetLightAAMerge：航母能否并入同格舰队的最高轻防空。
	FleetLightAAMerge bool

	// 水面战先手（突袭）判定里的阵营加成，单位是点数。
	SurpriseEliteBonus   int
	SurpriseNightBonus   int
	SurpriseTorpedoBonus int
	SurpriseRadarBonus   int
	SurpriseYearBonus    int
}

// opposingFaction 返回对手阵营。规则里"敌方雷达/敌方 CAP"都得靠它，
// 不要在各处写 `enemy := "B"; if faction == "B" { enemy = "A" }`。
func opposingFaction(faction string) string {
	switch faction {
	case "A":
		return "B"
	case "B":
		return "A"
	}
	return ""
}

// doctrineOf 取阵营的 doctrine；剧本没配时返回空串（走中立规则）。
func doctrineOf(state State, faction string) string {
	if state.Doctrines == nil {
		return ""
	}
	return state.Doctrines[faction]
}

// rulesFor 解析某阵营在当前剧本/年份/天色下的规则参数。
func rulesFor(state State, faction string) FactionRules {
	return rulesForDoctrine(doctrineOf(state, faction), state.Year, state.Night)
}

// rulesForDoctrine 是规则表的唯一来源：国家差异和年份、天色的耦合都在这里结算。
func rulesForDoctrine(doctrine string, year int, night bool) FactionRules {
	// 默认（未配置 doctrine 的第三方/沙盘势力）走中立表：
	// 不做任何阵营加成，也不套用美日任何一方的特权。
	rules := FactionRules{
		Doctrine:           doctrine,
		RepairChance:       10,
		RadioPenaltyPerHex: 10,
		AirfieldRadar:      [5]float64{60, 40, 20, 0, 0},
		CapitalShipRadar:   [5]float64{60, 40, 20, 0, 0},
		OtherRadar:         [5]float64{60, 40, 20, 0, 0},
		NoRadar:            [5]float64{30, 0, 0, 0, 0},
		AmmunitionBands:    [3]float64{4, 2, 0.5},
		DeckDestroyChance:  1,
	}
	switch doctrine {
	case DoctrineJapan:
		rules.RepairChance = 3
		rules.RadioPenaltyPerHex = 20
		rules.AirfieldRadar = [5]float64{80, 60, 40, 20, 0}
		rules.CapitalShipRadar = [5]float64{85, 65, 45, 25, 5}
		rules.AmmunitionBands = [3]float64{5, 3, 1}
		rules.AmmunitionLowInclusive = true
		rules.ShortRangeEngagePenalty = 50
		rules.DeckDestroyChance = 10
		rules.SurpriseTorpedoBonus = 1
		if night {
			rules.SurpriseNightBonus = 1
		}
		if year > 0 && year <= 1942 {
			// 开战初期的精锐机组：机型本身的数值不动，出手时给一次加成。
			rules.ElitePilotBonus = 1
			rules.EarlyWarReconBonus = 10
			rules.SurpriseEliteBonus = 1
		}
	case DoctrineUS, "usa":
		rules.RepairChance = 10
		rules.RadioPenaltyPerHex = 10
		rules.AirfieldRadar = [5]float64{100, 80, 60, 40, 20}
		rules.CapitalShipRadar = [5]float64{90, 70, 50, 30, 10}
		rules.AmmunitionBands = [3]float64{4, 2, 0.5}
		rules.AirRadarReconDetectBonus = 20
		rules.ReconInCAPBonus = 20
		// 美军侦察基础值 +10%：舰载机、陆基侦察机、水面舰队瞭望三个入口共用这一个字段。
		rules.ReconBaseBonus = 10
		rules.DeckDestroyChance = 1
		rules.FleetLightAAMerge = true
		rules.SurpriseRadarBonus = 1
		if year > 0 && year < 1944 {
			rules.EarlyRadarPenalty = 20
		}
		if year >= 1943 {
			rules.SurpriseYearBonus = 1
			rules.HeavyAALateBonus = 2
		}
	}
	return rules
}

// AmmunitionDepletionChance 是战斗机弹药耗尽的概率（%）：双方兵力比越悬殊越容易打空。
func (r FactionRules) AmmunitionDepletionChance(own, opposing int) float64 {
	if opposing < 1 {
		opposing = 1
	}
	ratio := float64(own) / float64(opposing)
	high, mid, low := r.AmmunitionBands[0], r.AmmunitionBands[1], r.AmmunitionBands[2]
	inLow := ratio > low
	if r.AmmunitionLowInclusive {
		inLow = ratio >= low
	}
	switch {
	case ratio > high:
		return 10
	case ratio > mid:
		return 25
	case inLow:
		return 50
	default:
		return 75
	}
}

// AirfieldRadarChance 返回机场对空雷达在某个距离上的探测率（%）。
func (r FactionRules) AirfieldRadarChance(distance int) float64 {
	if distance < 1 || distance > 5 {
		return 0
	}
	value := r.AirfieldRadar[distance-1]
	return math.Min(100, math.Max(0, value))
}
