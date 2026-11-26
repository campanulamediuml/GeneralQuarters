package game

import (
	"fmt"
	"sort"
)

// —— 敌方飞行编队的可见性 ——
//
// 规则书没有单独写"飞机怎么被看到"，但雷达一节（707–722 行）明确雷达能提前发现来袭编队，
// 侦察机则是"发现敌方舰队"的主力。这里按这个逻辑实现：
//
//   - 任何一方的**水面舰船或机场雷达**截获某个敌方编队时，该编队对防守方永久可见（同一次航线）；
//   - **侦察机**每回合的搜索圈如果盖住敌方编队所在格，同样标记为已发现；
//   - 空中编队互相靠近到 1 hex 内视为目视接触，也会被发现（否则"我在它头顶它看不见我"很荒谬）。
//
// 已发现编队只暴露对对手有意义的信息：编队种类、机种、数量、当前坐标与航向，
// 不暴露剩余航程、任务优先级这类内部调度数据。

// AirContact 是"我看到的一支敌方飞行编队"。
type AirContact struct {
	TaskID        string   `json:"task_id"`
	Faction       string   `json:"faction"`
	Kind          string   `json:"kind"`
	Models        []string `json:"models"`
	Count         int      `json:"count"`
	Coord         string   `json:"coord"`
	Heading       string   `json:"heading,omitempty"`
	TargetCoord   string   `json:"target_coord,omitempty"`
	DetectedBy    string   `json:"detected_by"` // radar / recon / visual
	Turn          int      `json:"turn"`
	Returning     bool     `json:"returning,omitempty"`
	AttackHolding bool     `json:"attack_holding,omitempty"`
}

// markAirContact 记录"观察者阵营已经发现该编队"。
func markAirContact(state *State, observer, taskID, how string) bool {
	if observer == "" || taskID == "" {
		return false
	}
	if state.AirContacts == nil {
		state.AirContacts = map[string]map[string]*AirContact{}
	}
	if state.AirContacts[observer] == nil {
		state.AirContacts[observer] = map[string]*AirContact{}
	}
	if _, seen := state.AirContacts[observer][taskID]; seen {
		return false
	}
	state.AirContacts[observer][taskID] = &AirContact{TaskID: taskID, DetectedBy: how, Turn: state.Turn}
	return true
}

// revealAirTask 把编队的当前信息写进观察者的联系人。
func revealAirTask(state *State, observer string, aircraft *AirUnit, how string) {
	if aircraft == nil || aircraft.Faction == observer || observer == "" {
		return
	}
	markAirContact(state, observer, aircraft.TaskID, how)
	contact := state.AirContacts[observer][aircraft.TaskID]
	models := map[string]bool{}
	count := 0
	for _, member := range state.Aircraft {
		if member.Faction != aircraft.Faction || member.TaskID != aircraft.TaskID || member.TaskID == "" {
			continue
		}
		if member.Status != "cap" && member.Status != "recon" && member.Status != "attack" && member.Status != "attack_holding" {
			continue
		}
		models[member.Model] = true
		count++
	}
	if count == 0 {
		return
	}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	contact.Models = names
	contact.Count = count
	contact.Faction = aircraft.Faction
	contact.Kind = aircraft.TaskKind
	contact.Coord = taskCoordOf(aircraft)
	contact.Heading = aircraft.Heading
	contact.TargetCoord = aircraft.TargetCoord
	contact.Turn = state.Turn
	contact.Returning = aircraft.Returning
	contact.AttackHolding = aircraft.Status == "attack_holding"
}

// taskCoordOf 返回算子当前所在空域：空中用 Coord，CAP 用巡逻/前出点。
func taskCoordOf(aircraft *AirUnit) string {
	if aircraft == nil {
		return ""
	}
	if aircraft.Coord != "" {
		return aircraft.Coord
	}
	return aircraft.PatrolCoord
}

// visibleAirTaskFaction 判断 observer 能否看到这一支编队。
func visibleAirTaskFaction(state State, observer string, aircraft *AirUnit) bool {
	if aircraft == nil || observer == "" || aircraft.Faction == observer {
		return false
	}
	if state.AirContacts[observer] != nil && state.AirContacts[observer][aircraft.TaskID] != nil {
		return true
	}
	return false
}

// detectAirTaskByRadar 让 observer 的舰船与机场雷达对敌方编队做一次发现判定。
// 雷达设备判定沿用 radarDetectionChance 的距离表（规则书 707–722 行）。
func detectAirTaskByRadar(state *State, observer string, aircraft *AirUnit) []EngineEvent {
	if aircraft == nil || aircraft.Faction == observer {
		return nil
	}
	coord := taskCoordOf(aircraft)
	if coord == "" {
		return nil
	}
	for _, id := range sortedUnitIDs(state.Units) {
		ship := state.Units[id]
		if ship.Faction != observer {
			continue
		}
		chance := radarDetectionChance(*state, ship, Distance(ship.Coord, coord))
		if chance <= 0 {
			continue
		}
		if !ship.Card.AirRadar || EnsureDamage(ship).AirRadarDisabled {
			chance = applyWeatherToDetection(chance, weatherAt(state.Map, coord))
		}
		if probabilityRoll(&state.RNGState) <= chance {
			before := visibleAirTaskFaction(*state, observer, aircraft)
			revealAirTask(state, observer, aircraft, "radar")
			if !before {
				return []EngineEvent{{Type: "air_contact", Audience: observer, Text: fmt.Sprintf("%s雷达发现敌方飞行编队：%s", ship.Name, coord)}}
			}
			return nil
		}
	}
	for _, airfield := range state.Airfields {
		if airfield.Faction != observer {
			continue
		}
		// 机场用自己的雷达表（美军 100/80/60/40/20，日军 80/60/40/20/0），
		// 没装雷达就只走"1 hex 内 30%"那一行，并且吃天气惩罚。
		distance := Distance(airfield.Coord, coord)
		chance := airfieldRadarChance(*state, airfield.Faction, distance)
		if !airfield.Radar {
			chance = applyWeatherToDetection(noRadarChance(distance), weatherAt(state.Map, coord))
		}
		if chance <= 0 {
			continue
		}
		if probabilityRoll(&state.RNGState) <= chance {
			before := visibleAirTaskFaction(*state, observer, aircraft)
			revealAirTask(state, observer, aircraft, "radar")
			if !before {
				return []EngineEvent{{Type: "air_contact", Audience: observer, Text: fmt.Sprintf("%s雷达发现敌方飞行编队：%s", airfield.Name, coord)}}
			}
			return nil
		}
	}
	return nil
}

// applyWeatherToDetection 给未装备雷达的观察者套天气惩罚（规则书 717–722 行）。
func applyWeatherToDetection(chance float64, weather string) float64 {
	switch weather {
	case "cloud":
		return chance * 0.5
	case "rain":
		return chance * 0.35
	case "storm":
		return chance * 0.2
	}
	return chance
}

// refreshKnownAirContacts 让已经被发现的编队持续更新位置与状态（只要编队还在空中）。
// 目视/雷达发现过一次之后，对手的航迹就是持续跟踪的——除非编队降落或消失。
func refreshKnownAirContacts(state *State) {
	for observer, contacts := range state.AirContacts {
		for taskID, contact := range contacts {
			contact.Count = 0
			var leader *AirUnit
			for _, member := range state.Aircraft {
				if member.Faction != contact.Faction || member.TaskID != taskID || !airborne(member) {
					continue
				}
				contact.Count++
				if leader == nil {
					leader = member
				}
			}
			if leader == nil || contact.Count == 0 {
				// 编队已经降落：从可见列表里移除。
				delete(contacts, taskID)
				continue
			}
			revealAirTask(state, observer, leader, contact.DetectedBy)
		}
		if len(contacts) == 0 {
			delete(state.AirContacts, observer)
		}
	}
}

// resolveAirVisibility 每回合刷新敌方编队的可见性：雷达判定、侦察扇面、目视接触。
func resolveAirVisibility(state *State) []EngineEvent {
	events := []EngineEvent{}
	refreshKnownAirContacts(state)
	observers := map[string]bool{}
	for faction := range state.Doctrines {
		observers[faction] = true
		for _, unit := range state.Units {
			if unit.Faction == faction {
				observers[faction] = true
			}
		}
	}
	// 每个观察方每回合只算一次"侦察已经扫过哪些格"，否则每架敌机都要重扫一遍扇面。
	coverage := map[string]map[string]bool{}
	// 雷达按"每个编队每回合一次"判定（规则书 734 行：每个具备探测能力的单位
	// 每回合对每个来袭编队判定一次）。以前这里是按机逐架判，一支 20 机的编队
	// 在同一距离上会连开 20 次，5 hex 处 5% 的日军主力舰雷达被放大到 64%，
	// 看起来就是"隔着几百海里照样抓得死死的"。
	radarChecked := map[string]bool{}
	ids := sortedAirUnitIDs(*state)
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		if aircraft == nil || !airborne(aircraft) || aircraft.TaskID == "" {
			continue
		}
		// 编队只派第一架（ID 最小、顺序稳定）做雷达代表，其余成员只参与侦察/目视判定。
		lead := !radarChecked[aircraft.TaskID]
		radarChecked[aircraft.TaskID] = true
		for observer := range observers {
			if observer == aircraft.Faction {
				continue
			}
			// 已经跟踪上的编队不再重掷：航迹由 refreshKnownAirContacts 每回合刷新。
			if lead && !visibleAirTaskFaction(*state, observer, aircraft) {
				events = append(events, detectAirTaskByRadar(state, observer, aircraft)...)
			}
			// 侦察扇面盖住也算发现：侦察机的本职就是搜索空域。
			covered := coverage[observer]
			if covered == nil {
				covered = reconCoveredCells(*state, observer)
				coverage[observer] = covered
			}
			if covered[taskCoordOf(aircraft)] {
				before := visibleAirTaskFaction(*state, observer, aircraft)
				revealAirTask(state, observer, aircraft, "recon")
				if !before {
					events = append(events, EngineEvent{Type: "air_contact", Audience: observer, Text: "侦察机发现敌方飞行编队：" + taskCoordOf(aircraft)})
				}
			}
			// 目视接触：双方编队同格或相邻。
			if visualContact(*state, observer, aircraft) {
				before := visibleAirTaskFaction(*state, observer, aircraft)
				revealAirTask(state, observer, aircraft, "visual")
				if !before {
					events = append(events, EngineEvent{Type: "air_contact", Audience: observer, Text: "目视发现敌方飞行编队：" + taskCoordOf(aircraft)})
				}
			}
		}
	}
	return events
}

// reconCoveredCells 返回 observer 本回合所有侦察机真正扫过的格子。
//
// 只能算扇面，不能算"以顶点为圆心、半径等于已飞距离的整圆"：
// 以前第二段判定是 Distance(origin, coord) <= ReconDistance，等于一架往东飞的
// 侦察机把西边同样远的空域也照亮了——敌方编队隔着十几个 hex 被发现，
// 还找不到是谁看见的。判断要跟规则一致：只有航向扇面内、飞过的距离圈以内才算。
func reconCoveredCells(state State, observer string) map[string]bool {
	covered := map[string]bool{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != observer || aircraft.Status != "recon" {
			continue
		}
		origin := aircraft.OriginCoord
		current := aircraft.Coord
		if origin == "" || current == "" {
			continue
		}
		// 这一回合正在判定的那一圈（含向前一格），和 UI 上高亮的搜索带是同一块。
		for _, cell := range ReconScannedRingHexes(state.Map, origin, current, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius) {
			covered[cell] = true
		}
		// 已经飞过的整块扇面：半径取已飞距离，但仍受航向和扇面限制。
		radius := aircraft.ReconDistance
		if radius > aircraft.ReconRadius {
			radius = aircraft.ReconRadius
		}
		if radius > 0 {
			for _, cell := range ReconScanBandHexes(state.Map, origin, aircraft.Heading, aircraft.SectorWidth, radius) {
				covered[cell] = true
			}
		}
	}
	return covered
}

// visualContact 判断 observer 的某支编队是否与目标编队同格或相邻。
func visualContact(state State, observer string, aircraft *AirUnit) bool {
	target := taskCoordOf(aircraft)
	if target == "" {
		return false
	}
	for _, other := range state.Aircraft {
		if other.Faction != observer || !airborne(other) {
			continue
		}
		coord := taskCoordOf(other)
		if coord == "" {
			continue
		}
		if Distance(coord, target) <= 1 {
			return true
		}
	}
	return false
}

// VisibleEnemyAir 返回 observer 已发现的敌方飞行编队。
func VisibleEnemyAir(state State, observer string) []AirContact {
	result := []AirContact{}
	contacts := state.AirContacts[observer]
	if len(contacts) == 0 {
		return result
	}
	ids := make([]string, 0, len(contacts))
	for id := range contacts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		contact := contacts[id]
		if contact == nil || contact.Count == 0 || contact.Coord == "" {
			continue
		}
		result = append(result, *contact)
	}
	return result
}
