package game

import "fmt"

// —— 机场算子的任务循环（规则书 188–191 行）——
//
// 舰载机的循环按"母舰能不能保障"判断（见 air_cycle.go）；机场算子没有母舰，
// 能不能再飞一次取决于三件事：机场还在不在我方手里、跑道还剩几条、机型是否仍适合这个任务。
// 落地（landGroundAircraft）会清空 TaskID/TaskKind，所以循环参数必须在落地前抄进 Cycle*，
// 否则"再飞一次"只会飞出一个没有航向、没有扇面、没有掩护点的空任务。

// rememberGroundCycleParams 在落地前抄一份循环参数（不动开关：玩家点掉之后不能偷偷恢复）。
func rememberGroundCycleParams(aircraft *AirfieldAirUnit, kind string) {
	if aircraft == nil || !cyclicTaskKind(kind) {
		return
	}
	aircraft.CycleKind = kind
	aircraft.CycleTaskID = aircraft.TaskID
	aircraft.CycleHeading = aircraft.Heading
	aircraft.CycleSector = aircraft.SectorWidth
	aircraft.CycleTarget = aircraft.TargetCoord
}

// enableGroundTaskCycle 打开循环并记住参数（新编任务、玩家点开、无线电改令）。
func enableGroundTaskCycle(aircraft *AirfieldAirUnit, kind string) {
	if aircraft == nil || !cyclicTaskKind(kind) {
		return
	}
	rememberGroundCycleParams(aircraft, kind)
	aircraft.OnCycle = true
	aircraft.CycleOptOut = false
}

// disarmGroundTaskCycle 关闭循环（主动回收、玩家点掉、循环无法继续）。
func disarmGroundTaskCycle(aircraft *AirfieldAirUnit) {
	if aircraft == nil {
		return
	}
	aircraft.OnCycle = false
	aircraft.CycleOptOut = true
}

// canRestartGroundCycle 判断这架机场算子还能不能再来一轮，返回不能继续的原因。
func canRestartGroundCycle(state State, aircraft *AirfieldAirUnit) (bool, string) {
	airfield := airfieldFor(&state, aircraft.AirfieldID)
	if airfield == nil || airfield.Faction != aircraft.Faction {
		return false, "机场已失守"
	}
	if RunwaysAvailable(airfield.Strength, airfield.Runways) == 0 {
		return false, "机场没有可用跑道"
	}
	switch MissionKind(aircraft.CycleKind) {
	case MissionCap, MissionCover:
		// 规则书：CAP 与空中掩护只能由战斗机执行。
		if aircraft.Kind != "fighter" {
			return false, "机型不能执行战斗机任务"
		}
	case MissionASW:
		if aircraft.Kind == "fighter" {
			return false, "战斗机不能执行反潜巡逻"
		}
	case MissionRecon:
		if aircraft.Kind == "fighter" {
			return false, "战斗机不能执行扇面搜索"
		}
		// 侦察半径不存旧值：每次都由机型上限重算（和舰载侦察一致），
		// 拿老半径去比上限会把循环判成"航程不够"直接掐掉。
		if !ValidReconHeading(aircraft.CycleHeading) {
			return false, "没有保存侦察航向，无法重新出动"
		}
	default:
		return false, "任务类型不支持循环"
	}
	return true, ""
}

// groundCycleTarget 决定下一轮 CAP／空中掩护飞到哪个 hex。
// 记住的掩护点如果还有友军舰队或友军机场，就继续守那儿；已经空了就改成离它最近的
// 友军舰队，实在没有才回守本机场——否则循环只会把飞机一遍遍送回一片空海。
func groundCycleTarget(state State, aircraft *AirfieldAirUnit) string {
	home := ""
	if airfield := airfieldFor(&state, aircraft.AirfieldID); airfield != nil {
		home = airfield.Coord
	}
	want := aircraft.CycleTarget
	if want == "" || want == home {
		return home
	}
	if friendlyAirfieldAtCoord(state, aircraft.Faction, want) != nil {
		return want
	}
	for _, unit := range sortedUnitsForCycle(state.Units) {
		if unit != nil && unit.Faction == aircraft.Faction && unit.Coord == want {
			return want
		}
	}
	best, bestDistance, found := home, 0, false
	for _, unit := range sortedUnitsForCycle(state.Units) {
		if unit == nil || unit.Faction != aircraft.Faction || unit.Coord == "" {
			continue
		}
		distance := Distance(want, unit.Coord)
		if !found || distance < bestDistance {
			best, bestDistance, found = unit.Coord, distance, true
		}
	}
	return best
}

// sortedUnitsForCycle 按 ID 排序取单位，保证同样的存档每次算出的掩护点一致。
func sortedUnitsForCycle(units map[string]*Unit) []*Unit {
	ids := sortedUnitIDs(units)
	list := make([]*Unit, 0, len(ids))
	for _, id := range ids {
		list = append(list, units[id])
	}
	return list
}

// restartCycledGroundTask 让降落回机库的循环算子重新进入整备。
// 损伤、检修、时间窗未开都只算"等"，不算失败；机场失守或跑道打光才结束循环并留一条战报。
func restartCycledGroundTask(state *State, aircraft *AirfieldAirUnit) []EngineEvent {
	if aircraft == nil || !aircraft.OnCycle || aircraft.Status != "hangar" {
		return nil
	}
	if aircraft.Damaged || aircraft.Maintenance > 0 {
		return nil
	}
	if !state.ServiceWindowOpen() {
		return nil
	}
	ok, reason := canRestartGroundCycle(*state, aircraft)
	if !ok {
		disarmGroundTaskCycle(aircraft)
		return []EngineEvent{{Type: "air_cycle_end", Audience: aircraft.Faction, Text: fmt.Sprintf("%s的%s循环结束：%s", aircraft.Model, cycleKindName(aircraft.CycleKind), reason)}}
	}
	airfield := airfieldFor(state, aircraft.AirfieldID)
	if airfield == nil {
		return nil
	}
	kind := aircraft.CycleKind
	taskID := aircraft.CycleTaskID
	if taskID == "" {
		if MissionKind(kind) == MissionCap {
			// 同一机场的 CAP 共用一个任务组，和初次分配时的命名保持一致。
			taskID = "af-" + airfield.ID + "-cap"
		} else {
			taskID = aircraft.ID
		}
	}
	priority := 1
	if members := groundTaskAircraft(*state, taskID); len(members) > 0 && members[0].Priority > 0 {
		priority = members[0].Priority
	} else {
		for _, other := range state.GroundAir {
			if other.Faction == airfield.Faction && other.Priority >= priority {
				priority = other.Priority + 1
			}
		}
	}
	beginGroundMission(aircraft, taskID, MissionKind(kind), priority)
	aircraft.Status = "service"
	aircraft.Returning = false
	aircraft.Coord = airfield.Coord
	aircraft.OriginCoord = airfield.Coord
	aircraft.ReconDistance = 0
	aircraft.TrackingFleetID = ""
	aircraft.TargetFleetID = ""
	aircraft.TargetCoord = ""
	aircraft.Heading = ""
	aircraft.SectorWidth = 0
	switch MissionKind(kind) {
	case MissionCap:
		assignGroundCapMission(aircraft, groundCycleTarget(*state, aircraft))
	case MissionCover:
		aircraft.TargetCoord = groundCycleTarget(*state, aircraft)
	case MissionASW:
		aircraft.TargetCoord = airfield.Coord
	case MissionRecon:
		assignGroundReconMission(aircraft, aircraft.CycleHeading, aircraft.CycleSector)
	}
	applyGroundLoadout(*state, aircraft)
	return []EngineEvent{{Type: "air_cycle_restart", Audience: aircraft.Faction, Text: fmt.Sprintf("%s完成上一轮%s任务，回到%s重新整备，准备再次出动", aircraft.Model, cycleKindName(kind), airfield.Name)}}
}
