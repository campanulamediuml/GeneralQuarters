package game

import (
	"fmt"
	"sort"
	"strings"
)

// AirfieldContact 是侦察发现的敌方机场（只上报类型与跑道数，不返回防空和燃料等细节）。
type AirfieldContact struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Faction      string       `json:"faction"`
	Coord        string       `json:"coord"`
	Type         AirfieldType `json:"type"`
	Runways      int          `json:"runways"`
	ContactLevel int          `json:"contact_level"`
}

// setAirfieldContact 记录/刷新敌方机场情报；每次发现把情报等级 +1（最高 4），并同步最新位置。
func setAirfieldContact(state *State, observer string, airfield *Airfield) {
	if state.AirfieldContacts == nil {
		state.AirfieldContacts = map[string]map[string]*AirfieldContact{}
	}
	if state.AirfieldContacts[observer] == nil {
		state.AirfieldContacts[observer] = map[string]*AirfieldContact{}
	}
	contact := state.AirfieldContacts[observer][airfield.ID]
	if contact == nil {
		contact = &AirfieldContact{ID: airfield.ID, Faction: airfield.Faction}
		state.AirfieldContacts[observer][airfield.ID] = contact
	}
	if contact.ContactLevel < 4 {
		contact.ContactLevel++
	}
	contact.Name = airfield.Name
	contact.Coord = airfield.Coord
	contact.Type = airfield.Type
	contact.Runways = airfield.Runways
}

// discoverEnemyAirfields 让侦察算子在每个距离圈上尝试发现敌方机场：
// 机场位于本圈、落在侦察扇面内时有 30% 基础发现率，发现后情报等级 +1。
func discoverEnemyAirfields(state *State, aircraft *AirfieldAirUnit, ring int) []EngineEvent {
	events := []EngineEvent{}
	if aircraft.Heading == "" || ring < 1 {
		return events
	}
	axis, ok := reconHeadings[strings.ToUpper(aircraft.Heading)]
	if !ok {
		return events
	}
	originRow, originCol, err := ParseCoord(aircraft.OriginCoord)
	if err != nil {
		return events
	}
	originX, originY := hexCenter(originRow, originCol)
	for _, id := range sortedAirfieldIDs(*state) {
		airfield := state.Airfields[id]
		if airfield == nil || airfield.Faction == aircraft.Faction || Distance(aircraft.OriginCoord, airfield.Coord) != ring {
			continue
		}
		row, col, err := ParseCoord(airfield.Coord)
		if err != nil {
			continue
		}
		x, y := hexCenter(row, col)
		if !sectorIntersectsHex(sectorPoint{x - originX, y - originY}, axis, float64(aircraft.SectorWidth)) {
			continue
		}
		if probabilityRoll(&state.RNGState) >= 30 {
			continue
		}
		setAirfieldContact(state, aircraft.Faction, airfield)
		events = append(events, EngineEvent{Type: "airfield_discovered", Audience: aircraft.Faction, Text: fmt.Sprintf("第%d圈发现敌方机场%s（%s，%d条跑道）", ring, airfield.Name, airfield.Coord, airfield.Runways)})
	}
	return events
}

// VisibleAirfields 返回当前阵营能看到的机场。
//
// 机场是固定的地面设施，不会移动，所以位置从一开始就完全可见（与舰队不同）：
// 未侦察时也知道"这里有个敌方机场"，但只给出粗略信息（类型/跑道按已侦察到的情报，
// 没侦察到就按初始值），防空值、岸防、剩余生命值等细节仍然要通过侦察获得。
func VisibleAirfields(state State, faction string) []AirfieldContact {
	result := []AirfieldContact{}
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield.Faction == faction {
			result = append(result, AirfieldContact{ID: airfield.ID, Name: airfield.Name, Faction: airfield.Faction, Coord: airfield.Coord, Type: airfield.Type, Runways: airfield.Runways, ContactLevel: 4})
			continue
		}
		contact := AirfieldContact{ID: airfield.ID, Name: airfield.Name, Faction: airfield.Faction, Coord: airfield.Coord, Type: airfield.Type, Runways: airfield.Runways, ContactLevel: 1}
		if scouted := state.AirfieldContacts[faction][id]; scouted != nil {
			contact = *scouted
		}
		result = append(result, contact)
	}
	return result
}

func sortedAirfieldIDs(state State) []string {
	ids := make([]string, 0, len(state.Airfields))
	for id := range state.Airfields {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// VisibleGroundAir 是航空面板用的机场整备状态（只含己方机场）。
func VisibleGroundAir(state State, faction string) []AirfieldAirStatus {
	result := []AirfieldAirStatus{}
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield.Faction != faction {
			continue
		}
		status := AirfieldAirStatus{
			UnitID: airfield.ID, Name: airfield.Name, Type: airfield.Type, Coord: airfield.Coord,
			Strength: airfield.Strength, MaxStrength: airfield.MaxStrength, MaxRunways: airfield.Runways,
			Runways: RunwaysAvailable(airfield.Strength, airfield.Runways), Radar: airfield.Radar,
			HeavyAA: airfield.HeavyAA, LightAA: airfield.LightAA, Coastal: airfield.Coastal,
			Aircraft: []AirfieldAirUnit{},
		}
		for _, aircraftID := range sortedGroundAirIDs(state, airfield.ID) {
			aircraft := state.GroundAir[aircraftID]
			status.Aircraft = append(status.Aircraft, *aircraft)
			if !airfieldAirborne(aircraft) {
				status.HangarUsed++
			}
		}
		result = append(result, status)
	}
	return result
}

func sortedGroundAirIDs(state State, airfieldID string) []string {
	ids := make([]string, 0, len(state.GroundAir))
	for id, aircraft := range state.GroundAir {
		if aircraft.AirfieldID == airfieldID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func airfieldAirborne(aircraft *AirfieldAirUnit) bool {
	return aircraft.Status == "cap" || aircraft.Status == "recon" || aircraft.Status == "attack_holding" || aircraft.Status == "attack"
}

// AdvanceAirfields 是机场的每回合推进：
// 1) 损管：每回合 d10 ≤ 2 时生命值 +1（不超过最大生命值）；
// 2) 航空作业：机库整备 → 滑行到跑道 → 起飞，每回合只推进一步。
func AdvanceAirfields(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, id := range sortedAirfieldIDs(*state) {
		airfield := state.Airfields[id]
		if airfield.Strength < airfield.MaxStrength && d10(state) <= 2 {
			airfield.Strength++
			events = append(events, EngineEvent{Type: "airfield_repaired", Audience: airfield.Faction, Text: fmt.Sprintf("%s修复1点跑道（%d/%d）", airfield.Name, airfield.Strength, airfield.MaxStrength)})
		}
		runways := RunwaysAvailable(airfield.Strength, airfield.Runways)
		if runways == 0 {
			continue
		}
		// 每条可用跑道每回合 12 次起降，超出部分顺延到后续回合；
		// OpsRemaining 小于 0 表示"预算已被人为耗尽"（机场被打断作业时扣掉名额），此时本回合不再放行。
		budget := runways * runwaySortiesPerTurn
		if airfield.OpsRemaining < 0 {
			budget = 0
		}
		for _, aircraftID := range sortedGroundAirIDs(*state, airfield.ID) {
			aircraft := state.GroundAir[aircraftID]
			switch aircraft.Status {
			case "service", "deck", "recovery":
				if budget <= 0 {
					events = append(events, EngineEvent{Type: "airfield_congested", Audience: airfield.Faction, Text: fmt.Sprintf("%s跑道作业已满，%s顺延到后续回合", airfield.Name, aircraft.Model)})
					continue
				}
				budget--
			}
			switch aircraft.Status {
			case "service":
				aircraft.Status = "deck"
				events = append(events, EngineEvent{Type: "airfield_taxi", Audience: airfield.Faction, Text: fmt.Sprintf("%s滑行至%s跑道", aircraft.Model, airfield.Name)})
			case "deck":
				aircraft.Status = "ready"
				events = append(events, EngineEvent{Type: "airfield_ready", Audience: airfield.Faction, Text: fmt.Sprintf("%s在%s跑道待命", aircraft.Model, airfield.Name)})
			case "recovery":
				aircraft.Status = "hangar"
			}
		}
		airfield.OpsRemaining = budget
	}
	// 整备完成（ready）的机场编队自动起飞，和航母的 launchReadyTasks 保持一致。
	// 之前只有玩家再发一次命令才会离地，导致机场飞机永远停在跑道上。
	events = append(events, launchReadyGroundTasks(state)...)
	return events
}

// launchReadyGroundTasks 让跑道待命（ready）的机场编队升空。
// 同一任务的飞机一起走，避免编队被拆散。
func launchReadyGroundTasks(state *State) []EngineEvent {
	events := []EngineEvent{}
	groups := map[string][]*AirfieldAirUnit{}
	for _, id := range sortedGroundAirIDsAll(*state) {
		aircraft := state.GroundAir[id]
		if aircraft == nil || aircraft.Status != "ready" || aircraft.TaskID == "" {
			continue
		}
		// 没有目标的攻击编队留在跑道上等目标，不升空烧油。
		if aircraft.TaskKind == "attack" && aircraft.TargetCoord == "" {
			continue
		}
		groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
	}
	taskIDs := make([]string, 0, len(groups))
	for taskID := range groups {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		members := groups[taskID]
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		for _, aircraft := range members {
			airfield := airfieldFor(state, aircraft.AirfieldID)
			if airfield == nil || RunwaysAvailable(airfield.Strength, airfield.Runways) == 0 {
				continue
			}
			// 时间线同样管机场：05:20 之前跑道上的算子只能待命（以前这里漏了闸，
			// 机场侦察机能在 00:40 起飞，而母舰甲板上的飞机要等到 05:20）。
			if !state.LaunchWindowOpen() {
				continue
			}
			launchGroundAircraft(state, aircraft, airfield)
			events = append(events, EngineEvent{Type: "air_launched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s从%s起飞", aircraft.Model, airfield.Name)})
		}
	}
	return events
}

// airfieldFor 返回机场，找不到时返回 nil。
func airfieldFor(state *State, id string) *Airfield {
	if state == nil || state.Airfields == nil {
		return nil
	}
	return state.Airfields[id]
}

// strikeAirfieldAt 返回某 hex 上属于敌方的机场；没有则返回 nil。
func strikeAirfieldAt(state State, faction, coord string) *Airfield {
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield.Faction != faction && airfield.Coord == coord {
			return airfield
		}
	}
	return nil
}

// groundHitChance 按规则书"对地攻击机制"计算单点命中率：基础 20%。
func groundHitChance(state State, target *Airfield, attacker *AirfieldAirUnit, capRemaining bool) float64 {
	chance := 20.0
	if !capRemaining {
		chance += 5
	}
	// 目标机场每完整 7 点轻防空值，-5%
	chance -= float64(int(target.LightAA/7) * 5)
	// 规则书 657 行：每个正在起降的算子 +1%。
	chance += float64(airfieldOpsCount(state, target.ID))
	switch weatherAt(state.Map, target.Coord) {
	case "rain":
		chance -= 10
	case "storm":
		chance -= 15
	}
	if attacker.Damaged {
		chance -= 5
	}
	if chance < 5 {
		return 5
	}
	return chance
}

// airfieldOpsCount 统计该机场正在起降作业中的地面算子（甲板滑行 / 待飞 / 回收）。
// 规则书 657 行：机场上每个正在起降的算子让对地攻击命中率 +1%。
func airfieldOpsCount(state State, airfieldID string) int {
	count := 0
	for _, aircraft := range state.GroundAir {
		if aircraft == nil || aircraft.AirfieldID != airfieldID {
			continue
		}
		switch aircraft.Status {
		case "deck", "ready", "recovery":
			count++
		}
	}
	return count
}

// airfieldCAPGroups 找出保护该机场的友军 CAP（机场所在 hex 与其母舰在 1 hex 内）。
func airfieldCAPGroups(state State, defender, coord string) [][]*AirUnit {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != defender || aircraft.Status != "cap" || aircraft.Returning || aircraft.TaskID == "" {
			continue
		}
		if Distance(capCoord(state, aircraft), coord) <= 1 {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := capCoord(state, groups[ids[i]][0]), capCoord(state, groups[ids[j]][0])
		if Distance(left, coord) == Distance(right, coord) {
			return ids[i] < ids[j]
		}
		return Distance(left, coord) < Distance(right, coord)
	})
	result := make([][]*AirUnit, 0, len(ids))
	for _, id := range ids {
		sort.Slice(groups[id], func(i, j int) bool { return groups[id][i].ID < groups[id][j].ID })
		result = append(result, groups[id])
	}
	return result
}

// airfieldHeavyAA 把机场重型防空逐点带入通用防空判定。
func airfieldHeavyAA(state *State, attackers []*AirUnit, airfield *Airfield, report *AirCombatReport) []EngineEvent {
	events := []EngineEvent{}
	chance := aaHitChance(*state, airfield.Coord, true, airfield.Faction)
	shots := int(airfield.HeavyAA)
	// 规则书 692 行：小数部分作为追加一次判定的概率。
	if probabilityRoll(&state.RNGState) <= (airfield.HeavyAA - float64(shots)) {
		shots++
	}
	for ; shots > 0; shots-- {
		if probabilityRoll(&state.RNGState) < chance {
			target := randomOperationalAircraft(state, attackers)
			if target == nil {
				break
			}
			report.AAHits++
			outcome, hitEvents := applyAircraftHit(state, target)
			recordAirHit(report, target, outcome)
			recordAircraftLoss(report, target, outcome, true)
			events = append(events, hitEvents...)
		} else {
			confuseRandomAircraft(state, attackers)
		}
	}
	return events
}

// airfieldLightAA 对单架攻击机执行机场轻型防空，返回命中次数和命中事件。
// 事件必须返回给调用方，之前直接丢弃，导致机场空袭战报里看不到"被击伤/被驱散"的明细。
func airfieldLightAA(state *State, attacker *AirUnit, airfield *Airfield, report *AirCombatReport) (int, []EngineEvent) {
	events := []EngineEvent{}
	chance := aaHitChance(*state, airfield.Coord, false, airfield.Faction)
	value := airfield.LightAA
	shots := int(value)
	// 小数防空：整数部分逐点判定，小数部分作为追加一次判定的概率
	if probabilityRoll(&state.RNGState) <= (value - float64(shots)) {
		shots++
	}
	hits := 0
	for ; shots > 0; shots-- {
		if !operationalAircraft(state, attacker) {
			break
		}
		if probabilityRoll(&state.RNGState) < chance {
			hits++
			report.AAHits++
			outcome, hitEvents := applyAircraftHit(state, attacker)
			recordAirHit(report, attacker, outcome)
			recordAircraftLoss(report, attacker, outcome, true)
			events = append(events, hitEvents...)
		} else {
			confuseRandomAircraft(state, []*AirUnit{attacker})
		}
	}
	return hits, events
}

// resolveGroundStrike 按规则书顺序结算对机场的空袭：
// CAP 拦截 → 重型防空 → 每个攻击机承受机场轻型防空 → 按对地攻击值逐点判定（基础 20%）→ 撤离。
func resolveGroundStrike(state *State, taskID, coord string) []EngineEvent {
	airfield := strikeAirfieldAt(*state, "", coord)
	if airfield == nil {
		return nil
	}
	members := taskAircraft(*state, taskID)
	attackers := []*AirUnit{}
	for _, aircraft := range members {
		if aircraft.Status == "attack" && !aircraft.Returning && aircraft.Coord == coord {
			attackers = append(attackers, aircraft)
		}
	}
	if len(attackers) == 0 {
		// 机场 hex 上没有本方攻击机时，可能只是"到达但尚未接敌"，保持静默。
		return nil
	}
	faction := attackers[0].Faction
	defender := airfield.Faction
	setAirfieldContact(state, faction, airfield)
	groups := airfieldCAPGroups(*state, defender, coord)
	report := AirCombatReport{
		Coord: coord, Target: string(StrikeTargetAir), TargetName: airfield.Name,
		AttackerFaction: faction, DefenderFaction: defender,
		AttackerModels: aircraftModels(attackers), AttackerCount: len(attackers),
		Airfield: &AirShipCard{ID: airfield.ID, Name: airfield.Name, Type: string(airfield.Type)},
	}
	for _, group := range groups {
		report.DefenderModels = append(report.DefenderModels, aircraftModels(group)...)
		report.DefenderCount += len(group)
	}
	report.DefenderModels = aircraftModelsFromNames(report.DefenderModels)
	events := []EngineEvent{}
	for _, group := range groups {
		events = append(events, resolveDogfight(state, attackers, group, &report)...)
	}
	bombers := []*AirUnit{}
	for _, aircraft := range attackers {
		if aircraft.Kind != "fighter" {
			bombers = append(bombers, aircraft)
		}
	}
	// 规则书 689 行：重防空的目标池是“当前参与空袭的算子”，护航战斗机也算。
	events = append(events, airfieldHeavyAA(state, attackers, airfield, &report)...)
	capRemaining := false
	for _, group := range groups {
		if len(activeAircraft(state, group)) > 0 {
			capRemaining = true
			break
		}
	}
	if report.TargetAllocations == nil {
		report.TargetAllocations = []StrikeAllocation{}
	}
	groundAircraftBefore := len(sortedGroundAirIDs(*state, airfield.ID))
	for _, aircraft := range bombers {
		if !operationalAircraft(state, aircraft) {
			continue
		}
		allocation := StrikeAllocation{Attacker: aircraft.ID, Model: aircraft.Model, ShipID: airfield.ID, ShipName: airfield.Name}
		report.TargetAllocations = append(report.TargetAllocations, allocation)
		_, lightEvents := airfieldLightAA(state, aircraft, airfield, &report)
		events = append(events, lightEvents...)
		if !operationalAircraft(state, aircraft) {
			continue
		}
		antiGround := aircraft.AntiGround
		if aircraft.Confusion && antiGround > 0 {
			antiGround--
		}
		chance := groundHitChance(*state, airfield, groundAircraftView(aircraft), capRemaining)
		hits := 0
		report.BombAttempts += antiGround
		for ; antiGround > 0; antiGround-- {
			if probabilityRoll(&state.RNGState) >= chance {
				continue
			}
			hits++
			report.StrikeHits++
			airfield.Strength--
			if airfield.Coastal > 0 {
				airfield.Coastal--
			}
			// 规则书：攻击机场造成伤害会影响正在准备的航空任务——跑道一被打，滑行/待飞的算子当场退回机库
			events = append(events, interruptAirfieldOperations(state, airfield)...)
		}
		for index := range report.TargetAllocations {
			if report.TargetAllocations[index].Attacker == aircraft.ID {
				report.TargetAllocations[index].Hits += hits
			}
		}
		events = append(events, EngineEvent{Type: "ground_strike", Audience: "all", Text: fmt.Sprintf("%s攻击%s：命中率%.1f%%，命中%d次", aircraft.Model, airfield.Name, chance, hits)})
	}
	// 轰炸命中摧毁机场上的飞机：每次命中至多 1 个随机地面算子
	report.GroundLoss = destroyGroundAircraft(state, airfield, report.StrikeHits)
	report.AirfieldHit = report.StrikeHits
	report.AirfieldBefore, report.AirfieldAfter = airfield.Strength+report.StrikeHits, airfield.Strength
	if report.AirfieldBefore > 0 {
		report.AirfieldRunwaysBefore = RunwaysAvailable(report.AirfieldBefore, airfield.Runways)
	}
	report.AirfieldRunwaysAfter = RunwaysAvailable(airfield.Strength, airfield.Runways)
	// 战斗机扫射跑道：只摧毁地面飞机，不减生命值和岸防
	sweeps := 0
	for _, aircraft := range attackers {
		if aircraft.Kind == "fighter" && operationalAircraft(state, aircraft) {
			sweeps++
		}
	}
	if sweeps > 0 {
		report.GroundLoss += destroyGroundAircraft(state, airfield, sweeps)
	}
	_ = groundAircraftBefore
	for _, group := range groups {
		events = append(events, resolveDogfight(state, attackers, group, &report)...)
	}
	attackersRemaining := len(activeAircraft(state, attackers))
	report.ID = fmt.Sprintf("air-combat-%x", nextUint64(&state.RNGState))
	switch {
	case attackersRemaining == 0:
		report.Winner = defender
	case report.StrikeHits > 0:
		report.Winner = faction
	default:
		report.Winner = "draw"
	}
	report.Summary = fmt.Sprintf("%s对机场%s空袭：防空命中%d，投弹%d次命中%d次，机场生命值%s→%s（跑道%d条），摧毁地面飞机%d，攻击方损失%d",
		coord, airfield.Name, report.AAHits, report.BombAttempts, report.StrikeHits,
		formatStrength(report.AirfieldBefore), formatStrength(report.AirfieldAfter),
		RunwaysAvailable(airfield.Strength, airfield.Runways), report.GroundLoss, report.AttackerLosses)
	state.AirCombats = append(state.AirCombats, report)
	if len(state.AirCombats) > 20 {
		state.AirCombats = state.AirCombats[len(state.AirCombats)-20:]
	}
	events = append(events, EngineEvent{Type: "air_combat_resolved", Audience: "all", Text: report.Summary})
	for _, aircraft := range attackers {
		if state.Aircraft[aircraft.ID] != nil {
			aircraft.Returning = true
		}
	}
	return events
}

// bombsOnly 过滤出仍可作战的轰炸机（护航战斗机不参与对地投弹）。
func bombsOnly(state *State, bombers []*AirUnit) []*AirUnit {
	result := []*AirUnit{}
	for _, aircraft := range bombers {
		if operationalAircraft(state, aircraft) {
			result = append(result, aircraft)
		}
	}
	return result
}

// groundAircraftView 把舰载算子投影成机场算子，只用于计算命中率修正（没有额外字段）。
func groundAircraftView(aircraft *AirUnit) *AirfieldAirUnit {
	if aircraft == nil {
		return &AirfieldAirUnit{}
	}
	return &AirfieldAirUnit{ID: aircraft.ID, Model: aircraft.Model, Kind: aircraft.Kind, Damaged: aircraft.Damaged || aircraft.Confusion}
}

// destroyGroundAircraft 摧毁机场上至多 n 个随机地面飞机算子（优先机库内整装待发的）。
func destroyGroundAircraft(state *State, airfield *Airfield, n int) int {
	ids := []string{}
	for _, id := range sortedGroundAirIDs(*state, airfield.ID) {
		aircraft := state.GroundAir[id]
		if aircraft.Status == "hangar" || aircraft.Status == "service" || aircraft.Status == "deck" || aircraft.Status == "ready" {
			ids = append(ids, id)
		}
	}
	destroyed := 0
	for ; destroyed < n && len(ids) > 0; destroyed++ {
		index := int(nextUint64(&state.RNGState) % uint64(len(ids)))
		delete(state.GroundAir, ids[index])
		ids = append(ids[:index], ids[index+1:]...)
	}
	return destroyed
}

// interruptAirfieldOperations 机场被命中时打断正在进行的整备（规则书 218 行"攻击会造成伤害，影响正在准备的航空任务"）：
// 跑道上的算子退回机库，并占用一次通行次数；可用通行次数不足时，被挤占的还会顺延到后续回合。
func interruptAirfieldOperations(state *State, airfield *Airfield) []EngineEvent {
	events := []EngineEvent{}
	for _, id := range sortedGroundAirIDs(*state, airfield.ID) {
		aircraft := state.GroundAir[id]
		switch aircraft.Status {
		case "service", "deck", "ready":
			aircraft.Status = "hangar"
			if airfield.OpsRemaining > 0 {
				airfield.OpsRemaining--
			}
			events = append(events, EngineEvent{Type: "airfield_operations_interrupted", Audience: airfield.Faction, Text: fmt.Sprintf("%s遭到空袭，%s的整备作业中断", airfield.Name, aircraft.Model)})
		}
	}
	return events
}

func formatStrength(value int) string {
	return fmt.Sprintf("%d", value)
}

func airfieldTaskAirUnits(state State, airfieldID string) map[string]*AirfieldAirUnit {
	result := map[string]*AirfieldAirUnit{}
	for id, aircraft := range state.GroundAir {
		if aircraft.AirfieldID == airfieldID {
			result[id] = aircraft
		}
	}
	return result
}

func groundTaskAircraft(state State, taskID string) []*AirfieldAirUnit {
	members := []*AirfieldAirUnit{}
	for _, aircraft := range state.GroundAir {
		if aircraft.TaskID == taskID {
			members = append(members, aircraft)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members
}

func isAirfieldTask(taskID string) bool {
	return strings.HasPrefix(taskID, "af-")
}

// ValidateGroundAirOrder 校验机场航空命令：LAUNCH_CAP / LAUNCH_RECON / LAUNCH_STRIKE / PRIORITIZE_AIR_TASK / SET_STRIKE_TARGET。
func ValidateGroundAirOrder(state State, order Order) error {
	if !state.ServiceWindowOpen() && nightBlockedAirOrder(order.Type) {
		return fmt.Errorf("夜间无法执行航空任务")
	}
	ids := airOrderIDs(order)
	if len(ids) == 0 || len(ids) > 20 {
		return fmt.Errorf("航空命令至少需要一个航空算子")
	}
	status := ""
	var airfield *Airfield
	list := make([]*AirfieldAirUnit, 0, len(ids))
	for _, id := range ids {
		aircraft := state.GroundAir[id]
		if aircraft == nil && (order.Type == "PRIORITIZE_AIR_TASK" || order.Type == "SET_STRIKE_TARGET" || order.Type == "SET_STRIKE_DISPERSION") {
			for _, candidate := range state.GroundAir {
				if candidate.TaskID == id {
					aircraft = candidate
					break
				}
			}
		}
		if aircraft == nil {
			return fmt.Errorf("航空算子不存在或不属于当前玩家")
		}
		if status == "" {
			status = aircraft.Status
			airfield = airfieldFor(&state, aircraft.AirfieldID)
		} else if aircraft.Status != status || aircraft.AirfieldID != airfield.ID {
			return fmt.Errorf("同一批命令的机场算子必须来自同一机场且处于相同作业阶段")
		}
		if aircraft.Damaged || aircraft.Maintenance > 0 {
			return fmt.Errorf("损伤或检修中的算子必须完成修复后才能分配任务")
		}
		list = append(list, aircraft)
	}
	if airfield == nil {
		return fmt.Errorf("机场不存在")
	}
	if RunwaysAvailable(airfield.Strength, airfield.Runways) == 0 {
		return fmt.Errorf("%s已经没有可用跑道", airfield.Name)
	}
	switch order.Type {
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE":
		if sameTaskAssignmentGround(list, order) {
			return nil
		}
		if status != "hangar" && status != "service" && status != "deck" && status != "ready" {
			return fmt.Errorf("只有机库或跑道上待命的算子可以分配任务")
		}
		for _, aircraft := range list {
			if aircraft.TaskID != "" {
				return fmt.Errorf("算子已经属于其他任务，请先取消原任务")
			}
			if order.Type == "LAUNCH_CAP" && aircraft.Kind != "fighter" {
				return fmt.Errorf("只有战斗机可以执行CAP")
			}
			if order.Type == "LAUNCH_RECON" && (aircraft.Kind == "fighter") {
				return fmt.Errorf("战斗机不能执行扇面搜索")
			}
			if order.Type == "LAUNCH_STRIKE" && aircraft.Kind == "fighter" && order.Coord == "" && len(list) == 1 {
				return fmt.Errorf("攻击编队只能由轰炸机和护航战斗机组成")
			}
		}
		if order.Type == "LAUNCH_RECON" {
			// 加入已有侦察组时继承原组参数，只需要 task_id。
			joiningExisting := order.TaskID != "" && len(groundTaskAircraft(state, order.TaskID)) > 0
			if !joiningExisting {
				if !ValidReconHeading(order.Heading) {
					return fmt.Errorf("侦察航向必须是E、SE、SW、W、NW或NE")
				}
				if order.SectorWidth != 60 && order.SectorWidth != 120 {
					return fmt.Errorf("侦察扇面只能选择60度或120度")
				}
			}
		}
		if order.Type == "LAUNCH_STRIKE" && order.Coord != "" {
			return ValidateStrikeTarget(state, airfield.Faction, order.Coord)
		}
		if order.TaskID != "" {
			members := groundTaskAircraft(state, order.TaskID)
			if len(members) == 0 || members[0].Faction != airfield.Faction {
				return fmt.Errorf("要加入的航空任务不存在")
			}
			if members[0].TaskKind != "recon" && members[0].TaskKind != "attack" {
				return fmt.Errorf("只有侦察组和攻击编队可以接受新算子")
			}
			for _, member := range members {
				if member.Status == "attack" {
					return fmt.Errorf("攻击编队已经离场，不能继续加入飞机")
				}
			}
		}
		return nil
	case "SET_STRIKE_TARGET":
		if len(list) != 1 || list[0].TaskKind != "attack" {
			return fmt.Errorf("只有攻击编队可以设置目标")
		}
		return ValidateStrikeTarget(state, airfield.Faction, order.Coord)
	case "SET_STRIKE_DISPERSION":
		for _, aircraft := range list {
			if aircraft.TaskKind != "attack" {
				return fmt.Errorf("只有攻击编队可以设置攻击方式")
			}
		}
		return nil
	case "RETURN_TO_HANGAR":
		for _, aircraft := range list {
			if aircraft.Status == "cap" || aircraft.Status == "recon" || aircraft.Status == "attack" || aircraft.Status == "attack_holding" {
				return fmt.Errorf("空中的算子请使用 RECOVER_AIR 或等待返航")
			}
		}
		return nil
	case "RECOVER_AIR":
		for _, aircraft := range list {
			if aircraft.Status != "cap" {
				return fmt.Errorf("只有空中的CAP算子可以主动回收")
			}
		}
		return nil
	case "PRIORITIZE_AIR_TASK":
		if len(list) != 1 || list[0].TaskID == "" {
			return fmt.Errorf("只有已分配的航空任务可以调整优先级")
		}
		return nil
	default:
		return fmt.Errorf("不支持的航空命令")
	}
}

func sameTaskAssignmentGround(list []*AirfieldAirUnit, order Order) bool {
	if order.TaskID == "" || (order.Type != "LAUNCH_RECON" && order.Type != "LAUNCH_STRIKE") {
		return false
	}
	taskKind := "recon"
	if order.Type == "LAUNCH_STRIKE" {
		taskKind = "attack"
	}
	for _, aircraft := range list {
		if aircraft.TaskID != order.TaskID || aircraft.TaskKind != taskKind {
			return false
		}
	}
	return len(list) > 0
}

// ApplyGroundAirOrder 执行机场航空命令。与舰载机保持同一套阶段语义，只是升降机换成滑行到跑道。
func ApplyGroundAirOrder(state *State, order Order) []EngineEvent {
	ids := airOrderIDs(order)
	list := make([]*AirfieldAirUnit, 0, len(ids))
	for _, id := range ids {
		if aircraft := state.GroundAir[id]; aircraft != nil {
			list = append(list, aircraft)
		}
	}
	if len(list) == 0 {
		return nil
	}
	airfield := airfieldFor(state, list[0].AirfieldID)
	if airfield == nil {
		return nil
	}
	switch order.Type {
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE":
		if sameTaskAssignmentGround(list, order) {
			return []EngineEvent{{Type: "air_task_unchanged", Audience: airfield.Faction, Text: "算子已在当前任务中，无需重复加入"}}
		}
		kind := missionKindFromOrder(order.Type)
		taskID := order.TaskID
		priority := 0
		if kind == MissionCap {
			taskID = "af-" + airfield.ID + "-cap"
		}
		if taskID == "" {
			taskID = list[0].ID
		}
		if existing := groundTaskAircraft(*state, taskID); len(existing) > 0 {
			priority = existing[0].Priority
		} else {
			priority = 1
			for _, aircraft := range state.GroundAir {
				if aircraft.Faction == airfield.Faction && aircraft.Priority >= priority {
					priority = aircraft.Priority + 1
				}
			}
		}
		events := []EngineEvent{}
		for _, aircraft := range list {
			beginGroundMission(aircraft, taskID, kind, priority)
			switch kind {
			case MissionRecon:
				heading := order.Heading
				sector := order.SectorWidth
				// 加入已有侦察组时没写参数：继承原组航向和扇面。
				if existing := groundTaskAircraft(*state, taskID); len(existing) > 0 {
					leader := existing[0]
					if !ValidReconHeading(strings.ToUpper(heading)) {
						heading = leader.Heading
					}
					if sector != 60 && sector != 120 {
						sector = leader.SectorWidth
					}
				}
				// 半径按机型上限，不来自命令：以前这里完全没赋值，机场侦察机的
				// ReconRadius 停在 0，起飞当回合就满足"已扫完扇面"直接返航。
				assignGroundReconMission(aircraft, heading, sector)
			case MissionStrike:
				assignGroundStrikeMission(state, aircraft, order.Coord)
			default:
				assignGroundCapMission(aircraft, order.Coord)
			}
			// 按任务换挂载：对舰优先鱼雷、对地挂炸弹（规则书 43–63 行）。
			applyGroundLoadout(*state, aircraft)
			if aircraft.Status == "hangar" {
				aircraft.Status = "service"
			}
			if aircraft.Status == "ready" && !(kind == MissionStrike && aircraft.TargetCoord == "") {
				if !state.LaunchWindowOpen() {
					events = append(events, EngineEvent{Type: "air_launch_delayed", Audience: airfield.Faction, Text: aircraft.Model + state.launchWindowHint()})
					continue
				}
				launchGroundAircraft(state, aircraft, airfield)
				events = append(events, EngineEvent{Type: "air_launched", Audience: airfield.Faction, Text: fmt.Sprintf("%s从%s跑道起飞", aircraft.Model, airfield.Name)})
			}
		}
		return events
	case "SET_STRIKE_TARGET":
		taskID := list[0].TaskID
		for _, aircraft := range groundTaskAircraft(*state, taskID) {
			setGroundStrikeTarget(state, aircraft, airfield.Faction, order.Coord)
		}
		return []EngineEvent{{Type: "strike_target_set", Audience: airfield.Faction, Text: fmt.Sprintf("攻击编队目标设为%s", order.Coord)}}
	case "SET_STRIKE_DISPERSION":
		taskID := list[0].TaskID
		mode := "集中攻击"
		if order.Disperse {
			mode = "分散攻击"
		}
		for _, aircraft := range groundTaskAircraft(*state, taskID) {
			aircraft.Disperse = order.Disperse
		}
		return []EngineEvent{{Type: "strike_dispersion_set", Audience: airfield.Faction, Text: "攻击编队改为" + mode}}
	case "RETURN_TO_HANGAR":
		for _, aircraft := range list {
			aircraft.Status = "hangar"
			aircraft.TaskID = ""
			aircraft.TaskKind = ""
			aircraft.TargetCoord = ""
			aircraft.TargetFleetID = ""
			aircraft.Heading = ""
		}
		return []EngineEvent{{Type: "air_returned_to_hangar", Audience: airfield.Faction, Text: "机场算子已取消任务并回到机库"}}
	case "RECOVER_AIR":
		for _, aircraft := range list {
			aircraft.Status = "recovery"
			aircraft.Returning = true
			aircraft.TaskID = ""
			aircraft.TaskKind = ""
		}
		return []EngineEvent{{Type: "air_recovered", Audience: airfield.Faction, Text: "机场CAP算子开始返航"}}
	case "PRIORITIZE_AIR_TASK":
		selected := list[0]
		for _, aircraft := range state.GroundAir {
			if aircraft.Faction != selected.Faction || aircraft.TaskID == "" {
				continue
			}
			if aircraft.TaskID == selected.TaskID {
				aircraft.Priority = 1
			} else if aircraft.Priority > 1 {
				aircraft.Priority++
			}
		}
		return []EngineEvent{{Type: "air_task_prioritized", Audience: airfield.Faction, Text: "机场任务已调整为最高优先级"}}
	}
	return nil
}

// launchGroundAircraft 让跑道上的算子离场：记录起飞顶点并进入对应任务状态。
func launchGroundAircraft(state *State, aircraft *AirfieldAirUnit, airfield *Airfield) {
	aircraft.Status = aircraft.TaskKind
	if aircraft.TaskKind == "attack" {
		aircraft.Status = "attack_holding"
	}
	aircraft.OriginCoord = airfield.Coord
	aircraft.Coord = airfield.Coord
	aircraft.ReconDistance = 0
	aircraft.Returning = false
	aircraft.TrackingFleetID = ""
}

// AdvanceGroundAir 推进机场上的空中算子：CAP 盘旋、侦察扇面、攻击编队飞行与接敌、返航降落。
func AdvanceGroundAir(state *State) []EngineEvent {
	events := []EngineEvent{}
	ids := make([]string, 0, len(state.GroundAir))
	for id := range state.GroundAir {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	processedAttack := map[string]bool{}
	for _, id := range ids {
		aircraft := state.GroundAir[id]
		if aircraft == nil {
			continue
		}
		if !airfieldAirborne(aircraft) {
			if aircraft.Status == "hangar" {
				refillGroundFuelForLoadout(aircraft)
			}
			continue
		}
		switch aircraft.Status {
		case "cap":
			if aircraft.Returning {
				events = append(events, advanceGroundAirReturn(state, aircraft)...)
			} else {
				aircraft.Spend(1)
			}
		case "recon":
			events = append(events, advanceGroundRecon(state, aircraft)...)
		case "attack_holding":
			// 机场编队同样要等全部算子升空、目标已定才离场；
			// 之前这里只扣航程不做状态切换，导致对地编队永远停在机场上空。
			if groupReady := groundTaskReadyToDepart(*state, aircraft.TaskID); groupReady {
				for _, member := range groundTaskAircraft(*state, aircraft.TaskID) {
					member.Status = "attack"
				}
				events = append(events, EngineEvent{Type: "attack_departed", Audience: aircraft.Faction, Text: fmt.Sprintf("%s攻击编队全部升空，离开机场", aircraft.Model)})
			} else {
				aircraft.Spend(1)
			}
		case "attack":
			if aircraft.Returning {
				events = append(events, advanceGroundAirReturn(state, aircraft)...)
			} else if !processedAttack[aircraft.TaskID] {
				events = append(events, advanceGroundAttack(state, aircraft)...)
				processedAttack[aircraft.TaskID] = true
			}
		}
	}
	return events
}

// advanceGroundAttack 六个方向按"正前方1格 + 两个斜前方各1格"移动；到目标 hex 后调用空袭结算。
func advanceGroundAttack(state *State, aircraft *AirfieldAirUnit) []EngineEvent {
	members := groundTaskAircraft(*state, aircraft.TaskID)
	if aircraft.TargetCoord == "" {
		for _, member := range members {
			member.Spend(1)
		}
		return nil
	}
	for _, member := range members {
		if member.Returning || member.Coord == member.TargetCoord {
			continue
		}
		next := nextGroundFlightCoord(*state, member, member.Coord, member.TargetCoord)
		if next == "" {
			continue
		}
		// 剩余航程 <= 回到机场所需距离时停止前进并返航（规则书 618 行），用路径距离而非直线距离。
		probe := *member
		probe.Coord = next
		probe.SetRange(member.Range() - 1)
		if airfieldFor(state, member.AirfieldID) != nil && probe.Range() <= airfieldRtbDistance(*state, &probe) {
			member.Returning = true
			continue
		}
		member.Coord = next
		member.Spend(1)
	}
	if aircraft.Coord != aircraft.TargetCoord {
		return nil
	}
	// 空袭结算（对舰/对地）都按 state.Aircraft 里的舰载机算子实现。
	// 机场编队住在 state.GroundAir，直接调用会找不到攻击者，所以先投影进去、
	// 结算完再同步回机场算子（航程、损伤、投弹标记都要回写）。
	events := retargetGroundStrikeToContact(state, aircraft, members)
	restore := projectGroundTaskIntoAircraft(state, aircraft.TaskID)
	if strikeAirfieldAt(*state, aircraft.Faction, aircraft.TargetCoord) != nil {
		events = append(events, resolveGroundStrike(state, aircraft.TaskID, aircraft.TargetCoord)...)
	} else {
		events = append(events, resolveAirStrike(state, aircraft.TaskID, aircraft.TargetCoord)...)
	}
	restore()
	return events
}

// retargetGroundStrikeToContact 是机场打击编队的追踪逻辑，规则与舰载编队一致：
// 目标 hex 上已经没有敌舰时，用最新情报把目标改到这支舰队现在的位置。
func retargetGroundStrikeToContact(state *State, aircraft *AirfieldAirUnit, members []*AirfieldAirUnit) []EngineEvent {
	if aircraft.TargetCoord == "" {
		return nil
	}
	for _, member := range members {
		if member.Returning {
			return nil
		}
	}
	if aircraft.TargetFleetID == "" {
		aircraft.TargetFleetID = resolveStrikeFleet(*state, aircraft.Faction, aircraft.TargetCoord)
		if aircraft.TargetFleetID == "" {
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
	}
	return []EngineEvent{{Type: "strike_retarget", Audience: aircraft.Faction, Text: fmt.Sprintf("打击目标%s已无敌舰，按最新情报改追%s", previous, coord)}}
}

// projectGroundTaskIntoAircraft 把机场任务临时投影成舰载机算子，返回一个回写函数。
func projectGroundTaskIntoAircraft(state *State, taskID string) func() {
	members := groundTaskAircraft(*state, taskID)
	restore := func() {}
	if len(members) == 0 {
		return restore
	}
	snapshot := map[string]AirfieldAirUnit{}
	projected := make([]*AirUnit, 0, len(members))
	for _, member := range members {
		snapshot[member.ID] = *member
		unit := groundAirUnit(*state, member)
		copied := unit
		state.Aircraft[member.ID] = &copied
		projected = append(projected, &copied)
	}
	return func() {
		for _, unit := range projected {
			original := state.GroundAir[unit.ID]
			if original == nil {
				delete(state.Aircraft, unit.ID)
				continue
			}
			original.Damaged = original.Damaged || unit.Damaged
			original.Returning = unit.Returning
			original.SetRange(unit.Range())
			delete(state.Aircraft, unit.ID)
		}
		for id, snap := range snapshot {
			if original := state.GroundAir[id]; original != nil && snap.Returning && !original.Returning {
				original.Returning = true
			}
		}
	}
}

// nextGroundFlightCoord 是空军版寻路：与 nextFlightCoord 相同，只是没有母舰时按"纯水面舰队"通行。
func nextGroundFlightCoord(state State, aircraft *AirfieldAirUnit, from, target string) string {
	probe := &AirUnit{ID: aircraft.ID, Faction: aircraft.Faction, Coord: from, CarrierID: ""}
	return nextFlightCoord(state, probe, target)
}

// advanceGroundAirReturn 朝机场返航；抵达且跑道可用时降落。
func advanceGroundAirReturn(state *State, aircraft *AirfieldAirUnit) []EngineEvent {
	airfield := airfieldFor(state, aircraft.AirfieldID)
	if airfield == nil {
		delete(state.GroundAir, aircraft.ID)
		return nil
	}
	for steps := 0; steps < 2 && aircraft.Coord != airfield.Coord; steps++ {
		next := nextGroundFlightCoord(*state, aircraft, aircraft.Coord, airfield.Coord)
		if next == "" {
			break
		}
		aircraft.Coord = next
		aircraft.Spend(1)
	}
	if aircraft.Coord != airfield.Coord {
		return nil
	}
	return landGroundAircraft(state, aircraft, airfield)
}

func landGroundAircraft(state *State, aircraft *AirfieldAirUnit, airfield *Airfield) []EngineEvent {
	crashed, events := resolveGroundLandingRisk(state, aircraft)
	if crashed {
		return events
	}
	aircraft.Status = "recovery"
	aircraft.Returning = true
	aircraft.TaskID = ""
	aircraft.TaskKind = ""
	aircraft.TargetCoord = ""
	aircraft.TargetFleetID = ""
	// 无跑道着陆：60% 概率迫降失败进入损伤状态
	if RunwaysAvailable(airfield.Strength, airfield.Runways) == 0 {
		if probabilityRoll(&state.RNGState) < 60 {
			aircraft.Damaged = true
			return append(events, EngineEvent{Type: "hard_landing", Audience: airfield.Faction, Text: fmt.Sprintf("%s在无跑道的%s迫降失败，机体检修中", aircraft.Model, airfield.Name)})
		}
	}
	return append(events, EngineEvent{Type: "air_landed", Audience: airfield.Faction, Text: fmt.Sprintf("%s降落在%s", aircraft.Model, airfield.Name)})
}

// advanceGroundRecon 与舰载侦察相同的扇面搜索规则，顶点是机场。
func advanceGroundRecon(state *State, aircraft *AirfieldAirUnit) []EngineEvent {
	airfield := airfieldFor(state, aircraft.AirfieldID)
	if airfield == nil {
		return nil
	}
	if aircraft.OriginCoord == "" {
		aircraft.OriginCoord = airfield.Coord
	}
	events := []EngineEvent{}
	for steps := 0; steps < 2; steps++ {
		if aircraft.Returning {
			if aircraft.Coord == airfield.Coord {
				return append(events, landGroundAircraft(state, aircraft, airfield)...)
			}
		} else if aircraft.ReconDistance >= aircraft.ReconRadius || aircraft.Range() <= Distance(aircraft.Coord, airfield.Coord) {
			aircraft.Returning = true
		}
		// 与舰载侦察一致：每步判"当前格 + 正前方一格"，去程返程各扫一遍。
		forward := ""
		if aircraft.Returning {
			forward = nextGroundFlightCoord(*state, aircraft, aircraft.Coord, airfield.Coord)
		} else {
			forward = nextReconCoord(state.Map, aircraft.Coord, aircraft.Heading)
		}
		events = append(events, searchGroundReconStep(state, aircraft, forward)...)
		if forward == "" {
			aircraft.Returning = true
			aircraft.Spend(1)
			continue
		}
		aircraft.Coord = forward
		aircraft.ReconDistance = Distance(aircraft.OriginCoord, forward)
		aircraft.Spend(1)
	}
	return events
}

// searchGroundReconStep 判定机场侦察机当前格与前方一格的敌方舰队和机场。
func searchGroundReconStep(state *State, aircraft *AirfieldAirUnit, forward string) []EngineEvent {
	coords := make([]string, 0, 2)
	if aircraft.Coord != "" {
		coords = append(coords, aircraft.Coord)
	}
	if forward != "" && forward != aircraft.Coord {
		coords = append(coords, forward)
	}
	if len(coords) == 0 {
		return nil
	}
	inside := make(map[string]bool, len(coords))
	for _, coord := range coords {
		inside[coord] = true
	}
	groups := map[string][]Unit{}
	for _, unit := range state.Units {
		if unit.Faction != aircraft.Faction && inside[unit.Coord] {
			groups[FleetKey(unit)] = append(groups[FleetKey(unit)], *unit)
		}
	}
	events := []EngineEvent{{Type: "recon_step_searched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s扫描%s（前方%s）", aircraft.Model, strings.Join(coords, "、"), forward)}}
	for _, fleetID := range sortedFleetIDs(groups) {
		members := groups[fleetID]
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		chance := groundReconChance(*state, aircraft, members)
		if probabilityRoll(&state.RNGState) >= chance {
			continue
		}
		level := recordContact(state, aircraft.Faction, fleetID, members)
		events = append(events, EngineEvent{Type: "recon_contact", Audience: aircraft.Faction, Text: fmt.Sprintf("%s发现敌方舰队：%d艘，情报等级%d", members[0].Coord, len(members), level)})
	}
	return append(events, discoverEnemyAirfieldsAt(state, aircraft, coords)...)
}
func groundReconChance(state State, aircraft *AirfieldAirUnit, members []Unit) float64 {
	chance := 25.0
	for _, unit := range members {
		if isCapitalShip(unit.Type) {
			chance += 5
		} else {
			chance++
		}
	}
	if len(members) > 0 {
		switch weatherAt(state.Map, members[0].Coord) {
		case "cloud":
			chance -= 5
		case "rain":
			chance -= 10
		case "storm":
			chance -= 15
		}
		chance -= 2 * float64(Distance(aircraft.OriginCoord, members[0].Coord))
	}
	if aircraft.SectorWidth == 120 {
		chance /= 2
	}
	if chance < 0 {
		return 0
	}
	return chance
}

func sortedFleetIDs(groups map[string][]Unit) []string {
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// GroundAviationView 是航空面板里"机场侧"的完整投影：机场状态与机场任务。
func GroundAviationView(state State, faction string) AviationView {
	view := AviationView{Carriers: []CarrierAirStatus{}, Tasks: []AirTask{}}
	for _, status := range VisibleGroundAir(state, faction) {
		carrier := CarrierAirStatus{UnitID: status.UnitID, Name: status.Name, Type: string(status.Type), DeckLength: status.Runways}
		carrier.Aircraft = []AirUnit{}
		for _, aircraft := range status.Aircraft {
			carrier.Aircraft = append(carrier.Aircraft, groundAirUnit(state, &aircraft))
		}
		view.Carriers = append(view.Carriers, carrier)
	}
	groups := map[string][]*AirfieldAirUnit{}
	for _, aircraft := range state.GroundAir {
		if aircraft.Faction == faction && aircraft.TaskID != "" {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	taskIDs := make([]string, 0, len(groups))
	for id := range groups {
		taskIDs = append(taskIDs, id)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		members := groups[taskID]
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		leader := members[0]
		task := AirTask{ID: taskID, Kind: leader.TaskKind, Name: leader.Model, Priority: leader.Priority, AircraftIDs: []string{}, Aircraft: []AirUnit{}, CarrierIDs: []string{leader.AirfieldID}, CarrierID: leader.AirfieldID}
		for _, member := range members {
			unit := groundAirUnit(state, member)
			task.Aircraft = append(task.Aircraft, unit)
			task.AircraftIDs = append(task.AircraftIDs, member.ID)
			if airfieldAirborne(member) {
				task.AirborneCount++
			}
		}
		task.Phase = "servicing"
		if task.AirborneCount > 0 {
			task.Phase = "active"
		}
		if leader.TaskKind == "attack" {
			task.TargetCoord = leader.TargetCoord
			task.Disperse = leader.Disperse
			task.CurrentCoord = leader.Coord
			task.Name = "攻击编队 · " + leader.Model
		} else if leader.TaskKind == "cap" {
			airfield := airfieldFor(&state, leader.AirfieldID)
			if airfield != nil {
				task.CurrentCoord = airfield.Coord
			}
			task.Name = "CAP · " + leader.Model
		} else {
			task.Name = "扇面侦察 · " + leader.Model
			task.CurrentCoord = leader.Coord
			task.Heading = leader.Heading
			task.SectorWidth = leader.SectorWidth
			task.ReconRadius = leader.ReconRadius
		}
		if len(members) > 1 {
			task.Name += fmt.Sprintf(" ×%d", len(members))
		}
		view.Tasks = append(view.Tasks, task)
	}
	sort.Slice(view.Tasks, func(i, j int) bool {
		if view.Tasks[i].Priority == view.Tasks[j].Priority {
			return view.Tasks[i].ID < view.Tasks[j].ID
		}
		return view.Tasks[i].Priority < view.Tasks[j].Priority
	})
	for index := range view.Tasks {
		view.Tasks[index].QueuePosition = index + 1
	}
	return view
}

// groundAirUnit 把机场算子投影成前端通用的 AirUnit，地图与任务栏因此可以复用同一套渲染。
func groundAirUnit(state State, aircraft *AirfieldAirUnit) AirUnit {
	return AirUnit{
		ID: aircraft.ID, DefinitionID: aircraft.DefinitionID, CarrierID: aircraft.AirfieldID, Faction: aircraft.Faction, Model: aircraft.Model, Kind: aircraft.Kind,
		Status: aircraft.Status, Strength: aircraft.Strength, MaxRange: aircraft.MaxRange, RemainingRange: aircraft.Range(),
		AirCombat: aircraft.AirCombat, AntiShip: aircraft.AntiShip, AntiGround: aircraft.AntiGround,
		TaskID: aircraft.TaskID, TaskKind: aircraft.TaskKind, Priority: aircraft.Priority,
		Coord: aircraft.Coord, OriginCoord: aircraft.OriginCoord, TargetCoord: aircraft.TargetCoord, Heading: aircraft.Heading,
		SectorWidth: aircraft.SectorWidth, ReconRadius: aircraft.ReconRadius, ReconDistance: aircraft.ReconDistance,
		ReconRadiusLimit: aircraft.ReconRadiusLimit, Returning: aircraft.Returning, TrackingFleetID: aircraft.TrackingFleetID,
		Confusion: aircraft.Damaged, Damaged: aircraft.Damaged, Maintenance: aircraft.Maintenance,
	}
}

// GroundAircraftView 返回本阵营所有机场算子的通用投影，供地图渲染飞行中的机群。
func GroundAircraftView(state State, faction string) []AirUnit {
	result := []AirUnit{}
	for _, id := range sortedGroundAirIDsByFaction(state, faction) {
		result = append(result, groundAirUnit(state, state.GroundAir[id]))
	}
	return result
}

func sortedGroundAirIDsByFaction(state State, faction string) []string {
	ids := make([]string, 0, len(state.GroundAir))
	for id, aircraft := range state.GroundAir {
		if aircraft.Faction == faction {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// forwardIntercept 处理"雷达截获 + CAP 自动前出"：
// 截获成功 → 2 hex 内的 CAP 自动前出（最多 2 hex）到攻击编队所在 hex，与本回合打一次空战；
// 截获失败 → 不拦截，攻击编队继续飞，最终只承受目标头顶的舰队/机场防空。
// 每回合最多一次，直到攻击机进入目标上空发起攻击。
func interceptIncoming(state *State, faction, coord string, attackers []*AirUnit, defender string) []EngineEvent {
	events := []EngineEvent{}
	if len(attackers) == 0 || defender == "" {
		return events
	}
	if !radarDetects(*state, defender, coord) {
		events = append(events, EngineEvent{Type: "radar_missed", Audience: defender, Text: "雷达未能截获来袭编队，CAP 未能前出"})
		return events
	}
	events = append(events, EngineEvent{Type: "radar_contact", Audience: defender, Text: "雷达截获来袭编队，CAP 自动前出拦截"})
	// 记录"已被提前发现"：后续空袭命中的 DCT 临时 +1（规则书 408 行）
	for _, attacker := range attackers {
		attacker.WarnedIntercept = true
	}
	return append(events, forwardIntercept(state, faction, coord, attackers, defender)...)
}
func forwardIntercept(state *State, faction, coord string, attackers []*AirUnit, defender string) []EngineEvent {
	if state == nil || len(attackers) == 0 || defender == "" {
		return nil
	}
	groups := capGroupsWithin(*state, defender, coord, 2)
	if len(groups) == 0 {
		return nil
	}
	report := AirCombatReport{
		ID:              fmt.Sprintf("air-intercept-%x", nextUint64(&state.RNGState)),
		Coord:           coord,
		Target:          "intercept",
		TargetName:      "雷达引导拦截",
		AttackerFaction: faction,
		DefenderFaction: defender,
		AttackerModels:  aircraftModels(attackers),
		AttackerCount:   len(attackers),
	}
	for _, group := range groups {
		report.DefenderModels = append(report.DefenderModels, aircraftModels(group)...)
		report.DefenderCount += len(group)
	}
	report.DefenderModels = aircraftModelsFromNames(report.DefenderModels)
	events := []EngineEvent{}
	for _, group := range groups {
		events = append(events, resolveDogfight(state, attackers, group, &report)...)
	}
	attackersRemaining := len(activeAircraft(state, attackers))
	capRemaining := false
	for _, group := range groups {
		if len(activeAircraft(state, group)) > 0 {
			capRemaining = true
		}
	}
	switch {
	case attackersRemaining == 0:
		report.Winner = defender
	case !capRemaining:
		report.Winner = faction
	case report.AttackerLosses > report.DefenderLosses:
		report.Winner = defender
	case report.AttackerLosses == report.DefenderLosses:
		report.Winner = "draw"
	default:
		report.Winner = faction
	}
	report.Summary = fmt.Sprintf("%s雷达截获来袭编队，CAP 前出拦截：攻击方%s，CAP%s", coord, airLossText(&report, true), airLossText(&report, false))
	state.AirCombats = append(state.AirCombats, report)
	if len(state.AirCombats) > 20 {
		state.AirCombats = state.AirCombats[len(state.AirCombats)-20:]
	}
	// CAP 自动前出：记录前出点（最多 2 hex），覆盖原来的巡逻点；前出+返回都按实际距离扣航程。
	// 被击中或弹药耗尽自动返航的 CAP 已由 resolveDogfight 置 Returning，这里不再改动；
	// 剩余航程不够返航的也直接返航。
	for _, group := range groups {
		for _, aircraft := range group {
			if state.Aircraft[aircraft.ID] == nil {
				continue
			}
			patrol := aircraft.PatrolCoord
			if patrol == "" {
				patrol = capCoord(*state, aircraft)
			}
			// 只扣去程：回程由下一回合 CAP 分支按实际距离扣，之前这里一次扣了往返(2d)，
			// 下一回合回巡逻点又扣一次 d，同一趟拦截被收了 3d 的油。
			aircraft.Spend(Distance(patrol, coord))
			aircraft.ForwardCoord = coord
			if Distance(patrol, coord) > 0 {
				// 真的挪了位置才算"本回合已前出"，回程由下一回合扣油；
				// 本来就在目标上空的 CAP 不标记，正常参与这一轮空袭的 CAP 结算。
				aircraft.InterceptedThisTurn = true
			}
			if aircraft.Range() <= 0 {
				aircraft.Returning = true
				aircraft.ForwardCoord = ""
			}
		}
	}
	events = append(events, EngineEvent{Type: "cap_intercepted", Audience: "all", Text: report.Summary})
	return events
}

// —— 岸轰（规则书 499–516 行）——

// trackFleetStay 记录每个舰队本回合结束时所在的 hex。
// 与上一回合同一 hex 即视为"已停留满 1 回合"，满足岸轰条件；
// 移动过的舰队会在自己的移动事件里清掉旧记录（见 ApplyImmediateMove / 结算移动）。
func trackFleetStay(state *State) {
	if state.FleetStays == nil {
		state.FleetStays = map[string]string{}
	}
	current := map[string]string{}
	for _, unit := range state.Units {
		if unit == nil {
			continue
		}
		fleetID := FleetKey(unit)
		if _, seen := current[fleetID]; seen {
			continue
		}
		current[fleetID] = unit.Coord
	}
	for fleetID := range state.FleetStays {
		if _, ok := current[fleetID]; !ok {
			delete(state.FleetStays, fleetID)
		}
	}
	// 记录本回合结束后各舰队所在 hex：下一回合仍在同一 hex 即视为"停留满 1 回合"
	for fleetID, coord := range current {
		state.FleetStays[fleetID] = coord
	}
}

// VisibleFleetStays 只把本阵营舰队的停留 hex 暴露给前端，用于提示"可以岸轰"。
func VisibleFleetStays(state State, faction string) map[string]string {
	result := map[string]string{}
	for _, unit := range state.Units {
		if unit == nil || unit.Faction != faction {
			continue
		}
		fleetID := FleetKey(unit)
		if coord, ok := state.FleetStays[fleetID]; ok {
			result[fleetID] = coord
		}
	}
	return result
}

// clearFleetStay 在舰队移动后调用，让岸轰的停留计时重新开始。
func clearFleetStay(state *State, fleetID string) {
	if state.FleetStays != nil {
		delete(state.FleetStays, fleetID)
	}
}

// shoreTargetAt 返回某 hex 上可供岸轰的敌方机场。
func shoreTargetAt(state State, faction, coord string) *Airfield {
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield.Faction != faction && airfield.Coord == coord {
			return airfield
		}
	}
	return nil
}

// canBombard 返回本舰队本回合可以岸轰的目标机场。
// 条件：有主炮、在目标 hex 停留满 1 回合、同 hex 没有敌方舰队、舰队混乱为 0。
func canBombard(state State, unit *Unit) *Airfield {
	if unit == nil || unit.Card.Gunnery <= 0 || unitSmoking(unit) {
		return nil
	}
	if state.FleetStays[FleetKey(unit)] != unit.Coord {
		return nil
	}
	if FleetConfusion(state, FleetKey(unit)) > 0 {
		return nil
	}
	for _, other := range state.Units {
		if other.Faction != unit.Faction && other.Coord == unit.Coord {
			return nil
		}
	}
	return shoreTargetAt(state, unit.Faction, unit.Coord)
}

// resolveShoreBombardment 结算岸轰：舰队先与岸炮互射，命中再作用于机场。
func resolveShoreBombardment(state *State) []EngineEvent {
	events := []EngineEvent{}
	done := map[string]bool{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		airfield := canBombard(*state, unit)
		if airfield == nil {
			continue
		}
		fleetID := FleetKey(unit)
		if done[fleetID] {
			continue
		}
		done[fleetID] = true
		events = append(events, bombardFleet(state, fleetID, airfield)...)
	}
	return events
}

func hexHitModifier(state State, coord string) float64 {
	switch weatherAt(state.Map, coord) {
	case "rain":
		return -5
	case "storm":
		return -10
	}
	return 0
}

func bombardFleet(state *State, fleetID string, airfield *Airfield) []EngineEvent {
	events := []EngineEvent{}
	// 岸炮先开火：命中随机舰船，每点岸炮 30% 独立判定
	coastalHits := 0
	for shots := airfield.Coastal; shots > 0; shots-- {
		if probabilityRoll(&state.RNGState) >= 30+hexHitModifier(*state, airfield.Coord) {
			continue
		}
		alive := fleetMembers(state, fleetID)
		if len(alive) == 0 {
			break
		}
		target := state.Units[alive[int(nextUint64(&state.RNGState)%uint64(len(alive)))].ID]
		if target == nil {
			continue
		}
		coastalHits++
		events = append(events, applyHits(state, []ShellHit{{AttackerID: airfield.ID, TargetID: target.ID}})...)
	}
	// 舰队炮击机场：普通舰每次命中 -1 生命值并摧毁至多 1 架地面飞机，战列舰 -2 / 至多 2 架
	// 舰队在开火前指定目标，同一次齐射只结算一类（规则书 513 行）：
	// 滩头物资仍有余量就打物资，否则打机场。
	gunneryHits, airfieldDamage, groundLoss := 0, 0, 0
	supplyHits := 0
	bombardFaction := ""
	for _, member := range fleetMembers(state, fleetID) {
		ship := state.Units[member.ID]
		if ship == nil || ship.Card.Gunnery <= 0 || unitSmoking(ship) {
			continue
		}
		bombardFaction = ship.Faction
		chance := 25.0 + hexHitModifier(*state, airfield.Coord)
		for shot := 0; shot < ship.Card.Gunnery; shot++ {
			if chance <= 0 || probabilityRoll(&state.RNGState) >= chance {
				continue
			}
			gunneryHits++
			if shoreSupplies(*state, bombardFaction, airfield.Coord) > 0 {
				removeShoreSupplies(state, bombardFaction, airfield.Coord, 1)
				supplyHits++
				continue
			}
			damage, aircraft := 1, 1
			if ship.Type == "BB" {
				damage, aircraft = 2, 2
			}
			if airfield.Strength > 0 {
				appliedDamage := damage
				if airfield.Strength-appliedDamage < 0 {
					appliedDamage = airfield.Strength
				}
				airfield.Strength -= appliedDamage
				airfieldDamage += appliedDamage
			}
			groundLoss += destroyGroundAircraft(state, airfield, aircraft)
			if airfield.Strength <= 0 {
				break
			}
		}
	}
	// 岸轰后获得 4 点混乱，必须等混乱衰减完才能再次岸轰
	for _, member := range fleetMembers(state, fleetID) {
		if ship := state.Units[member.ID]; ship != nil {
			ship.Confusion += 4
		}
	}
	leaderName := fleetID
	for _, member := range fleetMembers(state, fleetID) {
		leaderName = member.FleetName
		if leaderName == "" {
			leaderName = member.Name
		}
		break
	}
	supText := ""
	if supplyHits > 0 {
		supText = fmt.Sprintf("，摧毁滩头物资%d点", supplyHits)
	}
	events = append(events, EngineEvent{Type: "shore_bombardment", Audience: "all", Text: fmt.Sprintf("%s在%s对%s岸轰：命中机场%d次（生命值-%d，摧毁地面飞机%d%s），岸炮反击命中%d次，舰队混乱+4", leaderName, airfield.Coord, airfield.Name, gunneryHits, airfieldDamage, groundLoss, supText, coastalHits)})
	return events
}

// —— 港口：机场 hex 的陆上设施，舰队停靠后可以补给 ——

// portFor 返回某 hex 上的港口（即机场的海港部分）。
func portFor(state State, coord string) *Airfield {
	for _, id := range sortedAirfieldIDs(state) {
		if state.Airfields[id].Coord == coord {
			return state.Airfields[id]
		}
	}
	return nil
}

// resolvePortSupply 结算舰队停靠补给：舰队在友方机场 hex 停留满 1 回合时，清除岸轰积累的混乱。
func resolvePortSupply(state *State) []EngineEvent {
	events := []EngineEvent{}
	done := map[string]bool{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || done[FleetKey(unit)] {
			continue
		}
		port := portFor(*state, unit.Coord)
		if port == nil || port.Faction != unit.Faction {
			continue
		}
		fleetID := FleetKey(unit)
		// 港口里有正在燃烧的舰船时无法补给：着火的舰队顾不上组织补给作业。
		if fleetBurning(*state, fleetID) {
			continue
		}
		if state.FleetStays[fleetID] != unit.Coord {
			continue
		}
		done[fleetID] = true
		restored := 0
		for _, member := range fleetMembers(state, fleetID) {
			ship := state.Units[member.ID]
			if ship == nil {
				continue
			}
			if ship.Confusion > 0 {
				restored += ship.Confusion
				ship.Confusion = 0
			}
		}
		if restored > 0 {
			events = append(events, EngineEvent{Type: "port_supply", Audience: unit.Faction, Text: fmt.Sprintf("%s在%s完成补给，混乱-%d", unit.FleetName, port.Name, restored)})
		}
	}
	return events
}

// radarDetectionChance 按单位类型和距离给出"每回合发现来袭编队"的概率。
func radarDetectionChance(unit *Unit, distance int) float64 {
	if distance < 1 || distance > 5 {
		return 0
	}
	tables := map[string][5]float64{
		"us_capital":  {90, 70, 50, 30, 10},
		"jp_capital":  {85, 65, 45, 25, 5},
		"other_radar": {60, 40, 20, 0, 0},
		"no_radar":    {30, 0, 0, 0, 0},
	}
	damage := EnsureDamage(unit)
	key := "no_radar"
	if unit.Card.AirRadar && !damage.AirRadarDisabled {
		switch {
		case IsCarrier(unit) || unit.Type == "BB":
			key = "us_capital"
			if unit.Faction == "B" {
				key = "jp_capital"
			}
		default:
			key = "other_radar"
		}
	}
	return tables[key][distance-1]
}

// airfieldRadarChance 是机场雷达的发现概率（规则书 736–742 行）。
// 机场和舰艇不是同一张表：美军机场 100/80/60/40/20，日军机场 80/60/40/20/0。
// 以前机场是拿舰艇表算的：美军机场套"航母/战列舰"那行、日军机场套日军主力舰那行，
// 机场雷达比同距离的航母还弱，跟规则对不上。
func airfieldRadarChance(faction string, distance int) float64 {
	if distance < 1 || distance > 5 {
		return 0
	}
	if faction == "B" {
		return [5]float64{80, 60, 40, 20, 0}[distance-1]
	}
	return [5]float64{100, 80, 60, 40, 20}[distance-1]
}

// noRadarChance 是没装雷达的单位在 1 hex 内的发现概率（规则书 742 行）。
func noRadarChance(distance int) float64 {
	if distance != 1 {
		return 0
	}
	return 30
}

// radarDetects 判定防守方雷达是否发现位于 coord 的来袭编队。
func radarDetects(state State, defender, coord string) bool {
	for _, id := range sortedUnitIDs(state.Units) {
		ship := state.Units[id]
		if ship.Faction != defender {
			continue
		}
		chance := radarDetectionChance(ship, Distance(ship.Coord, coord))
		if chance <= 0 {
			continue
		}
		if !ship.Card.AirRadar || EnsureDamage(ship).AirRadarDisabled {
			switch weatherAt(state.Map, coord) {
			case "cloud":
				chance *= 0.5
			case "rain":
				chance *= 0.35
			case "storm":
				chance *= 0.2
			}
		}
		if probabilityRoll(&state.RNGState) <= chance {
			return true
		}
	}
	// 机场雷达同样能引导 CAP（规则书 806–808 行没把机场排除在外）。
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield == nil || airfield.Faction != defender {
			continue
		}
		distance := Distance(airfield.Coord, coord)
		chance := airfieldRadarChance(airfield.Faction, distance)
		if !airfield.Radar {
			chance = applyWeatherToDetection(noRadarChance(distance), weatherAt(state.Map, coord))
		}
		if chance <= 0 {
			continue
		}
		if probabilityRoll(&state.RNGState) <= chance {
			return true
		}
	}
	return false
}

// capGroupsWithin 找出某阵营在 coord 附近 distance 格内的 CAP 编队（按距离排序）。
func capGroupsWithin(state State, defender, coord string, distance int) [][]*AirUnit {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != defender || aircraft.Status != "cap" || aircraft.Returning || aircraft.TaskID == "" {
			continue
		}
		// 一回合只能前出拦截一次：多支攻击编队同回合经过时，同一队 CAP 不会被反复拉出去打架。
		if aircraft.InterceptedThisTurn {
			continue
		}
		if Distance(capCoord(state, aircraft), coord) <= distance {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := capCoord(state, groups[ids[i]][0]), capCoord(state, groups[ids[j]][0])
		if Distance(left, coord) == Distance(right, coord) {
			return ids[i] < ids[j]
		}
		return Distance(left, coord) < Distance(right, coord)
	})
	result := [][]*AirUnit{}
	for _, id := range ids {
		sort.Slice(groups[id], func(i, j int) bool { return groups[id][i].ID < groups[id][j].ID })
		result = append(result, groups[id])
	}
	return result
}

// groundTaskReadyToDepart 判断机场攻击编队是否可以离场：
// 全部成员都已升空（处于 attack_holding 或 attack）且已经设定目标。
func groundTaskReadyToDepart(state State, taskID string) bool {
	members := groundTaskAircraft(state, taskID)
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if member.Returning {
			return false
		}
		if member.Status != "attack_holding" && member.Status != "attack" {
			return false
		}
		if member.TargetCoord == "" {
			return false
		}
	}
	return true
}
