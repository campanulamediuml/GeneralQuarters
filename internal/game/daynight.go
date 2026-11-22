package game

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// —— 昼夜循环与剧本时间线（规则书 199、520–524、587–593 行）——
//
// 回合是 20 分钟（剧本可用 timeline.turn_minutes 覆盖）。剧本给出开局时间（DayHours）
// 与每天回合数（TurnsPerDay，默认 72 = 24 小时），引擎按回合号推算此刻是白天还是夜间。
//
// 剧本没有配置时间时保持旧行为：整备/放飞不做时间限制，昼夜仍按 TurnsPerDay 推算。
// 剧本配置了 timeline 之后：
//   - service_start 之前不进行机库整备（珊瑚海：04:00 开始整备）
//   - launch_start 之前不允许放飞（珊瑚海：05:20 第一轮升空）
//   - recover_by 之后算夜间（珊瑚海：19:40 全员回收截止）
//   - night_loss 为真时，入夜那一刻还在天上的算子直接坠海，不给迫降判定
//
// 规则书 199 行："所有自动循环进行的飞机会在天黑前返航，在天黑前一回合集中降落。"

// NightStartHour / DayStartHour 是没有 timeline 的剧本使用的默认昼夜边界
// （19:40 回收截止，05:20 第一轮升空）。
const (
	NightStartHour = 19.6666667 // 19:40 全员回收截止
	DayStartHour   = 5.3333333  // 05:20 第一轮升空
)

// hourToMinute 把小时数换成"当天第几分钟"。
// 昼夜边界必须按分钟比较：19:40 写成 19.6666667 是四舍五入过的常数，
// 直接比浮点数会让 19:40 这一回合判成白天，回收截止晚一个回合。
func hourToMinute(hour float64) int {
	minutes := int(math.Round(hour * 60))
	minutes %= 1440
	if minutes < 0 {
		minutes += 1440
	}
	return minutes
}

// windowOpen 判断时刻是否落在 [start, end) 之间。时间线不跨午夜
// （整备 04:00 开始、回收 19:40 截止），所以不需要处理环绕。
func windowOpen(hour, start, end float64) bool {
	startMinute, endMinute := hourToMinute(start), hourToMinute(end)
	if endMinute <= startMinute {
		return true
	}
	current := hourToMinute(hour)
	return current >= startMinute && current < endMinute
}

// clockAt 把回合号换算成当天的时刻（小时，0–24）。
func clockAt(state State) float64 {
	if state.TurnsPerDay <= 0 {
		return 0
	}
	day := float64(state.Turn-1) / float64(state.TurnsPerDay)
	hour := state.DayHours + day*24
	hour = hour - 24*float64(int(hour/24))
	if hour < 0 {
		hour += 24
	}
	return hour
}

// clockOfTurn 返回指定回合号的时刻，用于看"下一回合会不会天黑"。
func clockOfTurn(state State, turn int) float64 {
	return clockAt(State{Turn: turn, DayHours: state.DayHours, TurnsPerDay: state.TurnsPerDay})
}

// isNightHour 判断某个时刻是否属于夜间（默认边界）。
func isNightHour(hour float64) bool {
	return !windowOpen(hour, DayStartHour, NightStartHour)
}

// timelineHour 解析剧本时间线上的 "HH:MM"，解析失败返回 fallback。
func timelineHour(value string, fallback float64) float64 {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return fallback
	}
	h, errH := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, errM := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errH != nil || errM != nil || h < 0 || h > 24 || m < 0 || m > 59 {
		return fallback
	}
	return float64(h) + float64(m)/60
}

// ServiceStartHour 是本剧本允许开始机库整备的时刻。
func (state State) ServiceStartHour() float64 {
	if state.Timeline == nil {
		return DayStartHour
	}
	return timelineHour(state.Timeline.ServiceStart, DayStartHour)
}

// LaunchStartHour 是本剧本最早可以放飞的时刻。
func (state State) LaunchStartHour() float64 {
	if state.Timeline == nil {
		return DayStartHour
	}
	return timelineHour(state.Timeline.LaunchStart, DayStartHour)
}

// RecoverByHour 是本剧本的回收截止时刻：超过它就算夜间。
func (state State) RecoverByHour() float64 {
	if state.Timeline == nil {
		return NightStartHour
	}
	return timelineHour(state.Timeline.RecoverBy, NightStartHour)
}

// nightAt 判断某个时刻在本剧本里是否属于夜间。
func (state State) nightAt(hour float64) bool {
	return !windowOpen(hour, state.LaunchStartHour(), state.RecoverByHour())
}

// ServiceWindowOpen 判断当前回合能否进行机库整备。
// 没配 timeline 的剧本不做限制（保持旧行为）。
func (state State) ServiceWindowOpen() bool {
	if state.Timeline == nil {
		return true
	}
	return windowOpen(clockAt(state), state.ServiceStartHour(), state.RecoverByHour())
}

// LaunchWindowOpen 判断当前回合能否放飞。没配 timeline 的剧本不做限制。
func (state State) LaunchWindowOpen() bool {
	if state.Timeline == nil {
		return true
	}
	return windowOpen(clockAt(state), state.LaunchStartHour(), state.RecoverByHour())
}

// launchWindowHint 是给玩家的时间窗提示（"04:20 才能开始整备"这类）。
func (state State) launchWindowHint() string {
	return fmt.Sprintf("%s 之后才能放飞（当前 %s）", formatClock(state.LaunchStartHour()), formatClock(clockAt(state)))
}

// serviceWindowHint 是机库整备被时间窗挡住时的提示。
func (state State) serviceWindowHint() string {
	return fmt.Sprintf("%s 之后才能进行机库整备（当前 %s）", formatClock(state.ServiceStartHour()), formatClock(clockAt(state)))
}

// formatClock 把小时数格式化成 HH:MM。
func formatClock(hour float64) string {
	hour = hour - 24*float64(int(hour/24))
	if hour < 0 {
		hour += 24
	}
	h := int(hour)
	m := int((hour-float64(h))*60 + 0.5)
	if m >= 60 {
		h++
		m -= 60
	}
	if h >= 24 {
		h -= 24
	}
	return fmt.Sprintf("%02d:%02d", h, m)
}

// setNightState 切换天色并产生必要事件。配置了 timeline.night_loss 的剧本，
// 在入夜那一刻把还在天上的算子直接判为坠海（珊瑚海：19:40 之后没有迫降机会）。
func setNightState(state *State, night bool) []EngineEvent {
	if state.TurnsPerDay <= 0 || state.Night == night {
		return nil
	}
	state.Night = night
	if !night {
		return []EngineEvent{{Type: "day_night", Audience: "all", Text: "天色转亮，航空作业恢复正常"}}
	}
	events := []EngineEvent{{Type: "day_night", Audience: "all", Text: "夜幕降临：夜间降落有极高的摔毁风险"}}
	if state.Timeline != nil && state.Timeline.NightLoss {
		events = append(events, forceNightLanding(state)...)
	}
	// 入夜后舰上不再有"待发任务"：全部解除任务、收回机库。
	// 否则玩家看到的是"甲板上还挂着任务、又拖不回机库、也不能放飞"的死状态。
	events = append(events, parkAircraftForNight(state)...)
	return events
}

// ClockView 是给前端用的时间线快照：当前第几天几点、天色、以及两个作业窗口开没开。
// 玩家要按这三个时间点排整备/放飞/回收，光看回合号根本判断不了。
type ClockView struct {
	Day          int    `json:"day"`
	Time         string `json:"time"`
	Night        bool   `json:"night"`
	ServiceOpen  bool   `json:"service_open"`
	LaunchOpen   bool   `json:"launch_open"`
	ServiceStart string `json:"service_start"`
	LaunchStart  string `json:"launch_start"`
	RecoverBy    string `json:"recover_by"`
	TurnMinutes  int    `json:"turn_minutes"`
	// NightInTurns 是距离 19:40 回收截止还有几个回合；-1 表示没配时间线。
	NightInTurns int `json:"night_in_turns"`
}

// Clock 生成时间线快照。剧本没配 DayHours/TurnsPerDay 时返回空值（前端不显示时钟）。
func Clock(state State) *ClockView {
	if state.TurnsPerDay <= 0 {
		return nil
	}
	hour := clockAt(state)
	day := (state.Turn-1)/state.TurnsPerDay + 1
	turnMinutes := 0
	if state.Timeline != nil && state.Timeline.TurnMinutes > 0 {
		turnMinutes = state.Timeline.TurnMinutes
	}
	nightInTurns := -1
	if turns := turnsUntilNight(state); turns < (1 << 19) {
		nightInTurns = turns
	}
	return &ClockView{
		NightInTurns: nightInTurns,
		Day:          day, Time: formatClock(hour), Night: state.nightAt(hour),
		ServiceOpen: state.ServiceWindowOpen(), LaunchOpen: state.LaunchWindowOpen(),
		ServiceStart: formatClock(state.ServiceStartHour()), LaunchStart: formatClock(state.LaunchStartHour()),
		RecoverBy: formatClock(state.RecoverByHour()), TurnMinutes: turnMinutes,
	}
}

// UpdateDayNight 按当前回合号刷新 state.Night。剧本没配昼夜循环时保持原值。
func UpdateDayNight(state *State) []EngineEvent {
	if state.TurnsPerDay <= 0 {
		return nil
	}
	return setNightState(state, state.nightAt(clockAt(*state)))
}

// settleDayNightAtTurnEnd 按"下一回合的天色"收尾：
// 回合结算结束后服务端会把 turn+1 写进快照，night 必须跟那个回合号一致，
// 否则玩家在 05:20 这一回合看到的还是上一回合的夜间标记（功能上能起飞，界面上却写着夜间）。
// 入夜坠海也在这里结算：它恰好发生在"回收截止"的那一刻。
func settleDayNightAtTurnEnd(state *State) []EngineEvent {
	if state.TurnsPerDay <= 0 {
		return nil
	}
	events := setNightState(state, state.nightAt(clockOfTurn(*state, state.Turn+1)))
	// 以"整备窗口关闭"为准，而不是"能不能起飞"：04:00 整备一开就应该允许排明天的任务，
	// 05:20 只是放飞窗口。19:40 回收截止到次日 04:00 之间每回合扫一遍机库，
	// 摸黑返航落回来的算子也一起收掉，任务和自动循环都清空。
	if !state.ServiceWindowOpen() {
		events = append(events, parkAircraftForNight(state)...)
	}
	return events
}

// nightFallsWithinTurns 判断再过 turns 个回合是否已经入夜。
// 召回要提前发：召回命令在回合结束时登记，飞机要到下一回合才真的往回飞，
// 只提前一回合的召回等于没召回（飞机刚掉头就被判坠海）。
func nightFallsWithinTurns(state State, turns int) bool {
	if state.TurnsPerDay <= 0 {
		return false
	}
	return state.nightAt(clockOfTurn(state, state.Turn+turns))
}

// turnsUntilNight 返回距离入夜还有几个回合（入夜那一刻的回合数）。
func turnsUntilNight(state State) int {
	if state.TurnsPerDay <= 0 {
		return 1 << 20
	}
	for turns := 1; turns <= state.TurnsPerDay; turns++ {
		if state.nightAt(clockOfTurn(state, state.Turn+turns)) {
			return turns
		}
	}
	return 1 << 20
}

// TurnsToLanded 返回这架飞机"按现在的任务计划走完、并且落回母舰"还要几个回合，
// 已经含着舰排队的一回合余量。召回必须按这个数发，不能按固定提前量：
//   - 侦察机：先飞完还没走完的那段半径，再从中最远点回家（最坏情况就是它计划的最远圈）。
//   - 打击编队：还要飞到目标，再从目标飞回来——只算"当前位置到母舰"会漏掉整段出击航程。
//   - CAP/掩护：直接在母舰上空附近，算直线回航即可。
func TurnsToLanded(state State, aircraft *AirUnit) int {
	home := aircraft.Coord
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		home = carrier.Coord
	}
	if aircraft.Coord == "" || home == "" {
		return 1
	}
	speed := 2
	if aircraft.Seaplane || aircraft.Status == "recon" {
		speed = reconSpeed(aircraft)
	}
	if speed < 1 {
		speed = 1
	}
	distance := Distance(aircraft.Coord, home)
	switch aircraft.Status {
	case "recon":
		// 侦察机不需要提前规划整趟任务：它每回合都按"从当前位置回不回得去"判断，
		// 回不去就立刻掉头。所以这里只算当前位置到母舰的直线。
		distance = Distance(aircraft.Coord, home)
	case "attack", "attack_holding":
		target := aircraft.TargetCoord
		if target == "" {
			target = home
		}
		distance = Distance(aircraft.Coord, target) + Distance(target, home)
	}
	return (distance+speed-1)/speed + 1
}

// recallLoopingAircraftBeforeNight 让飞行任务在入夜前统一返航（规则书 199 行）。
// 触发条件是"这架飞机按自己离母舰的距离算，已经不够时间回家了"，
// 而不是全局固定提前两回合：同样的两回合，对 1 hex 外的 CAP 是浪费，对 13 hex 外的
// 水上侦察机是死刑。无法及时返航的编队会收到一条明确警告（玩家还能手动召回）。
func recallLoopingAircraftBeforeNight(state *State) []EngineEvent {
	if state.Night {
		return nil
	}
	remaining := turnsUntilNight(*state)
	events := []EngineEvent{}
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || !airborne(aircraft) || aircraft.Returning {
			continue
		}
		need := TurnsToLanded(*state, aircraft)
		// 按"这架飞机自己的计划"折算：走完任务再回家要多少回合 vs 距天黑还剩多少回合。
		// 不管赶不赶得上，都必须掉头——玩家要的是自动返航，不是死在天上。
		if need >= remaining-1 {
			label := map[string]string{"cap": "CAP", "cover": "空中掩护", "recon": "侦察", "attack": "打击编队", "attack_holding": "打击编队"}[aircraft.Status]
			if label == "" {
				label = "飞行任务"
			}
			aircraft.Returning = true
			aircraft.ForwardCoord = ""
			aircraft.TrackingFleetID = ""
			aircraft.TargetCoord = ""
			if need <= remaining {
				events = append(events, EngineEvent{Type: "night_recall", Audience: aircraft.Faction, Text: fmt.Sprintf("%s走完%s还要%d回合、距天黑只剩%d回合，强制返航", aircraft.Model, label, need, remaining)})
			} else {
				events = append(events, EngineEvent{Type: "night_warning", Audience: aircraft.Faction, Text: fmt.Sprintf("%s已经赶不上了：走完任务加返航要%d回合，距天黑只剩%d回合，立即掉头缩短距离", aircraft.Model, need, remaining)})
			}
			continue
		}
		// 还够时间，但快到天黑时自动循环的任务要收手，避免降落之后又自己飞出去一整轮。
		// 只在最后 4 个回合（约 80 分钟）这么做：整备+提升+飞一轮再加返航，已经排不下了。
		if aircraft.OnCycle && remaining <= 4 && (aircraft.Status == "cap" || aircraft.Status == "cover" || aircraft.Status == "recon") {
			aircraft.Returning = true
			aircraft.ForwardCoord = ""
			aircraft.TrackingFleetID = ""
			events = append(events, EngineEvent{Type: "night_recall", Audience: aircraft.Faction, Text: fmt.Sprintf("%s天黑前结束%s循环返航", aircraft.Model, cycleKindName(aircraft.CycleKind))})
		}
	}
	return events
}

// forceNightLanding 处理"天黑那一刻还在天上"的算子。
//
// 不再原地删除：一架 13 hex 外的侦察机和一架就在母舰上空盘旋的 CAP，
// 直接一起"坠海损失"既不公平、也看不到判定过程。现在的处理是：
//   - 全部强制转入返航（还在执行任务的一律终止）；
//   - 燃油够不够决定它能不能回家，不够的按燃油耗尽坠毁；
//   - 回到甲板的按夜间着舰判定（航母 70% 摔毁、机场 20%），回来一架算一架。
// 也就是说 19:40 之后不是"全灭"，而是"摸黑返航，然后各凭运气"。
func forceNightLanding(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || !airborne(aircraft) {
			continue
		}
		if aircraft.Returning {
			continue
		}
		aircraft.Returning = true
		aircraft.ForwardCoord = ""
		aircraft.TrackingFleetID = ""
		aircraft.TargetCoord = ""
		risk := "70%"
		if aircraft.Seaplane {
			risk = "20%"
		}
		events = append(events, EngineEvent{Type: "night_forced_return", Audience: aircraft.Faction, Text: fmt.Sprintf("%s天黑未能返航，强制摸黑返航（剩余航程%d，着舰摔毁风险%s）", aircraft.Model, aircraft.Range(), risk)})
	}
	return events
}

// clearCycleParams 抹掉自动循环参数。夜间清场用：第二天 04:00 之后由玩家重新分配任务，
// 而不是靠昨天的循环参数自己续上（那样等于无视"夜间全员回机库"）。
func clearCycleParams(aircraft *AirUnit) {
	if aircraft == nil {
		return
	}
	aircraft.OnCycle = false
	aircraft.CycleKind = ""
	aircraft.CycleTaskID = ""
	aircraft.CycleHeading = ""
	aircraft.CycleSector = 0
	aircraft.CycleTarget = ""
	aircraft.CyclePatrol = ""
}

// parkAircraftForNight 把舰上（未升空）的算子全部解除任务并收回机库。
// 夜间的航空作业窗口已经关闭，留着任务只会让界面出现"甲板上有任务、又不能放飞、
// 又拖不回机库"的死状态。升降机/回收中的算子不打断，它们下一回合自然落回机库。
// 任务和自动循环一起清掉：第二天 04:00 重新分配，昨天的 CAP/侦察循环不会自己续上。
func parkAircraftForNight(state *State) []EngineEvent {
	parked := 0
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || airborne(aircraft) {
			continue
		}
		if aircraft.TaskID == "" && aircraft.TaskKind == "" && aircraft.Status == "hangar" && !aircraft.OnCycle {
			continue
		}
		switch aircraft.Status {
		case "service", "deck", "ready":
			clearAirTask(aircraft)
			clearCycleParams(aircraft)
			aircraft.Status = "hangar"
			aircraft.ServiceTurns = 0
			aircraft.Armed = false
			parked++
		case "hangar":
			clearAirTask(aircraft)
			clearCycleParams(aircraft)
		}
	}
	// 机场算子同样入夜收场：跑道晚上不开工，任务和循环都不留到第二天。
	for _, id := range sortedGroundAirIDsAll(*state) {
		aircraft := state.GroundAir[id]
		if aircraft == nil || airfieldAirborne(aircraft) {
			continue
		}
		if aircraft.TaskID == "" && aircraft.TaskKind == "" && aircraft.Status == "hangar" {
			continue
		}
		aircraft.TaskID = ""
		aircraft.TaskKind = ""
		aircraft.TargetCoord = ""
		aircraft.TrackingFleetID = ""
		aircraft.Returning = false
		aircraft.Status = "hangar"
		parked++
	}
	if parked == 0 {
		return nil
	}
	return []EngineEvent{{Type: "air_night_parked", Audience: "all", Text: fmt.Sprintf("夜幕降临：%d 个算子解除任务并收回机库，明早重新整备", parked)}}
}

// cycleKindName 是循环任务类型的中文名，只用于战报文本。
func cycleKindName(kind string) string {
	switch kind {
	case "cap":
		return "CAP"
	case "cover":
		return "空中掩护"
	case "recon":
		return "侦察"
	}
	return "任务"
}
