package game

import "fmt"

// —— 任务循环（规则书 188–191 行）——
//
// 规则书 188 行："编入CAP、反潜巡逻、侦查任务的飞机将会自动进行任务循环。"
// 空中掩护按同一套机制循环（用户要求）：在掩护点值班的编队降落后自动重新整备、
// 再次飞回掩护点，直到航程不够、母舰不可用、需要修理或玩家取消。
//
// 规则书 190 行："主动回收的飞机会取消循环任务。"
// 规则书 191 行："点击算子可以让多个算子或本飞机算子开启或关闭自动任务循环。"
//
// 落地时 LandAircraft 会调用 clearAirTask 把航向/扇面/半径/掩护点全部清空，
// 所以循环参数必须在落地之前抄一份到 Cycle* 字段里，否则"再飞一次"只能飞一个空任务。

// cycleKindForStatus 把"在空中的任务状态"映射成循环类型。空串表示这个状态不参与循环。
func cycleKindForStatus(status string) string {
	switch MissionKind(status) {
	case MissionCap:
		return string(MissionCap)
	case MissionCover:
		return string(MissionCover)
	case MissionRecon:
		return string(MissionRecon)
	case MissionASW:
		return string(MissionASW)
	}
	return ""
}

// cyclicTaskKind 判断某类任务是否支持自动循环。
func cyclicTaskKind(kind string) bool {
	switch MissionKind(kind) {
	case MissionCap, MissionCover, MissionASW, MissionRecon:
		return true
	}
	return false
}

// rememberCycleParams 在落地前抄一份循环参数。
// 不动开关：玩家点掉循环之后，落地不能偷偷又把它打开（规则书 191 行），
// 只有重新编任务（configureAirTask）或玩家主动点开才会恢复循环。
func rememberCycleParams(aircraft *AirUnit, kind string) {
	if aircraft == nil || !cyclicTaskKind(kind) {
		return
	}
	aircraft.CycleKind = kind
	aircraft.CycleTaskID = aircraft.TaskID
	aircraft.CycleHeading = aircraft.Heading
	aircraft.CycleSector = aircraft.SectorWidth
	aircraft.CycleTarget = aircraft.TargetCoord
	aircraft.CyclePatrol = aircraft.PatrolCoord
}

// enableTaskCycle 打开循环开关并记住参数（新编任务、玩家点开、无线电改令）。
func enableTaskCycle(aircraft *AirUnit, kind string) {
	if aircraft == nil || !cyclicTaskKind(kind) {
		return
	}
	rememberCycleParams(aircraft, kind)
	aircraft.OnCycle = true
	aircraft.CycleOptOut = false
}

// disarmTaskCycle 关闭循环开关（主动回收、玩家点击取消、循环无法继续）。
func disarmTaskCycle(aircraft *AirUnit) {
	if aircraft == nil {
		return
	}
	aircraft.OnCycle = false
	aircraft.CycleOptOut = true
}

// canRestartCycle 判断这架飞机能否重新开始一轮循环，返回不能继续的原因。
func canRestartCycle(state State, aircraft *AirUnit) (bool, string) {
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		return false, "母舰已沉没"
	}
	if !AviationAvailable(carrier) {
		return false, "母舰损伤，无法进行航空作业"
	}
	switch aircraft.CycleKind {
	case "cap", "cover":
		if aircraft.Kind != "fighter" {
			return false, "机型不能执行战斗机任务"
		}
	case "asw":
		if aircraft.Kind == "fighter" {
			return false, "战斗机不能执行反潜巡逻"
		}
	case "recon":
		if !canSearch(*aircraft) {
			return false, "机型不能执行搜索"
		}
		if !ValidReconHeading(aircraft.CycleHeading) {
			return false, "没有保存侦察航向，无法重新出动"
		}
		// 半径不可设置、每次都由机型算出来，这里不再拿旧值去比上限——
		// 老任务里存着改规则前的大半径，一比就会把循环判成"航程不够"直接掐掉。
	default:
		return false, "任务类型不支持循环"
	}
	return true, ""
}

// cycleSortieTurns 估算"这一轮循环任务从放飞到达成再飞回来"要几个回合，
// 用来判断天黑前还值不值得放出去。按刚起飞的状态推算，不看当前燃油。
func cycleSortieTurns(state State, aircraft *AirUnit) int {
	if aircraft == nil {
		return 0
	}
	probe := *aircraft
	probe.Status = aircraft.CycleKind
	probe.Coord = aircraft.Coord
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		probe.Coord = carrier.Coord
	}
	probe.ReconDistance = 0
	probe.TrackingFleetID = ""
	// 侦察半径只有"机型上限"这一个来源，落地被 clearAirTask 清成 0 也不用另存一份。
	probe.ReconRadius = ReconRadiusLimit(*aircraft)
	if probe.Status == "cover" {
		probe.TargetCoord = coverCycleTarget(state, aircraft)
	}
	return TurnsToLanded(state, &probe)
}

// restartCycledTask 让已经落回机库的循环算子重新进入整备。
// 返回事件；不能继续时关闭循环并给出一条说明，避免玩家以为它还在自动值班。
func restartCycledTask(state *State, aircraft *AirUnit) []EngineEvent {
	if aircraft == nil || !aircraft.OnCycle || aircraft.Status != "hangar" {
		return nil
	}
	// 损伤/检修的飞机留在机库修，修好之后还能继续循环。
	if aircraft.Damaged || aircraft.Maintenance > 0 {
		return nil
	}
	// 时间窗没开（珊瑚海 04:00 之前 / 19:40 之后）就趴着等，不算失败。
	if !state.ServiceWindowOpen() {
		return nil
	}
	// 注意：这里不再预先算"天黑前飞不完一轮就不放飞"。
	// 规则是"能放飞就放飞，飞出去发现回不来就掉头"，由每一回合的召回判定负责收手，
	// 而不是让服务器替玩家判断"今天这一轮值不值得"。
	ok, reason := canRestartCycle(*state, aircraft)
	if !ok {
		disarmTaskCycle(aircraft)
		return []EngineEvent{{Type: "air_cycle_end", Audience: aircraft.Faction, Text: fmt.Sprintf("%s的%s循环结束：%s", aircraft.Model, cycleKindName(aircraft.CycleKind), reason)}}
	}
	carrier := state.Units[aircraft.CarrierID]
	kind := aircraft.CycleKind
	taskID := aircraft.CycleTaskID
	if taskID == "" {
		taskID = newFlightTag(state)
	}
	priority := nextAirTaskPriority(state, aircraft.Faction)
	if members := taskAircraft(*state, taskID); len(members) > 0 && members[0].Priority > 0 {
		priority = members[0].Priority
	}
	// 重新挂载前先清干净上一轮的任务参数：任务类型和参数在下面按 Cycle* 重设。
	mission := MissionKind(kind)
	clearAirTask(aircraft)
	aircraft.TaskID = taskID
	aircraft.TaskKind = string(mission)
	aircraft.Priority = priority
	aircraft.Status = "service"
	aircraft.ServiceTurns = 0
	aircraft.Returning = false
	aircraft.Engaged = false
	aircraft.Confusion = false
	aircraft.Coord = carrier.Coord
	aircraft.CannotLand = false
	switch mission {
	case MissionCap:
		patrol := aircraft.CyclePatrol
		if patrol == "" {
			patrol = carrier.Coord
		}
		assignCapMission(aircraft, patrol)
	case MissionRecon:
		// 侦察半径不可设置：老存档里可能存着 0 或旧值，一律重置成机型上限。
		assignReconMission(aircraft, aircraft.CycleHeading, aircraft.CycleSector)
		aircraft.ReconDistance = 0
		aircraft.OriginCoord = carrier.Coord
		aircraft.TrackingFleetID = ""
	case MissionCover:
		assignCoverMission(aircraft, coverCycleTarget(*state, aircraft))
		aircraft.CycleTarget = aircraft.TargetCoord
	case MissionASW:
		assignAswMission(aircraft, carrier.Coord)
	}
	applyLoadout(*state, aircraft)
	return []EngineEvent{{Type: "air_cycle_restart", Audience: aircraft.Faction, Text: fmt.Sprintf("%s完成上一轮%s任务，重新进入机库整备准备再次出动", aircraft.Model, cycleKindName(kind))}}
}

// coverCycleTarget 决定空中掩护编队下一轮飞到哪：优先继续掩护原掩护点上最近的友军舰队，
// 原掩护点已经没有友军时改为掩护离得最近的一支友军舰队（含自己的舰队）。
// 掩护点是一个固定 hex，但舰队会动；直接复用旧 hex 会出现"飞机去掩护一片空海"。
func coverCycleTarget(state State, aircraft *AirUnit) string {
	own := FleetKey(state.Units[aircraft.CarrierID])
	best := ""
	bestDistance := 0
	seen := map[string]bool{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Faction != aircraft.Faction {
			continue
		}
		key := FleetKey(unit)
		if seen[key] {
			continue
		}
		seen[key] = true
		distance := Distance(aircraft.CycleTarget, unit.Coord)
		if key == own {
			// 自己的舰队排在最后：只有在没有别的舰队可掩护时才拿它当掩护点。
			distance += 1000
		}
		if best == "" || distance < bestDistance {
			best = unit.Coord
			bestDistance = distance
		}
	}
	if best == "" {
		return aircraft.CycleTarget
	}
	return best
}

// cycleEnabledKinds 返回当前阵营里处于循环状态的任务类型集合，只用于面板/测试。
func cycleEnabledKinds(state State, faction string) map[string]int {
	counts := map[string]int{}
	for _, aircraft := range state.Aircraft {
		if aircraft == nil || aircraft.Faction != faction || !aircraft.OnCycle {
			continue
		}
		counts[aircraft.CycleKind]++
	}
	return counts
}
