package game

import (
	"fmt"
	"math"
	"sort"
)

// —— 情报等级的位置误差（规则书 142–147 行）——
//
// 情报等级 1：只显示阵营方块，位置误差 2 hex；
// 情报等级 2：只显示编队大小，误差 1 hex；
// 情报等级 3：显示船只类型，位置准确；
// 情报等级 4：显示前两艘船剪影，位置准确。
//
// 规则书原表给的是 5/3/1：5 hex 的误差比打击链路能承受的还大（整备 3 回合 +
// 每回合飞 2 hex），标记和真身隔着 5 格时飞行员连往哪飞都不知道，
// 结果是"有情报也打不着"。这里按实测下调为 2/1/0/0。
// 误差是针对"最后发现位置"的固定偏移：同一份情报每次投影到屏幕的位置应保持一致，
// 否则玩家每刷新一次看到的位置都在跳，反而比不显示更糟。

// intelErrorRadius 返回某情报等级的最大位置误差（hex）。
func intelErrorRadius(level int) int {
	switch {
	case level <= 1:
		return 2
	case level == 2:
		return 1
	}
	return 0
}

// intelDisplayCoord 把真实位置按情报等级换算成"玩家看到的位置"。
// 用舰队 ID 和情报等级做确定性散列，保证同一份情报的偏移稳定可复现。
// visualContactRange 是"不需要任何侦察手段、直接目视就能看见"的距离。
// VisibleUnits 按这个距离把敌方舰队画到地图上，所以情报侧必须用同一个半径记接触，
// 否则会出现"地图上看得到舰队、点下去服务端说没有情报、不能打"的割裂。
const visualContactRange = 2

// intelMaxLevel 是情报等级上限：4 级包含完整成员表、误差为 0（规则书 585–600 行）。
const intelMaxLevel = 4

func intelDisplayCoord(contact FleetContact) string {
	if contact.Level >= 4 || intelErrorRadius(contact.Level) <= 0 {
		return contact.Coord
	}
	// 已经落定的显示位置优先：规则书 593 行要求情报衰减不改变算子显示位置。
	if contact.DisplayCoord != "" {
		return contact.DisplayCoord
	}
	return projectedIntelCoord(contact)
}

// projectedIntelCoord 按情报等级把真实位置投影成带误差的显示位置。
// 方向只由舰队 ID 决定、幅度由等级决定：等级提高时标记沿同一条射线向真实位置收敛，
// 而情报衰减根本不会调用这里（decayContactIntel 只降 Level，不改 DisplayCoord），
// 所以地图上的"最后已知"不会自己跳走。
func projectedIntelCoord(contact FleetContact) string {
	radius := intelErrorRadius(contact.Level)
	if radius <= 0 {
		return contact.Coord
	}
	hash := uint32(2166136261)
	for _, char := range []byte(contact.FleetID) {
		hash ^= uint32(char)
		hash *= 16777619
	}
	// 规则书写的是"至少有 N hex 位置差异"：误差下限取上限的 60% 左右（向上取整），
	// 再在 [下限, 上限] 之间散列，避免情报 1 级只偏 1 格这种名不副实的情况。
	lower := (radius*3 + 4) / 5
	if lower < 1 {
		lower = 1
	}
	span := radius - lower + 1
	if span < 1 {
		span = 1
	}
	distance := radius - int(hash%uint32(span))
	if distance < lower {
		distance = lower
	}
	axis := int((hash >> 8) % 6)
	return stepInDirection(contact.Coord, axis, distance)
}

// stepInDirection 沿六个方向之一走 steps 格（方向 0..5 对应 E/SE/SW/W/NW/NE 的顺序）。
func stepInDirection(coord string, axis, steps int) string {
	directions := []string{"E", "SE", "SW", "W", "NW", "NE"}
	heading := directions[((axis%6)+6)%6]
	current := coord
	for index := 0; index < steps; index++ {
		next := neighborToward(MapDefinition{Rows: 99, Cols: 99}, current, directionVector(heading), nil)
		if next == "" {
			break
		}
		current = next
	}
	return current
}

// directionVector 把朝向名换算成 hex 平面上的单位方向。
func directionVector(heading string) sectorPoint {
	angle, ok := reconHeadings[heading]
	if !ok {
		angle = reconHeadings["E"]
	}
	return sectorPoint{x: math.Cos(angle * math.Pi / 180), y: math.Sin(angle * math.Pi / 180)}
}

// —— 情报过时（规则书 604–614 行）——

// contactDecayLimit 返回白天某等级的情报在脱离接触后再维持几个回合才衰减一级：
// 4 级（编成与状态已知）6 回合，3/2/1 级各 3 回合。
// 也就是从满级往下走 4→3（6 回合）→2（+3）→1（+3）→0 接触消失（+3），
// 满级情报全须全尾地保留 6 回合后开始一级一级变模糊，15 回合后彻底失去接触。
func contactDecayLimit(level int) int {
	if level >= intelMaxLevel {
		return 6
	}
	return 3
}

// contactNightDecayTurns 是夜间脱离接触的存续上限：夜间不逐级衰减，
// 满 4 回合直接归零（规则书 609 行）。夜里的目视和无线电条件太差，
// 分级模糊没有意义——要么还知道大致方位，要么彻底失去接触。
const contactNightDecayTurns = 4

// decayContactIntel 在回合结束时结算情报过时：
// 上一回合没有获得该舰队的任何新侦查结果、也没有侦察机持续跟踪时，视为脱离接触。
// 白天按等级逐级衰减（规则书 604–612 行），每降一级重新计时：
// 4 级撑 6 回合，之后 3/2/1 级各撑 3 回合，1 级再到期就归零（接触消失）。
// 夜间不逐级衰减：满 4 回合直接归零。
// 降级不改变显示位置——地图上的算子一直停在最后发现位置，
// 只是等级越低，玩家能看到的编成信息越少、坐标越不可信。
func decayContactIntel(state *State) []EngineEvent {
	events := []EngineEvent{}
	if len(state.Contacts) == 0 {
		return events
	}
	observers := make([]string, 0, len(state.Contacts))
	for observer := range state.Contacts {
		observers = append(observers, observer)
	}
	sort.Strings(observers)
	for _, observer := range observers {
		fleetIDs := make([]string, 0, len(state.Contacts[observer]))
		for fleetID := range state.Contacts[observer] {
			fleetIDs = append(fleetIDs, fleetID)
		}
		sort.Strings(fleetIDs)
		for _, fleetID := range fleetIDs {
			contact := state.Contacts[observer][fleetID]
			if contact == nil || contact.Level <= 0 {
				if contact != nil {
					delete(state.Contacts[observer], fleetID)
				}
				continue
			}
			if state.IntelFresh[observer+"|"+fleetID] {
				continue
			}
			if reconTrackingFleet(*state, observer, fleetID) {
				continue
			}
			key := observer + "|" + fleetID
			if state.IntelStaleTurns == nil {
				state.IntelStaleTurns = map[string]int{}
			}
			// IntelStaleTurns 只记"当前等级还剩几回合"的计数器，降级后清零重新计时；
			// StaleTurns 是这条情报总共几回合没更新，只增不减，给玩家和 AI 判断坐标可信度。
			state.IntelStaleTurns[key]++
			contact.StaleTurns++
			name := contact.FleetID
			if len(contact.Members) > 0 {
				name = contact.Members[0].FleetName
			}
			// 夜间不逐级衰减：满 4 回合直接归零，只留"下一回合失去接触"的预警。
			if state.Night {
				if contact.StaleTurns < contactNightDecayTurns {
					if contact.StaleTurns == contactNightDecayTurns-1 {
						events = append(events, EngineEvent{Type: "contact_stale", Audience: observer, Text: fmt.Sprintf("%s的情报已过期%d回合（夜间上限%d回合），下一回合将失去接触", name, contact.StaleTurns, contactNightDecayTurns)})
					}
					continue
				}
				delete(state.IntelStaleTurns, key)
				delete(state.Contacts[observer], fleetID)
				events = append(events, EngineEvent{Type: "contact_lost", Audience: observer, Text: fmt.Sprintf("%s脱离接触已%d回合（夜间上限%d回合），情报归零，接触消失", name, contact.StaleTurns, contactNightDecayTurns)})
				continue
			}
			limit := contactDecayLimit(contact.Level)
			if state.IntelStaleTurns[key] < limit {
				// 到期前一回合预警：4 级以上说明会掉到几级，1 级说明会失去接触。
				if state.IntelStaleTurns[key] == limit-1 {
					if contact.Level <= 1 {
						events = append(events, EngineEvent{Type: "contact_stale", Audience: observer, Text: fmt.Sprintf("%s的情报已过期%d回合，下一回合将失去接触", name, contact.StaleTurns)})
					} else {
						events = append(events, EngineEvent{Type: "contact_stale", Audience: observer, Text: fmt.Sprintf("%s的情报已过期%d回合（%d级上限%d回合），下一回合将降为%d级", name, contact.StaleTurns, contact.Level, limit, contact.Level-1)})
					}
				}
				continue
			}
			state.IntelStaleTurns[key] = 0
			contact.Level--
			if contact.Level <= 0 {
				delete(state.IntelStaleTurns, key)
				delete(state.Contacts[observer], fleetID)
				events = append(events, EngineEvent{Type: "contact_lost", Audience: observer, Text: fmt.Sprintf("%s脱离接触已%d回合，情报耗尽，接触消失", name, contact.StaleTurns)})
				continue
			}
			events = append(events, EngineEvent{Type: "contact_decayed", Audience: observer, Text: fmt.Sprintf("%s的情报降为%d级（脱离接触%d回合没有新情报）", name, contact.Level, contact.StaleTurns)})
		}
	}
	return events
}

// reconTrackingFleet 判断某阵营是否有侦察机正在持续跟踪该舰队。
func reconTrackingFleet(state State, observer, fleetID string) bool {
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == observer && aircraft.TrackingFleetID == fleetID && airborne(aircraft) {
			return true
		}
	}
	for _, aircraft := range state.GroundAir {
		if aircraft.Faction == observer && aircraft.TrackingFleetID == fleetID && airfieldAirborne(aircraft) {
			return true
		}
	}
	return false
}

// resetIntelFresh 在回合开始时清空"本回合是否获得新情报"的标记。
func resetIntelFresh(state *State) {
	state.IntelFresh = map[string]bool{}
}
