package game

import (
	"fmt"
	"sort"
	"strings"
)

func ValidateAirOrder(state State, faction string, order Order) error {
	aircraft := state.Aircraft[order.AircraftID]
	if aircraft == nil || aircraft.Faction != faction {
		return fmt.Errorf("航空算子不存在或不属于当前玩家")
	}
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil || carrier.Faction != faction || carrier.Card.HangarCapacity <= 0 {
		return fmt.Errorf("母舰不存在或无法执行航空作业")
	}
	operation := state.CarrierOperations[carrier.ID]
	switch order.Type {
	case "LAUNCH_CAP":
		if aircraft.Kind != "fighter" {
			return fmt.Errorf("只有战斗机可以执行CAP")
		}
		if !IsCarrier(carrier) {
			return fmt.Errorf("CAP舰载机只能从航母起飞")
		}
		return validateLaunch(state, aircraft, carrier, operation)
	case "LAUNCH_RECON":
		if aircraft.Kind != "recon" {
			return fmt.Errorf("战斗机和攻击机不能执行专用扇面侦察")
		}
		order.Heading = strings.ToUpper(order.Heading)
		if !ValidReconHeading(order.Heading) {
			return fmt.Errorf("侦察航向必须是E、SE、SW、W、NW或NE")
		}
		if order.SectorWidth != 60 && order.SectorWidth != 120 {
			return fmt.Errorf("侦察扇面只能选择60度或120度")
		}
		if order.ReconRadius < 1 || order.ReconRadius > ReconRadiusLimit(*aircraft) {
			return fmt.Errorf("侦察半径必须在1到%d hex之间", ReconRadiusLimit(*aircraft))
		}
		if !aircraft.Seaplane && !IsCarrier(carrier) {
			return fmt.Errorf("舰载机只能从航母起飞")
		}
		return validateLaunch(state, aircraft, carrier, operation)
	case "RECOVER_AIR":
		if aircraft.Status != "cap" {
			return fmt.Errorf("只有空中的CAP算子可以主动回收，侦察机需要完成返航")
		}
		if operation == "launch" {
			return fmt.Errorf("该航母本回合已经进行起飞，不能再降落")
		}
		return nil
	case "PRIORITIZE_AIR_TASK":
		if aircraft.Status != "cap" && aircraft.Status != "recon" {
			return fmt.Errorf("只有已升空的任务可以调整优先级")
		}
		return nil
	default:
		return fmt.Errorf("不支持的航空命令")
	}
}

func validateLaunch(state State, aircraft *AirUnit, carrier *Unit, operation string) error {
	if aircraft.Status != "hangar" {
		return fmt.Errorf("只有机库内的航空算子可以升空")
	}
	if operation == "recover" {
		return fmt.Errorf("该母舰本回合已经进行降落，不能再起飞")
	}
	used := 0
	for _, item := range state.Aircraft {
		if item.CarrierID == carrier.ID && item.Launched {
			used++
		}
	}
	if aircraft.Seaplane {
		if used >= 1 {
			return fmt.Errorf("该舰船本回合水上飞机起飞容量已满")
		}
		return nil
	}
	if carrier.Card.DeckCapacity <= 0 {
		return fmt.Errorf("该航母没有可用飞行甲板")
	}
	if used >= carrier.Card.DeckCapacity {
		return fmt.Errorf("该航母本回合起飞容量已满")
	}
	return nil
}

func ApplyAirOrder(state *State, order Order) []EngineEvent {
	if state.CarrierOperations == nil {
		state.CarrierOperations = map[string]string{}
	}
	aircraft := state.Aircraft[order.AircraftID]
	carrier := state.Units[aircraft.CarrierID]
	if order.Type == "LAUNCH_CAP" || order.Type == "LAUNCH_RECON" {
		priority := 1
		for _, item := range state.Aircraft {
			if item.Faction == aircraft.Faction && (item.Status == "cap" || item.Status == "recon") && item.Priority >= priority {
				priority = item.Priority + 1
			}
		}
		aircraft.Launched = true
		aircraft.Priority = priority
		carrier.Card.Aircraft--
		state.CarrierOperations[carrier.ID] = "launch"
		if order.Type == "LAUNCH_CAP" {
			aircraft.Status = "cap"
			return []EngineEvent{{Type: "cap_launched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s从%s升空执行CAP", aircraft.Model, carrier.Name)}}
		}
		aircraft.Status = "recon"
		aircraft.OriginCoord = carrier.Coord
		aircraft.Coord = carrier.Coord
		aircraft.Heading = strings.ToUpper(order.Heading)
		aircraft.SectorWidth = order.SectorWidth
		aircraft.ReconRadius = order.ReconRadius
		aircraft.ReconDistance = 0
		aircraft.Returning = false
		aircraft.TrackingFleetID = ""
		return []EngineEvent{{Type: "recon_launched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s从%s升空：%d° %s扇面，半径%d hex", aircraft.Model, carrier.Name, aircraft.SectorWidth, aircraft.Heading, aircraft.ReconRadius)}}
	}
	if order.Type == "PRIORITIZE_AIR_TASK" {
		for _, item := range state.Aircraft {
			if item.Faction == aircraft.Faction && (item.Status == "cap" || item.Status == "recon") && item.ID != aircraft.ID {
				item.Priority++
			}
		}
		aircraft.Priority = 1
		return []EngineEvent{{Type: "air_task_prioritized", Audience: aircraft.Faction, Text: aircraft.Model + "的航空任务已调整为最高优先级"}}
	}
	aircraft.Status = "hangar"
	aircraft.Launched = false
	aircraft.Priority = 0
	carrier.Card.Aircraft++
	state.CarrierOperations[carrier.ID] = "recover"
	return []EngineEvent{{Type: "air_recovered", Audience: aircraft.Faction, Text: fmt.Sprintf("%s已返回%s机库", aircraft.Model, carrier.Name)}}
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
		if aircraft.Status == "hangar" {
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
		carrier := CarrierAirStatus{UnitID: unit.ID, Name: unit.Name, Type: unit.Type, HangarUsed: unit.Card.Aircraft, HangarCapacity: unit.Card.HangarCapacity, DeckCapacity: unit.Card.DeckCapacity, Operation: state.CarrierOperations[unit.ID], Aircraft: []AirUnit{}}
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID != unit.ID {
				continue
			}
			if aircraft.Launched {
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
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != faction || (aircraft.Status != "cap" && aircraft.Status != "recon") {
			continue
		}
		task := AirTask{ID: aircraft.ID, Kind: aircraft.Status, CarrierID: aircraft.CarrierID, AircraftIDs: []string{aircraft.ID}, Priority: aircraft.Priority}
		if aircraft.Status == "cap" {
			task.Name = "CAP · " + aircraft.Model
		} else {
			task.Name = "扇面侦察 · " + aircraft.Model
			task.OriginCoord = aircraft.OriginCoord
			task.CurrentCoord = aircraft.Coord
			task.Heading = aircraft.Heading
			task.SectorWidth = aircraft.SectorWidth
			task.ReconRadius = aircraft.ReconRadius
			task.ReconDistance = aircraft.ReconDistance
			task.Returning = aircraft.Returning
			task.TrackingFleetID = aircraft.TrackingFleetID
			task.SectorHexes = ReconSectorHexes(state.Map, aircraft.OriginCoord, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius)
			task.CurrentRingHexes = ReconRingHexes(state.Map, aircraft.OriginCoord, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconDistance)
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
