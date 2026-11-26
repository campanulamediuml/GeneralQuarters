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

// pilotSkillBonus 是熟练度对卡面数值的修正：
// 新手 -1、普通 0、老兵 +1、王牌 +2。空战值最低压到 1，避免新手把自己的空战值打到 0 以下。
func pilotSkillBonus(skill string) int {
	switch NormalizePilotSkill(skill) {
	case PilotRookie:
		return -1
	case PilotVeteran:
		return 1
	case PilotAce:
		return 2
	}
	return 0
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

// applyPilotSkill 把熟练度折进卡面数值：所有机种加对空，俯冲/鱼雷机再加对舰。
// 轰炸机对地值不动——对地空袭的命中表已经在规则书里按"机场防空/混乱"调过一遍，
// 再叠熟练度会让陆攻一轮抹平机场。
func applyPilotSkill(skill string, kind string, airCombat, antiShip int) (int, int) {
	bonus := pilotSkillBonus(skill)
	if bonus == 0 {
		return airCombat, antiShip
	}
	airCombat += bonus
	if airCombat < 1 {
		airCombat = 1
	}
	if kind == "dive_bomber" || kind == "torpedo_bomber" {
		antiShip += bonus
		if antiShip < 0 {
			antiShip = 0
		}
	}
	return airCombat, antiShip
}
