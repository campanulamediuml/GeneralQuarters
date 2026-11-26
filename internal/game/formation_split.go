package game

import (
	"fmt"
	"sort"
	"strings"
)

// splitCarrierReturningMembers 把已经掉头返航的舰载机从打击编队里拆出去，单独成一组。
//
// 以前任务分组只认 TaskID，飞机开始返航后 TaskID 不会变，于是"攻击编队 ×22"的卡片里
// 一直混着几架"返航中"——玩家算不清还有多少攻击力，也容易以为它们还在打。
// 拆组只改任务号，不改任务类型和优先级：落地后照常清掉任务参数。
func splitCarrierReturningMembers(state *State) []EngineEvent {
	events := []EngineEvent{}
	if state == nil {
		return events
	}
	returning := map[string][]*AirUnit{}
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.TaskID == "" || aircraft.TaskKind != "attack" || !aircraft.Returning || !airborne(aircraft) {
			continue
		}
		returning[aircraft.TaskID] = append(returning[aircraft.TaskID], aircraft)
	}
	taskIDs := make([]string, 0, len(returning))
	for taskID := range returning {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		active := 0
		for _, member := range taskAircraft(*state, taskID) {
			if member != nil && !member.Returning {
				active++
			}
		}
		// 全员都在返航：整组本来就是"返航编队"，不用拆。
		if active == 0 {
			continue
		}
		rtbID := nextTaskID(state, MissionStrike)
		for _, aircraft := range returning[taskID] {
			aircraft.TaskID = rtbID
		}
		events = append(events, EngineEvent{Type: "attack_member_rtb", Audience: returning[taskID][0].Faction, Text: fmt.Sprintf("%s燃油不足，脱离攻击编队单独返航（%d个算子）", formationComposition(carrierModels(returning[taskID])), len(returning[taskID]))})
	}
	return events
}

// splitGroundReturningMembers 是机场侧的同一套拆分：返航的机场算子不再挂在攻击编队里。
func splitGroundReturningMembers(state *State) []EngineEvent {
	events := []EngineEvent{}
	if state == nil {
		return events
	}
	returning := map[string][]*AirfieldAirUnit{}
	for _, id := range sortedGroundAirIDsAll(*state) {
		aircraft := state.GroundAir[id]
		if aircraft == nil || aircraft.TaskID == "" || aircraft.TaskKind != "attack" || !aircraft.Returning || !airfieldAirborne(aircraft) {
			continue
		}
		returning[aircraft.TaskID] = append(returning[aircraft.TaskID], aircraft)
	}
	taskIDs := make([]string, 0, len(returning))
	for taskID := range returning {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		active := 0
		for _, member := range groundTaskAircraft(*state, taskID) {
			if member != nil && !member.Returning {
				active++
			}
		}
		if active == 0 {
			continue
		}
		rtbID := nextTaskID(state, MissionStrike)
		for _, aircraft := range returning[taskID] {
			aircraft.TaskID = rtbID
		}
		events = append(events, EngineEvent{Type: "attack_member_rtb", Audience: returning[taskID][0].Faction, Text: fmt.Sprintf("%s燃油不足，脱离打击编队单独返航（%d个机场算子）", formationComposition(groundModels(returning[taskID])), len(returning[taskID]))})
	}
	return events
}

// formationComposition 把成员按机型汇总成"SBD×2 / F4F×1"：
// 战报只说"3个算子"玩家没法核对，拆走的是哪几架得写清楚。
func formationComposition(models []string) string {
	counts := map[string]int{}
	order := []string{}
	for _, model := range models {
		if counts[model] == 0 {
			order = append(order, model)
		}
		counts[model]++
	}
	sort.Strings(order)
	parts := make([]string, 0, len(order))
	for _, model := range order {
		parts = append(parts, fmt.Sprintf("%s×%d", model, counts[model]))
	}
	return strings.Join(parts, " / ")
}

func carrierModels(members []*AirUnit) []string {
	models := make([]string, 0, len(members))
	for _, member := range members {
		models = append(models, member.Model)
	}
	return models
}

func groundModels(members []*AirfieldAirUnit) []string {
	models := make([]string, 0, len(members))
	for _, member := range members {
		models = append(models, member.Model)
	}
	return models
}
