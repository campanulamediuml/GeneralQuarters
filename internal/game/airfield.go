package game

import (
	"fmt"
	"sort"
	"strings"
)

// AirfieldContact 是侦察发现的敌方机场（只上报类型与跑道数，不返回防空和燃料等细节）。
type AirfieldContact struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Faction     string       `json:"faction"`
	Coord       string       `json:"coord"`
	Type        AirfieldType `json:"type"`
	Picture     string       `json:"picture"`
	Runways     int          `json:"runways"`
	MaxRunways  int          `json:"max_runways"`
	Strength    int          `json:"strength"`
	MaxStrength int          `json:"max_strength"`
	Radar       bool         `json:"radar"`
	HeavyAA     float64      `json:"heavy_aa"`
	LightAA     float64      `json:"light_aa"`
	Coastal     int          `json:"coastal"`
	HangarUsed  int          `json:"hangar_used"`
	// EnemySupplies 是敌方卸在这个机场上的滩头物资：只要有货，机场就处于被压制状态。
	EnemySupplies int `json:"enemy_supplies"`
	ContactLevel  int `json:"contact_level"`
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
	contact.Type = NormalizeAirfieldType(airfield.Type)
	contact.Picture = airfield.PictureOrDefault()
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
// 机场是固定的地面设施，不会移动，所以位置从一开始就完全可见（与舰队不同）。
// 以前敌方机场只暴露类型和跑道，防空、生命值、机库占用要侦察后才给；但地图上的
// 机场算子现在直接承担"炮击/轰炸效果面板"的角色，所以当前生命值、可用跑道、
// 防空、岸防和机库占用也实时返回。名字/类型仍按接触情报的等级展示，数值不藏。
func VisibleAirfields(state State, faction string) []AirfieldContact {
	result := []AirfieldContact{}
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		base := AirfieldContact{ContactLevel: 1}
		if airfield.Faction == faction {
			base.ContactLevel = 4
		} else if scouted := state.AirfieldContacts[faction][id]; scouted != nil {
			base = *scouted
		}
		result = append(result, airfieldContactView(state, airfield, base))
	}
	return result
}

// airfieldContactView 把机场的真实整备状态投影成前线可见的算子。
// 名字、类型、图片以情报里的值为准（未侦察时是基础值），其余数值一律取实时状态。
func airfieldContactView(state State, airfield *Airfield, contact AirfieldContact) AirfieldContact {
	contact.ID = airfield.ID
	contact.Name = airfield.Name
	contact.Faction = airfield.Faction
	contact.Coord = airfield.Coord
	contact.Type = NormalizeAirfieldType(airfield.Type)
	contact.Picture = airfield.PictureOrDefault()
	contact.MaxRunways = airfield.Runways
	contact.Runways = RunwaysAvailable(airfield.Strength, airfield.Runways)
	contact.Strength = airfield.Strength
	contact.MaxStrength = airfield.MaxStrength
	contact.Radar = airfield.Radar
	contact.HeavyAA = airfield.HeavyAA
	contact.LightAA = airfield.LightAA
	contact.Coastal = airfield.Coastal
	contact.HangarUsed = groundedAirCount(state, airfield.ID)
	contact.EnemySupplies = enemySuppliesAt(state, airfield)
	return contact
}

func groundedAirCount(state State, airfieldID string) int {
	count := 0
	for _, aircraft := range state.GroundAir {
		if aircraft.AirfieldID == airfieldID && !airfieldAirborne(aircraft) {
			count++
		}
	}
	return count
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
			UnitID: airfield.ID, Faction: airfield.Faction, Name: airfield.Name, Type: NormalizeAirfieldType(airfield.Type), Picture: airfield.PictureOrDefault(), Coord: airfield.Coord,
			Strength: airfield.Strength, MaxStrength: airfield.MaxStrength, MaxRunways: airfield.Runways, EnemySupplies: enemySuppliesAt(state, airfield),
			Runways: RunwaysAvailable(airfield.Strength, airfield.Runways), Radar: airfield.Radar,
			HangarCapacity: airfield.HangarCapacity,
			HeavyAA:        airfield.HeavyAA, LightAA: airfield.LightAA, Coastal: airfield.Coastal,
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

// unitIsSeaplane 判断机场算子是不是水上飞机。老存档没有 seaplane 标记，
// 按"它停在什么机场"兜底：停在水上飞机基地的就是水机。
func unitIsSeaplane(state State, aircraft *AirfieldAirUnit) bool {
	if aircraft == nil {
		return false
	}
	if aircraft.Seaplane {
		return true
	}
	if airfield := airfieldFor(&state, aircraft.AirfieldID); airfield != nil && NormalizeAirfieldType(airfield.Type) == AirfieldSeaplane {
		return true
	}
	return false
}

// AirfieldHangarUnlimited 是机场的默认上限：机场没有机库，飞机停在露天停机位，
// 不按航母机库那样卡容量，所以默认给一个"实际上等于不限"的大数。
// 检查逻辑和航母走同一套（airfieldHangarFull），剧本仍可用 airfields[].hangar_capacity
// 给某个小机场写一个真实上限。
const AirfieldHangarUnlimited = 99999

// airfieldHangarUsed 统计停在机场上的算子（机库/整备/跑道/回收），
// 不含已经升空执行任务的——那些返航时会落回来，但不占当前停机位。
// 必须和界面上的"机库 X / Y"同源，否则会出现"显示 10/18、转场却说已满"。
func airfieldHangarUsed(state State, airfieldID string) int {
	used := 0
	for _, aircraft := range state.GroundAir {
		if aircraft != nil && aircraft.AirfieldID == airfieldID && !airfieldAirborne(aircraft) {
			used++
		}
	}
	return used
}

// airfieldHangarFull 校验再收 incoming 个算子会不会超过机场上限。
// 容量 0 的老存档按"无上限"处理；默认 99999 时这条检查永远通过。
func airfieldHangarFull(state State, airfield *Airfield, incoming int) error {
	if airfield == nil || incoming <= 0 {
		return nil
	}
	capacity := airfield.HangarCapacity
	if capacity <= 0 {
		capacity = AirfieldHangarUnlimited
	}
	used := airfieldHangarUsed(state, airfield.ID)
	if used+incoming > capacity {
		return fmt.Errorf("%s停放机位已满（%d/%d 个航空算子），无法再接收转场", airfield.Name, used, capacity)
	}
	return nil
}

// NormalizeAirfieldHangar 给老存档补默认上限（机场没有机库，默认不限制）。
// 迁移：上一版默认值是"跑道数×6"，那个值不是剧本写的，读档时一并抬到无上限。
func NormalizeAirfieldHangar(state *State) {
	if state == nil {
		return
	}
	for _, airfield := range state.Airfields {
		if airfield == nil {
			continue
		}
		legacyDefault := airfield.Runways * 6
		if airfield.HangarCapacity <= 0 || airfield.HangarCapacity == legacyDefault {
			airfield.HangarCapacity = AirfieldHangarUnlimited
		}
	}
}

// friendlyCarrierHostAt 在指定 hex 找一艘能接收这架机场算子的己方航母（有空余机库、且该机型能上舰）。
// 陆基侦察机/轰炸机和带浮筒的水机不能上舰；舰载型的 F4F、SBD 这些可以从陆地机场补上航母。
func friendlyCarrierHostAt(state State, faction, coord string, aircraft *AirfieldAirUnit, incoming int) *Unit {
	if aircraft == nil || coord == "" || aircraft.AirfieldOnly || unitIsSeaplane(state, aircraft) {
		return nil
	}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Faction != faction || unit.Coord != coord || !IsCarrier(unit) {
			continue
		}
		if unit.Card.HangarCapacity <= 0 || unit.Card.Aircraft+incoming > unit.Card.HangarCapacity {
			continue
		}
		return unit
	}
	return nil
}

// boardGroundAircraftOnCarrier 把飞到航母上空的机场算子转成舰载机算子：停在甲板上等待下降整备。
func boardGroundAircraftOnCarrier(state *State, aircraft *AirfieldAirUnit, carrier *Unit) {
	if state.Aircraft == nil {
		state.Aircraft = map[string]*AirUnit{}
	}
	unit := groundAirUnit(*state, aircraft)
	unit.CarrierID = carrier.ID
	unit.Status = "recovery"
	unit.Launched = false
	unit.TaskID = ""
	unit.TaskKind = ""
	unit.TargetCoord = ""
	unit.Coord = carrier.Coord
	copied := unit
	state.Aircraft[aircraft.ID] = &copied
	delete(state.GroundAir, aircraft.ID)
	carrier.Card.Aircraft++
	carrier.Deck.Operation = "recover"
}

func airfieldAirborne(aircraft *AirfieldAirUnit) bool {
	switch aircraft.Status {
	case "cap", "cover", "recon", "attack_holding", "attack", "transfer":
		return true
	}
	return false
}

// friendlyAirfieldAtCoord 找到落在某个 hex 上的己方机场；转场目的地就是它。
func friendlyAirfieldAtCoord(state State, faction, coord string) *Airfield {
	if coord == "" {
		return nil
	}
	for _, airfield := range state.Airfields {
		if airfield != nil && airfield.Faction == faction && airfield.Coord == coord {
			return airfield
		}
	}
	return nil
}

// AdvanceAirfields 是机场的每回合推进：
// 1) 损管：每回合 d10 ≤ 2 时生命值 +1（不超过最大生命值）；
// 2) 航空作业：机库整备 → 滑行到跑道 → 起飞，每回合只推进一步。
func AdvanceAirfields(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, id := range sortedAirfieldIDs(*state) {
		airfield := state.Airfields[id]
		// 敌方滩头物资还压在这个机场上：生命值按 0 计、跑道全废，清滩之前完全不走修复逻辑。
		// 运输船把货卸在你的机场上，等于机场正在被占领——不修好也不放行作业。
		if enemySuppliesAt(*state, airfield) > 0 {
			if airfield.Strength > 0 {
				airfield.Strength = 0
				events = append(events, EngineEvent{Type: "airfield_occupied", Audience: airfield.Faction, Text: fmt.Sprintf("%s被敌方滩头物资压制，跑道全部瘫痪（清滩前无法修复）", airfield.Name)})
			}
			continue
		}
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
		// 没有目标的攻击编队、没有目的地的转场编队都留在跑道上等，
		// 不升空烧油（转场没目的地会在空中无处可去）。
		if (aircraft.TaskKind == "attack" || aircraft.TaskKind == "transfer") && aircraft.TargetCoord == "" {
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
	// 规则书 679–681 行：多云只压战略轰炸机，降雨 -10%，风暴 -15%。
	switch weatherAt(state.Map, target.Coord) {
	case "cloud":
		if isStrategicGroundBomber(attacker) {
			chance -= 5
		}
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

// airfieldLightAAPool 结算机场对"正在攻击本机场的整队飞机"的轻型防空。
//
// 规则书 735 行：轻防空的目标池是正在攻击本机场的那几架，一副骰子摊在整队上，
// 不是每架攻击机各掷一整副。旧实现按单架结算，莫尔兹比港 20 点轻防空碰上 12 架
// 编队就要掷 240 次，飞机越多防空越强——和打舰路径早就修掉的是同一个毛病。
func airfieldLightAAPool(state *State, attackers []*AirUnit, airfield *Airfield, report *AirCombatReport) []EngineEvent {
	if airfield == nil || len(attackers) == 0 {
		return nil
	}
	events := []EngineEvent{}
	chance := aaHitChance(*state, airfield.Coord, false, airfield.Faction)
	value := airfield.LightAA
	shots := int(value)
	// 小数防空：整数部分逐点判定，小数部分作为追加一次判定的概率
	if probabilityRoll(&state.RNGState) <= (value - float64(shots)) {
		shots++
	}
	for ; shots > 0; shots-- {
		pool := activeAircraft(state, attackers)
		if len(pool) == 0 {
			break
		}
		plane := pool[nextUint64(&state.RNGState)%uint64(len(pool))]
		// 高空投弹的轰炸机 25mm 够不着，只有重型防空能打：骰子选中它们时这一发作废，
		// 骰数本身不变，也不产生混乱判定。
		if ignoresLightAA(plane) {
			continue
		}
		if probabilityRoll(&state.RNGState) < chance {
			report.AAHits++
			outcome, hitEvents := applyAircraftHit(state, plane)
			recordAirHit(report, plane, outcome)
			recordAircraftLoss(report, plane, outcome, true)
			events = append(events, hitEvents...)
		} else {
			confuseRandomAircraft(state, attackers)
		}
	}
	return events
}

// resolveGroundStrike 按规则书顺序结算对机场的空袭：
// CAP 拦截 → 重型防空 → 机场轻防空对整队掷一副骰子 → 按对地攻击值逐点判定（基础 20%）→ 撤离。
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
	restoreDefenders := projectDefendingGroundAir(state, defender, coord, 1)
	defer restoreDefenders()
	groups := airfieldCAPGroups(*state, defender, coord)
	attackerModels, attackerPictures := aircraftModelCards(attackers)
	report := AirCombatReport{
		Coord: coord, Target: string(StrikeTargetAir), TargetName: airfield.Name,
		AttackerFaction: faction, DefenderFaction: defender,
		AttackerModels: attackerModels, AttackerPictures: attackerPictures, AttackerCount: len(attackers),
		Airfield: &AirShipCard{ID: airfield.ID, Name: airfield.Name, Type: string(airfield.Type)},
	}
	defenders := []*AirUnit{}
	for _, group := range groups {
		defenders = append(defenders, group...)
		report.DefenderCount += len(group)
	}
	report.DefenderModels, report.DefenderPictures = aircraftModelCards(defenders)
	events := []EngineEvent{}
	for _, group := range groups {
		events = append(events, resolveDogfight(state, attackers, group, &report)...)
	}
	bombers := []*AirUnit{}
	for _, aircraft := range attackers {
		// 挂弹的战斗机按投弹处理；不挂弹的战斗机是护航，后面按 1 点强度扫射。
		if aircraft.Kind != "fighter" || aircraft.Loadout.Weapon() == WeaponBomb {
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
	// 规则书 735 行：机场轻防空是一副骰子摊在正在攻击本机场的整队飞机上，
	// 在重防空之后、投弹之前掷一次。高空投弹的轰炸机只吃重防空。
	events = append(events, airfieldLightAAPool(state, attackers, airfield, &report)...)
	if report.TargetAllocations == nil {
		report.TargetAllocations = []StrikeAllocation{}
	}
	for _, aircraft := range bombers {
		if !operationalAircraft(state, aircraft) {
			continue
		}
		allocation := StrikeAllocation{Attacker: aircraft.ID, Model: aircraft.Model, ShipID: airfield.ID, ShipName: airfield.Name}
		report.TargetAllocations = append(report.TargetAllocations, allocation)
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
	// 不挂弹的护航战斗机扫射跑道：固定 1 点对地强度，按基础 20% 判定；
	// 命中只摧毁地面飞机，不减机场生命值和岸防。扫射机也要先吃轻防空。
	strafeHits := 0
	for _, aircraft := range attackers {
		if aircraft.Kind != "fighter" || aircraft.Loadout.Weapon() == WeaponBomb {
			continue
		}
		if !operationalAircraft(state, aircraft) {
			continue
		}
		chance := groundHitChance(*state, airfield, groundAircraftView(aircraft), capRemaining)
		report.StrafeAttempts++
		hits := 0
		if probabilityRoll(&state.RNGState) < chance {
			hits = 1
			strafeHits++
			report.StrafeHits++
		}
		events = append(events, EngineEvent{Type: "ground_strafe", Audience: "all", Text: fmt.Sprintf("%s扫射%s：命中率%.1f%%，命中%d次", aircraft.Model, airfield.Name, chance, hits)})
	}
	// 扫射命中同样摧毁机场上的飞机，但不会扣机场生命/岸防。
	report.GroundLoss += destroyGroundAircraft(state, airfield, strafeHits)
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
	report.Summary = fmt.Sprintf("%s对机场%s空袭：防空命中%d，投弹%d次命中%d次（扫射命中%d次），机场生命值%s→%s（跑道%d条），摧毁地面飞机%d，攻击方损失%d",
		coord, airfield.Name, report.AAHits, report.BombAttempts, report.StrikeHits, strafeHits,
		formatStrength(report.AirfieldBefore), formatStrength(report.AirfieldAfter),
		RunwaysAvailable(airfield.Strength, airfield.Runways), report.GroundLoss, report.AttackerLosses)
	report.Turn = state.Turn
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
	return &AirfieldAirUnit{ID: aircraft.ID, Model: aircraft.Model, Kind: aircraft.Kind, Seaplane: aircraft.Seaplane, Damaged: aircraft.Damaged || aircraft.Confusion}
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

// validateGroundCycleOrder 校验机场算子的自动任务循环开关（规则书 191 行）。
// 关循环永远允许：空中、损伤、跑道被打光的时候都能关掉，否则玩家只能干看着它自己再飞一轮。
// 开循环才要求这个算子身上有一个可循环的任务，并且不在检修状态。
func validateGroundCycleOrder(state State, order Order, ids []string) error {
	for _, id := range ids {
		aircraft := state.GroundAir[id]
		if aircraft == nil {
			return fmt.Errorf("航空算子不存在或不属于当前玩家")
		}
		if !order.Cycle {
			continue
		}
		kind := aircraft.TaskKind
		if kind == "" {
			kind = aircraft.CycleKind
		}
		if kind == "" && aircraft.Status == string(MissionCap) {
			kind = string(MissionCap)
		}
		if !cyclicTaskKind(kind) {
			return fmt.Errorf("%s没有可循环的任务（只有CAP、侦察和空中掩护支持自动循环）", aircraft.Model)
		}
		if aircraft.Damaged || aircraft.Maintenance > 0 {
			return fmt.Errorf("%s损伤或检修中，修好之后才能开启自动循环", aircraft.Model)
		}
	}
	return nil
}

// groundOrderNeedsRunway 判断命令是否要占用跑道起飞。
// 无跑道机场仍必须能召回、取消任务和改优先级——这些操作不产生起飞。
func groundOrderNeedsRunway(orderType string) bool {
	switch orderType {
	case "LAUNCH_CAP", "LAUNCH_COVER", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_TRANSFER":
		return true
	}
	return false
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
	if order.Type == "SET_AIR_CYCLE" {
		// 循环开关不参与下面的"同机场、同作业阶段"批量约束：玩家可以一次点掉一整个
		// 任务卡（里面可能有机库里的、也有还在天上值班的），也可以只点一架。
		return validateGroundCycleOrder(state, order, ids)
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
		} else if aircraft.AirfieldID != airfield.ID {
			return fmt.Errorf("%s与同批算子不在同一机场，不能同批下发", aircraft.Model)
		} else if aircraft.Status != status {
			return fmt.Errorf("%s处于%s，同批其他算子处于%s；同一批机场算子必须同机场、同状态，请按状态分批下发", aircraft.Model, groundStatusLabel(aircraft.Status), groundStatusLabel(status))
		}
		if aircraft.Damaged || aircraft.Maintenance > 0 {
			return fmt.Errorf("损伤或检修中的算子必须完成修复后才能分配任务")
		}
		list = append(list, aircraft)
	}
	if airfield == nil {
		return fmt.Errorf("机场不存在")
	}
	// 只有"起飞"类命令才需要跑道：规则书 222 行允许飞机在无跑道机场降落，
	// 召回、取消任务、改优先级这些不该被炸平的跑道一起锁死。
	if groundOrderNeedsRunway(order.Type) && RunwaysAvailable(airfield.Strength, airfield.Runways) == 0 {
		return fmt.Errorf("%s已经没有可用跑道", airfield.Name)
	}
	switch order.Type {
	case "LAUNCH_CAP", "LAUNCH_COVER", "LAUNCH_RECON", "LAUNCH_STRIKE":
		// 规则书 211 行：CAP 只掩护自己所在 hex 及周围 1 hex。
		// 212 行"指派 CAP 去其他友军上空"在游戏里就是空中掩护任务（LAUNCH_COVER），
		// 它会跟着被掩护的舰队走；CAP 不接受远程坐标，免得飞过去守一个不会跟的格子。
		if order.Type == "LAUNCH_CAP" && order.Coord != "" && order.Coord != airfield.Coord {
			return fmt.Errorf("CAP 只掩护本机场上空；要给其他友军上空提供掩护，请使用 LAUNCH_COVER")
		}
		if sameTaskAssignmentGround(list, order) {
			return nil
		}
		if status != "hangar" && status != "service" && status != "deck" && status != "ready" {
			return fmt.Errorf("只有机库或跑道上待命的算子可以分配任务")
		}
		// 是不是"加入已有的攻击编队"。这个判断要放在机型校验之前：编队里已经有轰炸机时，
		// 后补的战斗机就是护航机，不能按"单架战斗机建新编队"处理。
		joiningExisting := order.TaskID != "" && len(groundTaskAircraft(state, order.TaskID)) > 0
		for _, aircraft := range list {
			if aircraft.TaskID != "" {
				return fmt.Errorf("算子已经属于其他任务，请先取消原任务")
			}
			if order.Type == "LAUNCH_CAP" && aircraft.Kind != "fighter" {
				return fmt.Errorf("只有战斗机可以执行CAP")
			}
			if order.Type == "LAUNCH_COVER" {
				// 规则书：空中掩护只能由战斗机执行。目标 hex 可以留空，
				// 表示掩护本机场上空（默认值在 Apply 里落成机场坐标）。
				if aircraft.Kind != "fighter" {
					return fmt.Errorf("空中掩护只能由战斗机执行")
				}
				if order.Coord != "" && !inBounds(state.Map, order.Coord) {
					return fmt.Errorf("掩护目标 hex 无效")
				}
			}
			if order.Type == "LAUNCH_RECON" && (aircraft.Kind == "fighter") {
				return fmt.Errorf("战斗机不能执行扇面搜索")
			}
			// 单架战斗机不能自己组成攻击编队（它没有投弹能力）。但"给已有攻击编队补一架战斗机"
			// 是正常操作——以前这里不看 task_id，补一架 P-40 会被当成单架战斗机建新编队，
			// 报"攻击编队只能由轰炸机和护航战斗机组成"，而编队里明明已经有轰炸机了。
			if order.Type == "LAUNCH_STRIKE" && aircraft.Kind == "fighter" && order.Coord == "" && len(list) == 1 && !joiningExisting {
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
		if order.Type == "LAUNCH_STRIKE" {
			if order.TargetFleetID != "" {
				if err := ValidateStrikeFleetTarget(state, airfield.Faction, order.TargetFleetID, order.Coord); err != nil {
					return err
				}
			} else if order.Coord != "" {
				return ValidateStrikeTarget(state, airfield.Faction, order.Coord)
			}
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
	case "LAUNCH_TRANSFER":
		// 陆基转场：从当前机场挪到另一个己方机场。以前这里根本没有这个分支，
		// 机场算子只能 CAP/侦察/打击，凯恩斯、莫尔兹比这种后方基地的飞机
		// 上不了前线，等于一次性摆设。
		//
		// order.Coord 允许留空：界面上是先拖进"转场"任务栏、再在地图上点选目的地，
		// 和打击任务一样分两步提交；没有目的地时算子留在跑道上不起飞。
		if order.Coord == "" {
			return nil
		}
		dest := friendlyAirfieldAtCoord(state, airfield.Faction, order.Coord)
		// 目的地不是机场时，允许舰载型飞机补上航母：陆基机场起飞（或从别的机场转来）的
		// F4F/SBD 这类机型可以落在己方航母上，这条以前完全没实现，任务建了却永远设不上目标。
		var host *Unit
		if dest == nil {
			host = friendlyCarrierHostAt(state, airfield.Faction, order.Coord, list[0], len(list))
			if host == nil {
				return fmt.Errorf("转场目的地必须是己方机场或还有机库空位的航母")
			}
		}
		if dest != nil && dest.ID == airfield.ID {
			return fmt.Errorf("不能转场到同一个机场")
		}
		if dest != nil && RunwaysAvailable(dest.Strength, dest.Runways) == 0 {
			return fmt.Errorf("%s已经没有可用跑道，无法接收转场", dest.Name)
		}
		if dest != nil && airfield.Type == AirfieldSeaplane && dest.Type != AirfieldSeaplane {
			return fmt.Errorf("水上飞机基地的算子只能转场到另一个水上飞机基地")
		}
		for _, aircraft := range list {
			if dest != nil && !dest.HardensFor(aircraft.Kind, unitIsSeaplane(state, aircraft)) {
				return fmt.Errorf("%s无法起降%s", dest.Name, aircraft.Model)
			}
			if dest == nil && friendlyCarrierHostAt(state, airfield.Faction, order.Coord, aircraft, len(list)) == nil {
				return fmt.Errorf("%s无法在%s的航母上降落（机库已满或机型不能上舰）", aircraft.Model, order.Coord)
			}
			limit := SortieBaseRange(aircraft.BaseRange, LoadoutTransfer)
			if need := Distance(airfield.Coord, order.Coord); need > limit {
				return fmt.Errorf("%s转场航程%d hex（挂副油箱），飞到%s需要%d hex", aircraft.Model, limit, order.Coord, need)
			}
		}
		if dest == nil {
			return nil
		}
		return airfieldHangarFull(state, dest, len(list))
	case "SET_STRIKE_TARGET":
		if len(list) != 1 {
			return fmt.Errorf("只有编队可以设置目标")
		}
		switch list[0].TaskKind {
		case "attack":
			if order.TargetFleetID != "" {
				if err := ValidateStrikeFleetTarget(state, airfield.Faction, order.TargetFleetID, order.Coord); err != nil {
					return err
				}
			} else if err := ValidateStrikeTarget(state, airfield.Faction, order.Coord); err != nil {
				return err
			}
			return validateGroundStrikeLoadoutChange(state, groundTaskAircraft(state, list[0].TaskID), order.TargetFleetID, order.Coord)
		case "cover":
			// 掩护改目标：舰队所在格或机场格都行，只要求是地图内合法 hex。
			if order.Coord == "" || !inBounds(state.Map, order.Coord) {
				return fmt.Errorf("掩护目标 hex 无效")
			}
			return nil
		case "transfer":
			// 转场改目的地：己方机场，或者有空余机库的己方航母（舰载型飞机）。
			dest := friendlyAirfieldAtCoord(state, airfield.Faction, order.Coord)
			members := groundTaskAircraft(state, list[0].TaskID)
			if dest == nil {
				if host := friendlyCarrierHostAt(state, airfield.Faction, order.Coord, list[0], len(members)); host == nil {
					return fmt.Errorf("转场目的地必须是己方机场或还有机库空位的航母")
				}
			}
			if dest != nil && !dest.HardensFor(list[0].Kind, unitIsSeaplane(state, list[0])) {
				return fmt.Errorf("%s无法起降%s", dest.Name, list[0].Model)
			}
			if need, limit := Distance(airfield.Coord, order.Coord), SortieBaseRange(list[0].BaseRange, LoadoutTransfer); need > limit {
				return fmt.Errorf("%s转场航程%d hex，飞到%s需要%d hex", list[0].Model, limit, order.Coord, need)
			}
			if dest == nil {
				return nil
			}
			return airfieldHangarFull(state, dest, len(members))
		}
		return fmt.Errorf("只有攻击编队或转场编队可以设置目标")
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

// groundStatusLabel 把机场算子的内部状态翻成玩家能看懂的话。
// "作业阶段"这个说法容易被误解成机型或任务阶段，实际只是 hangar/service/ready 这类状态。
func groundStatusLabel(status string) string {
	switch status {
	case "hangar":
		return "机库"
	case "service":
		return "整备中"
	case "deck":
		return "跑道上"
	case "ready":
		return "待命"
	default:
		return status
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
	case "LAUNCH_CAP", "LAUNCH_COVER", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_TRANSFER":
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
			// 机场任务同样走发号器：以前直接拿第一架飞机的 ID 当任务号，
			// 同一批飞机第二次出击会和上一次的任务号撞在一起。
			taskID = nextTaskID(state, kind)
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
				if order.TargetFleetID != "" {
					aircraft.TargetFleetID = order.TargetFleetID
					if coord, ok := strikeFleetCoord(*state, aircraft.Faction, order.TargetFleetID); ok {
						aircraft.TargetCoord = coord
					}
				}
			case MissionCover:
				// 留空 = 掩护本机场上空；给了 hex 就守那个 hex（例如护着前出的舰队）。
				aircraft.TargetCoord = order.Coord
				if aircraft.TargetCoord == "" {
					aircraft.TargetCoord = airfield.Coord
				}
				aircraft.CoverUnitID = friendlyUnitIDAt(*state, aircraft.Faction, order.Coord)
				aircraft.Returning = false
			case MissionTransfer:
				aircraft.TargetCoord = order.Coord
				aircraft.Returning = false
			default:
				// CAP 同样是指定目标：不填就守本机场上空。
				target := order.Coord
				if target == "" {
					target = airfield.Coord
				}
				assignGroundCapMission(aircraft, target)
			}
			// 按任务换挂载：对舰优先鱼雷、对地挂炸弹（规则书 43–63 行）。
			applyGroundLoadout(*state, aircraft)
			if aircraft.Status == "hangar" {
				aircraft.Status = "service"
			}
			if aircraft.Status == "ready" && !((kind == MissionStrike || kind == MissionTransfer) && aircraft.TargetCoord == "") {
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
		kind := list[0].TaskKind
		for _, aircraft := range groundTaskAircraft(*state, taskID) {
			if kind == "attack" {
				if aircraft.Returning {
					continue
				}
				setGroundStrikeTarget(state, aircraft, airfield.Faction, order.Coord)
				if order.TargetFleetID != "" {
					aircraft.TargetFleetID = order.TargetFleetID
					if coord, ok := strikeFleetCoord(*state, airfield.Faction, order.TargetFleetID); ok {
						aircraft.TargetCoord = coord
					}
				}
				applyGroundStrikeLoadoutChange(state, aircraft)
				continue
			}
			// 掩护 / 转场改目标。
			aircraft.TargetCoord = order.Coord
			aircraft.Returning = false
			if kind == "cover" {
				aircraft.CoverUnitID = friendlyUnitIDAt(*state, airfield.Faction, order.Coord)
			}
		}
		text := fmt.Sprintf("攻击编队目标设为%s", order.Coord)
		switch kind {
		case "cover":
			text = fmt.Sprintf("空中掩护目标设为%s", order.Coord)
		case "transfer":
			text = fmt.Sprintf("转场目的地设为%s", order.Coord)
		}
		return []EngineEvent{{Type: "strike_target_set", Audience: airfield.Faction, Text: text}}
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
			// 规则书 190 行：主动撤回的算子同时取消自动循环。
			disarmGroundTaskCycle(aircraft)
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
			// 规则书 190 行：主动回收的飞机会取消循环任务。
			disarmGroundTaskCycle(aircraft)
			aircraft.TaskID = ""
			aircraft.TaskKind = ""
		}
		return []EngineEvent{{Type: "air_recovered", Audience: airfield.Faction, Text: "机场CAP算子开始返航"}}
	case "SET_AIR_CYCLE":
		// 规则书 191 行：点算子开关自动任务循环，机场算子和舰载机同一套语义。
		toggled := 0
		for _, aircraft := range list {
			if order.Cycle {
				kind := aircraft.TaskKind
				if kind == "" {
					kind = aircraft.CycleKind
				}
				if kind == "" && aircraft.Status == string(MissionCap) {
					kind = string(MissionCap)
				}
				enableGroundTaskCycle(aircraft, kind)
			} else {
				disarmGroundTaskCycle(aircraft)
			}
			toggled++
		}
		label := "已开启"
		if !order.Cycle {
			label = "已取消"
		}
		return []EngineEvent{{Type: "air_cycle_toggled", Audience: airfield.Faction, Text: fmt.Sprintf("%d个机场算子的自动任务循环%s", toggled, label)}}
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
	// 新的出击重新计"回不回得来"：上一轮留下的警告标记必须清掉。
	aircraft.NightWarned = false
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
			// 检修与损伤修复：以前只有舰载机会递减 Maintenance、掷修复判定，
			// 机场算子一旦带着待修标记落地就永远不能再分配任务（表现为"飞机明明停在
			// 机库里，却一直提示检修中"）。这里按同一套节奏处理。
			if aircraft.Maintenance > 0 {
				aircraft.Maintenance--
			}
			if aircraft.Damaged && aircraft.Status == "hangar" {
				if probabilityRoll(&state.RNGState) < aircraftRepairChance(*state, aircraft.Faction) {
					aircraft.Damaged = false
					events = append(events, EngineEvent{Type: "aircraft_repaired", Audience: aircraft.Faction, Text: aircraft.Model + "损伤修复完成，可以重新分配任务"})
				}
			}
			// 任务循环：落回机库的 CAP／空中掩护／侦察按原参数重新整备再出动（规则书 188 行）。
			events = append(events, restartCycledGroundTask(state, aircraft)...)
			continue
		}
		switch aircraft.Status {
		case "cap":
			// CAP 和空中掩护是同一套机制：飞向指定目标（舰队或机场）上空，
			// 覆盖目标周围 1 格，敌机进入就爆发空战。以前 CAP 只是原地烧油，
			// 指定别处舰队上空无异于设了不用。
			if aircraft.Returning {
				events = append(events, advanceGroundAirReturn(state, aircraft)...)
			} else if !processedAttack[aircraft.TaskID] {
				events = append(events, advanceGroundPatrol(state, aircraft, "cap")...)
				processedAttack[aircraft.TaskID] = true
			}
		case "recon":
			events = append(events, advanceGroundRecon(state, aircraft)...)
		case "attack_holding":
			// 被召回的等待编队要立刻往回飞：以前这里不看 Returning，召回后还在原地持
			// 续扣油，直到燃油耗尽——和舰载机那边修过的"卡在空中"是同一个形状。
			if aircraft.Returning {
				events = append(events, advanceGroundAirReturn(state, aircraft)...)
				continue
			}
			// 机场编队同样要等还没返航的算子全部升空、目标已定才离场；
			// 之前这里只扣航程不做状态切换，导致对地编队永远停在机场上空。
			if groupReady := groundTaskReadyToDepart(*state, aircraft.TaskID); groupReady {
				for _, member := range groundTaskAircraft(*state, aircraft.TaskID) {
					// 已经掉头返航的成员不跟着改状态：它们走 advanceGroundAirReturn 自己回家，
					// 编队剩下的人继续出击（"燃料不够的自己拆编队返航"）。
					if member.Returning {
						continue
					}
					member.Status = "attack"
				}
				events = append(events, EngineEvent{Type: "attack_departed", Audience: aircraft.Faction, Text: fmt.Sprintf("%s攻击编队全部升空，离开机场", aircraft.Model)})
			} else if airfieldFor(state, aircraft.AirfieldID) != nil && aircraft.Range() <= airfieldRtbDistance(*state, aircraft) {
				// 等编队的时候也要看油：剩的航程只够回家就自己退出，别在跑道上空烧干。
				aircraft.Returning = true
				events = append(events, EngineEvent{Type: "attack_rtb", Audience: aircraft.Faction, Text: fmt.Sprintf("%s剩余航程%d不足以往返，脱离攻击编队返航", aircraft.Model, aircraft.Range())})
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
		case "cover":
			// 返航的掩护机必须逐架处理：advanceGroundAirReturn 只飞传进去的那一架，
			// 以前它被 processedAttack 整队挡在门外，一个 3 机掩护队每回合只有排第一的那架
			// 往回飞，另外两架原地等着——入夜前根本回不了家（表现就是"掩护不自动返航"）。
			// 还在值班的才需要整队一次（advanceGroundPatrol 自己会遍历成员）。
			if aircraft.Returning {
				events = append(events, advanceGroundAirReturn(state, aircraft)...)
			} else if !processedAttack[aircraft.TaskID] {
				events = append(events, advanceGroundPatrol(state, aircraft, "cover")...)
				processedAttack[aircraft.TaskID] = true
			}
		case "transfer":
			// 转场同理：逐架往各自机场飞。以前这里不看 Returning，一直走 advanceGroundTransfer，
			// 目的地被清空后每回合都判一次"目的地不再是我方机场"，目标在原点和其他机场之间来回跳。
			if aircraft.Returning {
				events = append(events, advanceGroundAirReturn(state, aircraft)...)
			} else if !processedAttack[aircraft.TaskID] {
				events = append(events, advanceGroundTransfer(state, aircraft)...)
				processedAttack[aircraft.TaskID] = true
			}
		}
	}
	// 已经掉头返航的机场算子从打击编队里拆出去单独成组（与舰载机同一套逻辑）。
	events = append(events, splitGroundReturningMembers(state)...)
	return events
}

// groundAirSpeed 返回机场算子每回合能飞几格。
// 规则书 197 行：水上飞机和双翼机每回合移动 1 hex，其他飞机 2 hex。
// 机场算子上没有独立的水上飞机标记——水上飞机只可能停在水上基地，按基地类型判断。
func groundAirSpeed(state State, aircraft *AirfieldAirUnit) int {
	if aircraft == nil {
		return 2
	}
	// 规则书 200 行：水上飞机和双翼机每回合移动 1 hex，其他飞机 2 hex。
	// 按机种本身判断，而不是只看基地类型——水机停在陆上机场时同样是 1 hex。
	if unitIsSeaplane(state, aircraft) {
		return 1
	}
	return 2
}

// groundStrikeTargetPresent 判断目标 hex 上现在还有没有可打的目标（敌方舰队或机场）。
func groundStrikeTargetPresent(state State, faction, coord string) bool {
	if coord == "" {
		return false
	}
	for _, unit := range state.Units {
		if unit != nil && unit.Faction != faction && unit.Coord == coord {
			return true
		}
	}
	for _, airfield := range state.Airfields {
		if airfield != nil && airfield.Faction != faction && airfield.Coord == coord {
			return true
		}
	}
	return false
}

// groundStrikeSearch 目标 hex 上已经无敌舰时继续搜索：沿当前航向外扩一格，
// 剩余航程只够回家就停止搜索返航。
// 以前机场编队到了空 hex 就原地不动——既不搜索也不返航，油耗停在原地，
// 一队陆攻会在空中僵住几十回合。
func groundStrikeSearch(state *State, aircraft *AirfieldAirUnit, budget int) []EngineEvent {
	// budget 是本回合还剩几步可走：赶路已经用完整回合的 2 hex 时，这回合只做判定不再移动。
	if budget <= 0 {
		return nil
	}
	members := groundTaskAircraft(*state, aircraft.TaskID)
	events := []EngineEvent{}
	moved := false
	for _, member := range members {
		if member.Returning {
			continue
		}
		if member.Heading == "" {
			if heading := headingBetween(member.OriginCoord, member.TargetCoord); heading != "" {
				member.Heading = heading
			} else {
				member.Heading = "E"
			}
		}
		if member.Range() <= airfieldRtbDistance(*state, member)+2 {
			member.Returning = true
			events = append(events, EngineEvent{Type: "attack_rtb", Audience: member.Faction, Text: fmt.Sprintf("%s剩余航程%d，只够返航，停止搜索", member.Model, member.Range())})
			continue
		}
		next := nextReconCoord(state.Map, member.Coord, member.Heading)
		if next == "" {
			member.Returning = true
			continue
		}
		member.Coord = next
		// 把目标点跟着搜索位置一起前移：下一回合仍然走"已抵达目标格"的分支，
		// 继续判定有没有敌舰，而不是因为目标在身后就再也推进不了。
		member.TargetCoord = next
		member.Spend(1)
		moved = true
	}
	if len(members) > 0 {
		aircraft.TargetCoord = members[0].TargetCoord
	}
	if moved {
		events = append(events, EngineEvent{Type: "attack_searching_move", Audience: aircraft.Faction, Text: fmt.Sprintf("%s编队在%s没有发现敌舰，继续向外搜索", aircraft.Model, aircraft.Coord)})
	}
	return events
}

// advanceGroundAttack 让机场打击编队朝目标推进：非水上飞机每回合 2 格（规则书 197 行），
// 抵达目标 hex 后调用空袭结算；燃油只够回家时提前掉头。
func advanceGroundAttack(state *State, aircraft *AirfieldAirUnit) []EngineEvent {
	members := groundTaskAircraft(*state, aircraft.TaskID)
	if aircraft.TargetCoord == "" {
		for _, member := range members {
			member.Spend(1)
		}
		return nil
	}
	// 编队认的是"那支舰队"，不是起飞时那一格 hex：出航途中每一回合都按最新情报
	// 校正一次目标，再朝它飞。以前只在抵达之后才校正，于是整队对着旧坐标飞一整段、
	// 扑空、再掉头去追——白扔航程，还经常追不上。
	events := retargetGroundStrikeToContact(state, aircraft, members)
	// 本回合已经飞了几格：接敌失败后的外扩搜索同样受"每回合不超过 2 hex"限制。
	groundStepsFlown := 0
	for _, member := range members {
		// 只推进已经离场的成员：混在编队里的 attack_holding（后加入还没起飞/还在等编队）
		// 以前会被一起拖着飞，最后停在目标上空却还是"等待编队"，既不打也回不去。
		if member.Returning || member.Status != "attack" {
			continue
		}
		// 规则书 197 行：水上飞机和双翼机每回合移动 1 hex，其他飞机 2 hex。
		// 这里以前固定只走 1 格，拉包尔到莫尔兹比（19 hex）要 19 回合单程，
		// 于是任何一次陆攻都在入夜召回判定里"走完任务再加返航要 38 回合"——
		// 表现就是刚起飞就掉头，永远打不到莫尔兹比。
		groundStepsFlown = 0
		for step := 0; step < groundAirSpeed(*state, member) && member.Coord != member.TargetCoord; step++ {
			next := nextGroundFlightCoord(*state, member, member.Coord, member.TargetCoord)
			if next == "" {
				break
			}
			// 剩余航程 <= 回到机场所需距离时停止前进并返航（规则书 618 行），用路径距离而非直线距离。
			probe := *member
			probe.Coord = next
			probe.SetRange(member.Range() - 1)
			if airfieldFor(state, member.AirfieldID) != nil && probe.Range() <= airfieldRtbDistance(*state, &probe) {
				member.Returning = true
				break
			}
			member.Coord = next
			member.Spend(1)
			groundStepsFlown++
		}
	}
	if aircraft.Coord != aircraft.TargetCoord {
		// 还没到目标：把本回合已经攒下的战报（改向、接敌之类）带出去，不能吞掉。
		return events
	}
	// 空袭结算（对舰/对地）都按 state.Aircraft 里的舰载机算子实现。
	// 机场编队住在 state.GroundAir，直接调用会找不到攻击者，所以先投影进去、
	// 结算完再同步回机场算子（航程、损伤、投弹标记都要回写）。
	events = append(events, retargetGroundStrikeToContact(state, aircraft, members)...)
	// 目标格上已经没有敌舰：不能原地干等（既不搜索也不返航），
	// 按舰载编队的规则继续外扩搜索，燃油不够就掉头。
	if aircraft.Coord == aircraft.TargetCoord && !groundStrikeTargetPresent(*state, aircraft.Faction, aircraft.TargetCoord) {
		budget := groundAirSpeed(*state, aircraft) - groundStepsFlown
		if budget < 0 {
			budget = 0
		}
		return append(events, groundStrikeSearch(state, aircraft, budget)...)
	}
	restore := projectGroundTaskIntoAircraft(state, aircraft.TaskID)
	if aircraft.TargetFleetID == "" && strikeAirfieldAt(*state, aircraft.Faction, aircraft.TargetCoord) != nil {
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
	return []EngineEvent{{Type: "strike_retarget", Audience: aircraft.Faction, Text: fmt.Sprintf("打击编队按最新情报把目标从%s改到%s", previous, coord)}}
}

// projectDefendingGroundAir 把落在 coord 附近的机场 CAP / 空中掩护任务临时投影进
// state.Aircraft，让 strikeCAPGroups、airfieldCAPGroups、capGroupsWithin 这些
// 只扫 state.Aircraft 的防守判定能看到它们。返回回写函数，结算完必须调用。
//
// 之前陆基战斗机升空后在 GroundAir 里执勤，三处防守判定全都看不到，
// 机场挨炸时不会有一架起飞迎击——CAP/掩护任务对陆基飞机等于没做。
func projectDefendingGroundAir(state *State, faction, coord string, distance int) func() {
	if state == nil || coord == "" {
		return func() {}
	}
	tasks := map[string]bool{}
	for _, aircraft := range state.GroundAir {
		if aircraft == nil || aircraft.Faction != faction || aircraft.Returning || aircraft.TaskID == "" {
			continue
		}
		if aircraft.Status != "cap" && aircraft.Status != "cover" {
			continue
		}
		// 已经在投影里（同回合多层结算）：不重复投影，否则回写会互相覆盖。
		if state.Aircraft[aircraft.ID] != nil {
			continue
		}
		if Distance(aircraft.Coord, coord) > distance {
			continue
		}
		tasks[aircraft.TaskID] = true
	}
	if len(tasks) == 0 {
		return func() {}
	}
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	restores := make([]func(), 0, len(ids))
	for _, id := range ids {
		restores = append(restores, projectGroundTaskIntoAircraft(state, id))
	}
	return func() {
		for i := len(restores) - 1; i >= 0; i-- {
			restores[i]()
		}
	}
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
			// 被击落：applyAircraftHit 会把它从 state.Aircraft 里删掉。
			// 以前这里只回写损伤/返航/航程，被打成"摧毁"的机场算子会原地复活，
			// 陆基 CAP 和轰炸机于是永远打不死。
			if state.Aircraft[unit.ID] == nil {
				delete(state.GroundAir, unit.ID)
				continue
			}
			original.Damaged = original.Damaged || unit.Damaged
			original.Returning = unit.Returning
			original.Strength = unit.Strength
			// 空战打出"撤退"会进入检修，这条也要带回机场算子，否则落地就能再战。
			original.Maintenance = unit.Maintenance
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
	// 落地会清空任务参数：循环必须在清空之前把航向/扇面/掩护点抄进 Cycle*（规则书 188 行）。
	rememberGroundCycleParams(aircraft, aircraft.TaskKind)
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
	// 夜间降落：机场每个算子额外 20% 摔毁（规则书 535 行）。
	// 以前只有舰载机的 70% 判了，陆基机夜间落下等于白捡——同一个夜里两套待遇。
	if crashed, crashEvents := nightGroundLandingCrash(state, aircraft); crashed {
		return append(events, crashEvents...)
	}
	return append(events, EngineEvent{Type: "air_landed", Audience: airfield.Faction, Text: fmt.Sprintf("%s降落在%s", aircraft.Model, airfield.Name)})
}

// advanceGroundPatrol 是机场版 CAP / 空中掩护：飞到指定目标 hex 上空（舰队所在格或机场格），
// 覆盖目标周围 1 格；敌机进入这个范围就由 strikeCAPGroups 拉进空战（前提是这些算子被投影进
// state.Aircraft，见 projectDefendingGroundAir）。燃料只够回家就自动撤出，不用手动召回。
func advanceGroundPatrol(state *State, aircraft *AirfieldAirUnit, kind string) []EngineEvent {
	members := groundTaskAircraft(*state, aircraft.TaskID)
	if len(members) == 0 {
		return nil
	}
	home := airfieldFor(state, aircraft.AirfieldID)
	events := []EngineEvent{}
	for _, member := range members {
		if member.Returning {
			continue
		}
		// 空中掩护跟着被掩护的舰队走：认领目标 hex 上的一艘友军，
		// 舰队移动后把掩护点更新到它当前位置。
		if kind == "cover" && member.CoverUnitID != "" {
			if guarded := state.Units[member.CoverUnitID]; guarded != nil && guarded.Faction == member.Faction {
				member.TargetCoord = guarded.Coord
			} else {
				member.CoverUnitID = ""
			}
		}
		// 回家只剩一格油时就撤：盘旋每回合都要烧 1 点。
		homeCoord := member.OriginCoord
		if home != nil && home.Faction == member.Faction {
			homeCoord = home.Coord
		}
		if member.TargetCoord == "" {
			member.TargetCoord = homeCoord
		}
		if member.Range() <= Distance(member.Coord, homeCoord)+1 {
			member.Returning = true
			label := "空中掩护"
			if kind == "cap" {
				label = "CAP"
			}
			events = append(events, EngineEvent{Type: "cover_rtb", Audience: member.Faction, Text: fmt.Sprintf("%s燃料只够返航，撤出%s的%s", member.Model, member.TargetCoord, label)})
			continue
		}
		if member.Coord != member.TargetCoord {
			for steps := 0; steps < 2 && member.Coord != member.TargetCoord; steps++ {
				next := nextGroundFlightCoord(*state, member, member.Coord, member.TargetCoord)
				if next == "" {
					break
				}
				member.Coord = next
				member.Spend(1)
			}
			if member.Coord == member.TargetCoord {
				label := "空中掩护"
				if kind == "cap" {
					label = "CAP"
				}
				events = append(events, EngineEvent{Type: "cover_on_station", Audience: member.Faction, Text: fmt.Sprintf("%s抵达%s上空，开始执行%s，覆盖周围1格", member.Model, member.TargetCoord, label)})
			}
			continue
		}
		// 在掩护点上空值班：每回合消耗 1 点航程。
		member.Spend(1)
	}
	return events
}

// advanceGroundTransfer 让转场编队飞向目标机场：每回合 1 格（规则书 209 行），全部抵达后落到新基地。
//
// 目的地中途被敌方打掉（机场被占）就返航原基地，而不是凭空消失。
// 落地沿用 landGroundAircraft，所以无跑道迫降、损伤这些判定和正常回收一致。
func advanceGroundTransfer(state *State, aircraft *AirfieldAirUnit) []EngineEvent {
	members := groundTaskAircraft(*state, aircraft.TaskID)
	if len(members) == 0 {
		return nil
	}
	dest := friendlyAirfieldAtCoord(*state, aircraft.Faction, aircraft.TargetCoord)
	// 目的地也可以是己方航母：陆基机场的舰载型飞机补上航母就是这条路径。
	var host *Unit
	if dest == nil {
		host = friendlyCarrierHostAt(*state, aircraft.Faction, aircraft.TargetCoord, aircraft, 1)
		if host == nil {
			for _, member := range members {
				member.Returning = true
				member.TargetCoord = member.OriginCoord
			}
			return []EngineEvent{{Type: "transfer_aborted", Audience: aircraft.Faction, Text: fmt.Sprintf("%s的目的地已不再是我方机场或航母，转场中止返航", aircraft.Model)}}
		}
	}
	destCoord := aircraft.TargetCoord
	if dest != nil {
		destCoord = dest.Coord
	}
	destName := "航母"
	if dest != nil {
		destName = dest.Name
	} else if host != nil {
		destName = host.Name
	}
	events := []EngineEvent{}
	moved := false
	arrived := 0
	for _, member := range members {
		for steps := 0; steps < 1 && member.Coord != destCoord; steps++ {
			next := nextGroundFlightCoord(*state, member, member.Coord, destCoord)
			if next == "" {
				break
			}
			member.Coord = next
			member.Spend(1)
			moved = true
		}
		if member.Coord == destCoord {
			arrived++
		}
	}
	if arrived < len(members) {
		if moved {
			events = append(events, EngineEvent{Type: "transfer_enroute", Audience: aircraft.Faction, Text: fmt.Sprintf("%s转场编队飞往%s，当前在%s", aircraft.Model, destName, aircraft.Coord)})
		}
		return events
	}
	if host != nil {
		// 上舰：逐架转成舰载机算子停在甲板回收位，机库满的留给下一回合。
		for _, member := range members {
			if host.Card.HangarCapacity > 0 && host.Card.Aircraft >= host.Card.HangarCapacity {
				member.Returning = false
				continue
			}
			boardGroundAircraftOnCarrier(state, member, host)
			events = append(events, EngineEvent{Type: "transfer_arrived", Audience: aircraft.Faction, Text: fmt.Sprintf("%s转场抵达%s，加入舰载机队", aircraft.Model, host.Name)})
		}
		return events
	}
	for _, member := range members {
		events = append(events, landGroundAircraft(state, member, dest)...)
		if state.GroundAir[member.ID] == nil {
			continue
		}
		member.AirfieldID = dest.ID
		member.Coord = dest.Coord
		member.OriginCoord = dest.Coord
		member.TargetCoord = ""
		member.Returning = false
	}
	events = append(events, EngineEvent{Type: "transfer_arrived", Audience: aircraft.Faction, Text: fmt.Sprintf("%s转场抵达%s", aircraft.Model, dest.Name)})
	return events
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
	origin := aircraft.OriginCoord
	events := []EngineEvent{}
	// 每步扫「当前圈 + 下一圈」；同一回合内每圈只判一次，否则两格速度的飞机会把
	// 中间那圈再掷一遍骰子，等于白送一次发现判定。
	scannedRings := map[int]bool{}
	// 搜索航速同样按机种走：飞行艇／水侦每回合 1 hex，其余 2 hex（规则书 200 行）。
	// 以前这里写死 2，H6K 实际比规则快一倍。
	for steps := 0; steps < groundAirSpeed(*state, aircraft); steps++ {
		if aircraft.Returning {
			if aircraft.Coord == airfield.Coord {
				return append(events, landGroundAircraft(state, aircraft, airfield)...)
			}
		} else if aircraft.ReconDistance >= aircraft.ReconRadius || aircraft.Range() <= Distance(aircraft.Coord, airfield.Coord) {
			aircraft.Returning = true
		}
		forward := ""
		if aircraft.Returning {
			forward = nextGroundFlightCoord(*state, aircraft, aircraft.Coord, airfield.Coord)
		} else {
			forward = nextReconCoord(state.Map, aircraft.Coord, aircraft.Heading)
		}
		// 岸基巡逻机飞的是搜索扇面，不是航迹上的一条窄带：每步扫掉"当前距离圈 +
		// 下一距离圈"里落在扇面内的整段弧，和舰载侦察同一套几何。
		// 以前这里只判"本格 + 正前方一格"，H6K 从敌舰队旁边两格飞过去也发现不了。
		ring := Distance(origin, aircraft.Coord)
		coords := []string{}
		for _, r := range reconRingsToScan(ring, aircraft.ReconRadius, scannedRings) {
			coords = append(coords, ReconRingHexes(state.Map, origin, aircraft.Heading, aircraft.SectorWidth, r)...)
		}
		events = append(events, searchGroundReconStep(state, aircraft, coords, ring)...)
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
func searchGroundReconStep(state *State, aircraft *AirfieldAirUnit, coords []string, ring int) []EngineEvent {
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
	// 扇面全展开后一次可能覆盖十几格，战报只列前 24 格，避免刷屏。
	listed := coords
	if len(listed) > 24 {
		listed = listed[:24]
	}
	detail := strings.Join(listed, "、")
	if len(coords) > len(listed) {
		detail += fmt.Sprintf("等%d格", len(coords))
	}
	events := []EngineEvent{{Type: "recon_step_searched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s扫描第%d圈%s", aircraft.Model, ring, detail)}}
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
		// 与舰载侦察一致：每 hex -1%（规则书原值 -2%，实测有效半径太短）。
		// 天气不在这里扣点，统一放到最后乘系数。
		chance -= float64(Distance(aircraft.OriginCoord, members[0].Coord))
	}
	// 与舰载侦察一致：水上飞机最终发现率 ×2（飞得慢，但驻空久、效率高）。
	if unitIsSeaplane(state, aircraft) {
		chance *= 2
	}
	if len(members) > 0 {
		// 与舰载机一致：天气只乘系数（多云 ×0.5、降雨 ×0.35、风暴 ×0.2），不再按档扣点。
		chance *= weatherFactor(weatherAt(state.Map, members[0].Coord))
	}
	// 与舰载机同一顺序：宽扇面的 ÷2 放在最后，120 度永远正好是 60 度的一半。
	if aircraft.SectorWidth == 120 {
		chance /= 2
	}
	if chance < 0 {
		return 0
	}
	if chance > 100 {
		chance = 100
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
				// "这波天黑前回不回得来"：服务器不再替打击编队强制掉头，
				// 这条折算就是玩家唯一的护栏，前端据此显示"这波回不来"。
				if need := groundTurnsToLanded(state, member); need > task.ReturnTurns {
					task.ReturnTurns = need
				}
			}
		}
		if state.TurnsPerDay > 0 {
			task.TurnsToDark = turnsUntilNight(state)
			task.NightMargin = task.TurnsToDark - task.ReturnTurns
			task.NightSafe = task.NightMargin > 0
		} else {
			task.TurnsToDark = -1
			task.NightMargin = 0
			task.NightSafe = true
		}
		task.Phase = "servicing"
		if task.AirborneCount > 0 {
			task.Phase = "active"
		}
		// 起飞顶点：任务卡要写"目标 / 起飞位置"。机场编队的起飞顶点就是机场坐标，
		// 但转场/循环回来的算子 OriginCoord 可能是旧值，所以优先用当前机场。
		if airfield := airfieldFor(&state, leader.AirfieldID); airfield != nil {
			task.OriginCoord = airfield.Coord
		} else if leader.OriginCoord != "" {
			task.OriginCoord = leader.OriginCoord
		}
		if leader.TaskKind == "attack" {
			task.TargetCoord = leader.TargetCoord
			task.Disperse = leader.Disperse
			task.CurrentCoord = leader.Coord
			task.Returning = leader.Returning
			task.Name = "攻击编队 · " + leader.Model
			if task.TargetCoord != "" {
				if home := airfieldFor(&state, leader.AirfieldID); home != nil {
					toTarget, toLanded := EstimateGroundStrike(state, members, home)
					fillStrikeETA(state, &task, toTarget, toLanded)
				}
			}
		} else if leader.TaskKind == "cap" {
			// CAP 是"指定空域"：位置必须用飞机的实际坐标（它可能正飞往某支舰队上空），
			// 以前写死机场坐标，地图上的 CAP 牌永远挂在机场上，看不出它到底去哪儿值班。
			task.CurrentCoord = leader.Coord
			if task.CurrentCoord == "" {
				if airfield := airfieldFor(&state, leader.AirfieldID); airfield != nil {
					task.CurrentCoord = airfield.Coord
				}
			}
			task.TargetCoord = leader.TargetCoord
			task.Name = "CAP · " + leader.Model
		} else if leader.TaskKind == "cover" {
			// 空中掩护同样是"指定一个目标"，任务卡要带目标 hex 才能在地图上点选/改目标。
			task.TargetCoord = leader.TargetCoord
			task.CurrentCoord = leader.Coord
			task.Name = "空中掩护 · " + leader.Model
		} else if leader.TaskKind == "transfer" {
			// 转场任务必须带上目的地，否则界面只会显示"目标未设置"，
			// 玩家没法确认这队飞机到底要去哪儿（也点不了选目的地）。
			task.TargetCoord = leader.TargetCoord
			task.CurrentCoord = leader.Coord
			task.Name = "转场 · " + leader.Model
		} else {
			task.Name = "扇面侦察 · " + leader.Model
			task.CurrentCoord = leader.Coord
			task.Heading = leader.Heading
			task.SectorWidth = leader.SectorWidth
			task.ReconRadius = leader.ReconRadius
			// 机场侦察也要给出扇面几何：以前只给航向和宽度，地图上没有黄色扇面，
			// 玩家根本不知道这架飞机到底在扫哪一片，只能看着它飞出去又飞回来。
			task.ReconSectors = []ReconSector{}
			for _, member := range members {
				task.ReconSectors = append(task.ReconSectors, projectGroundReconSector(state, member))
			}
			if len(task.ReconSectors) > 0 {
				// 扇面顶点是"这次出海"的起飞点（转场过来之后就是新机场），以它为准。
				task.OriginCoord = task.ReconSectors[0].OriginCoord
				task.SectorHexes = task.ReconSectors[0].SectorHexes
				task.CurrentRingHexes = task.ReconSectors[0].CurrentRingHexes
			}
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

// projectGroundReconSector 是机场侦察的扇面投影：顶点是机场，航向/宽度/半径与判定完全一致。
func projectGroundReconSector(state State, aircraft *AirfieldAirUnit) ReconSector {
	origin := aircraft.OriginCoord
	if airfield := airfieldFor(&state, aircraft.AirfieldID); airfield != nil {
		origin = airfield.Coord
	}
	current := aircraft.Coord
	if aircraft.Status != "recon" {
		current = origin
	}
	sector := ReconSector{
		AircraftID: aircraft.ID, Status: aircraft.Status, OriginCoord: origin, CurrentCoord: current,
		Heading: aircraft.Heading, SectorWidth: aircraft.SectorWidth, ReconRadius: aircraft.ReconRadius,
		ReconDistance: aircraft.ReconDistance, Returning: aircraft.Returning, TrackingFleetID: aircraft.TrackingFleetID,
	}
	if aircraft.Status == "recon" {
		sector.SectorHexes = ReconScanBandHexes(state.Map, origin, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius)
		sector.CurrentRingHexes = ReconScannedRingHexes(state.Map, origin, current, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius)
		if current != "" {
			sector.DistanceReference = BuildDistanceReference(state.Map, current, 8)
		}
	}
	return sector
}

// groundAirUnit 把机场算子投影成前端通用的 AirUnit，地图与任务栏因此可以复用同一套渲染。
func groundAirUnit(state State, aircraft *AirfieldAirUnit) AirUnit {
	return AirUnit{
		ID: aircraft.ID, DefinitionID: aircraft.DefinitionID, CarrierID: aircraft.AirfieldID, Faction: aircraft.Faction, Model: aircraft.Model, Kind: aircraft.Kind, Picture: aircraft.Picture, PictureMode: aircraft.PictureMode,
		Status: aircraft.Status, Strength: aircraft.Strength, MaxRange: aircraft.MaxRange, RemainingRange: aircraft.Range(),
		AirCombat: aircraft.AirCombat, AntiShip: aircraft.AntiShip, AntiGround: aircraft.AntiGround,
		// 陆基机型标记必须带出去：前端靠它区分"航母舰载机"和"机场起降的陆基机"，
		// 漏掉这面旗子，陆攻的航程块会被画成舰载机的蓝底样式，看着像从航母上起飞的。
		AirfieldOnly: aircraft.AirfieldOnly, TorpedoCapable: aircraft.TorpedoCapable, HighAltitudeBomber: aircraft.HighAltitudeBomber,
		TaskID: aircraft.TaskID, TaskKind: aircraft.TaskKind, Priority: aircraft.Priority,
		Coord: aircraft.Coord, OriginCoord: aircraft.OriginCoord, TargetCoord: aircraft.TargetCoord, Heading: aircraft.Heading,
		SectorWidth: aircraft.SectorWidth, ReconRadius: aircraft.ReconRadius, ReconDistance: aircraft.ReconDistance,
		ReconRadiusLimit: aircraft.ReconRadiusLimit, Returning: aircraft.Returning, TrackingFleetID: aircraft.TrackingFleetID,
		Confusion: aircraft.Damaged, Damaged: aircraft.Damaged, Maintenance: aircraft.Maintenance,
		// 挂载与航程账面也要投影：机库里的陆基机同样能换挂载，
		// 少了这几个字段前端只能显示一个空的下拉框和"未配置"。
		BaseRange: aircraft.BaseRange, Loadout: aircraft.Loadout, LoadoutManual: aircraft.LoadoutManual,
		TankDropped: aircraft.TankDropped, Bombed: aircraft.Bombed,
		// 循环开关也要投影出去：机场算子的任务卡和舰载机共用一套渲染，
		// 少了 on_cycle 这里就永远画不出蓝色循环角标，玩家也点不到那盏灯。
		OnCycle: aircraft.OnCycle, CycleKind: aircraft.CycleKind, CycleTarget: aircraft.CycleTarget,
		CycleOptOut: aircraft.CycleOptOut,
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
	restoreDefenders := projectDefendingGroundAir(state, defender, coord, 2)
	defer restoreDefenders()
	groups := capGroupsWithin(*state, defender, coord, 2)
	if len(groups) == 0 {
		return nil
	}
	report := AirCombatReport{
		ID:               fmt.Sprintf("air-intercept-%x", nextUint64(&state.RNGState)),
		Coord:            coord,
		Target:           "intercept",
		TargetName:       "雷达引导拦截",
		AttackerFaction:  faction,
		DefenderFaction:  defender,
		AttackerModels:   aircraftModels(attackers),
		AttackerPictures: pictureCardsFor(attackers),
		AttackerCount:    len(attackers),
	}
	defenders := []*AirUnit{}
	for _, group := range groups {
		defenders = append(defenders, group...)
		report.DefenderCount += len(group)
	}
	report.DefenderModels, report.DefenderPictures = aircraftModelCards(defenders)
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
	report.Turn = state.Turn
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
// 水上飞机基地不算岸轰目标：它们锚在海上，没有跑道和固定设施可炸，
// 舰炮打过去只是往水里扔铁（规则上"炮击对水上飞机基地无效"）。
func shoreTargetAt(state State, faction, coord string) *Airfield {
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield.Faction == faction || airfield.Coord != coord {
			continue
		}
		if airfield.Type == AirfieldSeaplane {
			continue
		}
		return airfield
	}
	return nil
}

// canBombard 返回本舰队本回合可以岸轰的目标机场。
// 条件：有主炮、在目标 hex 停留满 1 回合、同 hex 没有敌方舰队、舰队混乱为 0。
// 目标通常是敌方机场；自家机场被敌人卸下滩头物资时也允许"清滩"——只打物资，不打自己的设施。
func canBombard(state State, unit *Unit) *Airfield {
	if unit == nil || unit.Card.Gunnery <= 0 || unitSmoking(unit) {
		return nil
	}
	// 舰队必须真的能待在这个 hex 上：旧存档里可能有航母编队停在机场格/浅滩上，
	// 那种"非法驻留"不能凭坐标蹭到岸轰。
	if !fleetCanEnter(state, unit.Coord, FleetHasCarrier(state, FleetKey(unit))) {
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
	if enemyAirfield := shoreTargetAt(state, unit.Faction, unit.Coord); enemyAirfield != nil {
		return enemyAirfield
	}
	// 防守方清滩：敌人在自家机场卸了货，等敌舰队撤走后可以岸轰滩头物资。
	// 只对"这个 hex 上确实有敌方滩头物资"开放，否则炮口就会对准自己的机场。
	enemy := OppositeFaction(unit.Faction)
	if shoreSupplies(state, enemy, unit.Coord) <= 0 {
		return nil
	}
	for _, id := range sortedAirfieldIDs(state) {
		airfield := state.Airfields[id]
		if airfield.Faction == unit.Faction && airfield.Coord == unit.Coord {
			return airfield
		}
	}
	return nil
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
	fleetFaction := ""
	for _, member := range fleetMembers(state, fleetID) {
		if member.Faction != "" {
			fleetFaction = member.Faction
			break
		}
	}
	// 清滩：炮击自家机场上被敌人卸下的滩头物资。岸炮是自家的不会还击，也不能把自己的跑道炸了。
	clearingBeach := fleetFaction != "" && airfield.Faction == fleetFaction
	// 岸炮先开火：命中随机舰船，每点岸炮 30% 独立判定
	coastalHits := 0
	if !clearingBeach {
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
			events = append(events, applyHits(state, []Hit{{AttackerID: airfield.ID, TargetID: target.ID, Cause: HitShell}})...)
		}
	}
	// 舰队炮击机场：普通舰每次命中 -1 生命值并摧毁至多 1 架地面飞机，战列舰 -2 / 至多 2 架
	// 舰队在开火前指定目标，同一次齐射只结算一类（规则书 513 行）：
	// 敌方滩头物资仍有余量就打物资，否则打机场。
	gunneryHits, airfieldDamage, groundLoss, coastalLoss := 0, 0, 0, 0
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
			// 规则书 520 行：只摧毁"敌方投送"的滩头物资，绝不炸自己刚卸下的货。
			enemyFaction := OppositeFaction(bombardFaction)
			if shoreSupplies(*state, enemyFaction, airfield.Coord) > 0 {
				removeShoreSupplies(state, enemyFaction, airfield.Coord, 1)
				supplyHits++
				continue
			}
			if clearingBeach {
				// 清滩只打物资：滩头清空后炮口不再对准自家机场，避免自伤跑道和岸炮。
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
			// 舰炮命中也会压制岸炮：每命中一次岸炮-1，最低为 0（与空袭对地攻击一致）。
			if airfield.Coastal > 0 {
				airfield.Coastal--
				coastalLoss++
			}
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
	// 炮击的舰队对敌方完全暴露（规则书 535 行），无论白天黑夜：
	// 上岸轰的舰必须把位置和编成一次交出来，不能再靠"没被发现"藏着。
	events = append(events, exposeFleetAtMaxIntel(state, fleetID, "对岸轰击")...)
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
	coastalText := ""
	if coastalLoss > 0 {
		coastalText = fmt.Sprintf("，岸炮-%d", coastalLoss)
	}
	events = append(events, EngineEvent{Type: "shore_bombardment", Audience: "all", Text: fmt.Sprintf("%s在%s对%s岸轰：命中%d次（机场生命值-%d，摧毁地面飞机%d%s%s），岸炮反击命中%d次，舰队混乱+4", leaderName, airfield.Coord, airfield.Name, gunneryHits, airfieldDamage, groundLoss, supText, coastalText, coastalHits)})
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
// 距离表来自 FactionRules：主力舰对空雷达美日不同（90/70/50/30/10 对 85/65/45/25/5），
// 其余带雷达舰船和没雷达的单位用同一张表。
func radarDetectionChance(state State, unit *Unit, distance int) float64 {
	if unit == nil || distance < 1 || distance > 5 {
		return 0
	}
	rules := rulesFor(state, unit.Faction)
	damage := EnsureDamage(unit)
	if !unit.Card.AirRadar || damage.AirRadarDisabled {
		return rules.NoRadar[distance-1]
	}
	if IsCarrier(unit) || unit.Type == "BB" {
		return rules.CapitalShipRadar[distance-1]
	}
	return rules.OtherRadar[distance-1]
}

// airfieldRadarChance 是机场雷达的发现概率（规则书 736–742 行）。
// 机场和舰艇不是同一张表：美军机场 100/80/60/40/20，日军机场 80/60/40/20/0。
// 以前机场是拿舰艇表算的，机场雷达比同距离的航母还弱，跟规则对不上。
func airfieldRadarChance(state State, faction string, distance int) float64 {
	return rulesFor(state, faction).AirfieldRadarChance(distance)
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
		chance := radarDetectionChance(state, ship, Distance(ship.Coord, coord))
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
		chance := airfieldRadarChance(state, airfield.Faction, distance)
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
// 还在编队里的成员都已升空（处于 attack_holding 或 attack）且已经设定目标。
//
// 已经在返航的成员不再挡住整个编队：以前编队里只要有一架因为燃油不足掉头，
// 剩下的飞机就永远停在原地等它——实测 12 架 B-17 卡在目标上空烧了十几回合油，
// 既不打也不回。返航的飞机自己走自己的路，编队用剩下的人继续出击。
func groundTaskReadyToDepart(state State, taskID string) bool {
	members := groundTaskAircraft(state, taskID)
	if len(members) == 0 {
		return false
	}
	active := 0
	for _, member := range members {
		if member.Returning {
			continue
		}
		active++
		if member.Status != "attack_holding" && member.Status != "attack" {
			return false
		}
		if member.TargetCoord == "" {
			return false
		}
	}
	return active > 0
}
