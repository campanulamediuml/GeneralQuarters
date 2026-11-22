package game

import "fmt"

// —— 胜利条件 ——
//
// 剧本可以声明若干目标（Objective）：控制某 hex 的滩头物资数量达到阈值，
// 或击沉对方若干舰船。回合上限到来时，按完成的目标点总和判定胜负。
//
// 规则书本身没有规定具体胜利条件，这里给出的是通用实现：剧本不写 Objectives
// 就退化成"双方全灭判定"——一方舰船与飞机全灭则另一方获胜。

// ObjectiveKind 是目标的种类。
type ObjectiveKind string

const (
	// ObjectiveSupplies：在指定坐标卸下至少 Value 点滩头物资。
	ObjectiveSupplies ObjectiveKind = "supplies"
	// ObjectiveSink：击沉至少 Value 艘敌方舰船。
	ObjectiveSink ObjectiveKind = "sink"
	// ObjectiveControl：回合结束时在指定坐标有己方舰队（持续时间由 Value 决定，默认 1 回合）。
	ObjectiveControl ObjectiveKind = "control"
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
}

// GameOutcome 是对局结果。
type GameOutcome struct {
	// Winner 为空表示平局。
	Winner  string         `json:"winner,omitempty"`
	Reason  string         `json:"reason"`
	Scores  map[string]int `json:"scores"`
	Turn    int            `json:"turn"`
	TurnCap bool           `json:"turn_cap,omitempty"`
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
		return &GameOutcome{Winner: alive[0], Reason: "对方全军覆没", Scores: map[string]int{}, Turn: state.Turn}
	}
	if len(alive) == 0 {
		return &GameOutcome{Reason: "双方全灭", Scores: map[string]int{}, Turn: state.Turn}
	}
	if len(state.Objectives) == 0 {
		return nil
	}
	scores := map[string]int{}
	for _, faction := range alive {
		for _, objective := range state.Objectives {
			if objectiveCompleted(state, faction, objective) {
				scores[faction] += objectivePoints(objective)
			}
		}
	}
	// 只有到达回合上限时才按目标点结算；否则继续打。
	if state.MaxTurns <= 0 || state.Turn < state.MaxTurns {
		return nil
	}
	winner := ""
	best := -1
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
	return &GameOutcome{Winner: winner, Reason: fmt.Sprintf("回合上限 %d 到达，按目标点判定", state.MaxTurns), Scores: scores, Turn: state.Turn, TurnCap: true}
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
