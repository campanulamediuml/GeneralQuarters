package game

import (
	"fmt"
	"sort"
	"strings"
)

// —— 舰队拆分 ——
//
// 玩家把一支舰队里的部分舰船拆成一支新舰队：比整编轻，只做"分家"这一件事。
// 它和整编（MERGE_FLEET / REORGANIZE_HEX）刻意分开：
//   - 整编会给新编队挂 3 回合移动锁，拆分不挂；
//   - 整编可能跨舰队重组，拆分只把选中的船分出去；
//   - 拆分不动坐标、朝向、冷却和停留计时，两队都按原计划行动。
//
// 反潜战里的"反潜支队"只是它的一个用法：把带深弹的船拆出去留在原地反潜，
// 正在追踪的潜艇会跟着改挂到新舰队。

// validateSplitFleet 校验拆分：选中的船必须属于这支舰队，且至少选一艘。
// 不限制"必须留下船"：全选等于把这支舰队整体改成新舰队，玩家想这么干也可以。
func validateSplitFleet(state State, faction string, order Order) error {
	unit := state.Units[order.UnitID]
	if unit == nil || unit.Faction != faction {
		return fmt.Errorf("舰队不存在或不属于当前玩家")
	}
	if len(order.SplitIDs) == 0 {
		return fmt.Errorf("先选择要拆出的舰船")
	}
	fleetID := FleetKey(unit)
	seen := map[string]bool{}
	for _, id := range order.SplitIDs {
		if seen[id] {
			return fmt.Errorf("拆分的舰船列表里有重复项")
		}
		seen[id] = true
		member := state.Units[id]
		if member == nil || member.Faction != faction {
			return fmt.Errorf("要拆出的舰船不存在或不属于当前玩家")
		}
		if FleetKey(member) != fleetID {
			return fmt.Errorf("%s不属于这支舰队", member.Name)
		}
	}
	return nil
}

// ApplySplitFleet 把选中的舰船拆成一支新舰队，留在同一格。
// 不挂移动锁、不改冷却/停留计时，两队都按原计划行动。
func ApplySplitFleet(state *State, order Order) []EngineEvent {
	leader := state.Units[order.UnitID]
	if leader == nil {
		return nil
	}
	oldFleetID := FleetKey(leader)
	newFleetID := oldFleetID + "-s1"
	for index := 2; ; index++ {
		if !fleetExists(*state, newFleetID) {
			break
		}
		newFleetID = fmt.Sprintf("%s-s%d", oldFleetID, index)
	}
	oldName := leader.FleetName
	if oldName == "" {
		oldName = leader.Name
	}
	newName := oldName + "分队"
	if strings.HasSuffix(oldName, "分队") {
		newName = oldName
	}

	splitIDs := append([]string(nil), order.SplitIDs...)
	sort.Strings(splitIDs)
	moved := 0
	for _, id := range splitIDs {
		member := state.Units[id]
		if member == nil || member.Faction != leader.Faction || FleetKey(member) != oldFleetID {
			continue
		}
		member.FleetID = newFleetID
		member.FleetName = newName
		moved++
	}
	if moved == 0 {
		return []EngineEvent{{Type: "fleet_split_empty", Audience: leader.Faction, Text: fmt.Sprintf("%s没有可拆分的舰船", oldName)}}
	}
	inheritSplitFleetState(state, oldFleetID, newFleetID)
	splitFleetContacts(state, oldFleetID, newFleetID, newName, splitIDs, moved)
	// 正在被这支舰队追踪的潜艇：只要拆出去的分队里有反潜舰，追踪就交给分队。
	// 这是"大舰队拆出驱逐舰继续反潜、主队继续走"的关键——不能等主队把反潜舰都拆光才交，
	// 否则主队留着哪怕一艘驱逐舰，拆出去的支队都接不到反潜任务。
	trackingMoved := false
	if fleetHasASWShips(*state, newFleetID) {
		for _, id := range sortedSubmarineIDs(state.Submarines) {
			sub := state.Submarines[id]
			if sub == nil || !sub.Discovered || sub.TrackedFleetID != oldFleetID {
				continue
			}
			sub.TrackedFleetID = newFleetID
			trackingMoved = true
		}
	}
	text := fmt.Sprintf("%s拆出%d艘舰船组成%s，留在%s", oldName, moved, newName, leader.Coord)
	if trackingMoved {
		text += "；被追踪的潜艇改由分队继续反潜"
	}
	return []EngineEvent{{Type: "fleet_split", Audience: leader.Faction, Text: text}}
}

func inheritSplitFleetState(state *State, oldFleetID, newFleetID string) {
	if value, ok := state.FleetSessions[oldFleetID]; ok {
		if state.FleetSessions == nil {
			state.FleetSessions = map[string]int{}
		}
		state.FleetSessions[newFleetID] = value
	}
	if value, ok := state.FleetStays[oldFleetID]; ok {
		if state.FleetStays == nil {
			state.FleetStays = map[string]string{}
		}
		state.FleetStays[newFleetID] = value
	}
	if value, ok := state.SurpriseLock[oldFleetID]; ok {
		if state.SurpriseLock == nil {
			state.SurpriseLock = map[string]int{}
		}
		state.SurpriseLock[newFleetID] = value
	}
	if value, ok := state.HeadingChanged[oldFleetID]; ok {
		if state.HeadingChanged == nil {
			state.HeadingChanged = map[string]bool{}
		}
		state.HeadingChanged[newFleetID] = value
	}
}

func splitFleetContacts(state *State, oldFleetID, newFleetID, newName string, splitIDs []string, moved int) {
	split := make(map[string]bool, len(splitIDs))
	for _, id := range splitIDs {
		split[id] = true
	}
	remaining := len(fleetMembers(state, oldFleetID))
	for observer, contacts := range state.Contacts {
		oldContact := contacts[oldFleetID]
		if oldContact == nil {
			continue
		}
		newContact := *oldContact
		newContact.FleetID = newFleetID
		newContact.Count = moved
		newContact.Members = nil
		oldMembers := make([]Unit, 0, len(oldContact.Members))
		for _, member := range oldContact.Members {
			if split[member.ID] {
				member.FleetID = newFleetID
				member.FleetName = newName
				newContact.Members = append(newContact.Members, member)
			} else {
				oldMembers = append(oldMembers, member)
			}
		}
		contacts[newFleetID] = &newContact
		oldContact.Count = remaining
		oldContact.Members = oldMembers
		if remaining == 0 {
			delete(contacts, oldFleetID)
		}
		oldKey, newKey := observer+"|"+oldFleetID, observer+"|"+newFleetID
		if value, ok := state.IntelFresh[oldKey]; ok {
			state.IntelFresh[newKey] = value
		}
		if value, ok := state.IntelRaised[oldKey]; ok {
			state.IntelRaised[newKey] = value
		}
		if value, ok := state.IntelStaleTurns[oldKey]; ok {
			state.IntelStaleTurns[newKey] = value
		}
	}
}

// fleetExists 判断某个 fleet_id 下还有没有船。
func fleetExists(state State, fleetID string) bool {
	for _, unit := range state.Units {
		if unit != nil && FleetKey(unit) == fleetID {
			return true
		}
	}
	return false
}

// fleetHasASWShips 判断一支舰队里还有没有反潜舰（深弹 > 0）。
func fleetHasASWShips(state State, fleetID string) bool {
	for _, unit := range state.Units {
		if unit != nil && FleetKey(unit) == fleetID && unit.Card.DepthCharges > 0 {
			return true
		}
	}
	return false
}
