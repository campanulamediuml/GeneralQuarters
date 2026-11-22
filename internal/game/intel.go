package game

import (
	"fmt"
	"math"
	"sort"
)

// —— 情报等级的位置误差（规则书 142–147 行）——
//
// 情报等级 1：只显示阵营方块，位置误差可达 5 hex；
// 情报等级 2：只显示编队大小，误差可达 3 hex；
// 情报等级 3：显示船只类型，误差可达 1 hex；
// 情报等级 4：显示前两艘船剪影，位置准确。
// 误差是针对"最后发现位置"的固定偏移：同一份情报每次投影到屏幕的位置应保持一致，
// 否则玩家每刷新一次看到的位置都在跳，反而比不显示更糟。

// intelErrorRadius 返回某情报等级的最大位置误差（hex）。
func intelErrorRadius(level int) int {
	switch {
	case level <= 1:
		return 5
	case level == 2:
		return 3
	case level == 3:
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
		next := neighborToward(MapDefinition{Rows: 99, Cols: 99}, current, directionVector(heading))
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

// —— 情报过时（规则书 587–593 行）——

// contactDecayInterval 返回白天时该等级情报掉一级需要"脱离接触"多少回合。
//
// 规则书只给了"每回合/每两回合"的粗粒度节奏，但 1–2 级情报本身误差就有 3–5 hex，
// 打击链路（整备 3 回合 + 飞行 2 hex/回合）根本来不及：实测里 1 级情报两回合就消失，
// 设过去的打击编队只能对着空海搜索。低等级情报给更长的尾巴，高等级维持原节奏。
// contactDecayInterval 白天脱离接触后每 2 回合降一级（规则书 591 行）。
// 不要按等级放宽：情报存续时间是规则的一部分，"1 级活太久"属于数值问题，
// 应该通过侦察节奏或打击链路去调，而不是改规则。
func contactDecayInterval(level int) int {
	return 2
}

// decayContactIntel 在回合结束时衰减情报：
// 上一回合没有获得该舰队的任何新侦查结果、也没有侦察机持续跟踪时，视为脱离接触。
// 白天按情报等级每 2–4 回合 -1 级（等级越低存续越久），夜间每 1 回合 -1 级；
// 衰减不改变显示位置，继续停留在最后发现位置。低等级情报在掉级前会提前发"即将失效"提示。
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
			if !state.Night {
				if state.IntelStaleTurns == nil {
					state.IntelStaleTurns = map[string]int{}
				}
				state.IntelStaleTurns[key]++
				interval := contactDecayInterval(contact.Level)
				if state.IntelStaleTurns[key]%interval != 0 {
					// 低等级情报快到期时提前预警，别让玩家以为坐标还在有效期内。
					if contact.Level <= 2 && state.IntelStaleTurns[key]%interval == interval-1 {
						name := contact.FleetID
						if len(contact.Members) > 0 {
							name = contact.Members[0].FleetName
						}
						events = append(events, EngineEvent{Type: "contact_stale", Audience: observer, Text: fmt.Sprintf("%s的%d级情报已过期%d回合，位置误差最大%d hex，下一回合将降级", name, contact.Level, state.IntelStaleTurns[key], intelErrorRadius(contact.Level))})
					}
					continue
				}
			}
			if state.IntelStaleTurns != nil {
				state.IntelStaleTurns[key] = 0
			}
			contact.Level--
			name := contact.FleetID
			if len(contact.Members) > 0 {
				name = contact.Members[0].FleetName
			}
			if contact.Level <= 0 {
				delete(state.Contacts[observer], fleetID)
				events = append(events, EngineEvent{Type: "contact_lost", Audience: observer, Text: fmt.Sprintf("%s的情报已完全过时，接触消失", name)})
				continue
			}
			events = append(events, EngineEvent{Type: "contact_decay", Audience: observer, Text: fmt.Sprintf("%s脱离接触，情报等级降为%d", name, contact.Level)})
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
