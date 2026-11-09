package game

import (
	"fmt"
	"sort"
	"strings"
)

func airOrderIDs(order Order) []string {
	ids := append([]string(nil), order.AircraftIDs...)
	if len(ids) == 0 && order.AircraftID != "" {
		ids = []string{order.AircraftID}
	}
	sort.Strings(ids)
	return ids
}

func aircraftForOrder(state State, faction string, order Order) ([]*AirUnit, *Unit, error) {
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
				if candidate.TaskID == id && candidate.Faction == faction && (candidate.Status == "cap" || candidate.Status == "recon") {
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
		} else if aircraft.CarrierID != carrierID {
			return nil, nil, fmt.Errorf("批量航空命令必须使用同一母舰的算子")
		}
		aircraftList = append(aircraftList, aircraft)
	}
	carrier := state.Units[carrierID]
	if carrier == nil || carrier.Faction != faction || carrier.Card.HangarCapacity <= 0 {
		return nil, nil, fmt.Errorf("母舰不存在或无法执行航空作业")
	}
	return aircraftList, carrier, nil
}

func ValidateAirOrder(state State, faction string, order Order) error {
	aircraftList, carrier, err := aircraftForOrder(state, faction, order)
	if err != nil {
		return err
	}
	operation := state.CarrierOperations[carrier.ID]
	switch order.Type {
	case "LAUNCH_CAP", "LAUNCH_RECON":
		if operation == "recover" || operation == "prepare" {
			return fmt.Errorf("该母舰本回合已经进行其他航空作业，不能再起飞")
		}
		if err := validateLaunchCapacity(state, aircraftList, carrier); err != nil {
			return err
		}
		definitionID := aircraftList[0].DefinitionID
		requiredStatus := "deck"
		if aircraftList[0].Seaplane {
			requiredStatus = "ready"
		}
		for _, aircraft := range aircraftList {
			if aircraft.DefinitionID != definitionID {
				return fmt.Errorf("同一航空任务必须使用同型算子")
			}
			if aircraft.Status != requiredStatus {
				return fmt.Errorf("航空算子必须先完成机库整备和起飞准备，才能升空")
			}
			if order.Type == "LAUNCH_CAP" && aircraft.Kind != "fighter" {
				return fmt.Errorf("只有战斗机可以执行CAP")
			}
			if order.Type == "LAUNCH_RECON" && aircraft.Kind != "recon" {
				return fmt.Errorf("战斗机和攻击机不能执行专用扇面侦察")
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
		return nil
	case "PREPARE_AIR":
		seaplane := aircraftList[0].Seaplane
		if !seaplane && (!IsCarrier(carrier) || carrier.Card.DeckCapacity <= 0) {
			return fmt.Errorf("该舰没有可用于舰载机整备的飞行甲板")
		}
		if operation != "" {
			return fmt.Errorf("该母舰本回合已经进行航空作业")
		}
		status := aircraftList[0].Status
		if status != "hangar" && status != "service" {
			return fmt.Errorf("只有未整备或已完成机库整备的算子可以进入下一阶段")
		}
		for _, aircraft := range aircraftList {
			if aircraft.Status != status || aircraft.Seaplane != seaplane {
				return fmt.Errorf("同批整备算子必须处于相同阶段和类型")
			}
		}
		if status == "service" && !seaplane {
			used := 0
			for _, aircraft := range state.Aircraft {
				if aircraft.CarrierID == carrier.ID && aircraft.Status == "deck" {
					used++
				}
			}
			if used+len(aircraftList) > carrier.Card.DeckCapacity {
				return fmt.Errorf("飞行甲板没有足够停放容量")
			}
		}
		return nil
	case "RETURN_TO_HANGAR":
		for _, aircraft := range aircraftList {
			if aircraft.Status != "service" && aircraft.Status != "deck" && aircraft.Status != "ready" {
				return fmt.Errorf("只有整备或起飞准备中的算子可以拉回机库")
			}
		}
		return nil
	case "RECOVER_AIR":
		if operation == "launch" || operation == "prepare" {
			return fmt.Errorf("该母舰本回合已经进行其他航空作业，不能再降落")
		}
		for _, aircraft := range aircraftList {
			if aircraft.Status != "cap" {
				return fmt.Errorf("只有空中的CAP算子可以主动回收，侦察机需要完成返航")
			}
		}
		return nil
	case "PRIORITIZE_AIR_TASK":
		if len(aircraftList) != 1 || (aircraftList[0].Status != "cap" && aircraftList[0].Status != "recon") {
			return fmt.Errorf("只有已升空的任务可以调整优先级")
		}
		return nil
	default:
		return fmt.Errorf("不支持的航空命令")
	}
}

func validateLaunchCapacity(state State, aircraftList []*AirUnit, carrier *Unit) error {
	used := 0
	for _, aircraft := range state.Aircraft {
		if aircraft.CarrierID == carrier.ID && aircraft.Launched {
			used++
		}
	}
	if aircraftList[0].Seaplane {
		if used+len(aircraftList) > 1 {
			return fmt.Errorf("该舰船本回合水上飞机起飞容量已满")
		}
		return nil
	}
	if carrier.Card.DeckCapacity <= 0 {
		return fmt.Errorf("该航母没有可用飞行甲板")
	}
	if used+len(aircraftList) > carrier.Card.DeckCapacity {
		return fmt.Errorf("该航母本回合起飞容量已满")
	}
	return nil
}

func nextAirTaskPriority(state *State, faction string) int {
	priority := 1
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == faction && (aircraft.Status == "cap" || aircraft.Status == "recon") && aircraft.Priority >= priority {
			priority = aircraft.Priority + 1
		}
	}
	return priority
}

func ApplyAirOrder(state *State, order Order) []EngineEvent {
	if state.CarrierOperations == nil {
		state.CarrierOperations = map[string]string{}
	}
	aircraftList, carrier, err := aircraftForOrder(*state, orderAircraftFaction(*state, order), order)
	if err != nil || len(aircraftList) == 0 || carrier == nil {
		return nil
	}
	switch order.Type {
	case "PREPARE_AIR":
		nextStatus := "service"
		message := fmt.Sprintf("%s完成%d个航空算子的机库整备", carrier.Name, len(aircraftList))
		if aircraftList[0].Status == "service" {
			nextStatus = "deck"
			message = fmt.Sprintf("%s将%d个航空算子提升至飞行甲板", carrier.Name, len(aircraftList))
			if aircraftList[0].Seaplane {
				nextStatus = "ready"
				message = fmt.Sprintf("%s完成%d个水上飞机算子的起飞准备", carrier.Name, len(aircraftList))
			}
		}
		for _, aircraft := range aircraftList {
			aircraft.Status = nextStatus
			aircraft.TaskID = ""
		}
		state.CarrierOperations[carrier.ID] = "prepare"
		return []EngineEvent{{Type: "air_prepared", Audience: carrier.Faction, Text: message}}
	case "RETURN_TO_HANGAR":
		for _, aircraft := range aircraftList {
			aircraft.Status = "hangar"
		}
		if state.CarrierOperations[carrier.ID] == "prepare" {
			delete(state.CarrierOperations, carrier.ID)
		}
		return []EngineEvent{{Type: "air_returned_to_hangar", Audience: carrier.Faction, Text: fmt.Sprintf("%s将%d个整备中算子拉回机库并重置整备", carrier.Name, len(aircraftList))}}
	case "LAUNCH_CAP", "LAUNCH_RECON":
		priority := nextAirTaskPriority(state, carrier.Faction)
		taskID := aircraftList[0].ID
		for _, aircraft := range aircraftList {
			aircraft.Launched = true
			aircraft.Priority = priority
			aircraft.TaskID = taskID
			carrier.Card.Aircraft--
		}
		state.CarrierOperations[carrier.ID] = "launch"
		if order.Type == "LAUNCH_CAP" {
			for _, aircraft := range aircraftList {
				aircraft.Status = "cap"
			}
			return []EngineEvent{{Type: "cap_launched", Audience: carrier.Faction, Text: fmt.Sprintf("%s从%s升空%d个CAP算子", aircraftList[0].Model, carrier.Name, len(aircraftList))}}
		}
		radius := order.ReconRadius
		if radius == 0 {
			radius = ReconRadiusLimit(*aircraftList[0])
		}
		for _, aircraft := range aircraftList {
			aircraft.Status = "recon"
			aircraft.OriginCoord = carrier.Coord
			aircraft.Coord = carrier.Coord
			aircraft.Heading = strings.ToUpper(order.Heading)
			aircraft.SectorWidth = order.SectorWidth
			aircraft.ReconRadius = radius
			aircraft.ReconDistance = 0
			aircraft.Returning = false
			aircraft.TrackingFleetID = ""
		}
		return []EngineEvent{{Type: "recon_launched", Audience: carrier.Faction, Text: fmt.Sprintf("%s从%s升空%d个侦察算子：%d° %s扇面，半径%d hex", aircraftList[0].Model, carrier.Name, len(aircraftList), order.SectorWidth, strings.ToUpper(order.Heading), radius)}}
	case "RECOVER_AIR":
		for _, aircraft := range aircraftList {
			aircraft.Status = "hangar"
			aircraft.Launched = false
			aircraft.Priority = 0
			aircraft.TaskID = ""
			carrier.Card.Aircraft++
		}
		state.CarrierOperations[carrier.ID] = "recover"
		return []EngineEvent{{Type: "air_recovered", Audience: carrier.Faction, Text: fmt.Sprintf("%d个CAP算子已返回%s机库", len(aircraftList), carrier.Name)}}
	case "PRIORITIZE_AIR_TASK":
		selected := aircraftList[0]
		selectedTask := selected.TaskID
		if selectedTask == "" {
			selectedTask = selected.ID
		}
		seen := map[string]bool{}
		for _, aircraft := range state.Aircraft {
			if aircraft.Faction != selected.Faction || (aircraft.Status != "cap" && aircraft.Status != "recon") {
				continue
			}
			taskID := aircraft.TaskID
			if taskID == "" {
				taskID = aircraft.ID
			}
			if taskID != selectedTask {
				seen[taskID] = true
			}
		}
		for _, aircraft := range state.Aircraft {
			if aircraft.Faction != selected.Faction || (aircraft.Status != "cap" && aircraft.Status != "recon") {
				continue
			}
			taskID := aircraft.TaskID
			if taskID == "" {
				taskID = aircraft.ID
			}
			if taskID == selectedTask {
				aircraft.Priority = 1
			} else if seen[taskID] {
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

func AdvanceAviation(state *State) []EngineEvent {
	if state.CarrierOperations == nil {
		state.CarrierOperations = map[string]string{}
	}
	events := []EngineEvent{}
	ids := make([]string, 0, len(state.Aircraft))
	for id := range state.Aircraft {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.Launched = false
		if aircraft.Status == "hangar" || aircraft.Status == "service" || aircraft.Status == "deck" || aircraft.Status == "ready" {
			aircraft.RemainingRange = aircraft.MaxRange
			continue
		}
		if aircraft.Status == "cap" {
			aircraft.RemainingRange--
		} else if aircraft.Status == "recon" {
			events = append(events, advanceRecon(state, aircraft)...)
		} else {
			continue
		}
		if aircraft.RemainingRange <= -5 {
			events = append(events, EngineEvent{Type: "air_destroyed", Audience: aircraft.Faction, Text: aircraft.Model + "燃油耗尽，航空算子被摧毁"})
			delete(state.Aircraft, id)
		}
	}
	state.CarrierOperations = map[string]string{}
	return events
}

func VisibleAviation(state State, faction string) AviationView {
	view := AviationView{Carriers: []CarrierAirStatus{}, Tasks: []AirTask{}}
	for _, unit := range state.Units {
		if unit.Faction != faction || unit.Card.HangarCapacity <= 0 {
			continue
		}
		carrier := CarrierAirStatus{UnitID: unit.ID, Name: unit.Name, Type: unit.Type, HangarCapacity: unit.Card.HangarCapacity, DeckCapacity: unit.Card.DeckCapacity, Operation: state.CarrierOperations[unit.ID], Aircraft: []AirUnit{}}
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID != unit.ID {
				continue
			}
			if aircraft.Launched {
				carrier.LaunchUsed++
			}
			switch aircraft.Status {
			case "hangar", "service", "ready":
				carrier.HangarUsed++
			case "deck":
				carrier.DeckUsed++
			}
			projection := *aircraft
			if projection.Kind == "recon" {
				projection.ReconRadiusLimit = ReconRadiusLimit(projection)
			}
			carrier.Aircraft = append(carrier.Aircraft, projection)
		}
		sort.Slice(carrier.Aircraft, func(i, j int) bool { return carrier.Aircraft[i].ID < carrier.Aircraft[j].ID })
		view.Carriers = append(view.Carriers, carrier)
	}
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != faction || (aircraft.Status != "cap" && aircraft.Status != "recon") {
			continue
		}
		taskID := aircraft.TaskID
		if taskID == "" {
			taskID = aircraft.ID
		}
		groups[taskID] = append(groups[taskID], aircraft)
	}
	for taskID, aircraftList := range groups {
		sort.Slice(aircraftList, func(i, j int) bool { return aircraftList[i].ID < aircraftList[j].ID })
		leader := aircraftList[0]
		task := AirTask{ID: taskID, Kind: leader.Status, CarrierID: leader.CarrierID, AircraftIDs: []string{}, Aircraft: []AirUnit{}, Priority: leader.Priority}
		for _, aircraft := range aircraftList {
			task.AircraftIDs = append(task.AircraftIDs, aircraft.ID)
			task.Aircraft = append(task.Aircraft, *aircraft)
		}
		if leader.Status == "cap" {
			task.Name = "CAP · " + leader.Model
			task.CurrentCoord = state.Units[leader.CarrierID].Coord
		} else {
			task.Name = "扇面侦察 · " + leader.Model
			task.OriginCoord = leader.OriginCoord
			task.CurrentCoord = leader.Coord
			task.Heading = leader.Heading
			task.SectorWidth = leader.SectorWidth
			task.ReconRadius = leader.ReconRadius
			task.ReconDistance = leader.ReconDistance
			task.Returning = leader.Returning
			task.TrackingFleetID = leader.TrackingFleetID
			task.SectorHexes = ReconSectorHexes(state.Map, leader.OriginCoord, leader.Heading, leader.SectorWidth, leader.ReconRadius)
			task.CurrentRingHexes = ReconRingHexes(state.Map, leader.OriginCoord, leader.Heading, leader.SectorWidth, leader.ReconDistance)
		}
		if len(aircraftList) > 1 {
			task.Name += fmt.Sprintf(" ×%d", len(aircraftList))
		}
		task.DistanceReference = BuildDistanceReference(state.Map, task.CurrentCoord, 8)
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
