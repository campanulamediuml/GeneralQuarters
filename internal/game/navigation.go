package game

import (
	"fmt"
	"sort"
)

// —— 自动航行与交战设置（规则书 280–283、362–370 行）——
//
// 舰队可以设置自动航行目的地：设置后，舰队在每次可移动时沿最短可通行路线向目标推进。
// 手动移动与自动移动共用移动机会，抵达目的地后自动航行自动结束。
//
// 交战设置决定自动航行遇到敌军时的行为：
//   逢敌必战：下一个格子出现敌军就进入；
//   优势交战：下一个格子有敌军，且情报显示对方舰船数少于己方才进入；
//   避免近战：尝试绕开敌军格；没有可通行绕行路线时停止移动。

// EngagementSetting 是自动航行时的交战偏好。
type EngagementSetting string

const (
	EngageAggressive EngagementSetting = "aggressive" // 逢敌必战
	EngageAdvantage  EngagementSetting = "advantage"  // 优势交战
	EngageAvoid      EngagementSetting = "avoid"      // 避免近战
)

// NormalizeEngagementSetting 把空值当成默认的"逢敌必战"。
func NormalizeEngagementSetting(setting EngagementSetting) EngagementSetting {
	switch setting {
	case EngageAdvantage, EngageAvoid:
		return setting
	default:
		return EngageAggressive
	}
}

func (s EngagementSetting) Label() string {
	switch NormalizeEngagementSetting(s) {
	case EngageAdvantage:
		return "优势交战"
	case EngageAvoid:
		return "避免近战"
	default:
		return "逢敌必战"
	}
}

// fleetNavLeader 返回该舰队用作自动航行状态载体的首舰。
func fleetNavLeader(state *State, fleetID string) *Unit {
	ids := []string{}
	for id, unit := range state.Units {
		if FleetKey(unit) == fleetID {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	sort.Strings(ids)
	return state.Units[ids[0]]
}

// FleetDestination 返回舰队的自动航行目的地（空表示没有设置）。
func FleetDestination(state State, fleetID string) string {
	if leader := fleetNavLeader(&state, fleetID); leader != nil {
		return leader.Destination
	}
	return ""
}

// FleetEngagementSetting 返回舰队的交战设置。
// FleetHasCarrier 判断某舰队是否含航母。含航母的舰队固定"消极避战"：
// 航母是打击核心，被水面舰贴近就是灾难，规则上不允许它主动寻求炮战。
func FleetHasCarrier(state State, fleetID string) bool {
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID && IsCarrier(unit) {
			return true
		}
	}
	return false
}

func FleetEngagementSetting(state State, fleetID string) EngagementSetting {
	if FleetHasCarrier(state, fleetID) {
		return EngageAvoid
	}
	if leader := fleetNavLeader(&state, fleetID); leader != nil {
		return NormalizeEngagementSetting(EngagementSetting(leader.Engagement))
	}
	return EngageAggressive
}

// validateNavOrder 校验自动航行命令：目的地必须是地图内坐标，且存在一条可通行航线。
func validateNavOrder(state State, faction string, order Order) error {
	unit := state.Units[order.UnitID]
	if unit == nil || unit.Faction != faction {
		return fmt.Errorf("舰队不存在或不属于当前玩家")
	}
	// 空坐标 = 取消自动航行。以前这里直接报"请选择目的地"，
	// 于是取消按钮永远发不出去（ApplyNavOrder 明明已经处理了空坐标的情况）。
	if order.Coord == "" {
		return nil
	}
	if !inBounds(state.Map, order.Coord) {
		return fmt.Errorf("目的地超出地图范围")
	}
	carrierFleet := FleetHasCarrier(state, FleetKey(unit))
	if !fleetCanEnter(state, order.Coord, carrierFleet) {
		return fmt.Errorf("目的地不可通行")
	}
	if _, ok := shortestFleetPath(state, unit.Coord, order.Coord, carrierFleet); !ok {
		return fmt.Errorf("没有可通行的航线到达该目的地")
	}
	return nil
}

// ApplyNavOrder 设置或取消舰队的自动航行目的地。
func ApplyNavOrder(state *State, order Order) []EngineEvent {
	leader := fleetNavLeader(state, FleetKey(state.Units[order.UnitID]))
	if leader == nil {
		return nil
	}
	for _, unit := range state.Units {
		if FleetKey(unit) != FleetKey(leader) {
			continue
		}
		unit.Destination = order.Coord
		unit.Engagement = string(NormalizeEngagementSetting(EngagementSetting(order.Engagement)))
	}
	if order.Coord == "" {
		return []EngineEvent{{Type: "nav_cleared", Audience: leader.Faction, Text: "已取消自动航行"}}
	}
	return []EngineEvent{{Type: "nav_set", Audience: leader.Faction, Text: fmt.Sprintf("%s设置自动航行目的地%s（%s）", leader.Name, order.Coord, NormalizeEngagementSetting(EngagementSetting(order.Engagement)).Label())}}
}

// ApplyEngagementOrder 只修改交战设置，不影响已设置的目的地。
func ApplyEngagementOrder(state *State, order Order) []EngineEvent {
	leader := fleetNavLeader(state, FleetKey(state.Units[order.UnitID]))
	if leader == nil {
		return nil
	}
	setting := NormalizeEngagementSetting(EngagementSetting(order.Engagement))
	for _, unit := range state.Units {
		if FleetKey(unit) == FleetKey(leader) {
			unit.Engagement = string(setting)
		}
	}
	return []EngineEvent{{Type: "engagement_set", Audience: leader.Faction, Text: fmt.Sprintf("%s改为%s", leader.Name, setting.Label())}}
}

// shortestFleetPath 用 BFS 找最短可通行航线（不含起点），第二个返回值表示是否可达。
func shortestFleetPath(state State, from, to string, carrierFleet bool) ([]string, bool) {
	return shortestFleetPathAvoiding(state, from, to, carrierFleet, nil)
}

// enemyUnitsAt 返回该 hex 上其他阵营的舰船。
func enemyUnitsAt(state State, faction, coord string) []Unit {
	result := []Unit{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit.Coord == coord && unit.Faction != faction {
			result = append(result, *unit)
		}
	}
	return result
}

// fleetShipCount 统计舰队现有舰船数。
func fleetShipCount(state State, fleetID string) int {
	count := 0
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID {
			count++
		}
	}
	return count
}

// intelShipCount 返回情报上看到的敌方编队舰船数（情报等级 2 以下只能给出粗略规模）。
func intelShipCount(state State, faction, fleetID string) (int, bool) {
	contact := state.Contacts[faction][fleetID]
	if contact == nil || contact.Level < 2 {
		return 0, false
	}
	return contact.Count, true
}

// autoNavStep 为舰队计算本回合的自动航行目标格。
// 返回空字符串表示本回合不移动（已抵达、被交战设置拦下或没有航线）。
func autoNavStep(state *State, unit *Unit) (string, string) {
	destination := unit.Destination
	if destination == "" || unit.Coord == destination {
		return "", ""
	}
	faction := unit.Faction
	carrierFleet := FleetHasCarrier(*state, FleetKey(unit))
	setting := FleetEngagementSetting(*state, FleetKey(unit))
	blocked := map[string]bool{}
	for {
		// 避免近战时把有敌军的格子当成障碍；逢敌必战/优势交战则先看下一格是谁。
		path, ok := shortestFleetPathAvoiding(*state, unit.Coord, destination, carrierFleet, blocked)
		if !ok {
			if len(blocked) == 0 {
				return "", "没有可通行的航线"
			}
			return "", "没有可通行的绕行路线，停止移动"
		}
		if len(path) == 0 {
			return "", ""
		}
		next := path[0]
		if enemy := enemyUnitsAt(*state, faction, next); len(enemy) > 0 {
			switch setting {
			case EngageAvoid:
				// 尝试绕开：把这一格当障碍重新寻路。
				blocked[next] = true
				continue
			case EngageAdvantage:
				enemyFleet := FleetKey(&enemy[0])
				count, known := intelShipCount(*state, faction, enemyFleet)
				if !known || count >= fleetShipCount(*state, FleetKey(unit)) {
					return "", "下一格敌军兵力不占优势，停止移动"
				}
			}
		}
		return next, ""
	}
}

// shortestFleetPathAvoiding 与 shortestFleetPath 相同，但额外把 blocked 里的格子当作障碍。
func shortestFleetPathAvoiding(state State, from, to string, carrierFleet bool, blocked map[string]bool) ([]string, bool) {
	if from == to {
		return nil, true
	}
	prev := map[string]string{from: ""}
	queue := []string{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range Neighbors(current) {
			if blocked[next] {
				continue
			}
			if _, seen := prev[next]; seen {
				continue
			}
			if !fleetCanEnter(state, next, carrierFleet) {
				continue
			}
			prev[next] = current
			if next == to {
				path := []string{}
				for node := to; node != from; node = prev[node] {
					path = append([]string{node}, path...)
				}
				return path, true
			}
			queue = append(queue, next)
		}
	}
	return nil, false
}

// resolveAutoNavigation 在结算阶段让所有设置了自动航行的舰队推进一格（共用移动机会）。
func resolveAutoNavigation(state *State) []EngineEvent {
	events := []EngineEvent{}
	fleets := map[string]bool{}
	for _, unit := range state.Units {
		if unit.Destination != "" {
			fleets[FleetKey(unit)] = true
		}
	}
	ids := make([]string, 0, len(fleets))
	for id := range fleets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, fleetID := range ids {
		leader := fleetNavLeader(state, fleetID)
		if leader == nil || MovementBlocked(*state, leader) {
			continue
		}
		next, reason := autoNavStep(state, leader)
		if next == "" {
			if reason != "" {
				events = append(events, EngineEvent{Type: "nav_blocked", Audience: leader.Faction, Text: leader.Name + "自动航行：" + reason})
			}
			continue
		}
		for _, unit := range state.Units {
			if FleetKey(unit) != fleetID {
				continue
			}
			from := unit.Coord
			unit.Coord = next
			unit.Heading = headingBetween(from, next)
			unit.Moved = true
			if interval := FleetMoveInterval(*state, unit); movementCooldownAfterMove(interval) > unit.MoveCooldown {
				unit.MoveCooldown = movementCooldownAfterMove(interval)
			}
		}
		clearFleetStay(state, fleetID)
		arrived := next == leader.Destination
		if arrived {
			for _, unit := range state.Units {
				if FleetKey(unit) == fleetID {
					unit.Destination = ""
				}
			}
			events = append(events, EngineEvent{Type: "nav_arrived", Audience: leader.Faction, Text: fmt.Sprintf("%s自动航行抵达%s，指令结束", leader.Name, next)})
		} else {
			events = append(events, EngineEvent{Type: "nav_moved", Audience: leader.Faction, Text: fmt.Sprintf("%s自动航行至%s（目的地%s）", leader.Name, next, leader.Destination)})
		}
	}
	return events
}
