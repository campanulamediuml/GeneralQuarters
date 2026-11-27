package game

import "fmt"

// ValidateStrikeTarget 校验打击目标。
//
// 两种目标走不同的规则：
//   - 舰队：不能盲打。必须先由侦察机（或雷达/舰队侦查）发现，拿到情报后才能派出机群。
//     规则书 606 行"攻击机必须自行搜索并成功接敌"说的是抵达目标附近之后的接敌判定，
//     不是"可以对着任意一格随手扔飞机"——没有情报时玩家根本不知道该往哪飞。
//   - 机场：可以盲打。机场固定不动、永远显示在地图上（"机场又不会动，机场一直显示出来"），
//     玩家能直接在地图上选中它，所以不需要额外情报就能派出机群。
//
// 舰队的匹配必须按「显示位置」而不是情报里的真实坐标（规则书 144–147 行：
// 情报等级越低，显示位置与真实位置的误差越大），否则玩家点了看得见的标记反而会被拒。
func ValidateStrikeTarget(state State, faction, coord string) error {
	if coord == "" {
		return fmt.Errorf("打击任务必须选择目标")
	}
	if !inBounds(state.Map, coord) {
		return fmt.Errorf("打击目标不在地图内")
	}
	// 机场：地图上始终可见，可以直接打。
	if enemyAirfieldAt(state, faction, coord) {
		return nil
	}
	if !knownStrikeCoord(state, faction, coord) {
		return fmt.Errorf("该坐标没有情报，攻击编队不能盲打舰队：请先用侦察机搜索出敌方舰队（打机场可以直接选中）")
	}
	return nil
}

// enemyAirfieldAt 判断 coord 上是否有敌方机场。机场固定不动、位置永远公开，
// 所以这里不看情报等级——只要求是敌方机场。
func enemyAirfieldAt(state State, faction, coord string) bool {
	for _, airfield := range state.Airfields {
		if airfield.Faction != faction && airfield.Coord == coord {
			return true
		}
	}
	return false
}

// knownStrikeCoord 判断 coord 是否落在己方对敌方舰队的「显示位置」情报上。
func knownStrikeCoord(state State, faction, coord string) bool {
	for _, contact := range state.Contacts[faction] {
		if contact == nil || contact.Level <= 0 || contact.Faction == faction {
			continue
		}
		if intelDisplayCoord(*contact) == coord || contact.Coord == coord {
			return true
		}
	}
	// 已侦察到的敌方机场情报同样算数（虽然机场本来就可以直接打）。
	for _, contact := range state.AirfieldContacts[faction] {
		if contact == nil || contact.Faction == faction {
			continue
		}
		if contact.Coord == coord {
			return true
		}
	}
	return false
}

// setStrikeTarget 统一写打击目标：换目标时清掉认领的舰队，
// 下一回合由 retargetStrikeToContact 按最新情报重新认领，避免追着旧舰队不放。
func setStrikeTarget(state *State, aircraft *AirUnit, faction, coord string) {
	if aircraft == nil {
		return
	}
	aircraft.TargetCoord = coord
	aircraft.TargetFleetID = ""
	aircraft.SearchTurns = 0
	aircraft.SearchHeading = ""
	// 认领必须发生在"下命令"这一刻：玩家点是哪个情报标记就去追哪支舰队。
	// 等到编队飞到目标 hex 再认领就晚了——舰队常常已经挪走，届时目标 hex 上
	// 已经没有任何情报，编队会失去追踪对象、对着空 hex 兜圈子。
	if state != nil {
		aircraft.TargetFleetID = resolveStrikeFleet(*state, faction, coord)
	}
}

func setGroundStrikeTarget(state *State, aircraft *AirfieldAirUnit, faction, coord string) {
	if aircraft == nil {
		return
	}
	aircraft.TargetCoord = coord
	aircraft.TargetFleetID = ""
	if state != nil {
		aircraft.TargetFleetID = resolveStrikeFleet(*state, faction, coord)
	}
}

// applyStrikeFleet 把玩家显式点名的那支敌方舰队落到编队上：无线电改目标也认这个字段，
// 否则同格停着几支舰队时只会按情报顺序认领第一支，玩家点的是谁并不作数。
func applyStrikeFleet(state State, aircraft *AirUnit, faction, fleetID string) {
	if aircraft == nil || fleetID == "" {
		return
	}
	aircraft.TargetFleetID = fleetID
	if coord, ok := strikeFleetCoord(state, faction, fleetID); ok {
		aircraft.TargetCoord = coord
	}
}

// resolveStrikeFleet 找出当前目标 hex 对应的敌方舰队，供"目标丢失后继续追踪"使用。
func resolveStrikeFleet(state State, faction, coord string) string {
	if coord == "" {
		return ""
	}
	for _, contact := range state.Contacts[faction] {
		if contact == nil || contact.Faction == faction || contact.Level <= 0 {
			continue
		}
		if intelDisplayCoord(*contact) == coord || contact.Coord == coord {
			return contact.FleetID
		}
	}
	return ""
}

// strikeableContactCoord 返回这支舰队现在"能点到的位置"：
// 在目视范围内直接用真实坐标，否则用当前情报显示坐标（带误差）。
func strikeableContactCoord(state State, faction, fleetID string) (string, bool) {
	contact := state.Contacts[faction][fleetID]
	if contact == nil || contact.Faction == faction || contact.Level <= 0 {
		return "", false
	}
	for _, unit := range state.Units {
		if unit != nil && unit.Faction == faction && Distance(unit.Coord, contact.Coord) <= visualContactRange {
			return contact.Coord, true
		}
	}
	return intelDisplayCoord(*contact), true
}

// retargetStrikeToContact 让打击编队跟着情报走：目标 hex 上已经没有敌舰时，
// 按对这支舰队的最新情报把目标改到它现在的位置，而不是对着空 hex 兜圈子直到油尽。
func retargetStrikeToContact(state *State, aircraft *AirUnit, members []*AirUnit) []EngineEvent {
	if aircraft.TargetCoord == "" || aircraft.Returning {
		return nil
	}
	for _, member := range members {
		if member.Engaged {
			return nil
		}
	}
	if aircraft.TargetFleetID == "" {
		aircraft.TargetFleetID = resolveStrikeFleet(*state, aircraft.Faction, aircraft.TargetCoord)
		if aircraft.TargetFleetID == "" {
			// 打的是固定机场，或者本来就没有这支舰队的情报：没有可追的目标。
			return nil
		}
		for _, member := range members {
			member.TargetFleetID = aircraft.TargetFleetID
		}
	}
	coord, ok := strikeableContactCoord(*state, aircraft.Faction, aircraft.TargetFleetID)
	if !ok || coord == aircraft.TargetCoord {
		return nil
	}
	previous := aircraft.TargetCoord
	for _, member := range members {
		member.TargetCoord = coord
		member.Engaged = false
		// 目标换了地方，搜索计时和搜索航向重新开始。
		member.SearchTurns = 0
		member.SearchHeading = ""
	}
	return []EngineEvent{{Type: "strike_retarget", Audience: aircraft.Faction, Text: fmt.Sprintf("打击编队按最新情报把目标从%s改到%s", previous, coord)}}
}

// interceptAirTask 在编队飞行途中触发雷达引导的 CAP 前出接敌（每个编队每回合一次）。
func interceptAirTask(state *State, faction, coord string, members []*AirUnit) []EngineEvent {
	if len(members) == 0 {
		return nil
	}
	defender := ""
	for _, unit := range state.Units {
		if unit.Faction != faction {
			defender = unit.Faction
			break
		}
	}
	attackers := []*AirUnit{}
	for _, member := range members {
		if operationalAircraft(state, member) {
			attackers = append(attackers, member)
		}
	}
	return interceptIncoming(state, faction, coord, attackers, defender)
}

// nextFlightCoord 返回飞行编队下一格：先用 BFS 判断目标是否可达。
// 可达时优先直飞（距离更近的邻格），不可达（被岛屿/暗礁隔断）时才走绕行路径，
// 避免"岛背面绕路"时反而远离目标。
func nextFlightCoord(state State, aircraft *AirUnit, target string) string {
	if aircraft.Coord == target {
		return ""
	}
	if target == "" {
		return ""
	}
	path := flightPathStep(state, aircraft, target)
	next := nextCoordToward(state.Map, aircraft.Coord, target)
	if next != "" && flightCanEnter(state, aircraft, next) && Distance(next, target) < Distance(aircraft.Coord, target) {
		// 直飞只会更近；若 BFS 已判定要走更远的绕行（岛背面的道），以 BFS 为准，避免来回横跳
		if path == "" || Distance(next, target) <= Distance(path, target) {
			return next
		}
	}
	return path
}

// flightCanEnter 与舰队通行规则保持一致：暗礁一律不可通行；含航母的编队不能进入陆地与浅滩。
// flightCanEnter 判断飞机能否飞入某个 hex。
//
// 飞机在空中，不受舰队的水面通行限制：岛屿、浅滩都能飞越；
// 只有地图边界限制飞行（暗礁是从海面识别的水下障碍，对空袭没有意义）。
// 之前直接复用舰队规则，导致飞往机场（岛屿 hex）的攻击编队永远到不了目标。
func flightCanEnter(state State, aircraft *AirUnit, coord string) bool {
	return inBounds(state.Map, coord)
}

// flightPathStep 用 BFS 在可通行格上找从当前位置到 target 的下一格；找不到返回空字符串。
func flightPathStep(state State, aircraft *AirUnit, target string) string {
	type node struct {
		coord string
		prev  string
	}
	visited := map[string]bool{aircraft.Coord: true}
	queue := []node{{coord: aircraft.Coord}}
	parent := map[string]string{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.coord == target {
			// 回溯到起点的下一格
			step := current.coord
			for {
				prev, ok := parent[step]
				if !ok || prev == aircraft.Coord {
					return step
				}
				step = prev
			}
		}
		for _, neighbor := range Neighbors(current.coord) {
			if visited[neighbor] || !flightCanEnter(state, aircraft, neighbor) {
				continue
			}
			visited[neighbor] = true
			parent[neighbor] = current.coord
			queue = append(queue, node{coord: neighbor})
		}
	}
	return ""
}

func advanceAttack(state *State, aircraft *AirUnit) []EngineEvent {
	if aircraft.Returning {
		return advanceAircraftReturn(state, aircraft)
	}
	members := []*AirUnit{aircraft}
	if aircraft.TaskID != "" {
		members = taskAircraft(*state, aircraft.TaskID)
	}
	if aircraft.TargetCoord == "" {
		for _, member := range members {
			member.Spend(1)
		}
		return nil
	}
	// 本回合每个算子已经飞了几格：一回合的移动预算只有 2 hex（规则书 196 行），
	// 接敌失败后的"继续搜索"也得从这个预算里扣，不能变成"飞 2 格再搜索 1 格"。
	stepsUsed := map[string]int{}
	// 已经接敌成功的编队：本回合直接发起空袭，不再前进、不再搜索。
	// 这一步必须放在"移动 / 雷达拦截"之前——一旦先移动，编队就会从接敌点飞走，
	// 之后既不在 TargetCoord 上、接敌标记也还在，形成"永远接敌但永远不打"的死循环。
	events := []EngineEvent{}
	if aircraft.Engaged && !aircraft.Returning {
		var strikeEvents []EngineEvent
		if aircraft.TargetFleetID == "" && strikeAirfieldAt(*state, aircraft.Faction, aircraft.TargetCoord) != nil {
			strikeEvents = resolveGroundStrike(state, aircraft.TaskID, aircraft.TargetCoord)
		} else {
			strikeEvents = resolveAirStrike(state, aircraft.TaskID, aircraft.TargetCoord)
		}
		if len(strikeEvents) > 0 {
			return strikeEvents
		}
		// 目标格上已经没有任何敌舰：敌舰队可能在上一回合的移动阶段跑掉了。
		// 此时必须作废接敌状态，否则编队会停在空海面，不耗油、不返航、不发事件，永远卡死。
		for _, member := range members {
			member.Engaged = false
		}
		events = append(events, EngineEvent{Type: "attack_target_lost", Audience: aircraft.Faction, Text: fmt.Sprintf("%s的目标%s已无敌军，重新搜索", aircraft.Model, aircraft.TargetCoord)})
	}
	// 目标丢了就按最新情报改追：打击任务记的是 hex，敌方舰队一移动，
	// 编队就会对着空 hex 兜圈子直到油尽，所以每回合用情报校正一次目标位置。
	events = append(events, retargetStrikeToContact(state, aircraft, members)...)
	// 雷达引导拦截：被敌方雷达发现的编队会在路上被 CAP 前出接敌
	events = append(events, interceptAirTask(state, aircraft.Faction, aircraft.Coord, members)...)
	// 本回合开始时就已经在目标 hex 的编队只做接敌判定，不再推进，也不重复消耗航程。
	alreadyOnTarget := aircraft.Coord == aircraft.TargetCoord
	arrived := alreadyOnTarget
	for _, member := range members {
		// 剩余航程 <= 回到母舰/备降机场所需距离时停止前进并返航（规则书 618 行）。
		// 这里用"剩余航程 == 返航距离"作阈值，而不是等到燃油真的耗尽。
		// 只推进已经离场的成员：混在编队里的 attack_holding（后加入的护航/还没起飞）
		// 不能跟着一起飞到目标上空——那会让它停在目标上却还挂着"等待编队"。
		if member.Returning || member.Status != "attack" || member.Coord == member.TargetCoord || alreadyOnTarget {
			continue
		}
		// 每回合按机种走 1–2 hex（规则书 200 行），但每一步之前都要确认走完还回得了家。
		for steps := 0; steps < aircraftSpeed(member) && member.Coord != member.TargetCoord; steps++ {
			next := nextFlightCoord(*state, member, member.TargetCoord)
			if next == "" {
				break
			}
			probe := *member
			probe.Coord = next
			probe.SetRange(member.Range() - 1)
			// 抵达目标的那一步不做返航预判：到了目标上空才算"完成任务"，之后由接敌/空袭流程处理。
			if next != member.TargetCoord && hasRTBDestination(*state, member) && probe.Range() <= rtbDistance(*state, &probe) {
				member.Returning = true
				events = append(events, EngineEvent{Type: "attack_rtb", Audience: member.Faction, Text: fmt.Sprintf("%s剩余航程%d已到达返航下限（返航需%d），停止搜索返航", member.Model, member.Range(), rtbDistance(*state, member))})
				break
			}
			member.Coord = next
			member.Spend(1)
			stepsUsed[member.ID]++
		}
	}
	if !arrived && aircraft.Coord == aircraft.TargetCoord {
		// 编队抵达目标上空：API 文档承诺记录 attack_arrived 战报，之前只有单机任务能记到。
		events = append(events, EngineEvent{Type: "attack_arrived", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s目标上空，等待后续空袭结算", aircraft.Model, aircraft.TargetCoord)})
	}
	if aircraft.TaskID != "" && aircraft.Coord == aircraft.TargetCoord {
		// 接敌判定：目标同 hex 基础 200%、相邻 1 hex 基础 100% 乘天气系数并叠加修正。
		// 接敌成功后要"下一回合"才能进入目标上空发起空袭（规则书 609 行）。
		// 已经抵达目标 hex：本回合不再推进，只做接敌判定。
		if !aircraft.Engaged {
			// 目标 hex 上是敌方机场：机场不是水面舰队，engageChance 永远返回 0，
			// 直接视为接敌成功，下一回合交给 resolveGroundStrike 结算对地攻击。
			if strikeAirfieldAt(*state, aircraft.Faction, aircraft.TargetCoord) != nil {
				for _, member := range members {
					member.Engaged = true
				}
				return append(events, EngineEvent{Type: "attack_engaged", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s敌方机场上空，下回合发起对地攻击", aircraft.Model, aircraft.TargetCoord)})
			}
			if !engagedThisTurn(state, aircraft) {
				events = append(events, tryEngage(state, aircraft, aircraft.Coord)...)
			}
			if aircraft.Engaged {
				// 接敌成功的这一回合停在目标上空，下一回合才发起空袭（规则书 609 行）。
				return events
			}
			// 接敌失败：每回合沿当前航向继续搜索（规则书 617 行），
			// 正前方 50%、两个斜前方各 25%；移动后仍失败就留在新位置，下一回合再判定。
			budget := 2 - stepsUsed[aircraft.ID]
			if budget < 0 {
				budget = 0
			}
			return append(events, searchAfterFailedEngage(state, aircraft, budget)...)
		}
		// 已接敌：本回合发起空袭（目标 hex 上有敌方机场走对地攻击，否则对舰）
		if strikeAirfieldAt(*state, aircraft.Faction, aircraft.TargetCoord) != nil {
			return append(events, resolveGroundStrike(state, aircraft.TaskID, aircraft.TargetCoord)...)
		}
		return append(events, resolveAirStrike(state, aircraft.TaskID, aircraft.TargetCoord)...)
	}
	// 还在飞往目标的路上：保留本回合累计的事件（改追目标、雷达拦截、抵达）。
	// 之前这里直接 return nil，导致 strike_retarget / attack_arrived 从来没有真正下发过。
	return events
}

// searchAfterFailedEngage 实现规则书 617 行：接敌失败的攻击编队每回合沿当前航向继续搜索——
// 向正前方移动 1hex 的概率为 50%，向两个斜前方各移动 1hex 的概率分别为 25%，移动后再判定接敌。
// 如果移动后的位置也到不了（出界/暗礁），就退化成可选的方向里重新归一化。
// engagedThisTurn 判断该编队本回合是否已经做过接敌判定，
// 避免同一编队在一回合里被不同算子反复触发判定。
func engagedThisTurn(state *State, aircraft *AirUnit) bool {
	if state.EngageTried == nil {
		return false
	}
	return state.EngageTried[aircraft.TaskID]
}

func searchAfterFailedEngage(state *State, aircraft *AirUnit, budget int) []EngineEvent {
	// budget 是本回合还剩几步可走：赶路已经把 2 hex 用完时，这回合只做接敌判定、不再移动。
	if aircraft == nil || aircraft.Returning || budget <= 0 {
		return nil
	}
	members := []*AirUnit{aircraft}
	if aircraft.TaskID != "" {
		members = taskAircraft(*state, aircraft.TaskID)
	}
	events := []EngineEvent{}
	for _, member := range members {
		if member.Returning || member.Coord == "" {
			continue
		}
		// 搜索是有成本的：对着过期情报在空海上兜圈，每一回合都在烧油。
		// 走之前先确认回得去（留 2 点余量），烧到临界点就停止搜索返航；
		// 不设固定回合上限——规则书 622 行只要求"剩余航程不够返航时才停止搜索"。
		if hasRTBDestination(*state, member) {
			if need := rtbDistance(*state, member); member.Range() <= need+2 {
				member.Returning = true
				events = append(events, EngineEvent{Type: "attack_rtb", Audience: member.Faction, Text: fmt.Sprintf("%s剩余航程%d，只够返航（需%d），停止搜索返航", member.Model, member.Range(), need)})
				continue
			}
		}
		member.SearchTurns++
		// 搜索方向只有一个初始值：第一次搜索时朝最后已知位置（或者母舰航向）。
		// 之后再也不用"往坐标飞"来算方向——那样编队一到目标点就会折回去，
		// 26 架飞机在 2 格见方里来回摆，等于原地打转。
		if member.SearchHeading == "" {
			member.SearchHeading = initialSearchHeading(*state, member)
		}
		type step struct {
			heading string
			coord   string
		}
		options := searchHeadings(member.SearchHeading)
		candidates := []step{}
		for _, candidateHeading := range options {
			if next := nextSearchStep(*state, member, candidateHeading); next != "" {
				candidates = append(candidates, step{heading: candidateHeading, coord: next})
			}
		}
		if len(candidates) == 0 {
			continue
		}
		// 正前方权重 2/4（50%），两个斜前方各 1/4（25%）；方向不可用时在剩余方向里等概率。
		chosen := candidates[0]
		if len(candidates) == len(options) {
			switch nextUint64(&state.RNGState) % 4 {
			case 0, 1:
				chosen = candidates[0]
			case 2:
				chosen = candidates[1]
			default:
				chosen = candidates[2]
			}
		} else {
			chosen = candidates[nextUint64(&state.RNGState)%uint64(len(candidates))]
		}
		member.Coord = chosen.coord
		member.SearchHeading = chosen.heading
		member.Returning = false
		if member.Range() > 0 {
			member.Spend(1)
		}
		events = append(events, EngineEvent{Type: "attack_searching_move", Audience: aircraft.Faction, Text: fmt.Sprintf("%s沿%s方向继续搜索，转移至%s", member.Model, chosen.heading, chosen.coord)})
	}
	return events
}

// initialSearchHeading 给出搜索的起始航向：优先朝最后已知位置，
// 已经站在那个 hex 上时就顺着编队自己的航向往前压。
func initialSearchHeading(state State, aircraft *AirUnit) string {
	if aircraft.TargetCoord != "" && aircraft.Coord != aircraft.TargetCoord {
		if heading := headingBetween(aircraft.Coord, aircraft.TargetCoord); heading != "" {
			return heading
		}
	}
	if aircraft.Heading != "" {
		return aircraft.Heading
	}
	if heading := originHeading(*aircraft); heading != "" {
		return heading
	}
	return "E"
}

// searchHeadings 返回搜索时可选的三个方向：正前、左斜前、右斜前。
func searchHeadings(heading string) []string {
	order := []string{"E", "SE", "SW", "W", "NW", "NE"}
	index := 0
	for i, item := range order {
		if item == heading {
			index = i
			break
		}
	}
	right := order[(index+1)%6]
	left := order[(index+5)%6]
	return []string{heading, right, left}
}

// nextSearchStep 沿指定方向走一格；不可通行时返回空串。
func nextSearchStep(state State, aircraft *AirUnit, heading string) string {
	next := nextReconCoord(state.Map, aircraft.Coord, heading)
	if next == "" || !flightCanEnter(state, aircraft, next) {
		return ""
	}
	return next
}

// originHeading 在编队没有记录航向时，用出发点到当前位置推断朝向。
func originHeading(aircraft AirUnit) string {
	if aircraft.OriginCoord == "" || aircraft.Coord == "" {
		return ""
	}
	return headingBetween(aircraft.OriginCoord, aircraft.Coord)
}

// ValidateStrikeFleetTarget 校验“显式指定打某支舰队”。
// 同一 hex 同时有敌方机场和舰队时，玩家用 target_fleet_id 选择舰队；
// coord 可以为空，服务端会取该舰队当前的情报显示位置。
func ValidateStrikeFleetTarget(state State, faction, fleetID, coord string) error {
	contact := state.Contacts[faction][fleetID]
	if contact == nil || contact.Faction == faction || contact.Level <= 0 {
		return fmt.Errorf("没有这支舰队的情报，不能指定为打击目标")
	}
	if coord != "" && intelDisplayCoord(*contact) != coord && contact.Coord != coord {
		return fmt.Errorf("舰队%s当前不在%s", fleetID, coord)
	}
	return nil
}

// strikeFleetCoord 返回显式指定舰队时应使用的目标坐标。
func strikeFleetCoord(state State, faction, fleetID string) (string, bool) {
	contact := state.Contacts[faction][fleetID]
	if contact == nil || contact.Faction == faction || contact.Level <= 0 {
		return "", false
	}
	return intelDisplayCoord(*contact), true
}
