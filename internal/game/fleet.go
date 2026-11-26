package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// —— 重创脱离（规则书 392–403 行）——

// unitCrippled 判断一艘船是否被重创：已损失生命值达到初始生命值的 80% 或更多、失去动力、起火燃烧。
//
// "起火燃烧"(burning) 和 "小型火灾"(small) 是两种不同的损伤，不能混为一谈：
//   - 小型火灾只封锁本回合的航空作业、岸轰和卸载，回合开始时自动扑灭（规则书 432 行）；
//   - 起火燃烧每回合损管判定，可能弃船，才构成重创（规则书 398、433 行）。
//
// 之前用 Fire != "" 判断，结果一颗炸弹引燃的小火就让满血航母强制脱离战场。
func unitCrippled(unit *Unit) bool {
	if unit == nil {
		return false
	}
	// 航母不自行脱离：航母是整场战斗的核心，退出等于直接交权；打不动就留在场上当靶子，
	// 要么继续打、要么被打沉，不会被"重创"状态自动带下场。护航舰陪着撤离的是其他舰种。
	if IsCarrier(unit) {
		return false
	}
	if unit.MaxStrength > 0 && (unit.MaxStrength-unit.Strength)*5 >= unit.MaxStrength*4 {
		return true
	}
	damage := EnsureDamage(unit)
	if damage.Disabled || len(damage.Flooding) > unit.Card.DamageControl {
		return true
	}
	return damage.Fire == "burning"
}

// escortPriority 是重创舰带走护航舰的选择顺序：DD → CL → CA → BB，同一类型里多艘由控制方选择。
// 控制方选择在引擎里退化为"选第一艘"（按 ID 排序），保证结算可复现。
func escortPriority(unitType string) int {
	switch unitType {
	case "DD":
		return 0
	case "CL":
		return 1
	case "CA":
		return 2
	case "BB":
		return 3
	}
	return 99
}

// pickEscort 从主队里挑一艘护航舰跟着重创舰撤出：跳过航母、重创舰本身和已经重创的船。
func pickEscort(state State, fleetID, excludeID string) *Unit {
	candidates := []*Unit{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || FleetKey(unit) != fleetID || unit.ID == excludeID || unitCrippled(unit) {
			continue
		}
		if escortPriority(unit.Type) >= 99 {
			continue
		}
		candidates = append(candidates, unit)
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := escortPriority(candidates[i].Type), escortPriority(candidates[j].Type)
		if left == right {
			return candidates[i].ID < candidates[j].ID
		}
		return left < right
	})
	return candidates[0]
}

// resolveDetachments 结算重创脱离：
// 重创舰带走一艘护航舰组成退避队，航向地图边缘；如果主队只剩航母，整个主队强制撤退。
func resolveDetachments(state *State) []EngineEvent {
	events := []EngineEvent{}
	fleetIDs := map[string]bool{}
	for _, unit := range state.Units {
		fleetIDs[FleetKey(unit)] = true
	}
	ordered := make([]string, 0, len(fleetIDs))
	for id := range fleetIDs {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, fleetID := range ordered {
		members := fleetMembers(state, fleetID)
		if len(members) == 0 {
			continue
		}
		crippled := []*Unit{}
		healthy := []*Unit{}
		for _, member := range members {
			unit := state.Units[member.ID]
			if unit == nil {
				continue
			}
			// 已经脱离过一次的舰船不再重复脱离：否则每回合都会把它再摘出去一次，
			// 舰队名会滚成"第一战列舰队·脱离·脱离·脱离…"，并且不断产生新的空壳舰队。
			if unit.Withdrawing {
				continue
			}
			if unitCrippled(unit) {
				crippled = append(crippled, unit)
			} else {
				healthy = append(healthy, unit)
			}
		}
		// "主队只剩航母就无法继续由玩家控制"这条规则与有没有重创舰无关（规则书 402 行），
		// 所以即使本回合没有重创舰也要检查。
		if len(crippled) == 0 {
			events = append(events, forceCarrierOnlyWithdrawal(state, fleetID, fleetMembers(state, fleetID))...)
			continue
		}
		sort.Slice(crippled, func(i, j int) bool { return crippled[i].ID < crippled[j].ID })
		for index, unit := range crippled {
			detachID := fmt.Sprintf("%s-detach-%d", fleetID, index+1)
			escort := pickEscort(*state, fleetID, unit.ID)
			unit.FleetID = detachID
			unit.FleetName = unit.FleetName + "·脱离"
			unit.Withdrawing = true
			text := fmt.Sprintf("%s重创脱离主队，航向地图边缘撤离", unit.Name)
			if escort != nil {
				escort.FleetID = detachID
				escort.FleetName = unit.FleetName
				escort.Withdrawing = true
				text = fmt.Sprintf("%s重创脱离主队，由%s护航，航向地图边缘撤离", unit.Name, escort.Name)
			}
			clearFleetStay(state, detachID)
			events = append(events, EngineEvent{Type: "unit_detached", Audience: unit.Faction, Text: text})
		}
		// 主队只剩航母：整个主队无法继续由玩家控制，直接强制撤退。
		events = append(events, forceCarrierOnlyWithdrawal(state, fleetID, fleetMembers(state, fleetID))...)
	}
	return events
}

// forceCarrierOnlyWithdrawal 检查主队是否只剩航母；是的话整队转入强制撤退。
func forceCarrierOnlyWithdrawal(state *State, fleetID string, remaining []Unit) []EngineEvent {
	if len(remaining) == 0 {
		return nil
	}
	onlyCarriers := true
	for _, member := range remaining {
		unit := state.Units[member.ID]
		if unit == nil || !IsCarrier(unit) {
			onlyCarriers = false
			break
		}
	}
	if !onlyCarriers {
		return nil
	}
	// 已经处于撤退状态的编队不重复触发。
	for _, member := range remaining {
		if unit := state.Units[member.ID]; unit != nil && unit.Withdrawing {
			return nil
		}
	}
	withdrawID := fleetID + "-withdraw"
	for _, member := range remaining {
		unit := state.Units[member.ID]
		if unit == nil {
			continue
		}
		unit.FleetID = withdrawID
		unit.Withdrawing = true
	}
	clearFleetStay(state, withdrawID)
	return []EngineEvent{{Type: "fleet_forced_withdraw", Audience: remaining[0].Faction, Text: fmt.Sprintf("%s只剩航母，失去控制，强制撤退", remaining[0].FleetName)}}
}

// atMapEdge 判断某个 hex 是否在地图边缘。
func atMapEdge(gameMap MapDefinition, coord string) bool {
	row, col, err := ParseCoord(coord)
	if err != nil {
		return false
	}
	return row == 1 || col == 1 || row == gameMap.Rows || col == gameMap.Cols
}

// edgeHeadingToward 返回舰队撤离时朝哪个方向走（正前 / 左前 / 右前）。
func edgeHeadingToward(state State, unit *Unit) string {
	angle, ok := reconHeadings[strings.ToUpper(unit.Heading)]
	if !ok {
		angle = reconHeadings["E"]
	}
	axis := sectorPoint{x: math.Cos(angle * math.Pi / 180), y: math.Sin(angle * math.Pi / 180)}
	options := []sectorPoint{{x: axis.x, y: axis.y}, {x: axis.x*0.5 - axis.y*0.866, y: axis.x*0.866 + axis.y*0.5}, {x: axis.x*0.5 + axis.y*0.866, y: -axis.x*0.866 + axis.y*0.5}}
	best, bestGap := "", 999
	for _, option := range options {
		next := neighborToward(state.Map, unit.Coord, option, nil)
		if next == "" {
			continue
		}
		gap := Distance(next, centerCoord(state.Map))
		if gap < bestGap {
			bestGap = gap
			best = next
		}
	}
	return best
}

// centerCoord 返回地图中心 hex（撤离时用它做"离边缘越来越近"的比较基准）。
func centerCoord(gameMap MapDefinition) string {
	return Coord((gameMap.Rows+1)/2, (gameMap.Cols+1)/2)
}

// neighborToward 找出当前 hex 里最接近给定方向的相邻可通行 hex。
// 暗礁一律跳过；allowed 非空时再叠一层通行判定——舰队撤离用 fleetCanEnter，
// 情报投影这类纯几何用途传 nil。
func neighborToward(gameMap MapDefinition, coord string, direction sectorPoint, allowed func(string) bool) string {
	fromRow, fromCol, err := ParseCoord(coord)
	if err != nil {
		return ""
	}
	fromX, fromY := hexCenter(fromRow, fromCol)
	best, bestDot := "", -2.0
	for _, candidate := range Neighbors(coord) {
		row, col, err := ParseCoord(candidate)
		if err != nil || row > gameMap.Rows || col > gameMap.Cols {
			continue
		}
		if terrainAt(gameMap, candidate) == TerrainReef {
			continue
		}
		if allowed != nil && !allowed(candidate) {
			continue
		}
		x, y := hexCenter(row, col)
		dx, dy := x-fromX, y-fromY
		length := dx*dx + dy*dy
		if length == 0 {
			continue
		}
		dot := (dx*direction.x + dy*direction.y) / length
		if dot > bestDot {
			bestDot = dot
			best = candidate
		}
	}
	return best
}

// advanceTowardEdge 让退避队每回合向地图边缘移动一格（岸轰/卸载的停留计时照样重置）。
func advanceTowardEdge(state *State) []EngineEvent {
	events := []EngineEvent{}
	withdrawing := map[string]*Unit{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || !unit.Withdrawing {
			continue
		}
		if _, ok := withdrawing[FleetKey(unit)]; !ok {
			withdrawing[FleetKey(unit)] = unit
		}
	}
	fleetIDs := make([]string, 0, len(withdrawing))
	for id := range withdrawing {
		fleetIDs = append(fleetIDs, id)
	}
	sort.Strings(fleetIDs)
	for _, fleetID := range fleetIDs {
		leader := withdrawing[fleetID]
		members := fleetMembers(state, fleetID)
		if len(members) == 0 {
			delete(withdrawing, fleetID)
			continue
		}
		if atMapEdge(state.Map, members[0].Coord) {
			events = append(events, removeFleet(state, fleetID, members[0].Faction)...)
			continue
		}
		next := edgeHeadingToward(*state, leader)
		if next == "" {
			continue
		}
		for _, member := range members {
			unit := state.Units[member.ID]
			if unit == nil {
				continue
			}
			unit.Heading = headingBetween(unit.Coord, next)
			unit.Coord = next
			unit.Moved = true
		}
		clearFleetStay(state, fleetID)
		events = append(events, EngineEvent{Type: "fleet_withdrawing", Audience: leader.Faction, Text: fmt.Sprintf("%s向%s撤离一hex", leader.FleetName, next)})
	}
	return events
}

// removeFleet 让退出战场的舰队消失（舰载机留在空中，交给失去母舰的回收流程）。
func removeFleet(state *State, fleetID, faction string) []EngineEvent {
	removed := []string{}
	for _, member := range fleetMembers(state, fleetID) {
		unit := state.Units[member.ID]
		if unit == nil {
			continue
		}
		if unit.Faction != faction {
			continue
		}
		removed = append(removed, unit.Name)
		delete(state.Units, unit.ID)
	}
	if len(removed) == 0 {
		return nil
	}
	return []EngineEvent{{Type: "fleet_withdrawn", Audience: faction, Text: fmt.Sprintf("%s退出战场：%s", strings.Join(removed, "、"), "撤出地图边缘")}}
}

// —— 岸轰 / 卸载的暴露（规则书 535 行）——

// enemyFactionOf 返回对手阵营。剧本只有 A/B 两方，未知阵营直接返回空串（不做暴露）。
func enemyFactionOf(faction string) string {
	switch faction {
	case "A":
		return "B"
	case "B":
		return "A"
	}
	return ""
}

// exposeFleetAtMaxIntel 把正在岸轰或卸载的舰队对敌方暴露到最高情报等级。
// 规则书 535 行："运输船卸货和炮击的舰队会自动以最高情报等级暴露行踪，无论白天和黑夜。"
//
// 三点实现约定：
//  1. 直接写满级而不是走 recordContact 的 +1 累加——炮击本身就该一次给到 4 级；
//  2. 满级情报含真实位置与完整编成，Coord / DisplayCoord / Members 一起刷新，不给误差；
//  3. 打上 IntelFresh 并清掉过时计数，否则回合末的过时判定会在同一回合就把这条
//     刚暴露的情报当"没刷新"处理（岸轰结算在过时判定之前）。
//
// 已经满级时不重复发事件，但位置/编成仍会刷新：同一回合先岸轰后卸载的舰队只报一次。
func exposeFleetAtMaxIntel(state *State, fleetID, reason string) []EngineEvent {
	members := fleetMembers(state, fleetID)
	if len(members) == 0 {
		return nil
	}
	observer := enemyFactionOf(members[0].Faction)
	if observer == "" {
		return nil
	}
	wasLevel := 0
	if contact := state.Contacts[observer][fleetID]; contact != nil {
		wasLevel = contact.Level
	}
	raiseContactToMax(state, observer, fleetID, members)
	if wasLevel >= intelMaxLevel {
		return nil
	}
	name := members[0].FleetName
	if name == "" {
		name = fleetID
	}
	return []EngineEvent{{
		Type:     "fleet_exposed",
		Audience: observer,
		Text:     fmt.Sprintf("%s在%s%s，行踪完全暴露：位置与编成已知（情报4）", name, members[0].Coord, reason),
	}}
}

// —— 运输卸载（规则书 493–497 行）——

// unitUnloading 判断一艘运输船本回合能不能卸载物资：
// 船上有货、舰队在敌方机场 hex 停留满 3 回合、舰队没有翻沉/停滞。
func unitUnloading(state State, unit *Unit) *Airfield {
	if unit == nil || unit.Type != "TRS" || unit.Card.Cargo <= 0 {
		return nil
	}
	if SessionOf(state, FleetKey(unit)) < 3 {
		return nil
	}
	// 舰队必须真的能待在这个 hex 上：旧存档里可能有航母编队停在机场格/浅滩上，
	// 那种"非法驻留"不能凭坐标蹭到卸载或岸轰。
	if !fleetCanEnter(state, unit.Coord, FleetHasCarrier(state, FleetKey(unit))) {
		return nil
	}
	airfield := shoreTargetAt(state, unit.Faction, unit.Coord)
	if airfield == nil {
		return nil
	}
	return airfield
}

// unitSmoking 判断本回合能否进行航空作业、岸轰或卸载：小型火灾只持续到下一回合开始（规则书 432 行）。
func unitSmoking(unit *Unit) bool {
	if unit == nil {
		return false
	}
	fire := EnsureDamage(unit).Fire
	return fire == "small" || fire == "burning"
}

// SessionOf 返回舰队在停靠的 build-up 计数（从到达 hex 起算，含到达那回合）。
func SessionOf(state State, fleetID string) int {
	if state.FleetSessions == nil {
		return 0
	}
	return state.FleetSessions[fleetID]
}

// —— 运输卸载结算（规则书 493–497 行）——

// shoreSupplies 返回某 hex 上由某阵营投送、尚未被摧毁的滩头物资。
func shoreSupplies(state State, faction, coord string) int {
	if state.ShoreSupplies == nil {
		return 0
	}
	return state.ShoreSupplies[faction+"|"+coord]
}

func addShoreSupplies(state *State, faction, coord string, amount int) int {
	if state.ShoreSupplies == nil {
		state.ShoreSupplies = map[string]int{}
	}
	key := faction + "|" + coord
	if state.ShoreSupplies[key] < 0 {
		state.ShoreSupplies[key] = 0
	}
	state.ShoreSupplies[key] += amount
	return state.ShoreSupplies[key]
}

func removeShoreSupplies(state *State, faction, coord string, amount int) int {
	key := faction + "|" + coord
	left := state.ShoreSupplies[key] - amount
	if left < 0 {
		left = 0
	}
	if state.ShoreSupplies == nil {
		state.ShoreSupplies = map[string]int{}
	}
	state.ShoreSupplies[key] = left
	return left
}

// enemySuppliesAt 返回某机场 hex 上由敌方卸下的滩头物资点数。
func enemySuppliesAt(state State, airfield *Airfield) int {
	if airfield == nil {
		return 0
	}
	return shoreSupplies(state, OppositeFaction(airfield.Faction), airfield.Coord)
}

// VisibleEnemyShoreSupplies 返回"敌方卸在己方机场上的滩头物资"。
// 运输船在自家机场卸了几十吨货，防守方不可能不知道；不把数量告诉防守方，
// 清滩这条反制就只有上帝视角玩得到，实际打起来没人会想到回防。
func VisibleEnemyShoreSupplies(state State, faction string) map[string]int {
	result := map[string]int{}
	owned := map[string]bool{}
	for _, airfield := range state.Airfields {
		if airfield != nil && airfield.Faction == faction {
			owned[airfield.Coord] = true
		}
	}
	if len(owned) == 0 {
		return result
	}
	for key, value := range state.ShoreSupplies {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 || parts[0] == faction || !owned[parts[1]] {
			continue
		}
		result[parts[1]] += value
	}
	return result
}

// VisibleShoreSupplies 只把自己投送的滩头物资告诉前端。
func VisibleShoreSupplies(state State, faction string) map[string]int {
	result := map[string]int{}
	for key, value := range state.ShoreSupplies {
		prefix := faction + "|"
		if strings.HasPrefix(key, prefix) {
			result[strings.TrimPrefix(key, prefix)] = value
		}
	}
	return result
}

// resolveUnloading 结算本回合所有运输船的卸载：
// 舰队在敌方机场 hex 停留满 3 回合后开始卸货，每艘船每回合卸 1 物资（风暴天每 2 回合 1 物资）；
// 卸空后舰队强制撤离（交给退避队的逐回合撤离流程）。
func resolveUnloading(state *State) []EngineEvent {
	events := []EngineEvent{}
	done := map[string]bool{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Type != "TRS" || done[FleetKey(unit)] {
			continue
		}
		airfield := unitUnloading(*state, unit)
		if airfield == nil {
			continue
		}
		fleetID := FleetKey(unit)
		done[fleetID] = true
		storm := weatherAt(state.Map, unit.Coord) == "storm"
		delivered := 0
		for _, member := range fleetMembers(state, fleetID) {
			ship := state.Units[member.ID]
			if ship == nil || ship.Type != "TRS" || ship.Card.Cargo <= 0 || unitSmoking(ship) {
				continue
			}
			amount := 1
			if storm {
				// 风暴天每 2 回合卸 1 物资：奇偶回合交替，跨回合状态存在 FleetSessions 之外的计数器里
				if nextUint64(&state.RNGState)%2 == 1 {
					amount = 0
				}
			}
			if amount == 0 {
				continue
			}
			ship.Card.Cargo -= amount
			if ship.Card.Cargo < 0 {
				ship.Card.Cargo = 0
			}
			delivered += amount
		}
		if delivered > 0 {
			total := addShoreSupplies(state, unit.Faction, unit.Coord, delivered)
			events = append(events, EngineEvent{Type: "troop_unloading", Audience: unit.Faction, Text: fmt.Sprintf("%s在%s卸载物资%d点（累计%d点）", unit.FleetName, airfield.Name, delivered, total)})
			// 货一落地，机场就进入被压制状态：生命值归 0、跑道全废，清滩之前修不回来。
			if airfield.Strength > 0 {
				airfield.Strength = 0
				events = append(events, EngineEvent{Type: "airfield_occupied", Audience: "all", Text: fmt.Sprintf("%s被敌方滩头物资压制，跑道全部瘫痪（清滩前无法修复）", airfield.Name)})
			}
		}
		// 卸货的舰队对敌方完全暴露（规则书 535 行），无论白天黑夜。
		// 放在卸载战报之后：玩家先看到"卸了多少"，再看到"因此暴露了"。
		events = append(events, exposeFleetAtMaxIntel(state, fleetID, "卸载物资")...)
		// 运输船卸空后舰队强制撤离
		left := false
		for _, member := range fleetMembers(state, fleetID) {
			ship := state.Units[member.ID]
			if ship != nil && ship.Type == "TRS" && ship.Card.Cargo > 0 {
				left = true
			}
		}
		if left {
			continue
		}
		for _, member := range fleetMembers(state, fleetID) {
			ship := state.Units[member.ID]
			if ship != nil {
				ship.Withdrawing = true
			}
		}
		events = append(events, EngineEvent{Type: "fleet_ordered_withdraw", Audience: unit.Faction, Text: fmt.Sprintf("%s卸载完毕，全体撤离战场", unit.FleetName)})
	}
	return events
}

// trackFleetSessions 在每回合结束时更新停留计数：
// 与上一回合同一 hex 则 +1，否则按新到达记 1（到达当回合算第 1 回合）。
// 岸轰用"停留满 1 回合"，运输卸载用"停留满 3 回合"。
func trackFleetSessions(state *State) {
	if state.FleetSessions == nil {
		state.FleetSessions = map[string]int{}
	}
	current := map[string]string{}
	for _, unit := range state.Units {
		if unit == nil {
			continue
		}
		fleetID := FleetKey(unit)
		if _, seen := current[fleetID]; !seen {
			current[fleetID] = unit.Coord
		}
	}
	for fleetID := range state.FleetSessions {
		if _, ok := current[fleetID]; !ok {
			delete(state.FleetSessions, fleetID)
		}
	}
	for fleetID, coord := range current {
		if state.FleetStays[fleetID] == coord {
			state.FleetSessions[fleetID]++
		} else {
			// 新到达（或刚移动过）：到达当回合算第 1 回合，所以不是 0。
			state.FleetSessions[fleetID] = 1
		}
	}
}

// VisibleFleetSessions 把舰队停留计数告诉前端（用于提示"还剩几回合开始卸载"）。
func VisibleFleetSessions(state State, faction string) map[string]int {
	result := map[string]int{}
	for _, unit := range state.Units {
		if unit == nil || unit.Faction != faction {
			continue
		}
		result[FleetKey(unit)] = SessionOf(state, FleetKey(unit))
	}
	return result
}

// fleetBurning 判断舰队里是否有正在燃烧（小型火灾或大火）的舰船。
func fleetBurning(state State, fleetID string) bool {
	for _, member := range fleetMembers(&state, fleetID) {
		unit := state.Units[member.ID]
		if unit != nil && unitSmoking(unit) {
			return true
		}
	}
	return false
}
