package game

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// —— 胜利条件 ——
//
// 剧本可以声明若干目标（Objective）：控制某 hex 的滩头物资数量达到阈值，
// 或击沉对方若干舰船。声明 kind:"score" 的剧本把战役目标和击沉战果折算成 VP，
// 按双方比值判平分秋色/战术/战略胜利；没有 score 目标的老剧本仍按完成的目标点总和判定。
//
// 剧本不写 Objectives 就退化成"双方全灭判定"——一方舰船与飞机全灭则另一方获胜。

// ObjectiveKind 是目标的种类。
type ObjectiveKind string

const (
	// ObjectiveSupplies：在指定坐标卸下至少 Value 点滩头物资。
	ObjectiveSupplies ObjectiveKind = "supplies"
	// ObjectiveSink：击沉至少 Value 艘敌方舰船。
	ObjectiveSink ObjectiveKind = "sink"
	// ObjectiveControl：回合结束时在指定坐标有己方舰队（持续时间由 Value 决定，默认 1 回合）。
	ObjectiveControl ObjectiveKind = "control"
	// ObjectiveScore：不按目标条目逐条计点，而是按战果分判定胜负
	// （航母按机库容量，战列舰 6、重巡 3、轻巡 2、驱逐 1，见 ShipVictoryPoints）。
	ObjectiveScore ObjectiveKind = "score"
)

// VictoryLevel 是 VP 比值给出的胜利等级。Winner 为空且 Level=draw 表示平分秋色。
type VictoryLevel string

const (
	VictoryLevelDraw      VictoryLevel = "draw"
	VictoryLevelTactical  VictoryLevel = "tactical"
	VictoryLevelStrategic VictoryLevel = "strategic"
)

// Objective 是一条剧本胜利条件。
type Objective struct {
	ID    string        `json:"id"`
	Text  string        `json:"text,omitempty"`
	Kind  ObjectiveKind `json:"kind"`
	Coord string        `json:"coord,omitempty"`
	// Faction 为空表示"任何一方"都能去完成它。
	Faction string `json:"faction,omitempty"`
	Value   int    `json:"value,omitempty"`
	Points  int    `json:"points,omitempty"`
	// Decisive 只用于没有 score 目标的老剧本："一达成即结束对局"。
	// 声明 score 的 VP 剧本忽略它，统一到回合上限结算。
	Decisive bool `json:"decisive,omitempty"`
}

// GameOutcome 是对局结果。
type GameOutcome struct {
	// Winner 为空表示平局。Level 是 VP 比值给出的等级（draw/tactical/strategic）。
	Winner  string             `json:"winner,omitempty"`
	Level   VictoryLevel       `json:"level,omitempty"`
	Reason  string             `json:"reason"`
	Scores  map[string]float64 `json:"scores"`
	Turn    int                `json:"turn"`
	TurnCap bool               `json:"turn_cap,omitempty"`
}

// OppositeFaction 返回对手阵营：A↔B。剧本只允许两个阵营，
// 其他取值原样返回（以后加第三方剧本时在这里扩展）。
func OppositeFaction(faction string) string {
	switch faction {
	case "A":
		return "B"
	case "B":
		return "A"
	}
	return faction
}

func objectivePoints(o Objective) int {
	if o.Points != 0 {
		return o.Points
	}
	return 1
}

// objectiveLabel 是给玩家看的简短目标说明，优先用剧本写好的 text。
func objectiveLabel(o Objective) string {
	if o.Text != "" {
		return o.Text
	}
	return o.ID
}

// factionCargoLeft 统计某阵营运输船上还剩多少货物。
// 用来提前判定决定性投送目标是否已经不可能完成。
func factionCargoLeft(state State, faction string) int {
	total := 0
	for _, unit := range state.Units {
		if unit == nil || unit.Faction != faction || unit.Type != "TRS" {
			continue
		}
		if unit.Card.Cargo > 0 {
			total += unit.Card.Cargo
		}
	}
	return total
}

// factionHasUnits 判断该阵营是否还有任何可作战单位（舰船或航空算子）。
func factionHasUnits(state State, faction string) bool {
	for _, unit := range state.Units {
		if unit.Faction == faction {
			return true
		}
	}
	// 只剩飞机不算"还有战力"：母舰全沉后，机库里的飞机既飞不出去也没有航母可降落，
	// 实际上已经失去作战能力。之前把这些飞机也算成存活，导致一方舰船被打光后
	// 对局还要拖到回合上限（自走实测：B 方舰船 T11 就全灭，却一直拖到 T60）。
	// 只有机场上的地面飞机仍算战力——它们还能从机场起飞。
	for _, aircraft := range state.GroundAir {
		if aircraft.Faction == faction {
			return true
		}
	}
	return false
}

// factionShipCount 统计某阵营现存的舰船数。
func factionShipCount(state State, faction string) int {
	count := 0
	for _, unit := range state.Units {
		if unit.Faction == faction {
			count++
		}
	}
	return count
}

// supplyObjectiveValue 是投送目标的满分分值；没写 value 时按 1 点货物算。
func supplyObjectiveValue(o Objective) int {
	if o.Value > 0 {
		return o.Value
	}
	return 1
}

// deliveredToObjective 返回某条投送目标已经卸下的货物，最多不超过目标值。
func deliveredToObjective(state State, o Objective) int {
	if o.Faction == "" || o.Coord == "" {
		return 0
	}
	delivered := state.ShoreSupplies[o.Faction+"|"+o.Coord]
	if delivered > supplyObjectiveValue(o) {
		delivered = supplyObjectiveValue(o)
	}
	if delivered < 0 {
		delivered = 0
	}
	return delivered
}

// factionCompletedSupplyObjective 判断某阵营是否已经完成它的全部投送目标。
// 珊瑚海只有一个目标：向莫尔兹比港卸下 20 点货物。
func factionCompletedSupplyObjective(state State, faction string) bool {
	required, delivered := 0, 0
	for _, objective := range state.Objectives {
		if objective.Kind != ObjectiveSupplies || objective.Faction != faction {
			continue
		}
		required += supplyObjectiveValue(objective)
		delivered += deliveredToObjective(state, objective)
	}
	return required > 0 && delivered >= required
}

func factionHasSupplyObjective(state State, faction string) bool {
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveSupplies && objective.Faction == faction {
			return true
		}
	}
	return false
}

// objectiveCompleted 判断某条目标是否已被某阵营完成。
func objectiveCompleted(state State, faction string, o Objective) bool {
	target := o.Faction
	if target == "" {
		target = faction
	} else if target != faction {
		return false
	}
	switch o.Kind {
	case ObjectiveSupplies:
		value := o.Value
		if value <= 0 {
			value = 1
		}
		return state.ShoreSupplies[target+"|"+o.Coord] >= value
	case ObjectiveControl:
		if o.Coord == "" {
			return false
		}
		for _, unit := range state.Units {
			if unit.Faction == target && unit.Coord == o.Coord {
				return true
			}
		}
		return false
	case ObjectiveScore:
		// 战果分是双方共用的记分板，不是某一条目标"完成/未完成"，所以这里永远返回 false。
		return false
	case ObjectiveSink:
		// 目标写在哪个阵营名下，就是"这个阵营要去击沉对方"——被统计的是对方的舰船。
		// 之前这里直接用 target（己方）来算沉没数，等于"自己损失得越多越接近胜利"，
		// 数据上写着"击沉日军全部8艘"、逻辑上却在数美军沉了几艘。
		value := o.Value
		if value <= 0 {
			value = 1
		}
		enemy := OppositeFaction(target)
		initial := state.InitialShipCounts[enemy]
		return initial-factionShipCount(state, enemy) >= value
	}
	return false
}

// EvaluateOutcome 判定对局是否结束。返回 nil 表示继续。
//
// 声明了 score 目标的剧本走 VP 判定：战役目标和击沉战果都折算成 VP，
// 只有回合上限强制结算，再按双方比值给出平分秋色 / 战术胜利 / 战略胜利。
// 没有 score 目标的老剧本仍按目标点判定，行为不变。
func EvaluateOutcome(state State) *GameOutcome {
	factions := map[string]bool{}
	for _, unit := range state.Units {
		factions[unit.Faction] = true
	}
	for _, aircraft := range state.Aircraft {
		factions[aircraft.Faction] = true
	}
	for faction := range state.Doctrines {
		factions[faction] = true
	}
	alive := []string{}
	for faction := range factions {
		if factionHasUnits(state, faction) {
			alive = append(alive, faction)
		}
	}
	if len(alive) == 1 {
		return &GameOutcome{Winner: alive[0], Reason: "对方全军覆没", Scores: map[string]float64{}, Turn: state.Turn}
	}
	if len(alive) == 0 {
		return &GameOutcome{Reason: "双方全灭", Scores: map[string]float64{}, Turn: state.Turn}
	}
	if len(state.Objectives) == 0 {
		return nil
	}

	usesVictoryPoints := false
	for _, objective := range state.Objectives {
		if objective.Kind == ObjectiveScore {
			usesVictoryPoints = true
			break
		}
	}
	if usesVictoryPoints {
		// VP 剧本只有回合上限一个强制结算点。投送完成/失败、
		// 航母被打光都不提前结束，先让 360 回合把账算完。
		if state.MaxTurns <= 0 || state.Turn < state.MaxTurns {
			return nil
		}
		scores := FinalVictoryPoints(state)
		for _, faction := range alive {
			if _, ok := scores[faction]; !ok {
				scores[faction] = 0
			}
		}
		return victoryPointOutcome(state, scores, fmt.Sprintf("回合上限 %d 到达", state.MaxTurns), true)
	}

	// —— 老剧本：按目标点判定 ——
	scores := map[string]float64{}

	// 决定性目标一旦达成立即结束对局，不必等到回合上限。
	for _, objective := range state.Objectives {
		if !objective.Decisive {
			continue
		}
		candidates := alive
		if objective.Faction != "" {
			candidates = []string{objective.Faction}
		}
		for _, faction := range candidates {
			if !slices.Contains(alive, faction) {
				continue
			}
			if objectiveCompleted(state, faction, objective) {
				return &GameOutcome{Winner: faction, Reason: "完成决定性目标：" + objectiveLabel(objective), Scores: scores, Turn: state.Turn}
			}
		}
	}

	// 决定性补给目标已经不可能完成：货已卸空或运输船全被击沉。
	for _, objective := range state.Objectives {
		if !objective.Decisive || objective.Kind != ObjectiveSupplies || objective.Faction == "" {
			continue
		}
		if !slices.Contains(alive, objective.Faction) {
			continue
		}
		if factionCargoLeft(state, objective.Faction) == 0 {
			winner := OppositeFaction(objective.Faction)
			return &GameOutcome{Winner: winner, Reason: objective.Faction + "方已无货可卸，无法完成决定性目标：" + objectiveLabel(objective), Scores: scores, Turn: state.Turn}
		}
	}

	if state.MaxTurns <= 0 || state.Turn < state.MaxTurns {
		return nil
	}

	declared := []Objective{}
	for _, objective := range state.Objectives {
		if objective.Decisive && objective.Faction != "" {
			declared = append(declared, objective)
		}
	}
	if len(declared) > 0 {
		factions := map[string]bool{}
		labels := []string{}
		for _, objective := range declared {
			factions[objective.Faction] = true
			labels = append(labels, objectiveLabel(objective))
		}
		if len(factions) == 1 {
			for faction := range factions {
				return &GameOutcome{
					Winner:  OppositeFaction(faction),
					Reason:  fmt.Sprintf("回合上限 %d 到达：%s方未完成决定性目标（%s），对手达成阻止", state.MaxTurns, faction, strings.Join(labels, "；")),
					Scores:  scores,
					Turn:    state.Turn,
					TurnCap: true,
				}
			}
		}
	}

	reason := fmt.Sprintf("回合上限 %d 到达，按目标点判定", state.MaxTurns)
	for _, faction := range alive {
		for _, objective := range state.Objectives {
			if objectiveCompleted(state, faction, objective) {
				scores[faction] += float64(objectivePoints(objective))
			}
		}
	}
	winner := ""
	best := -1.0
	tied := false
	for _, faction := range alive {
		score := scores[faction]
		if score > best {
			best = score
			winner = faction
			tied = false
		} else if score == best {
			tied = true
		}
	}
	if tied {
		winner = ""
	}
	return &GameOutcome{Winner: winner, Reason: reason, Scores: scores, Turn: state.Turn, TurnCap: true}
}

// victoryPointOutcome 按 VP 比值给出最终等级。比值小于 1.25 平分秋色，
// 1.25 到 2 战术胜利，大于 2 战略胜利。日军没有投送满 20 单位时，
// 即使比值进入战略档，也只算战术胜利。
func victoryPointOutcome(state State, scores map[string]float64, reason string, turnCap bool) *GameOutcome {
	scoreA, scoreB := scores["A"], scores["B"]
	highFaction, high, low := "A", scoreA, scoreB
	if scoreB > scoreA {
		highFaction, high, low = "B", scoreB, scoreA
	}
	if high == low {
		return &GameOutcome{
			Level:   VictoryLevelDraw,
			Reason:  fmt.Sprintf("%s；VP %s:%s，平分秋色", reason, formatVPScore(scoreA), formatVPScore(scoreB)),
			Scores:  scores,
			Turn:    state.Turn,
			TurnCap: turnCap,
		}
	}

	ratio := math.Inf(1)
	if low > 0 {
		ratio = high / low
	}
	level := VictoryLevelStrategic
	if ratio < 1.25 {
		level = VictoryLevelDraw
	} else if ratio <= 2 {
		level = VictoryLevelTactical
	}

	capNote := ""
	if level == VictoryLevelStrategic && highFaction == "B" && factionHasSupplyObjective(state, "B") && !factionCompletedSupplyObjective(state, "B") {
		level = VictoryLevelTactical
		capNote = "；日军未投送满20单位，战略胜利降为战术胜利"
	}

	winner := highFaction
	if level == VictoryLevelDraw {
		winner = ""
	}
	return &GameOutcome{
		Winner:  winner,
		Level:   level,
		Reason:  fmt.Sprintf("%s；VP %s:%s（%s:1），%s%s", reason, formatVPScore(scoreA), formatVPScore(scoreB), formatVPRatio(ratio), outcomeLevelLabel(level, winner), capNote),
		Scores:  scores,
		Turn:    state.Turn,
		TurnCap: turnCap,
	}
}

func outcomeLevelLabel(level VictoryLevel, winner string) string {
	switch level {
	case VictoryLevelDraw:
		return "平分秋色"
	case VictoryLevelTactical:
		return winner + "方战术胜利"
	case VictoryLevelStrategic:
		return winner + "方战略胜利"
	}
	return ""
}

func formatVPScore(value float64) string {
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return fmt.Sprintf("%.0f", value)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func formatVPRatio(value float64) string {
	if math.IsInf(value, 1) {
		return "∞"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

// settleOutcome 在对局未结束时检查并记录结果，只产生一次事件。
func settleOutcome(state *State) []EngineEvent {
	if state.Outcome != nil {
		return nil
	}
	if outcome := EvaluateOutcome(*state); outcome != nil {
		state.Outcome = outcome
		text := "对局结束：" + outcome.Reason
		if outcome.Winner != "" {
			text += "，" + outcome.Winner + "获胜"
		} else {
			text += "，平局"
		}
		return []EngineEvent{{Type: "game_over", Audience: "all", Text: text}}
	}
	return nil
}
