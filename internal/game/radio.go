package game

import (
	"fmt"
	"sort"
	"strings"
)

// —— 无线电命令（规则书 597–602 行）——
//
// 在空中的机队可以通过无线电改令或返航：基础成功率 100%，
// 距离最近的友军舰队或机场每 1hex 美军 -10%、日军 -20%，距离 0 无惩罚；
// 没有轰炸机的编队距离惩罚翻倍；每回合只能进行一次无线电通信，失败时保留原任务和原目标。

// RadioKind 是一次无线电命令的意图。
type RadioKind string

const (
	RadioRecall   RadioKind = "RECALL"   // 召回：编队立即返航
	RadioRetask   RadioKind = "RETASK"   // 改令：换成另一种航空任务（CAP / 侦察 / 空袭）
	RadioRetarget RadioKind = "RETARGET" // 改目标：保留任务，换一个目标 hex
)

// radioKindFromOrder 把命令类型映射成无线电意图。
func radioKindFromOrder(orderType string) (RadioKind, bool) {
	switch orderType {
	case "RECALL_AIR":
		return RadioRecall, true
	case "RETASK_AIR":
		return RadioRetask, true
	case "RETARGET_AIR":
		return RadioRetarget, true
	}
	return "", false
}

func isRadioOrder(orderType string) bool {
	_, ok := radioKindFromOrder(orderType)
	return ok
}

// IsRadioOrder 从服务端转调：服务端要在提交阶段就拦下重复的无线电命令。
func IsRadioOrder(orderType string) bool {
	return isRadioOrder(orderType)
}

// radioLossPerHex 返回每 1hex 的距离惩罚：日军没有无线电信标，衰减翻倍。
func radioLossPerHex(state State, faction string) float64 {
	return rulesFor(state, faction).RadioPenaltyPerHex
}

// radioRange 返回编队到最近友军舰队或机场的距离（0 表示同格）。
func radioRange(state State, faction, coord string) int {
	best := -1
	for _, unit := range state.Units {
		if unit == nil || unit.Faction != faction {
			continue
		}
		distance := Distance(unit.Coord, coord)
		if best < 0 || distance < best {
			best = distance
		}
	}
	for _, airfield := range state.Airfields {
		if airfield == nil || airfield.Faction != faction {
			continue
		}
		distance := Distance(airfield.Coord, coord)
		if best < 0 || distance < best {
			best = distance
		}
	}
	return best
}

// radioHasBomber 判断编队里是否还有攻击机（没有的话距离惩罚翻倍）。
func radioHasBomber(aircraftList []*AirUnit) bool {
	for _, aircraft := range aircraftList {
		if aircraft == nil {
			continue
		}
		if aircraft.Kind != "fighter" {
			return true
		}
	}
	return false
}

// radioSuccessChance 按规则书计算一次无线电通信的成功率。
func radioSuccessChance(state State, faction, coord string, aircraftList []*AirUnit) (float64, int) {
	distance := radioRange(state, faction, coord)
	if distance < 0 {
		distance = 0
	}
	penalty := radioLossPerHex(state, faction)
	if !radioHasBomber(aircraftList) {
		penalty *= 2
	}
	chance := 100 - float64(distance)*penalty
	if chance < 0 {
		chance = 0
	}
	if chance > 100 {
		chance = 100
	}
	return chance, distance
}

// airTaskMembers 收集某个任务当前在空中的舰载机算子。
func airTaskMembers(state State, taskID string) []*AirUnit {
	members := []*AirUnit{}
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.TaskID != taskID {
			continue
		}
		if airborne(aircraft) {
			members = append(members, aircraft)
		}
	}
	return members
}

func sortedAirUnitIDs(state State) []string {
	ids := make([]string, 0, len(state.Aircraft))
	for id := range state.Aircraft {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// groundTaskMembers 收集某个任务当前在空中的机场飞机算子。
func groundTaskMembers(state State, taskID string) []*AirfieldAirUnit {
	members := []*AirfieldAirUnit{}
	for _, id := range sortedGroundAirIDsAll(state) {
		aircraft := state.GroundAir[id]
		if aircraft == nil || aircraft.TaskID != taskID {
			continue
		}
		if airfieldAirborne(aircraft) {
			members = append(members, aircraft)
		}
	}
	return members
}

func sortedGroundAirIDsAll(state State) []string {
	ids := make([]string, 0, len(state.GroundAir))
	for id := range state.GroundAir {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// validateRadioOrder 校验无线电改令本身是否合法（不含概率判定）。
func validateRadioOrder(state State, faction string, order Order) error {
	if order.TaskID == "" {
		return fmt.Errorf("无线电命令必须指定航空任务")
	}
	kind, _ := radioKindFromOrder(order.Type)
	carrierMembers := airTaskMembers(state, order.TaskID)
	groundMembers := groundTaskMembers(state, order.TaskID)
	if len(carrierMembers) == 0 && len(groundMembers) == 0 {
		return fmt.Errorf("该航空任务没有在空中执行任务的编队")
	}
	// 每个空中编队每回合只有一次无线电机会，发出后不论成败都用掉（规则书 601 行）。
	if _, used := state.RadioUsed[order.TaskID]; used {
		return fmt.Errorf("该编队本回合已经发过无线电，不能再通信")
	}
	// 编队类型决定能用哪种无线电操作：
	//   CAP 编队只会被动自动前出，无线电只能 RTB，不能改任务；
	//   只有打击编队和转场编队才可以用无线电改目标或直接召回。
	formationKind := radioFormationKind(state, order.TaskID, carrierMembers, groundMembers)
	switch formationKind {
	case "cap":
		if kind != RadioRecall {
			return fmt.Errorf("CAP编队只能通过无线电返航（RTB），不能改任务或改目标")
		}
	case "recon":
		// 侦察组以前完全不能通信：飞机不飞完不落地，侦察组名额就一直被占着，
		// 想换个方向搜只能干等它自己油尽返航。现在允许无线电召回（不可改任务/改目标）。
		if kind != RadioRecall {
			return fmt.Errorf("侦察编队只能通过无线电召回返航（RTB），不能改任务或改目标")
		}
	case "attack", "transfer":
		// 改目标和召回都允许，见下方分支。
	default:
		return fmt.Errorf("该编队类型不支持无线电通信")
	}
	for _, aircraft := range airTaskMembersEveryStatus(state, order.TaskID) {
		if aircraft != nil && aircraft.Faction != faction {
			return fmt.Errorf("该航空任务不属于当前玩家")
		}
	}
	for _, aircraft := range groundTaskMembersEveryStatus(state, order.TaskID) {
		if aircraft != nil && aircraft.Faction != faction {
			return fmt.Errorf("该航空任务不属于当前玩家")
		}
	}
	if kind == RadioRecall {
		return nil
	}
	if kind == RadioRetask {
		// 改令只能在打击/转场之间切换：CAP 和侦察都是起飞前就编好的任务，不能在空中临时切换。
		switch strings.ToLower(order.TaskKind) {
		case "attack", "transfer":
			return nil
		case "cap":
			return fmt.Errorf("不能在空里把编队改成CAP，CAP必须在起飞前编入")
		case "recon":
			return fmt.Errorf("不能在空里把编队改成侦察任务")
		}
		return fmt.Errorf("改令目标必须是空袭或转场任务")
	}
	if !inBounds(state.Map, order.Coord) {
		return fmt.Errorf("新目标坐标不在地图内")
	}
	// 改目标时可以点名"同格里打哪支舰队"，点名的必须是已侦测到的敌方舰队。
	if formationKind == "attack" && order.TargetFleetID != "" {
		if err := ValidateStrikeFleetTarget(state, faction, order.TargetFleetID, order.Coord); err != nil {
			return err
		}
	}
	return nil
}

// radioFormationKind 返回编队当前的任务类型（cap / recon / attack / transfer），用于限定可用的无线电操作。
func radioFormationKind(state State, taskID string, carrierMembers []*AirUnit, groundMembers []*AirfieldAirUnit) string {
	for _, aircraft := range carrierMembers {
		if aircraft.TaskKind != "" {
			return aircraft.TaskKind
		}
		if aircraft.Status == "cap" {
			return "cap"
		}
	}
	for _, aircraft := range groundMembers {
		if aircraft.TaskKind != "" {
			return aircraft.TaskKind
		}
		if aircraft.Status == "cap" {
			return "cap"
		}
	}
	return ""
}

// groundTaskMembersEveryStatus 找机场任务里所有还活着的算子（用来校验归属）。
func groundTaskMembersEveryStatus(state State, taskID string) []*AirfieldAirUnit {
	members := []*AirfieldAirUnit{}
	for _, id := range sortedGroundAirIDsAll(state) {
		if aircraft := state.GroundAir[id]; aircraft != nil && aircraft.TaskID == taskID {
			members = append(members, aircraft)
		}
	}
	return members
}

// airTaskMembersEveryStatus 找任务里所有还活着的算子（用来校验归属）。
func airTaskMembersEveryStatus(state State, taskID string) []*AirUnit {
	members := []*AirUnit{}
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft != nil && aircraft.TaskID == taskID {
			members = append(members, aircraft)
		}
	}
	return members
}

// ApplyRadioOrder 结算一次无线电通信，返回事件（结果写进事件文本，前端照常显示战报）。
func ApplyRadioOrder(state *State, faction string, order Order) []EngineEvent {
	if validateRadioOrder(*state, faction, order) != nil {
		return nil
	}
	kind, _ := radioKindFromOrder(order.Type)
	carrierMembers := airTaskMembers(*state, order.TaskID)
	groundMembers := groundTaskMembers(*state, order.TaskID)
	coord := ""
	if len(carrierMembers) > 0 {
		coord = carrierMembers[0].Coord
	} else if len(groundMembers) > 0 {
		coord = groundMembers[0].Coord
	}
	list := carrierMembers
	if len(list) == 0 {
		for _, aircraft := range groundMembers {
			list = append(list, airUnitFromGround(aircraft))
		}
	}
	chance, distance := radioSuccessChance(*state, faction, coord, list)
	label := map[RadioKind]string{RadioRecall: "召回", RadioRetask: "改令", RadioRetarget: "改目标"}[kind]
	// 一经发射就占掉这个编队本回合的机会：失败也算用掉，不能再发第二次。
	if state.RadioUsed == nil {
		state.RadioUsed = map[string]string{}
	}
	state.RadioUsed[order.TaskID] = faction
	if probabilityRoll(&state.RNGState) >= chance {
		return []EngineEvent{{Type: "radio_failed", Audience: faction, Text: fmt.Sprintf("%s无线电通信失败（%s，最近友军 %d hex，成功率 %.0f%%），编队保持原任务，本回合该编队不能再次通信", coord, label, distance, chance)}}
	}
	events := []EngineEvent{}
	switch kind {
	case RadioRecall:
		for _, aircraft := range carrierMembers {
			recallCarrierAircraft(aircraft)
		}
		for _, aircraft := range groundMembers {
			recallGroundAircraft(aircraft)
		}
		events = append(events, EngineEvent{Type: "radio_recall", Audience: faction, Text: fmt.Sprintf("%s无线电召回成功（最近友军 %d hex，成功率 %.0f%%），编队返航", coord, distance, chance)})
	case RadioRetask:
		events = append(events, retaskRadio(state, faction, order, carrierMembers, groundMembers, distance, chance)...)
	case RadioRetarget:
		for _, aircraft := range carrierMembers {
			if aircraft.Returning {
				continue
			}
			setStrikeTarget(state, aircraft, faction, order.Coord)
			// 玩家在目标清单里点的是哪支舰队就认哪支：同格几支舰队时不能按情报顺序瞎认。
			// 转场编队的目标舰队是自家船，走的是另一套（friendly），这里只管打击编队。
			if aircraft.TaskKind == "attack" {
				applyStrikeFleet(*state, aircraft, faction, order.TargetFleetID)
			}
			aircraft.Engaged = false
		}
		for _, aircraft := range groundMembers {
			if aircraft.Returning {
				continue
			}
			setGroundStrikeTarget(state, aircraft, faction, order.Coord)
			if aircraft.TaskKind == "attack" && order.TargetFleetID != "" {
				aircraft.TargetFleetID = order.TargetFleetID
				if coord, ok := strikeFleetCoord(*state, faction, order.TargetFleetID); ok {
					aircraft.TargetCoord = coord
				}
			}
		}
		events = append(events, EngineEvent{Type: "radio_retarget", Audience: faction, Text: fmt.Sprintf("%s无线电改目标成功（最近友军 %d hex，成功率 %.0f%%），新目标 %s", coord, distance, chance, order.Coord)})
	}
	return events
}

func airUnitFromGround(aircraft *AirfieldAirUnit) *AirUnit {
	return &AirUnit{ID: aircraft.ID, Faction: aircraft.Faction, Kind: aircraft.Kind, Returning: aircraft.Returning, Status: aircraft.Status}
}

// recallCarrierAircraft 让编队返航，并取消自动循环（主动回收的飞机会取消循环任务，规则书 190 行）。
func recallCarrierAircraft(aircraft *AirUnit) {
	aircraft.Returning = true
	disarmTaskCycle(aircraft)
	aircraft.Engaged = false
	aircraft.TargetCoord = ""
}

func recallGroundAircraft(aircraft *AirfieldAirUnit) {
	aircraft.Returning = true
	aircraft.TargetCoord = ""
}

// retaskRadio 把编队换成新任务：CAP / 侦察 / 空袭 / 转场。
func retaskRadio(state *State, faction string, order Order, carrierMembers []*AirUnit, groundMembers []*AirfieldAirUnit, distance int, chance float64) []EngineEvent {
	kind := strings.ToLower(strings.TrimSpace(order.TaskKind))
	events := []EngineEvent{}
	if len(carrierMembers) > 0 {
		target := order.Coord
		if kind == "cap" && target == "" {
			target = carrierMembers[0].Coord
		}
		mission := MissionKind(kind)
		for _, aircraft := range carrierMembers {
			// 已经在返航的成员收不到改令：以前这里无条件把 Returning 清掉，
			// 一架掉头回家的飞机会被"改令 → 打击"重新塞回打击编队。
			if aircraft.Returning {
				continue
			}
			aircraft.TaskKind = kind
			aircraft.Returning = false
			aircraft.Engaged = false
			switch mission {
			case MissionCap:
				aircraft.Status = "cap"
				assignCapMission(aircraft, target)
				enableTaskCycle(aircraft, string(MissionCap))
			case MissionRecon:
				aircraft.Status = "recon"
				// 侦察半径不可设置：命令里没有这个字段，一律按机型上限。
				assignReconMission(aircraft, order.Heading, order.SectorWidth)
				disarmTaskCycle(aircraft)
				aircraft.TargetCoord = ""
			default:
				aircraft.Status = "attack"
				aircraft.TaskKind = string(MissionStrike)
				disarmTaskCycle(aircraft)
				assignStrikeMission(state, aircraft, target)
			}
		}
	}
	mission := MissionKind(kind)
	for _, aircraft := range groundMembers {
		if aircraft.Returning {
			continue
		}
		aircraft.TaskKind = kind
		aircraft.Returning = false
		switch mission {
		case MissionCap:
			aircraft.Status = "cap"
			assignGroundCapMission(aircraft, order.Coord)
		case MissionRecon:
			aircraft.Status = "recon"
			// 侦察半径不可设置：命令里没有这个字段，一律按机型上限。
			assignGroundReconMission(aircraft, order.Heading, order.SectorWidth)
			aircraft.TargetCoord = ""
		default:
			aircraft.TaskKind = string(MissionStrike)
			aircraft.Status = "attack"
			assignGroundStrikeMission(state, aircraft, order.Coord)
		}
	}
	name := map[string]string{"cap": "CAP", "recon": "侦察", "attack": "空袭", "transfer": "转场"}[kind]
	events = append(events, EngineEvent{Type: "radio_retask", Audience: faction, Text: fmt.Sprintf("无线电改令成功（最近友军 %d hex，成功率 %.0f%%），编队转为%s任务", distance, chance, name)})
	return events
}

// VisibleRadioUsage 告诉前端本回合已经把无线电机会用掉的编队（任务 ID）。
func VisibleRadioUsage(state State, faction string) []string {
	used := []string{}
	for taskID, owner := range state.RadioUsed {
		if owner == faction {
			used = append(used, taskID)
		}
	}
	sort.Strings(used)
	return used
}

// RadioTaskStatus 是前端显示"这个编队本回合还能不能发无线电"所需的最小信息。
// 规则书 601 行：每个空中编队每回合各一次机会，发出即消耗（失败也消耗）。
type RadioTaskStatus struct {
	TaskID   string `json:"task_id"`
	Kind     string `json:"kind"`
	Coord    string `json:"coord"`
	Airborne int    `json:"airborne"`
	Used     bool   `json:"used"`
	Chance   int    `json:"chance"`
	Distance int    `json:"distance"`
}

// AirborneRadioTasks 列出本方所有还在空中、可以有无线电机会的编队。
func AirborneRadioTasks(state State, faction string) []RadioTaskStatus {
	result := []RadioTaskStatus{}
	taskIDs := map[string]bool{}
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Faction != faction || aircraft.TaskID == "" || !airborne(aircraft) {
			continue
		}
		taskIDs[aircraft.TaskID] = true
	}
	for _, id := range sortedGroundAirIDsAll(state) {
		aircraft := state.GroundAir[id]
		if aircraft == nil || aircraft.Faction != faction || aircraft.TaskID == "" || !airfieldAirborne(aircraft) {
			continue
		}
		taskIDs[aircraft.TaskID] = true
	}
	ordered := make([]string, 0, len(taskIDs))
	for taskID := range taskIDs {
		ordered = append(ordered, taskID)
	}
	sort.Strings(ordered)
	for _, taskID := range ordered {
		members := airTaskMembers(state, taskID)
		kind, coord := "", ""
		if len(members) > 0 {
			kind = members[0].TaskKind
			coord = members[0].Coord
		} else {
			ground := groundTaskMembers(state, taskID)
			if len(ground) == 0 {
				continue
			}
			kind = ground[0].TaskKind
			coord = ground[0].Coord
		}
		chance, distance := radioSuccessChance(state, faction, coord, members)
		_, used := state.RadioUsed[taskID]
		result = append(result, RadioTaskStatus{TaskID: taskID, Kind: kind, Coord: coord, Airborne: len(members), Used: used, Chance: int(chance + 0.5), Distance: distance})
	}
	return result
}
