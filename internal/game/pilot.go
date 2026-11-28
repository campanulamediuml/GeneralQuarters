package game

import "strings"

// —— 飞行员熟练度 ——
//
// 机型卡讲的是"这架飞机能做什么"，熟练度讲的是"坐在里面的人什么水平"。
// 熟练度不改写任何卡面数值：空战查表和投弹命中率只认机型值，人的差别全在
// DC 临场发挥这一掷上——过了就给这一次出手 +1，没过按原值打。
//
// 1942 年及以前日军的"精锐机组"（FactionRules.ElitePilotBonus）也走这条路径：
// 把掷骰档位升一档，而不是往对空/对舰值上加 1。1944 年日军飞行队被打空、
// 补充兵只飞过几十小时，这层差距按【机队】写，靠剧本的 skill 字段体现。
const (
	PilotRookie  = "rookie"
	PilotRegular = "regular"
	PilotVeteran = "veteran"
	PilotAce     = "ace"
)

// NormalizePilotSkill 把剧本/老存档里的写法收敛到四档，认不出来的一律按普通。
func NormalizePilotSkill(skill string) string {
	switch strings.ToLower(strings.TrimSpace(skill)) {
	case PilotRookie, "green", "新手":
		return PilotRookie
	case PilotVeteran, "老兵":
		return PilotVeteran
	case PilotAce, "王牌":
		return PilotAce
	}
	return PilotRegular
}

// PilotSkillLabel 是给界面/战报看的名字。
func PilotSkillLabel(skill string) string {
	switch NormalizePilotSkill(skill) {
	case PilotRookie:
		return "新手"
	case PilotVeteran:
		return "老兵"
	case PilotAce:
		return "王牌"
	}
	return "普通"
}

// PilotDiceAdvantage 返回熟练度在 DC 判定上的骰子档位：
// 新手 -1 = 无加成、普通 0 = 2 骰取劣（25%）、老兵 1 = 单骰（50%）、王牌 2 = 2 骰取优（75%）。
//
// 熟练度不再加减卡面数值：机型值定死在卡上，飞行员之间的差别只体现在 DC 这一掷——
// 通过就给这一次出手 +1，没过就没有。和机库优势用的是同一套"骰子优/劣"语言。
func PilotDiceAdvantage(skill string) int {
	switch NormalizePilotSkill(skill) {
	case PilotRookie:
		return -1
	case PilotRegular:
		return 0
	case PilotAce:
		return 2
	}
	return 1
}

// PilotAdvantageLabel 是给界面看的档位名：无加成 / 2 骰劣势 / 单骰 / 2 骰优势。
func PilotAdvantageLabel(skill string) string {
	switch PilotDiceAdvantage(skill) {
	case -1:
		return "无加成"
	case 0:
		return "2 骰劣势"
	case 2:
		return "2 骰优势"
	}
	return "单骰"
}

// elitePilotSkill 把"开战初期精锐机组"折进熟练度档位：1942 年及以前日军机组
// 掷 DC 时升一档（新手→普通、普通→老兵、老兵→王牌、王牌不变），卡面数值一律不动。
// 这是"国家 + 年份"的规则，不是机型属性：1944 年的同一架 A6M2 不带这层加成。
func elitePilotSkill(state State, faction, skill string) string {
	normalized := NormalizePilotSkill(skill)
	if rulesFor(state, faction).ElitePilotBonus == 0 {
		return normalized
	}
	switch normalized {
	case PilotRookie:
		return PilotRegular
	case PilotRegular:
		return PilotVeteran
	}
	return PilotAce
}

// pilotSkillFromID 从机场算子的 ID 里读熟练度（例如 af-b-saipan-a6m5-rookie-1）。
// 机场的 aircraft 列表只是一串 ID，不像舰载机部署那样有结构化字段；
// 与其为地面航空再加一层 schema，不如让 ID 自己带段——解析器本来就按 "-" 找机种定义。
func pilotSkillFromID(id string) string {
	for _, segment := range strings.Split(strings.ToLower(id), "-") {
		switch segment {
		case PilotRookie, PilotVeteran, PilotAce:
			return segment
		}
	}
	return PilotRegular
}
