package game

import (
	"fmt"
	"sort"
	"strings"
)

func EnsureAviation(state *State) {
	for _, unit := range state.Units {
		if unit.Card.HangarCapacity <= 0 {
			unit.Deck = nil
			continue
		}
		length := unit.Card.DeckCapacity
		if length <= 0 {
			length = 1
		}
		count := unit.Card.ElevatorCount
		if count <= 0 {
			count = 1
		}
		if unit.Deck == nil {
			unit.Deck = &FlightDeck{Elevators: []*AircraftElevator{}}
		}
		if unit.Deck.Length <= 0 {
			unit.Deck.Length = length
		}
		unit.Card.DeckCapacity = unit.Deck.Length
		kind := "elevator"
		capacity := 2
		if !IsCarrier(unit) {
			kind = "crane"
			capacity = 1
		}
		damage := EnsureDamage(unit)
		for index := 0; index < count; index++ {
			id := fmt.Sprintf("%s-elevator-%d", unit.ID, index+1)
			if index >= len(unit.Deck.Elevators) {
				unit.Deck.Elevators = append(unit.Deck.Elevators, &AircraftElevator{ID: id, AircraftIDs: []string{}})
			}
			elevator := unit.Deck.Elevators[index]
			elevator.ID = id
			elevator.Kind = kind
			if elevator.Capacity <= 0 {
				elevator.Capacity = capacity
			}
			if elevator.Status == "" {
				elevator.Status = "idle"
			}
			if elevator.AircraftIDs == nil {
				elevator.AircraftIDs = []string{}
			}
			if index < damage.ElevatorDamage {
				elevator.Status = "damaged"
				elevator.TaskID = ""
				elevator.AircraftIDs = []string{}
			} else if elevator.Status == "damaged" {
				elevator.Status = "idle"
			}
		}
		unit.Card.ElevatorCount = len(unit.Deck.Elevators)
	}
	for _, aircraft := range state.Aircraft {
		if aircraft.Status != "elevator" {
			aircraft.ElevatorID = ""
		}
	}
}

func AviationAvailable(unit *Unit) bool {
	if unit == nil || unit.Deck == nil {
		return false
	}
	damage := EnsureDamage(unit)
	if damage.Disabled || damage.Tilt || damage.Fire != "" || damage.DeckDamaged || damage.DeckDestroyed {
		return false
	}
	available := 0
	for _, elevator := range unit.Deck.Elevators {
		if elevator.Status != "damaged" {
			available++
		}
	}
	return available > 0
}

func airOrderIDs(order Order) []string {
	ids := append([]string(nil), order.AircraftIDs...)
	if len(ids) == 0 && order.AircraftID != "" {
		ids = []string{order.AircraftID}
	}
	sort.Strings(ids)
	return ids
}

func aircraftForOrder(state State, faction string, order Order) ([]*AirUnit, *Unit, error) {
	EnsureAviation(&state)
	ids := airOrderIDs(order)
	if len(ids) == 0 || len(ids) > 20 {
		return nil, nil, fmt.Errorf("航空命令必须包含1到20个航空算子")
	}
	seen := map[string]bool{}
	aircraftList := make([]*AirUnit, 0, len(ids))
	carrierID := ""
	for _, id := range ids {
		if seen[id] {
			return nil, nil, fmt.Errorf("航空命令包含重复算子")
		}
		seen[id] = true
		aircraft := state.Aircraft[id]
		if aircraft == nil && order.Type == "PRIORITIZE_AIR_TASK" && len(ids) == 1 {
			for _, candidate := range state.Aircraft {
				if candidate.TaskID == id && candidate.Faction == faction {
					aircraft = candidate
					break
				}
			}
		}
		if aircraft == nil || aircraft.Faction != faction {
			return nil, nil, fmt.Errorf("航空算子不存在或不属于当前玩家")
		}
		if carrierID == "" {
			carrierID = aircraft.CarrierID
		} else if order.Type != "LAUNCH_STRIKE" && aircraft.CarrierID != carrierID {
			return nil, nil, fmt.Errorf("该航空命令必须使用同一母舰的算子")
		}
		aircraftList = append(aircraftList, aircraft)
	}
	carrier := state.Units[carrierID]
	if carrier == nil || carrier.Faction != faction || !AviationAvailable(carrier) {
		return nil, nil, fmt.Errorf("母舰不存在或损伤状态无法执行航空作业")
	}
	return aircraftList, carrier, nil
}

func ValidateAirOrder(state State, faction string, order Order) error {
	aircraftList, carrier, err := aircraftForOrder(state, faction, order)
	if err != nil {
		return err
	}
	switch order.Type {
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE":
		status := aircraftList[0].Status
		if status != "hangar" && status != "service" && status != "deck" && status != "ready" {
			return fmt.Errorf("只有机库、整备中或已完成提升的算子可以分配任务")
		}
		definitionID := aircraftList[0].DefinitionID
		fleetID := FleetKey(carrier)
		for _, aircraft := range aircraftList {
			if aircraft.Status != status {
				return fmt.Errorf("同批分配的算子必须处于相同作业阶段")
			}
			if aircraft.TaskID != "" {
				return fmt.Errorf("算子已经属于其他任务，请先取消原任务")
			}
			ship := state.Units[aircraft.CarrierID]
			if ship == nil || ship.Faction != faction || !AviationAvailable(ship) {
				return fmt.Errorf("航空算子的母舰损伤状态无法执行航空作业")
			}
			if order.Type != "LAUNCH_STRIKE" && aircraft.DefinitionID != definitionID {
				return fmt.Errorf("CAP和侦察任务必须使用同型算子")
			}
			if order.Type == "LAUNCH_CAP" && aircraft.Kind != "fighter" {
				return fmt.Errorf("只有战斗机可以执行CAP")
			}
			if order.Type == "LAUNCH_RECON" && !canSearch(*aircraft) {
				return fmt.Errorf("战斗机不能执行扇面搜索，TBF、SB2C和侦察机可以执行搜索")
			}
			if order.Type == "LAUNCH_STRIKE" {
				if aircraft.Seaplane || (aircraft.Kind != "dive_bomber" && aircraft.Kind != "torpedo_bomber") {
					return fmt.Errorf("攻击编队只能由舰载俯冲轰炸机和鱼雷机组成")
				}
				if FleetKey(ship) != fleetID {
					return fmt.Errorf("攻击编队的母舰必须属于同一舰队")
				}
			}
		}
		if order.Type == "LAUNCH_CAP" && !IsCarrier(carrier) {
			return fmt.Errorf("CAP舰载机只能从航母起飞")
		}
		if order.Type == "LAUNCH_RECON" {
			order.Heading = strings.ToUpper(order.Heading)
			if !ValidReconHeading(order.Heading) {
				return fmt.Errorf("侦察航向必须是E、SE、SW、W、NW或NE")
			}
			if order.SectorWidth != 60 && order.SectorWidth != 120 {
				return fmt.Errorf("侦察扇面只能选择60度或120度")
			}
			limit := ReconRadiusLimit(*aircraftList[0])
			if order.ReconRadius < 0 || order.ReconRadius > limit {
				return fmt.Errorf("侦察半径不能超过%d hex", limit)
			}
			if !aircraftList[0].Seaplane && !IsCarrier(carrier) {
				return fmt.Errorf("舰载机只能从航母起飞")
			}
		}
		if order.TaskID != "" {
			members := taskAircraft(state, order.TaskID)
			if len(members) == 0 || members[0].Faction != faction {
				return fmt.Errorf("要加入的航空任务不存在")
			}
			if members[0].TaskKind != "recon" && members[0].TaskKind != "attack" {
				return fmt.Errorf("只有侦察组和攻击编队可以接受新算子")
			}
			if order.Type == "LAUNCH_RECON" && members[0].TaskKind != "recon" {
				return fmt.Errorf("要加入的侦察组不存在")
			}
			if order.Type == "LAUNCH_RECON" && FleetKey(state.Units[members[0].CarrierID]) != fleetID {
				return fmt.Errorf("只能加入同一舰队的侦察组")
			}
			if order.Type == "LAUNCH_STRIKE" {
				if members[0].TaskKind != "attack" {
					return fmt.Errorf("要加入的攻击编队不存在")
				}
				if FleetKey(state.Units[members[0].CarrierID]) != fleetID {
					return fmt.Errorf("只能加入同一舰队的攻击编队")
				}
				for _, member := range members {
					if member.Status == "attack" {
						return fmt.Errorf("攻击编队已经离场，不能继续加入飞机")
					}
				}
			}
			if order.Type == "LAUNCH_CAP" {
				return fmt.Errorf("CAP算子各自独立，不能加入侦察组或攻击编队")
			}
		} else if order.Type == "LAUNCH_RECON" {
			groups := map[string]bool{}
			for _, aircraft := range state.Aircraft {
				ship := state.Units[aircraft.CarrierID]
				if aircraft.Faction == faction && ship != nil && FleetKey(ship) == fleetID && aircraft.TaskKind == "recon" && aircraft.TaskID != "" {
					groups[aircraft.TaskID] = true
				}
			}
			if len(groups) >= 2 {
				return fmt.Errorf("最多同时保留两个侦察组，请把飞机加入已有任务栏")
			}
		}
		if order.Type == "LAUNCH_STRIKE" && order.TaskID == "" {
			if err := ValidateStrikeTarget(state, faction, order.Coord); err != nil {
				return err
			}
		}
		if status == "deck" || status == "ready" {
			return validateLaunchCapacity(state, aircraftList)
		}
		return nil
	case "PREPARE_AIR":
		return fmt.Errorf("整备和提升必须绑定航空任务，请把飞机拖入任务栏")
	case "RETURN_TO_HANGAR":
		for _, aircraft := range aircraftList {
			if aircraft.Status != "service" && aircraft.Status != "deck" && aircraft.Status != "ready" {
				return fmt.Errorf("升降机上的算子不能中途撤回，甲板回收算子必须等待下降完成")
			}
			if (aircraft.Status == "deck" || aircraft.Status == "ready") && carrier.Deck.Operation != "" {
				return fmt.Errorf("该舰甲板正在起飞或回收，不能同时安排下降")
			}
		}
		return nil
	case "RECOVER_AIR":
		if carrier.Deck.Operation != "" {
			return fmt.Errorf("该舰甲板本回合已经进行其他作业，不能再降落")
		}
		for _, aircraft := range aircraftList {
			if aircraft.Status != "cap" {
				return fmt.Errorf("只有空中的CAP算子可以主动回收，侦察和攻击算子需要完成任务后返航")
			}
		}
		if deckUsed(state, carrier.ID)+len(aircraftList) > carrier.Deck.Length {
			return fmt.Errorf("飞行甲板没有足够空间接收降落算子")
		}
		return nil
	case "PRIORITIZE_AIR_TASK":
		if len(aircraftList) != 1 || aircraftList[0].TaskID == "" {
			return fmt.Errorf("只有已分配的航空任务可以调整优先级")
		}
		return nil
	default:
		return fmt.Errorf("不支持的航空命令")
	}
}

func deckUsed(state State, carrierID string) int {
	used := 0
	for _, aircraft := range state.Aircraft {
		if aircraft.CarrierID == carrierID && (aircraft.Status == "deck" || aircraft.Status == "ready" || aircraft.Status == "recovery") {
			used++
		}
	}
	return used
}

func validateLaunchCapacity(state State, aircraftList []*AirUnit) error {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range aircraftList {
		groups[aircraft.CarrierID] = append(groups[aircraft.CarrierID], aircraft)
	}
	for carrierID, group := range groups {
		carrier := state.Units[carrierID]
		if carrier == nil || carrier.Deck == nil || carrier.Deck.Length <= 0 || !AviationAvailable(carrier) {
			return fmt.Errorf("母舰没有可用甲板或航空设施受损")
		}
		if carrier.Deck.Operation == "recover" {
			return fmt.Errorf("%s正在回收飞机，不能同时起飞", carrier.Name)
		}
		used := 0
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID == carrierID && aircraft.Launched {
				used++
			}
		}
		if used+len(group) > carrier.Deck.Length {
			return fmt.Errorf("%s本回合甲板起飞容量不足", carrier.Name)
		}
	}
	return nil
}

func nextAirTaskPriority(state *State, faction string) int {
	priority := 1
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == faction && aircraft.TaskID != "" && aircraft.Priority >= priority {
			priority = aircraft.Priority + 1
		}
	}
	return priority
}

func newAirTaskID(state *State, aircraftList []*AirUnit) string {
	base := aircraftList[0].ID
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		if len(taskAircraft(*state, candidate)) == 0 {
			return candidate
		}
	}
}

func clearAirTask(aircraft *AirUnit) {
	aircraft.Priority = 0
	aircraft.TaskID = ""
	aircraft.TaskKind = ""
	aircraft.OriginCoord = ""
	aircraft.Coord = ""
	aircraft.TargetCoord = ""
	aircraft.Heading = ""
	aircraft.SectorWidth = 0
	aircraft.ReconRadius = 0
	aircraft.ReconDistance = 0
	aircraft.Returning = false
	aircraft.TrackingFleetID = ""
}

func configureAirTask(state *State, aircraftList []*AirUnit, order Order) {
	taskID := order.TaskID
	priority := 0
	var existingTask []*AirUnit
	if taskID == "" {
		taskID = newAirTaskID(state, aircraftList)
		priority = nextAirTaskPriority(state, aircraftList[0].Faction)
	} else {
		existingTask = taskAircraft(*state, taskID)
		priority = existingTask[0].Priority
	}
	taskKind := "cap"
	if order.Type == "LAUNCH_RECON" {
		taskKind = "recon"
	} else if order.Type == "LAUNCH_STRIKE" {
		taskKind = "attack"
	}
	if taskKind == "recon" && len(existingTask) > 0 {
		order.Heading = existingTask[0].Heading
		order.SectorWidth = existingTask[0].SectorWidth
		order.ReconRadius = existingTask[0].ReconRadius
	}
	if taskKind == "attack" && len(existingTask) > 0 {
		order.Coord = existingTask[0].TargetCoord
	}
	for _, aircraft := range aircraftList {
		clearAirTask(aircraft)
		aircraft.TaskID = taskID
		aircraft.TaskKind = taskKind
		aircraft.Priority = priority
		if taskKind == "recon" {
			aircraft.Heading = strings.ToUpper(order.Heading)
			aircraft.SectorWidth = order.SectorWidth
			aircraft.ReconRadius = order.ReconRadius
			if aircraft.ReconRadius == 0 {
				aircraft.ReconRadius = ReconRadiusLimit(*aircraft)
			}
		}
		if taskKind == "attack" {
			aircraft.TargetCoord = order.Coord
		}
	}
}

func taskAircraft(state State, taskID string) []*AirUnit {
	members := []*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskID == taskID {
			members = append(members, aircraft)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members
}

func airborne(aircraft *AirUnit) bool {
	return aircraft.Status == "cap" || aircraft.Status == "recon" || aircraft.Status == "attack_holding" || aircraft.Status == "attack"
}

func updateAttackFormation(state *State, taskID string) []EngineEvent {
	members := taskAircraft(*state, taskID)
	if len(members) == 0 {
		return nil
	}
	ready := true
	airborneCount := 0
	for _, aircraft := range members {
		if airborne(aircraft) {
			airborneCount++
		}
		if aircraft.Status != "attack_holding" && aircraft.Status != "attack" {
			ready = false
		}
	}
	if !ready {
		return []EngineEvent{{Type: "attack_assembling", Audience: members[0].Faction, Text: fmt.Sprintf("攻击编队%d/%d个算子已升空，先起飞的飞机在舰队上空等待", airborneCount, len(members))}}
	}
	for _, aircraft := range members {
		aircraft.Status = "attack"
	}
	return []EngineEvent{{Type: "attack_departed", Audience: members[0].Faction, Text: fmt.Sprintf("攻击编队%d个算子全部到齐，编队离开舰队", len(members))}}
}

func launchAirTask(state *State, aircraftList []*AirUnit) []EngineEvent {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range aircraftList {
		groups[aircraft.CarrierID] = append(groups[aircraft.CarrierID], aircraft)
	}
	carrierIDs := make([]string, 0, len(groups))
	for carrierID := range groups {
		carrierIDs = append(carrierIDs, carrierID)
	}
	sort.Strings(carrierIDs)
	events := []EngineEvent{}
	for _, carrierID := range carrierIDs {
		group := groups[carrierID]
		carrier := state.Units[carrierID]
		for _, aircraft := range group {
			aircraft.Launched = true
			aircraft.Status = aircraft.TaskKind
			if aircraft.TaskKind == "attack" {
				aircraft.Status = "attack_holding"
			}
			aircraft.ElevatorID = ""
			carrier.Card.Aircraft--
			if aircraft.TaskKind == "recon" || aircraft.TaskKind == "attack" {
				aircraft.OriginCoord = carrier.Coord
				aircraft.Coord = carrier.Coord
				aircraft.ReconDistance = 0
				aircraft.Returning = false
				aircraft.TrackingFleetID = ""
			}
		}
		carrier.Deck.Operation = "launch"
		leader := group[0]
		events = append(events, EngineEvent{Type: "air_launched", Audience: carrier.Faction, Text: fmt.Sprintf("%s通过飞行甲板放飞%d个%s算子", carrier.Name, len(group), leader.Model)})
	}
	if aircraftList[0].TaskKind == "attack" {
		events = append(events, updateAttackFormation(state, aircraftList[0].TaskID)...)
	}
	return events
}

func ApplyAirOrder(state *State, order Order) []EngineEvent {
	EnsureAviation(state)
	aircraftList, carrier, err := aircraftForOrder(*state, orderAircraftFaction(*state, order), order)
	if err != nil || len(aircraftList) == 0 || carrier == nil {
		return nil
	}
	switch order.Type {
	case "RETURN_TO_HANGAR":
		lowered := 0
		cancelled := 0
		for _, aircraft := range aircraftList {
			if aircraft.Status == "service" {
				aircraft.Status = "hangar"
				cancelled++
			} else {
				aircraft.Status = "recovery"
				lowered++
			}
			clearAirTask(aircraft)
		}
		if lowered > 0 {
			carrier.Deck.Operation = "recover"
		}
		return []EngineEvent{{Type: "air_returned_to_hangar", Audience: carrier.Faction, Text: fmt.Sprintf("%s取消%d个机库任务算子，%d个甲板算子等待升降机下降", carrier.Name, cancelled, lowered)}}
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE":
		status := aircraftList[0].Status
		configureAirTask(state, aircraftList, order)
		if status == "deck" || status == "ready" {
			return launchAirTask(state, aircraftList)
		}
		for _, aircraft := range aircraftList {
			aircraft.Status = "service"
		}
		return []EngineEvent{{Type: "air_task_assigned", Audience: carrier.Faction, Text: fmt.Sprintf("%d个算子加入%s任务并开始机库整备，后续提升由母舰升降机执行", len(aircraftList), aircraftList[0].TaskKind)}}
	case "RECOVER_AIR":
		for _, aircraft := range aircraftList {
			aircraft.Status = "recovery"
			aircraft.Launched = false
			clearAirTask(aircraft)
			carrier.Card.Aircraft++
		}
		carrier.Deck.Operation = "recover"
		return []EngineEvent{{Type: "air_recovered", Audience: carrier.Faction, Text: fmt.Sprintf("%d个CAP算子已降落在%s甲板，等待升降机下降", len(aircraftList), carrier.Name)}}
	case "PRIORITIZE_AIR_TASK":
		selected := aircraftList[0]
		selectedTask := selected.TaskID
		seen := map[string]bool{}
		for _, aircraft := range state.Aircraft {
			if aircraft.Faction == selected.Faction && aircraft.TaskID != "" && aircraft.TaskID != selectedTask {
				seen[aircraft.TaskID] = true
			}
		}
		for _, aircraft := range state.Aircraft {
			if aircraft.Faction != selected.Faction || aircraft.TaskID == "" {
				continue
			}
			if aircraft.TaskID == selectedTask {
				aircraft.Priority = 1
			} else if seen[aircraft.TaskID] {
				aircraft.Priority++
			}
		}
		return []EngineEvent{{Type: "air_task_prioritized", Audience: selected.Faction, Text: selected.Model + "的航空任务已调整为最高优先级"}}
	}
	return nil
}

func orderAircraftFaction(state State, order Order) string {
	for _, id := range airOrderIDs(order) {
		if aircraft := state.Aircraft[id]; aircraft != nil {
			return aircraft.Faction
		}
	}
	if order.Type == "PRIORITIZE_AIR_TASK" {
		for _, aircraft := range state.Aircraft {
			if aircraft.TaskID == order.AircraftID {
				return aircraft.Faction
			}
		}
	}
	return ""
}

func completeElevatorCycles(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, carrierID := range sortedUnitIDs(state.Units) {
		carrier := state.Units[carrierID]
		if carrier.Deck == nil {
			continue
		}
		for _, elevator := range carrier.Deck.Elevators {
			if elevator.Status != "lifting" && elevator.Status != "lowering" {
				continue
			}
			for _, id := range elevator.AircraftIDs {
				aircraft := state.Aircraft[id]
				if aircraft == nil {
					continue
				}
				if elevator.Status == "lifting" {
					aircraft.Status = "deck"
					if aircraft.Seaplane {
						aircraft.Status = "ready"
					}
				} else {
					aircraft.Status = "hangar"
				}
				aircraft.ElevatorID = ""
			}
			action := "提升至甲板"
			if elevator.Status == "lowering" {
				action = "下降至机库"
			}
			events = append(events, EngineEvent{Type: "elevator_completed", Audience: carrier.Faction, Text: fmt.Sprintf("%s的%s完成%s：%d个算子", carrier.Name, elevator.ID, action, len(elevator.AircraftIDs))})
			elevator.Status = "idle"
			elevator.TaskID = ""
			elevator.AircraftIDs = []string{}
		}
	}
	return events
}

func sortedTaskGroups(state State, filter func(*AirUnit) bool) [][]*AirUnit {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskID != "" && filter(aircraft) {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	taskIDs := make([]string, 0, len(groups))
	for taskID := range groups {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Slice(taskIDs, func(i, j int) bool {
		left, right := groups[taskIDs[i]][0], groups[taskIDs[j]][0]
		if left.Priority == right.Priority {
			return left.TaskID < right.TaskID
		}
		return left.Priority < right.Priority
	})
	result := make([][]*AirUnit, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		sort.Slice(groups[taskID], func(i, j int) bool { return groups[taskID][i].ID < groups[taskID][j].ID })
		result = append(result, groups[taskID])
	}
	return result
}

func launchReadyTasks(state *State) []EngineEvent {
	events := []EngineEvent{}
	groups := sortedTaskGroups(*state, func(aircraft *AirUnit) bool { return aircraft.Status == "deck" || aircraft.Status == "ready" })
	for _, taskMembers := range groups {
		byCarrier := map[string][]*AirUnit{}
		for _, aircraft := range taskMembers {
			byCarrier[aircraft.CarrierID] = append(byCarrier[aircraft.CarrierID], aircraft)
		}
		carrierIDs := make([]string, 0, len(byCarrier))
		for carrierID := range byCarrier {
			carrierIDs = append(carrierIDs, carrierID)
		}
		sort.Strings(carrierIDs)
		for _, carrierID := range carrierIDs {
			group := byCarrier[carrierID]
			if err := validateLaunchCapacity(*state, group); err != nil {
				events = append(events, EngineEvent{Type: "air_launch_delayed", Audience: group[0].Faction, Text: err.Error() + "，任务在甲板等待"})
				continue
			}
			events = append(events, launchAirTask(state, group)...)
		}
	}
	return events
}

func scheduleElevators(state *State) []EngineEvent {
	events := []EngineEvent{}
	taskGroups := sortedTaskGroups(*state, func(aircraft *AirUnit) bool { return aircraft.Status == "service" })
	for _, carrierID := range sortedUnitIDs(state.Units) {
		carrier := state.Units[carrierID]
		if !AviationAvailable(carrier) {
			continue
		}
		reserved := 0
		delayed := false
		for _, elevator := range carrier.Deck.Elevators {
			if elevator.Status != "idle" {
				continue
			}
			recovery := []*AirUnit{}
			for _, aircraft := range state.Aircraft {
				if aircraft.CarrierID == carrierID && aircraft.Status == "recovery" {
					recovery = append(recovery, aircraft)
				}
			}
			sort.Slice(recovery, func(i, j int) bool { return recovery[i].ID < recovery[j].ID })
			if len(recovery) > 0 {
				count := len(recovery)
				if count > elevator.Capacity {
					count = elevator.Capacity
				}
				selected := recovery[:count]
				elevator.Status = "lowering"
				elevator.TaskID = ""
				elevator.AircraftIDs = []string{}
				for _, aircraft := range selected {
					aircraft.Status = "elevator"
					aircraft.ElevatorID = elevator.ID
					elevator.AircraftIDs = append(elevator.AircraftIDs, aircraft.ID)
				}
				events = append(events, EngineEvent{Type: "elevator_started", Audience: carrier.Faction, Text: fmt.Sprintf("%s的%s开始下降%d个回收算子", carrier.Name, elevator.ID, len(selected))})
				continue
			}
			available := carrier.Deck.Length - deckUsed(*state, carrierID) - reserved
			if available <= 0 {
				delayed = true
				continue
			}
			var selectedTask []*AirUnit
			for _, taskMembers := range taskGroups {
				candidates := []*AirUnit{}
				for _, aircraft := range taskMembers {
					if aircraft.CarrierID == carrierID && aircraft.Status == "service" {
						candidates = append(candidates, aircraft)
					}
				}
				if len(candidates) > 0 {
					selectedTask = candidates
					break
				}
			}
			if len(selectedTask) == 0 {
				continue
			}
			count := len(selectedTask)
			if count > elevator.Capacity {
				count = elevator.Capacity
			}
			if count > available {
				count = available
			}
			selected := selectedTask[:count]
			elevator.Status = "lifting"
			elevator.TaskID = selected[0].TaskID
			elevator.AircraftIDs = []string{}
			for _, aircraft := range selected {
				aircraft.Status = "elevator"
				aircraft.ElevatorID = elevator.ID
				elevator.AircraftIDs = append(elevator.AircraftIDs, aircraft.ID)
			}
			reserved += len(selected)
			events = append(events, EngineEvent{Type: "elevator_started", Audience: carrier.Faction, Text: fmt.Sprintf("%s的%s开始提升%d个任务算子", carrier.Name, elevator.ID, len(selected))})
		}
		if delayed {
			for _, taskMembers := range taskGroups {
				for _, aircraft := range taskMembers {
					if aircraft.CarrierID == carrierID && aircraft.Status == "service" {
						events = append(events, EngineEvent{Type: "air_preparation_delayed", Audience: carrier.Faction, Text: carrier.Name + "甲板长度不足，整备完成的算子等待下一次提升"})
						delayed = false
						break
					}
				}
				if !delayed {
					break
				}
			}
		}
	}
	return events
}

func AdvanceAviation(state *State) []EngineEvent {
	EnsureAviation(state)
	for _, aircraft := range state.Aircraft {
		aircraft.Launched = false
	}
	events := completeElevatorCycles(state)
	ids := make([]string, 0, len(state.Aircraft))
	for id := range state.Aircraft {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		if !airborne(aircraft) {
			aircraft.RemainingRange = aircraft.MaxRange
			continue
		}
		switch aircraft.Status {
		case "cap", "attack_holding":
			aircraft.RemainingRange--
		case "attack":
			events = append(events, advanceAttack(state, aircraft)...)
		case "recon":
			events = append(events, advanceRecon(state, aircraft)...)
		}
		if aircraft.RemainingRange <= -5 {
			events = append(events, EngineEvent{Type: "air_destroyed", Audience: aircraft.Faction, Text: aircraft.Model + "燃油耗尽，航空算子被摧毁"})
			delete(state.Aircraft, id)
		}
	}
	events = append(events, launchReadyTasks(state)...)
	events = append(events, scheduleElevators(state)...)
	for _, unit := range state.Units {
		if unit.Deck != nil {
			unit.Deck.Operation = ""
		}
	}
	return events
}

func CanLandAircraft(state State, carrier *Unit) bool {
	return AviationAvailable(carrier) && carrier.Deck.Operation != "launch" && deckUsed(state, carrier.ID) < carrier.Deck.Length
}

func LandAircraft(state *State, aircraft *AirUnit, carrier *Unit) {
	aircraft.Status = "recovery"
	aircraft.Launched = false
	clearAirTask(aircraft)
	carrier.Card.Aircraft++
	carrier.Deck.Operation = "recover"
}

func taskPhase(members []*AirUnit) string {
	if members[0].TaskKind == "attack" {
		allDeparted := true
		launched := false
		for _, aircraft := range members {
			if aircraft.Status != "attack" {
				allDeparted = false
			}
			if aircraft.Launched {
				launched = true
			}
		}
		if allDeparted {
			if launched {
				return "launching"
			}
			return "active"
		}
		for _, aircraft := range members {
			if aircraft.Status == "attack_holding" {
				return "assembling"
			}
		}
	}
	launched := false
	airborneOnly := true
	for _, aircraft := range members {
		if !airborne(aircraft) {
			airborneOnly = false
		}
		if aircraft.Launched {
			launched = true
		}
		if aircraft.Status == "elevator" {
			return "lifting"
		}
	}
	if airborneOnly {
		if launched {
			return "launching"
		}
		return "active"
	}
	for _, aircraft := range members {
		if aircraft.Status == "deck" || aircraft.Status == "ready" {
			return "preparing"
		}
	}
	return "servicing"
}

func VisibleAviation(state State, faction string) AviationView {
	EnsureAviation(&state)
	view := AviationView{Carriers: []CarrierAirStatus{}, Tasks: []AirTask{}}
	for _, unit := range state.Units {
		if unit.Faction != faction || unit.Deck == nil {
			continue
		}
		carrier := CarrierAirStatus{UnitID: unit.ID, Name: unit.Name, Type: unit.Type, HangarCapacity: unit.Card.HangarCapacity, DeckLength: unit.Deck.Length, DeckCapacity: unit.Deck.Length, Operation: unit.Deck.Operation, Elevators: []AircraftElevator{}, Aircraft: []AirUnit{}}
		for _, elevator := range unit.Deck.Elevators {
			projection := *elevator
			projection.AircraftIDs = append([]string{}, elevator.AircraftIDs...)
			carrier.Elevators = append(carrier.Elevators, projection)
		}
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID != unit.ID {
				continue
			}
			if aircraft.Launched {
				carrier.LaunchUsed++
			}
			switch aircraft.Status {
			case "hangar", "service":
				carrier.HangarUsed++
			case "deck", "ready", "recovery":
				carrier.DeckUsed++
			}
			projection := *aircraft
			if canSearch(projection) {
				projection.ReconRadiusLimit = ReconRadiusLimit(projection)
			}
			carrier.Aircraft = append(carrier.Aircraft, projection)
		}
		sort.Slice(carrier.Aircraft, func(i, j int) bool { return carrier.Aircraft[i].ID < carrier.Aircraft[j].ID })
		view.Carriers = append(view.Carriers, carrier)
	}
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == faction && aircraft.TaskID != "" {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	for taskID, aircraftList := range groups {
		sort.Slice(aircraftList, func(i, j int) bool { return aircraftList[i].ID < aircraftList[j].ID })
		leader := aircraftList[0]
		kind := leader.TaskKind
		phase := taskPhase(aircraftList)
		task := AirTask{ID: taskID, Kind: kind, Phase: phase, CarrierID: leader.CarrierID, CarrierIDs: []string{}, AircraftIDs: []string{}, Aircraft: []AirUnit{}, Priority: leader.Priority}
		carrierSet := map[string]bool{}
		for _, aircraft := range aircraftList {
			task.AircraftIDs = append(task.AircraftIDs, aircraft.ID)
			task.Aircraft = append(task.Aircraft, *aircraft)
			if !carrierSet[aircraft.CarrierID] {
				carrierSet[aircraft.CarrierID] = true
				task.CarrierIDs = append(task.CarrierIDs, aircraft.CarrierID)
			}
			if airborne(aircraft) {
				task.AirborneCount++
			}
		}
		sort.Strings(task.CarrierIDs)
		carrier := state.Units[leader.CarrierID]
		if carrier != nil {
			task.FleetID = FleetKey(carrier)
		}
		switch kind {
		case "cap":
			task.Name = "CAP · " + leader.Model
			if carrier != nil {
				task.CurrentCoord = carrier.Coord
			}
		case "attack":
			task.Name = "攻击编队 · " + leader.Model
			task.OriginCoord = leader.OriginCoord
			task.CurrentCoord = leader.Coord
			task.TargetCoord = leader.TargetCoord
		default:
			reconLeader := leader
			for _, aircraft := range aircraftList {
				if aircraft.Status == "recon" {
					reconLeader = aircraft
					break
				}
			}
			task.Name = "扇面侦察 · " + leader.Model
			task.OriginCoord = reconLeader.OriginCoord
			task.CurrentCoord = reconLeader.Coord
			task.Heading = reconLeader.Heading
			task.SectorWidth = reconLeader.SectorWidth
			task.ReconRadius = reconLeader.ReconRadius
			task.ReconDistance = reconLeader.ReconDistance
			task.Returning = reconLeader.Returning
			task.TrackingFleetID = reconLeader.TrackingFleetID
			if reconLeader.Status == "recon" {
				task.SectorHexes = ReconSectorHexes(state.Map, reconLeader.OriginCoord, reconLeader.Heading, reconLeader.SectorWidth, reconLeader.ReconRadius)
				task.CurrentRingHexes = ReconRingHexes(state.Map, reconLeader.OriginCoord, reconLeader.Heading, reconLeader.SectorWidth, reconLeader.ReconDistance)
			}
		}
		if len(aircraftList) > 1 {
			task.Name += fmt.Sprintf(" ×%d", len(aircraftList))
		}
		if task.AirborneCount > 0 && task.CurrentCoord != "" {
			task.DistanceReference = BuildDistanceReference(state.Map, task.CurrentCoord, 8)
		}
		view.Tasks = append(view.Tasks, task)
	}
	sort.Slice(view.Carriers, func(i, j int) bool { return view.Carriers[i].UnitID < view.Carriers[j].UnitID })
	sort.Slice(view.Tasks, func(i, j int) bool {
		if view.Tasks[i].Priority == view.Tasks[j].Priority {
			return view.Tasks[i].ID < view.Tasks[j].ID
		}
		return view.Tasks[i].Priority < view.Tasks[j].Priority
	})
	return view
}
