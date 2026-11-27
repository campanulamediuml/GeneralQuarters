package game

import (
	"fmt"
	"sort"
)

// —— 舰队拆分（反潜支队） ——
//
// 触发反潜战的舰队可以把带深水炸弹的船拆成一支"反潜支队"留在原地继续反潜，
// 主队照常执行本回合的移动命令。它和整编（MERGE_FLEET / REORGANIZE_HEX）刻意分开：
//   - 整编会给新编队挂 3 回合移动锁，拆分不挂；
//   - 整编可能跨舰队重组，拆分只做一件事：把 ASW 船分出去；
//   - 拆分不动坐标、朝向、冷却和停留计时，主队该走走、该留留。

// validateSplitFleet 校验拆分：舰队里既要有反潜舰，也要留下非反潜舰。
func validateSplitFleet(state State, faction string, order Order) error {
	if InDeployPhase(state) {
		return fmt.Errorf("部署阶段请用舰队整编调整编成")
	}
	unit := state.Units[order.UnitID]
	if unit == nil || unit.Faction != faction {
		return fmt.Errorf("舰队不存在或不属于当前玩家")
	}
	fleetID := FleetKey(unit)
	asw, rest := splitFleetCounts(state, fleetID)
	if asw == 0 {
		return fmt.Errorf("舰队里没有装备深水炸弹的反潜舰，无法拆分")
	}
	if rest == 0 {
		return fmt.Errorf("不能把整支舰队都拆成反潜支队：至少留下一艘船")
	}
	return nil
}

// splitFleetCounts 数一支舰队里反潜舰和其余舰船的数量。
// 反潜舰的判定和潜艇战一致：装备深水炸弹且剩余数量不为 0。
func splitFleetCounts(state State, fleetID string) (asw, rest int) {
	for _, unit := range state.Units {
		if unit == nil || FleetKey(unit) != fleetID || unit.Strength <= 0 {
			continue
		}
		if unit.Card.DepthCharges > 0 {
			asw++
			continue
		}
		rest++
	}
	return asw, rest
}

// ApplySplitFleet 把舰队里的反潜舰拆成新支队，留在同一格。
// 不挂移动锁、不改冷却/停留计时，主队和支队都按原计划行动。
func ApplySplitFleet(state *State, order Order) []EngineEvent {
	leader := state.Units[order.UnitID]
	if leader == nil {
		return nil
	}
	oldFleetID := FleetKey(leader)
	newFleetID := oldFleetID + "-asw"
	for index := 2; ; index++ {
		exists := false
		for _, unit := range state.Units {
			if unit != nil && FleetKey(unit) == newFleetID {
				exists = true
				break
			}
		}
		if !exists {
			break
		}
		newFleetID = fmt.Sprintf("%s-asw%d", oldFleetID, index)
	}
	oldName := leader.FleetName
	if oldName == "" {
		oldName = leader.Name
	}
	newName := oldName + "反潜支队"

	splitIDs := []string{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Faction != leader.Faction || FleetKey(unit) != oldFleetID {
			continue
		}
		if unit.Card.DepthCharges <= 0 {
			continue
		}
		unit.FleetID = newFleetID
		unit.FleetName = newName
		splitIDs = append(splitIDs, unit.ID)
	}
	if len(splitIDs) == 0 {
		return nil
	}
	sort.Strings(splitIDs)
	// 正在被这支舰队追踪的潜艇改挂到新支队：反潜舰留在原地，追踪不该断。
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if sub == nil || !sub.Discovered || sub.TrackedFleetID != oldFleetID {
			continue
		}
		sub.TrackedFleetID = newFleetID
	}
	return []EngineEvent{{Type: "fleet_split", Audience: leader.Faction, Text: fmt.Sprintf("%s分出%d艘反潜舰组成%s，留在%s继续反潜；主队照常行动", oldName, len(splitIDs), newName, leader.Coord)}}
}
