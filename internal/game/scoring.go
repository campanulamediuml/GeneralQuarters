package game

// —— 战果计分 ——
//
// 剧本用 kind="score" 的目标声明"按击沉战果分排名"：一艘航母的价值不是拍脑袋定的常数，
// 而是它机库能装多少架——打掉翔鹤和打掉一艘护航航母不该是一个价钱。
// 其余舰种按重量级折算：战列舰/战列巡洋舰 6、重巡 3、轻巡 2、驱逐 1、运输船 1；
// 每摧毁一个航空算子再加 0.25 分，这样"打光对方的飞机"和"打沉对方的船"都能计到账。
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

// VictoryPoints 返回双方已经拿到的战果分：击沉/击落对方舰船与航空算子的分数之和，
// 加上已经完成的投送目标（按其 value 计 VP）。未完成的投送目标不计分；
// 对局结束时由 FinalVictoryPoints 把这条目标的分数判给阻止方。
func VictoryPoints(state State) map[string]float64 {
	scores := map[string]float64{"A": 0, "B": 0}
	for side, ledger := range state.Losses {
		if ledger == nil {
			continue
		}
		enemy := OppositeFaction(side)
		for _, loss := range ledger.Ships {
			scores[enemy] += float64(ShipVictoryPoints(loss.Type, loss.Card))
		}
		aircraft := ledger.AircraftTotal
		if aircraft == 0 {
			// 老存档可能只写了按型号聚合的列表没写总数。
			for _, loss := range ledger.Aircraft {
				aircraft += loss.Count
			}
		}
		scores[enemy] += float64(aircraft) * scorePerAircraft
	}
	for _, objective := range state.Objectives {
		if objective.Kind != ObjectiveSupplies || objective.Faction == "" {
			continue
		}
		if objectiveCompleted(state, objective.Faction, objective) {
			scores[objective.Faction] += float64(supplyObjectiveValue(objective))
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
