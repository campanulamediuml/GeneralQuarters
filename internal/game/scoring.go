package game

import "fmt"

// —— 战果计分 ——
//
// 剧本用 kind="score" 的目标声明"按击沉战果分排名"：一艘航母的价值不是拍脑袋定的常数，
// 而是它机库能装多少架——打掉翔鹤和打掉一艘护航航母不该是一个价钱。
// 其余舰种按重量级折算：战列舰/战列巡洋舰 6、重巡 3、轻巡 2、驱逐 1、运输船 1；
// 每摧毁一个航空算子再加 0.25 分，这样"打光对方的飞机"和"打沉对方的船"都能计到账。
// 航母还有一条"打瘫不算沉"的计分：飞行甲板被摧毁、结构损失超过 80%、或失去动力，
// 按机库容量的一半记一次，沉没之后改按整艘算（不重复领）。
// 沉没记录里的 Type 与 Card 是沉没那一刻的快照，所以历史战果不会被后来的改卡影响。
const (
	scoreBattleship   = 6
	scoreHeavyCruiser = 3
	scoreLightCruiser = 2
	scoreDestroyer    = 1
	scoreTransport    = 1
	// scorePerAircraft 是一个航空算子的战果分。
	scorePerAircraft = 0.25
)

// ShipVictoryPoints 把一艘沉没舰船折算成战果分：航母类按机库容量，其余按舰种。
func ShipVictoryPoints(shipType string, card ShipCard) int {
	switch upperType(shipType) {
	case "CV", "CVL", "CVE":
		if card.HangarCapacity > 0 {
			return card.HangarCapacity
		}
		// 老存档/简化卡可能没写机库容量：至少给 1 分，别让一艘航母白沉。
		return 1
	case "BB", "BC":
		return scoreBattleship
	case "CA":
		return scoreHeavyCruiser
	case "CL":
		return scoreLightCruiser
	case "DD":
		return scoreDestroyer
	case "TRS":
		return scoreTransport
	}
	return 0
}

// CrippledCarrierVictoryPoints 是航母"丧失作战能力"（甲板摧毁 / 结构损失超过 80% / 失去动力）
// 折算的战果分：机库容量的一半。
// 之所以不是打沉的全分，是因为船还浮着——它还有被拖回去、被修好的可能，
// 只是这一仗打不了了。机库是奇数时保留 0.5（15 机库 = 7.5 分），不四舍五入。
func CrippledCarrierVictoryPoints(card ShipCard) float64 {
	hangar := card.HangarCapacity
	if hangar <= 0 {
		// 老存档/简化卡没写机库容量：给 1 分，和击沉计分的兜底一致，别让一艘航母白瘫。
		hangar = 2
	}
	return float64(hangar) / 2
}

// carrierCrippledReason 判断一艘航母是否已经丧失作战能力，返回原因（空串表示还能打）。
// 三个条件任一成立即可，判定顺序就是规则书的书写顺序。
func carrierCrippledReason(unit Unit) string {
	switch upperType(unit.Type) {
	case "CV", "CVL", "CVE":
	default:
		return ""
	}
	// 沉没走击沉计分，不在这里重复记一笔。
	if unit.Strength <= 0 {
		return ""
	}
	if unit.Damage != nil && unit.Damage.DeckDestroyed {
		return "飞行甲板被摧毁"
	}
	// "超过 80%"是严格的：刚好 80% 不算。
	if unit.MaxStrength > 0 && (unit.MaxStrength-unit.Strength)*5 > unit.MaxStrength*4 {
		return "结构损失超过80%"
	}
	if unit.Damage != nil && unit.Damage.Disabled {
		return "失去动力"
	}
	return ""
}

// AwardCrippledCarriers 在回合结算末尾把新瘫痪的航母记进战果台账。
// 只记第一次：损管能把进水/动力修回来，但这一次的战果分不该跟着回退。
func AwardCrippledCarriers(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil {
			continue
		}
		reason := carrierCrippledReason(*unit)
		if reason == "" {
			continue
		}
		if !RecordCrippledCarrier(state, unit, reason) {
			continue
		}
		// 战果分对双方都是公开的（快照里本来就有比分），所以事件发给全场：
		// 受害方看得见"我的船废了"，得分方看得见"这分是怎么来的"。
		events = append(events, EngineEvent{
			Type:     "carrier_crippled",
			Audience: "all",
			Text:     fmt.Sprintf("%s丧失作战能力（%s），战果分 +%s", unit.Name, reason, formatVPScore(CrippledCarrierVictoryPoints(unit.Card))),
		})
	}
	return events
}

// ScoreLine 是一条战果分的来源，供界面"得分明细"逐条列出。
// 记分方向天然是反向的：己方的损失记在对方账上，所以 Faction 记的是"这笔损失是谁的"，
// 前端拿它和自己的阵营比一下就知道该显示"我方损失"还是"敌方损失"。
// Text 写成中性描述，不带主语，让前端决定冠什么词。
type ScoreLine struct {
	// Kind 取值 ship / crippled / aircraft / supply。
	Kind string `json:"kind"`
	// Faction 是这条记录的归属方：损失条目是吃亏的一方，投送目标是得分的一方。
	Faction string  `json:"faction"`
	Text    string  `json:"text"`
	ShipID  string  `json:"ship_id,omitempty"`
	Type    string  `json:"type,omitempty"`
	Turn    int     `json:"turn,omitempty"`
	Count   int     `json:"count,omitempty"`
	Points  float64 `json:"points"`
}

// ScoreBreakdown 返回 得分阵营 → 逐条明细。VictoryPoints 就是它的求和，
// 这样"总分"和"明细"永远不会对不上——两个函数各算一遍是最容易出岔子的写法。
//
// 关于保密：敌方的飞机损失在终局前本来就不进 VisibleLosses，但战果分本身是公开的，
// 而"击落敌机"这一项可以直接从比分里反解出来（自己的击沉战果是可见的），
// 所以明细里如实写架数并不会多泄露一分情报。
func ScoreBreakdown(state State) map[string][]ScoreLine {
	lines := map[string][]ScoreLine{"A": {}, "B": {}}
	// 固定按 A、B 遍历：明细要参与"逐字节判重"的推送，不能带 map 随机序。
	for _, side := range []string{"A", "B"} {
		ledger := state.Losses[side]
		if ledger == nil {
			continue
		}
		enemy := OppositeFaction(side)
		for _, loss := range ledger.Ships {
			points := ShipVictoryPoints(loss.Type, loss.Card)
			if points == 0 {
				// 认不出的舰种不给分，也不占一条空明细（求和结果不变）。
				continue
			}
			lines[enemy] = append(lines[enemy], ScoreLine{
				Kind: "ship", Faction: side, Turn: loss.Turn, ShipID: loss.ID, Type: loss.Type,
				Text:   fmt.Sprintf("击沉「%s」（%s）", loss.Name, loss.Type),
				Points: float64(points),
			})
		}
		// 瘫痪航母按机库一半计分；但沉没的船改按整艘算，不能让"先瘫后沉"领两次。
		sunk := make(map[string]bool, len(ledger.Ships))
		for _, loss := range ledger.Ships {
			sunk[loss.ID] = true
		}
		for _, crippled := range ledger.Crippled {
			if sunk[crippled.ID] {
				continue
			}
			lines[enemy] = append(lines[enemy], ScoreLine{
				Kind: "crippled", Faction: side, Turn: crippled.Turn, ShipID: crippled.ID, Type: crippled.Type,
				Text:   fmt.Sprintf("「%s」丧失作战能力（%s）", crippled.Name, crippled.Reason),
				Points: CrippledCarrierVictoryPoints(crippled.Card),
			})
		}
		aircraft := ledger.AircraftTotal
		if aircraft == 0 {
			// 老存档可能只写了按型号聚合的列表没写总数。
			for _, loss := range ledger.Aircraft {
				aircraft += loss.Count
			}
		}
		if aircraft > 0 {
			lines[enemy] = append(lines[enemy], ScoreLine{
				Kind: "aircraft", Faction: side, Count: aircraft,
				Text:   fmt.Sprintf("航空算子损失 %d 架", aircraft),
				Points: float64(aircraft) * scorePerAircraft,
			})
		}
	}
	for _, objective := range state.Objectives {
		if objective.Kind != ObjectiveSupplies || objective.Faction == "" {
			continue
		}
		if !objectiveCompleted(state, objective.Faction, objective) {
			continue
		}
		lines[objective.Faction] = append(lines[objective.Faction], ScoreLine{
			Kind: "supply", Faction: objective.Faction,
			Text:   "完成投送目标：" + objectiveLabel(objective),
			Points: float64(supplyObjectiveValue(objective)),
		})
	}
	return lines
}

// VictoryPoints 返回双方已经拿到的战果分：击沉/击落对方舰船与航空算子的分数之和，
// 加上已经完成的投送目标（按其 value 计 VP）。未完成的投送目标不计分；
// 对局结束时由 FinalVictoryPoints 把这条目标的分数判给阻止方。
func VictoryPoints(state State) map[string]float64 {
	scores := map[string]float64{"A": 0, "B": 0}
	for faction, breakdown := range ScoreBreakdown(state) {
		for _, line := range breakdown {
			scores[faction] += line.Points
		}
	}
	return scores
}

// FinalVictoryPoints 是对局结束时的最终 VP：在 VictoryPoints 的基础上，
// 把没有完成的投送目标按目标分值判给阻止方。这样"日军没送到 20 点"
// 本身就是美军实打实的战役成果，而不是只在文字上判美军赢。
func FinalVictoryPoints(state State) map[string]float64 {
	scores := VictoryPoints(state)
	for _, objective := range state.Objectives {
		if objective.Kind != ObjectiveSupplies || objective.Faction == "" {
			continue
		}
		if objectiveCompleted(state, objective.Faction, objective) {
			continue
		}
		scores[OppositeFaction(objective.Faction)] += float64(supplyObjectiveValue(objective))
	}
	return scores
}

func upperType(shipType string) string {
	out := make([]byte, 0, len(shipType))
	for index := 0; index < len(shipType); index++ {
		ch := shipType[index]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		if ch == ' ' || ch == '\t' {
			continue
		}
		out = append(out, ch)
	}
	return string(out)
}
