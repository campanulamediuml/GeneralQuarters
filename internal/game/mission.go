package game

import "strings"

// —— 航空任务抽象 ——
//
// 规则里有四种互相独立的空中行为：扇面侦察、战斗空中巡逻（CAP）、空中掩护、打击编队
// （外加转场，本质是"换基地的移动"）。它们各有自己的参数集合：
//
//	侦察：航向 + 扇面 + 半径 + 顶点 + 已飞距离 + 跟踪目标
//	CAP ：巡逻点 + 前出点
//	掩护：掩护点
//	打击：目标 hex + 目标舰队 + 分配方式 + 挂载
//
// 以前这些参数全平铺在 AirUnit 的 60 个字段里，靠裸字符串 TaskKind 区分，
// 初始化散在接单/改令/循环重启四个入口。漏写一个字段就会得到"合法但不正确"的状态：
// 侦察半径停在 0、当回合就满足"扫完扇面"直接返航，肖特兰的 H6K 就这么废了整场。
//
// 现在任务类型是 MissionKind 常量，每种行为的参数只允许在对应的 assignXxx 里写，
// 接单入口只负责决定"哪架飞机、哪个任务组"，不再各自拼字段。
type MissionKind string

const (
	// MissionCap 战斗空中巡逻：跟着母舰走，可以被雷达拉出去前出拦截。
	MissionCap MissionKind = "cap"
	// MissionRecon 扇面侦察：从顶点出发沿航向扫一个 60/120 度的锥形区域。
	MissionRecon MissionKind = "recon"
	// MissionStrike 打击编队：多架算子共享一个目标 hex。
	MissionStrike MissionKind = "attack"
	// MissionCover 空中掩护：飞到某个 hex 上空替友军舰队挡空袭。
	MissionCover MissionKind = "cover"
	// MissionTransfer 转场：换一艘母舰或上岸。
	MissionTransfer MissionKind = "transfer"
)

// missionKindFromOrder 把命令类型翻译成任务类型。
func missionKindFromOrder(orderType string) MissionKind {
	switch orderType {
	case "LAUNCH_RECON":
		return MissionRecon
	case "LAUNCH_STRIKE":
		return MissionStrike
	case "LAUNCH_COVER":
		return MissionCover
	case "LAUNCH_TRANSFER":
		return MissionTransfer
	}
	return MissionCap
}

// beginAirMission 是所有舰载任务接单的共同部分：清参数、挂任务组、优先级、循环开关。
func beginAirMission(aircraft *AirUnit, taskID string, kind MissionKind, priority int) {
	clearAirTask(aircraft)
	aircraft.TaskID = taskID
	aircraft.TaskKind = string(kind)
	aircraft.Priority = priority
	// 规则书 188 行：新编入 CAP／反潜巡逻／侦察任务的飞机自动进入任务循环。
	// 玩家点掉循环开关之后落地也不会自己恢复（见 rememberCycleParams）。
	aircraft.OnCycle = cyclicTaskKind(string(kind))
	aircraft.CycleOptOut = false
	// 换任务就是重新整备一轮（规则书 258 行换挂载还要额外 1 回合）。
	aircraft.ServiceTurns = 0
}

// assignReconMission 写死侦察任务的全部派生参数。
// 半径不来自任何输入：一律按机型上限算，所以构造完必定大于 0。
// 顶点和已飞距离不在这里写——那是起飞时（launchAirTask）和每回合推进时的事。
func assignReconMission(aircraft *AirUnit, heading string, sector int) {
	aircraft.Heading = strings.ToUpper(heading)
	if !ValidReconHeading(aircraft.Heading) {
		aircraft.Heading = "E"
	}
	aircraft.SectorWidth = sector
	if aircraft.SectorWidth != 120 {
		aircraft.SectorWidth = 60
	}
	aircraft.ReconRadius = ReconRadiusLimit(*aircraft)
}

// assignCapMission 设置巡逻点。patrol 为空表示"跟着母舰走"——
// capCoord 会逐回合回落到母舰当前 hex，比在接单时写死一个坐标更不会过期。
func assignCapMission(aircraft *AirUnit, patrol string) {
	aircraft.PatrolCoord = patrol
	aircraft.ForwardCoord = ""
	aircraft.TargetCoord = ""
}

// assignStrikeMission 认领打击目标：目标 hex 打空之后靠 TargetFleetID 追最新情报位置。
func assignStrikeMission(state *State, aircraft *AirUnit, coord string) {
	setStrikeTarget(state, aircraft, aircraft.Faction, coord)
}

// assignCoverMission 设置掩护点。
func assignCoverMission(aircraft *AirUnit, coord string) {
	aircraft.TargetCoord = coord
}

// assignTransferMission 设置转场目的地。
func assignTransferMission(aircraft *AirUnit, coord string) {
	aircraft.TargetCoord = coord
}

// —— 机场航空器（没有母舰，规则不同，参数集单独一套）——

// beginGroundMission 是机场任务接单的共同部分。机场任务没有升降机循环，
// 也没有 OnCycle 开关：起飞靠跑道，落地靠跑道。
func beginGroundMission(aircraft *AirfieldAirUnit, taskID string, kind MissionKind, priority int) {
	aircraft.TaskID = taskID
	aircraft.TaskKind = string(kind)
	aircraft.Priority = priority
}

// assignGroundReconMission 写死机场侦察的参数，半径同样只取机型上限。
func assignGroundReconMission(aircraft *AirfieldAirUnit, heading string, sector int) {
	aircraft.Heading = strings.ToUpper(heading)
	if !ValidReconHeading(aircraft.Heading) {
		aircraft.Heading = "E"
	}
	aircraft.SectorWidth = sector
	if aircraft.SectorWidth != 120 {
		aircraft.SectorWidth = 60
	}
	aircraft.ReconRadius = GroundReconRadius(aircraft)
}

// assignGroundCapMission 设置机场 CAP 的巡逻点：没指定就是机场本身。
func assignGroundCapMission(aircraft *AirfieldAirUnit, coord string) {
	aircraft.TargetCoord = coord
	if aircraft.TargetCoord == "" {
		aircraft.TargetCoord = aircraft.Coord
	}
}

// assignGroundStrikeMission 认领对舰/对地打击目标。
func assignGroundStrikeMission(state *State, aircraft *AirfieldAirUnit, coord string) {
	setGroundStrikeTarget(state, aircraft, aircraft.Faction, coord)
}
