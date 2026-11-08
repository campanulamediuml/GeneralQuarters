package game

import (
	"fmt"
	"sort"
)

func ValidateAirOrder(state State, faction string, order Order) error {
	aircraft := state.Aircraft[order.AircraftID]
	if aircraft == nil || aircraft.Faction != faction {
		return fmt.Errorf("航空算子不存在或不属于当前玩家")
	}
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil || carrier.Faction != faction || !IsCarrier(carrier) {
		return fmt.Errorf("母舰不存在或无法执行航空作业")
	}
	operation := state.CarrierOperations[carrier.ID]
	switch order.Type {
	case "LAUNCH_CAP":
		if aircraft.Kind != "fighter" {
			return fmt.Errorf("当前航空测试只允许战斗机执行CAP")
		}
		if aircraft.Status != "hangar" {
			return fmt.Errorf("只有机库内的航空算子可以升空")
		}
		if operation == "recover" {
			return fmt.Errorf("该航母本回合已经进行降落，不能再起飞")
		}
		if carrier.Card.DeckCapacity <= 0 {
			return fmt.Errorf("该航母没有可用飞行甲板")
		}
		used := 0
		for _, item := range state.Aircraft {
			if item.CarrierID == carrier.ID && item.Launched {
				used++
			}
		}
		if used >= carrier.Card.DeckCapacity {
			return fmt.Errorf("该航母本回合起飞容量已满")
		}
		return nil
	case "RECOVER_AIR":
		if aircraft.Status != "cap" {
			return fmt.Errorf("只有空中的CAP算子可以回收")
		}
		if operation == "launch" {
			return fmt.Errorf("该航母本回合已经进行起飞，不能再降落")
		}
		return nil
	case "PRIORITIZE_AIR_TASK":
		if aircraft.Status != "cap" {
			return fmt.Errorf("只有已升空的任务可以调整优先级")
		}
		return nil
	default:
		return fmt.Errorf("不支持的航空命令")
	}
}

func ApplyAirOrder(state *State, order Order) []EngineEvent {
	if state.CarrierOperations == nil {
		state.CarrierOperations = map[string]string{}
	}
	aircraft := state.Aircraft[order.AircraftID]
	carrier := state.Units[aircraft.CarrierID]
	if order.Type == "LAUNCH_CAP" {
		priority := 1
		for _, item := range state.Aircraft {
			if item.Faction == aircraft.Faction && item.Status == "cap" && item.Priority >= priority {
				priority = item.Priority + 1
			}
		}
		aircraft.Status = "cap"
		aircraft.Launched = true
		aircraft.Priority = priority
		carrier.Card.Aircraft--
		state.CarrierOperations[carrier.ID] = "launch"
		return []EngineEvent{{Type: "cap_launched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s从%s升空执行CAP", aircraft.Model, carrier.Name)}}
	}
	if order.Type == "PRIORITIZE_AIR_TASK" {
		for _, item := range state.Aircraft {
			if item.Faction == aircraft.Faction && item.Status == "cap" && item.ID != aircraft.ID {
				item.Priority++
			}
		}
		aircraft.Priority = 1
		return []EngineEvent{{Type: "air_task_prioritized", Audience: aircraft.Faction, Text: aircraft.Model + "的CAP任务已调整为最高优先级"}}
	}
	aircraft.Status = "hangar"
	aircraft.Launched = false
	aircraft.Priority = 0
	carrier.Card.Aircraft++
	state.CarrierOperations[carrier.ID] = "recover"
	return []EngineEvent{{Type: "air_recovered", Audience: aircraft.Faction, Text: fmt.Sprintf("%s已返回%s机库", aircraft.Model, carrier.Name)}}
}

func AdvanceAviation(state *State) []EngineEvent {
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
		if aircraft.Status != "cap" {
			continue
		}
		aircraft.RemainingRange--
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
			carrier.Aircraft = append(carrier.Aircraft, *aircraft)
		}
		sort.Slice(carrier.Aircraft, func(i, j int) bool { return carrier.Aircraft[i].ID < carrier.Aircraft[j].ID })
		view.Carriers = append(view.Carriers, carrier)
	}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == faction && aircraft.Status == "cap" {
			view.Tasks = append(view.Tasks, AirTask{ID: aircraft.ID, Kind: "cap", Name: "CAP · " + aircraft.Model, CarrierID: aircraft.CarrierID, AircraftIDs: []string{aircraft.ID}, Priority: aircraft.Priority})
		}
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
