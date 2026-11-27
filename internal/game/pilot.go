package game

import "strings"

// —— 飞行员熟练度 ——
//
// 机型卡讲的是"这架飞机能做什么"，熟练度讲的是"坐在里面的人什么水平"。
// 1942 年的精锐机组走了 FactionRules.ElitePilotBonus 那条国家+年份的规则；
// 1944 年日军飞行队被打空、补充兵只飞过几十小时，这层差距必须按【机队】写，
// 不能让同一架 A6M5 在航母上是新手、在拉包尔又变回精锐。
//
// 熟练度直接改写算子的对空/对舰数值（和精英机组加成同一条路径），
// 不新增一套命中修正——空战查表和投弹命中率都只认卡面数值，这样"老手"在
// 空战和投弹两条线上都是同一个加成，规则书里查表也只需要加一列熟练度。
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
// 新手 0 = 劣势（2 骰取劣）、普通 1 = 单骰、老兵 2 = 2 骰取优、王牌 3 = 3 骰取优。
//
// 熟练度不再加减卡面数值：机型值定死在卡上，飞行员之间的差别只体现在 DC 这一掷——
// 通过就给这一次出手 +1，没过就没有。和机库优势用的是同一套"骰子优/劣"语言。
func PilotDiceAdvantage(skill string) int {
	switch NormalizePilotSkill(skill) {
	case PilotRookie:
		return 0
	case PilotVeteran:
		return 2
	case PilotAce:
		return 3
	}
	return 1
}

// PilotAdvantageLabel 是给界面看的档位名：劣势 / 普通 / 2 骰优势 / 3 骰优势。
func PilotAdvantageLabel(skill string) string {
	switch PilotDiceAdvantage(skill) {
	case 0:
		return "劣势"
	case 2:
		return "2 骰优势"
	case 3:
		return "3 骰优势"
	}
	return "普通"
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


