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
			return fmt.Errorf("该母舰本回合已经进行其他航空作业，不能再分配或起飞")
		}
		definitionID := aircraftList[0].DefinitionID
		status := aircraftList[0].Status
		readyStatus := "deck"
		if aircraftList[0].Seaplane {
			readyStatus = "ready"
		}
		if status != "hangar" && status != "service" && status != readyStatus {
			return fmt.Errorf("只有机库、整备中或已完成起飞准备的算子可以分配任务")
		}
		if status == readyStatus {
			if err := validateLaunchCapacity(state, aircraftList, carrier); err != nil {
				return err
			}
		}
		for _, aircraft := range aircraftList {
			if aircraft.DefinitionID != definitionID {
				return fmt.Errorf("同一航空任务必须使用同型算子")
			}
			if aircraft.Status != status {
				return fmt.Errorf("同一航空任务的算子必须处于相同整备阶段")
			}
			if order.Type == "LAUNCH_CAP" && aircraft.Kind != "fighter" {
				return fmt.Errorf("只有战斗机可以执行CAP")
			}
			if order.Type == "LAUNCH_RECON" && !canSearch(*aircraft) {
				return fmt.Errorf("战斗机不能执行扇面搜索，TBF、SB2C和侦察机可以执行搜索")
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
		if len(aircraftList) != 1 || aircraftList[0].TaskID == "" {
			return fmt.Errorf("只有已分配的航空任务可以调整优先级")
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
		if aircraft.Faction == faction && aircraft.TaskID != "" && aircraft.Priority >= priority {
			priority = aircraft.Priority + 1
		}
	}
	return priority
}

func newAirTaskID(state *State, aircraftList []*AirUnit) string {
	selected := map[string]bool{}
	for _, aircraft := range aircraftList {
		selected[aircraft.ID] = true
	}
	base := aircraftList[0].ID
	for suffix := 1; ; suffix++ {
		candidate := base
		if suffix > 1 {
			candidate = fmt.Sprintf("%s-%d", base, suffix)
		}
		conflict := false
		for _, aircraft := range state.Aircraft {
			if aircraft.TaskID == candidate && !selected[aircraft.ID] {
				conflict = true
				break
			}
		}
		if !conflict {
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
	aircraft.Heading = ""
	aircraft.SectorWidth = 0
	aircraft.ReconRadius = 0
	aircraft.ReconDistance = 0
	aircraft.Returning = false
	aircraft.TrackingFleetID = ""
}

func configureAirTask(state *State, aircraftList []*AirUnit, order Order) {
	taskID := newAirTaskID(state, aircraftList)
	priority := nextAirTaskPriority(state, aircraftList[0].Faction)
	taskKind := "cap"
	if order.Type == "LAUNCH_RECON" {
		taskKind = "recon"
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
	}
}

func launchAirTask(state *State, aircraftList []*AirUnit, carrier *Unit) []EngineEvent {
	for _, aircraft := range aircraftList {
		aircraft.Launched = true
		aircraft.Status = aircraft.TaskKind
		carrier.Card.Aircraft--
		if aircraft.TaskKind == "recon" {
			aircraft.OriginCoord = carrier.Coord
			aircraft.Coord = carrier.Coord
			aircraft.ReconDistance = 0
			aircraft.Returning = false
			aircraft.TrackingFleetID = ""
		}
	}
	state.CarrierOperations[carrier.ID] = "launch"
	leader := aircraftList[0]
	if leader.TaskKind == "cap" {
		return []EngineEvent{{Type: "cap_launched", Audience: carrier.Faction, Text: fmt.Sprintf("%s从%s升空%d个CAP算子", leader.Model, carrier.Name, len(aircraftList))}}
	}
	return []EngineEvent{{Type: "recon_launched", Audience: carrier.Faction, Text: fmt.Sprintf("%s从%s升空%d个侦察算子：%d° %s扇面，半径%d hex", leader.Model, carrier.Name, len(aircraftList), leader.SectorWidth, leader.Heading, leader.ReconRadius)}}
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
			if aircraft.TaskKind == "" {
				clearAirTask(aircraft)
			}
		}
		state.CarrierOperations[carrier.ID] = "prepare"
		return []EngineEvent{{Type: "air_prepared", Audience: carrier.Faction, Text: message}}
	case "RETURN_TO_HANGAR":
		for _, aircraft := range aircraftList {
			aircraft.Status = "hangar"
			clearAirTask(aircraft)
		}
		if state.CarrierOperations[carrier.ID] == "prepare" {
			delete(state.CarrierOperations, carrier.ID)
		}
		return []EngineEvent{{Type: "air_returned_to_hangar", Audience: carrier.Faction, Text: fmt.Sprintf("%s将%d个整备中算子拉回机库、取消任务并重置整备", carrier.Name, len(aircraftList))}}
	case "LAUNCH_CAP", "LAUNCH_RECON":
		status := aircraftList[0].Status
		configureAirTask(state, aircraftList, order)
		if status == "deck" || status == "ready" {
			return launchAirTask(state, aircraftList, carrier)
		}
		for _, aircraft := range aircraftList {
			if aircraft.Status == "hangar" {
				aircraft.Status = "service"
			}
		}
		state.CarrierOperations[carrier.ID] = "prepare"
		return []EngineEvent{{Type: "air_task_assigned", Audience: carrier.Faction, Text: fmt.Sprintf("%s将%d个算子加入%s任务，开始跨回合整备", carrier.Name, len(aircraftList), aircraftList[0].TaskKind)}}
	case "RECOVER_AIR":
		for _, aircraft := range aircraftList {
			aircraft.Status = "hangar"
			aircraft.Launched = false
			clearAirTask(aircraft)
			carrier.Card.Aircraft++
		}
		state.CarrierOperations[carrier.ID] = "recover"
		return []EngineEvent{{Type: "air_recovered", Audience: carrier.Faction, Text: fmt.Sprintf("%d个CAP算子已返回%s机库", len(aircraftList), carrier.Name)}}
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

func advanceAssignedAirTasks(state *State) []EngineEvent {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskID == "" || (aircraft.Status != "hangar" && aircraft.Status != "service" && aircraft.Status != "deck" && aircraft.Status != "ready") {
			continue
		}
		groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
	}
	taskIDs := make([]string, 0, len(groups))
	for taskID := range groups {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	events := []EngineEvent{}
	for _, taskID := range taskIDs {
		aircraftList := groups[taskID]
		sort.Slice(aircraftList, func(i, j int) bool { return aircraftList[i].ID < aircraftList[j].ID })
		leader := aircraftList[0]
		carrier := state.Units[leader.CarrierID]
		if carrier == nil {
			continue
		}
		status := leader.Status
		uniform := true
		for _, aircraft := range aircraftList {
			if aircraft.Status != status {
				uniform = false
				break
			}
		}
		if !uniform {
			continue
		}
		switch status {
		case "hangar":
			for _, aircraft := range aircraftList {
				aircraft.Status = "service"
			}
			events = append(events, EngineEvent{Type: "air_prepared", Audience: leader.Faction, Text: fmt.Sprintf("%s完成%d个任务算子的机库整备", carrier.Name, len(aircraftList))})
		case "service":
			if !leader.Seaplane {
				used := 0
				for _, aircraft := range state.Aircraft {
					if aircraft.CarrierID == carrier.ID && aircraft.Status == "deck" {
						used++
					}
				}
				if used+len(aircraftList) > carrier.Card.DeckCapacity {
					events = append(events, EngineEvent{Type: "air_preparation_delayed", Audience: leader.Faction, Text: carrier.Name + "飞行甲板容量不足，任务继续等待整备"})
					continue
				}
			}
			for _, aircraft := range aircraftList {
				aircraft.Status = "deck"
				if aircraft.Seaplane {
					aircraft.Status = "ready"
				}
			}
			events = append(events, EngineEvent{Type: "air_prepared", Audience: leader.Faction, Text: fmt.Sprintf("%s完成%d个任务算子的起飞准备", carrier.Name, len(aircraftList))})
		case "deck", "ready":
			if err := validateLaunchCapacity(*state, aircraftList, carrier); err != nil {
				events = append(events, EngineEvent{Type: "air_launch_delayed", Audience: leader.Faction, Text: carrier.Name + "本回合起飞容量不足，任务推迟起飞"})
				continue
			}
			events = append(events, launchAirTask(state, aircraftList, carrier)...)
		}
	}
	return events
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
	events = append(events, advanceAssignedAirTasks(state)...)
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
		if kind == "" {
			kind = leader.Status
		}
		phase := "active"
		switch leader.Status {
		case "hangar", "service":
			phase = "servicing"
		case "deck", "ready":
			phase = "preparing"
		case "cap", "recon":
			if leader.Launched {
				phase = "launching"
			}
		}
		airborne := leader.Status == "cap" || leader.Status == "recon"
		task := AirTask{ID: taskID, Kind: kind, Phase: phase, CarrierID: leader.CarrierID, AircraftIDs: []string{}, Aircraft: []AirUnit{}, Priority: leader.Priority}
		for _, aircraft := range aircraftList {
			task.AircraftIDs = append(task.AircraftIDs, aircraft.ID)
			task.Aircraft = append(task.Aircraft, *aircraft)
		}
		carrier := state.Units[leader.CarrierID]
		if kind == "cap" {
			task.Name = "CAP · " + leader.Model
			if carrier != nil {
				task.CurrentCoord = carrier.Coord
			}
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
			if airborne {
				task.SectorHexes = ReconSectorHexes(state.Map, leader.OriginCoord, leader.Heading, leader.SectorWidth, leader.ReconRadius)
				task.CurrentRingHexes = ReconRingHexes(state.Map, leader.OriginCoord, leader.Heading, leader.SectorWidth, leader.ReconDistance)
			}
		}
		if len(aircraftList) > 1 {
			task.Name += fmt.Sprintf(" ×%d", len(aircraftList))
		}
		if airborne {
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
