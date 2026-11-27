package game

import (
	"fmt"
	"math"
	"sort"
)

func contactCoords(state State) []string {
	set := map[string]bool{}
	for _, unit := range state.Units {
		set[unit.Coord] = true
	}
	coords := make([]string, 0, len(set))
	for coord := range set {
		coords = append(coords, coord)
	}
	sort.Strings(coords)
	return coords
}

func unitsAt(state State, coord string) []Unit {
	units := []Unit{}
	for _, unit := range state.Units {
		if unit.Coord == coord {
			units = append(units, *unit)
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })
	return units
}

func detectContacts(state *State) []EngineEvent {
	events := detectVisualContacts(state)
	for _, coord := range contactCoords(*state) {
		members := unitsAt(*state, coord)
		factions := map[string][]Unit{}
		for _, unit := range members {
			factions[unit.Faction] = append(factions[unit.Faction], unit)
		}
		if len(factions) < 2 {
			continue
		}
		observers := make([]string, 0, len(factions))
		for observer := range factions {
			observers = append(observers, observer)
		}
		sort.Strings(observers)
		for _, observer := range observers {
			friendly := factions[observer]
			for _, enemy := range members {
				if enemy.Faction == observer {
					continue
				}
				recordContact(state, observer, FleetKey(&enemy), fleetMembers(state, FleetKey(&enemy)))
			}
			events = append(events, EngineEvent{Type: "contact", Audience: observer, Text: fmt.Sprintf("%s在%s与敌方舰队发生接触（%d艘己方舰船）", friendly[0].FleetName, coord, len(friendly))})
		}
	}
	return events
}

// detectVisualContacts 把 visualContactRange 内互相看得见的舰队记成最高等级情报。
// VisibleUnits 会把这些舰队按真实坐标画出来，但如果不留接触记录，
// 玩家在地图上点得到目标、服务端却判定"没有情报不能盲打"，打击流程就断在这里。
func detectVisualContacts(state *State) []EngineEvent {
	events := []EngineEvent{}
	seen := map[string]bool{}
	// 按 ID 排序遍历：事件顺序和记录顺序必须可复现，否则同种子两局会对不上。
	for _, enemyID := range sortedUnitIDs(state.Units) {
		enemy := state.Units[enemyID]
		if enemy == nil {
			continue
		}
		fleetID := FleetKey(enemy)
		for _, friendlyID := range sortedUnitIDs(state.Units) {
			friendly := state.Units[friendlyID]
			if friendly == nil || friendly.Faction == enemy.Faction {
				continue
			}
			if Distance(friendly.Coord, enemy.Coord) > visualContactRange {
				continue
			}
			key := friendly.Faction + "|" + fleetID
			if seen[key] {
				continue
			}
			seen[key] = true
			members := fleetMembers(state, fleetID)
			if len(members) == 0 {
				continue
			}
			// 目视只是"确认这里有敌人"，情报等级仍按正常节奏每回合 +1 级（上限 4 级），
			// 不直接跳满：等级决定成员表能不能看全，位置则按真实坐标记录，
			// 这样"地图上画出来的位置"和"能点打击的位置"始终是同一个 hex。
			level := recordContact(state, friendly.Faction, fleetID, members)
			events = append(events, EngineEvent{Type: "visual_contact", Audience: friendly.Faction, Text: fmt.Sprintf("目视发现%s在%s（%d艘），情报等级升至%d", enemy.FleetName, enemy.Coord, len(members), level)})
		}
	}
	return events
}

func resolveContacts(state *State) []EngineEvent {
	prior := map[string]map[string]bool{}
	for _, coord := range contactCoords(*state) {
		prior[coord] = map[string]bool{}
		for _, unit := range unitsAt(*state, coord) {
			enemies := []Unit{}
			for _, enemy := range unitsAt(*state, coord) {
				if enemy.Faction != unit.Faction {
					enemies = append(enemies, enemy)
				}
			}
			prior[coord][unit.Faction] = priorDetection(*state, unit.Faction, enemies)
		}
	}
	events := detectContacts(state)
	for _, coord := range contactCoords(*state) {
		members := unitsAt(*state, coord)
		factions := map[string][]Unit{}
		for _, unit := range members {
			factions[unit.Faction] = append(factions[unit.Faction], unit)
		}
		if len(factions) < 2 {
			continue
		}
		events = append(events, resolveSurfaceBattle(state, coord, factions, prior[coord])...)
	}
	return events
}

// aliveUnits 只留下还浮在水面的船：沉没的舰在 state.Units 里已经没有了。
// 舰队对轰的目标池用它刷新——被打沉的舰一旦退出就不再吃炮弹，
// 否则开火舰船会"选中"一艘已经沉掉的船，整轮齐射白白浪费。
func aliveUnits(state *State, units []Unit) []Unit {
	alive := make([]Unit, 0, len(units))
	for _, unit := range units {
		if state.Units[unit.ID] != nil {
			alive = append(alive, unit)
		}
	}
	return alive
}

func totalGunnery(members []Unit) int {
	total := 0
	for _, unit := range members {
		total += unit.Card.Gunnery
	}
	return total
}

func maxConfusion(members []Unit) int {
	confusion := 0
	for _, unit := range members {
		if unit.Confusion > confusion {
			confusion = unit.Confusion
		}
	}
	return confusion
}

// hasSurfaceRadar 判断舰队里是否还有可用的对海搜索雷达。
// 突袭判定的"对海雷达+1"是"先敌发现"的加成，属于搜索功能，所以火控雷达坏了不影响这里。
func hasSurfaceRadar(members []Unit) bool {
	for _, unit := range members {
		if unit.Card.SurfaceRadar && !EnsureDamage(&unit).SurfaceRadarDisabled {
			return true
		}
	}
	return false
}

// hasFireControlRadar 判断开火舰船的火控雷达是否还能给炮击加成。
// 搜索雷达损坏不影响命中率，只有火控雷达损坏才扣掉这 10%。
func hasFireControlRadar(unit *Unit) bool {
	if EnsureDamage(unit).FireControlRadarDisabled {
		return false
	}
	if unit.Card.FireControlRadar != nil {
		return *unit.Card.FireControlRadar
	}
	// 老存档没有这个字段：沿用旧语义（有对海雷达就算有火控）。
	return unit.Card.SurfaceRadar
}

func priorDetection(state State, observer string, enemies []Unit) bool {
	for _, enemy := range enemies {
		if contact := state.Contacts[observer][FleetKey(&enemy)]; contact != nil && contact.Level > 0 {
			return true
		}
	}
	return false
}

func surpriseScore(state *State, faction string, members, enemies []Unit, prior bool) (int, []string) {
	score := d6(state)
	modifiers := []string{}
	rules := rulesFor(*state, faction)
	if rules.SurpriseEliteBonus > 0 {
		score += rules.SurpriseEliteBonus
		modifiers = append(modifiers, "精锐+1")
	}
	if rules.SurpriseNightBonus > 0 {
		score += rules.SurpriseNightBonus
		modifiers = append(modifiers, "夜战+1")
	}
	if rules.SurpriseTorpedoBonus > 0 {
		torpedoFleet := true
		for _, unit := range members {
			switch unit.Type {
			case "TRS", "BB", "BC", "CV", "CVE", "CVL":
				torpedoFleet = false
			}
		}
		if torpedoFleet {
			score += rules.SurpriseTorpedoBonus
			modifiers = append(modifiers, "雷击队+1")
		}
	}
	if rules.SurpriseRadarBonus > 0 && hasSurfaceRadar(members) {
		score += rules.SurpriseRadarBonus
		modifiers = append(modifiers, "对海雷达+1")
	}
	if rules.SurpriseYearBonus > 0 {
		score += rules.SurpriseYearBonus
		modifiers = append(modifiers, "年份+1")
	}
	if prior {
		score++
		modifiers = append(modifiers, "预先发现+1")
	}
	if totalGunnery(members) > totalGunnery(enemies) {
		score++
		modifiers = append(modifiers, "火炮优势+1")
	}
	confusion := maxConfusion(members)
	score -= confusion
	if confusion > 0 {
		modifiers = append(modifiers, fmt.Sprintf("混乱-%d", confusion))
	}
	return score, modifiers
}

func targetRow(unitType string) int {
	switch unitType {
	case "BB":
		return 0
	case "BC":
		return 1
	case "CV", "CVL", "CVE":
		return 2
	case "CA":
		return 3
	case "CL":
		return 4
	case "DD":
		return 5
	default:
		return 6
	}
}

func targetWeight(attacker, target string) int {
	row := targetRow(target)
	attacker = CarrierEquivalentType(attacker)
	table := map[string][7]int{
		"BB": {7, 7, 9, 7, 1, 1, 3},
		"BC": {5, 7, 9, 7, 3, 1, 3},
		"CA": {3, 3, 7, 5, 7, 7, 1},
		"CL": {1, 1, 1, 3, 7, 7, 1},
		"DD": {1, 1, 1, 1, 7, 7, 1},
		"CV": {1, 1, 1, 1, 3, 7, 1},
	}
	weights, ok := table[attacker]
	if !ok {
		return 1
	}
	return weights[row]
}

func selectWeightedTarget(state *State, attacker *Unit, enemies []Unit) *Unit {
	total := 0
	for _, target := range enemies {
		total += targetWeight(attacker.Type, target.Type)
	}
	if total == 0 {
		return nil
	}
	roll := int(nextUint64(&state.RNGState) % uint64(total))
	for index := range enemies {
		roll -= targetWeight(attacker.Type, enemies[index].Type)
		if roll < 0 {
			return state.Units[enemies[index].ID]
		}
	}
	return nil
}

func seaplaneSpotting(state State, attacker *Unit) bool {
	carrier := state.Units[attacker.ID]
	if carrier == nil {
		return false
	}
	for _, aircraft := range state.Aircraft {
		mother := state.Units[aircraft.CarrierID]
		if aircraft.Seaplane && aircraft.Status == "recon" && mother != nil && FleetKey(mother) == FleetKey(carrier) {
			return true
		}
	}
	return false
}

func gunneryChance(state State, attacker, target *Unit, enemies []Unit) float64 {
	chance := 25.0
	if hasFireControlRadar(attacker) {
		chance += 10
	}
	if seaplaneSpotting(state, attacker) {
		chance += 10
	}
	if EnsureDamage(target).Fire == "burning" {
		chance += 10
	}
	// 规则书 310–312 行："表中靠下的单位攻击上方 1 行 / 2~3 行 / 超过 3 行"分别 -5 / -10 / -15。
	// 表里行号越大越靠下（BB=0 … TRS=6），所以"目标在攻击者上方几行"= 目标行 - 攻击者行。
	// 之前写成 攻击者行 - 目标行，BB(0) 打 DD(5) 得到 -5，三个正数分支全部落空，
	// 于是"以小博大/力不从心/螳臂当车"从来没生效过——统计测试抓到的就是这个。
	// 行号越大越靠下，所以"目标在攻击者上方"= 攻击者行号 > 目标行号。
	upward := targetRow(attacker.Type) - targetRow(target.Type)
	switch {
	case upward == 1:
		chance -= 5
	case upward >= 2 && upward <= 3:
		chance -= 10
	case upward > 3:
		chance -= 15
	}
	switch weatherAt(state.Map, target.Coord) {
	case "rain":
		chance -= 5
	case "storm":
		chance -= 10
	}
	confusionDifference := maxConfusion(fleetMembers(&state, FleetKey(attacker))) - maxConfusion(enemies)
	if confusionDifference > 0 {
		chance -= 5 * float64(confusionDifference)
	}
	return math.Max(0, math.Min(100, chance))
}

// gunneryVolley 结算一轮齐射。halfGuns 为真时是 T 劣的第一轮炮战：
// 只有一半火炮能开火，每艘船的炮击值向下取整（炮击值 1 的船这一轮打不出来）。
func gunneryVolley(state *State, attackers, enemies []Unit, halfGuns bool) ([]Hit, []EngineEvent) {
	hits := []Hit{}
	events := []EngineEvent{}
	enemies = aliveUnits(state, enemies)
	if len(enemies) == 0 {
		return hits, events
	}
	for _, attacker := range attackers {
		weapon := state.Units[attacker.ID]
		if weapon == nil || weapon.Card.Gunnery <= 0 {
			continue
		}
		target := selectWeightedTarget(state, weapon, enemies)
		if target == nil {
			continue
		}
		// 炮击值为 0 的船（航母）不参加炮战：以前航母卡上被填了 Gunnery=4，
		// 于是"列克星敦炮击妙高：4 次判定"这种战报会出现。炮击值才是权威，主炮数只用于展示。
		if weapon.Card.Gunnery <= 0 {
			continue
		}
		shots := weapon.Card.Gunnery
		if halfGuns {
			shots /= 2
		}
		if shots <= 0 {
			continue
		}
		chance := gunneryChance(*state, weapon, target, enemies)
		count := 0
		for shot := 0; shot < shots; shot++ {
			if probabilityRoll(&state.RNGState) < chance {
				hits = append(hits, Hit{AttackerID: weapon.ID, TargetID: target.ID, Cause: HitShell})
				count++
			}
		}
		label := ""
		if halfGuns {
			label = "（T 劣，半数火炮）"
		}
		events = append(events, EngineEvent{Type: "gunnery_volley", Audience: "all", Text: fmt.Sprintf("%s%s炮击%s：%d次判定，命中率%.1f%%，命中%d次", weapon.Name, label, target.Name, shots, chance, count)})
	}
	return hits, events
}

// torpedoChance 计算一次齐射的命中率（规则书 336–343 行）：
// 基础 25%；慢速 +10、失去动力 +20、快速 -10、驱逐舰 -10、夜间 +10；
// 美军 1944 年以前 -20；己方混乱每点 -5。
func torpedoChance(state State, weapon *Unit, target *Unit) float64 {
	chance := 25.0
	switch target.Card.SpeedClass {
	case "slow":
		chance += 10
	case "fast":
		chance -= 10
	}
	if EnsureDamage(target).Disabled {
		chance += 20
	}
	if target.Type == "DD" {
		chance -= 10
	}
	if state.Night {
		chance += 10
	}
	chance -= rulesFor(state, weapon.Faction).EarlyRadarPenalty
	chance -= 5 * float64(maxConfusion(fleetMembers(&state, FleetKey(weapon))))
	return math.Max(0, math.Min(100, chance))
}

func torpedoVolley(state *State, attackers, enemies []Unit) ([]Hit, []EngineEvent) {
	hits := []Hit{}
	events := []EngineEvent{}
	enemies = aliveUnits(state, enemies)
	if len(enemies) == 0 {
		return hits, events
	}
	for _, attacker := range attackers {
		weapon := state.Units[attacker.ID]
		if weapon == nil || weapon.Card.TorpedoSalvos <= 0 || EnsureDamage(weapon).TorpedoDisabled {
			continue
		}
		target := enemies[nextUint64(&state.RNGState)%uint64(len(enemies))]
		chance := torpedoChance(*state, weapon, &target)
		weapon.Card.TorpedoSalvos--
		count := 0
		for shot := 0; shot < 4; shot++ {
			if probabilityRoll(&state.RNGState) < chance {
				hits = append(hits, Hit{AttackerID: weapon.ID, TargetID: target.ID, Cause: HitTorpedo})
				count++
			}
		}
		events = append(events, EngineEvent{Type: "torpedo_volley", Audience: "all", Text: fmt.Sprintf("%s向%s齐射4枚鱼雷：命中率%.1f%%，命中%d次", weapon.Name, target.Name, chance, count)})
	}
	return hits, events
}

// tCrossingStayTurns 是"战列线已展开"要求的连续停留回合数。
// FleetSessions 按到达当回合记 1，所以 3 表示已经原地摆了 3 回合。
const tCrossingStayTurns = 3

// fleetMovedThisTurn 判断一支舰队本回合有没有移动过（主动抢入 vs 原地待机）。
func fleetMovedThisTurn(state *State, members []Unit) bool {
	for _, member := range members {
		if unit := state.Units[member.ID]; unit != nil && unit.Moved {
			return true
		}
	}
	return false
}

// fleetCanFormLine 判断编队能不能摆战列线：有航母或运输船就没法排纵队。
func fleetCanFormLine(members []Unit) bool {
	for _, member := range members {
		switch member.Type {
		case "CV", "CVL", "CVE", "TRS":
			return false
		}
	}
	return true
}

// battleLineTState 判断这次炮战里谁摆好了战列线。
//
// 条件（规则书"战列线（T 优 / T 劣）"）：
//   - 守方本回合没动，且已经在同一 hex 连续停留 ≥3 回合——有时间摆队；
//   - 守方编队里没有航母和运输船（航母在场就不会按战列线摆队，运输船是护航队形）；
//   - 守方在交战前就发现了对方（prior[守方]），不是被摸到脸上才反应；
//   - 对方是本回合主动移动进来的。
//
// 返回 (T优阵营, T劣阵营, 判定依据)；不满足时全为空。
func battleLineTState(state *State, factions map[string][]Unit, prior map[string]bool) (string, string, string) {
	names := make([]string, 0, len(factions))
	for faction := range factions {
		names = append(names, faction)
	}
	if len(names) != 2 {
		return "", "", ""
	}
	sort.Strings(names)
	for _, defender := range names {
		attacker := names[0]
		if defender == names[0] {
			attacker = names[1]
		}
		members := factions[defender]
		if len(members) == 0 || fleetMovedThisTurn(state, members) {
			continue
		}
		if !fleetCanFormLine(members) || !prior[defender] {
			continue
		}
		sessions := 0
		if state.FleetSessions != nil {
			sessions = state.FleetSessions[FleetKey(&members[0])]
		}
		if sessions < tCrossingStayTurns {
			continue
		}
		if !fleetMovedThisTurn(state, factions[attacker]) {
			continue
		}
		return defender, attacker, fmt.Sprintf("已原地停留 %d 回合、编队无航母/运输船、交战前已发现对方", sessions)
	}
	return "", "", ""
}

func resolveSurfaceBattle(state *State, coord string, factions map[string][]Unit, prior map[string]bool) []EngineEvent {
	names := make([]string, 0, len(factions))
	for faction := range factions {
		names = append(names, faction)
	}
	sort.Strings(names)
	left, right := names[0], names[1]
	leftScore, leftMods := surpriseScore(state, left, factions[left], factions[right], prior[left])
	rightScore, rightMods := surpriseScore(state, right, factions[right], factions[left], prior[right])
	events := []EngineEvent{{Type: "surprise_rolls", Audience: "all", Text: fmt.Sprintf("%s突袭判定：%s=%d（%v），%s=%d（%v）", coord, left, leftScore, leftMods, right, rightScore, rightMods)}}
	threshold := 4
	if state.Night {
		threshold = 2
	}
	surprised := ""
	winner := ""
	if leftScore-rightScore >= threshold {
		winner, surprised = left, right
	} else if rightScore-leftScore >= threshold {
		winner, surprised = right, left
	}
	hitTargets := map[string]map[string]bool{left: {}, right: {}}
	// 战列线（T 优 / T 劣）：守方原地摆好队形、抢入的那一方只影响自己的第一轮炮战。
	tAdvantage, tDisadvantaged, tReason := battleLineTState(state, factions, prior)
	halfGuns := map[string]bool{}
	if tAdvantage != "" {
		halfGuns[tDisadvantaged] = true
		events = append(events, EngineEvent{Type: "t_crossing", Audience: "all", Text: fmt.Sprintf("%s 在 %s 展开战列线（%s），%s 抢入炮战即 T 劣：第一轮只有一半火炮能开火", tAdvantage, coord, tReason, tDisadvantaged)})
	}
	takeHalf := func(faction string) bool {
		if halfGuns[faction] {
			halfGuns[faction] = false
			return true
		}
		return false
	}
	if winner != "" {
		events = append(events, EngineEvent{Type: "surprise", Audience: "all", Text: winner + "取得突袭优势"})
		torpedoHits, torpedoEvents := torpedoVolley(state, factions[winner], factions[surprised])
		events = append(events, torpedoEvents...)
		for _, hit := range torpedoHits {
			hitTargets[surprised][hit.TargetID] = true
		}
		events = append(events, applyHits(state, torpedoHits)...)
		openingHits, openingEvents := gunneryVolley(state, factions[winner], factions[surprised], takeHalf(winner))
		events = append(events, openingEvents...)
		for _, hit := range openingHits {
			hitTargets[surprised][hit.TargetID] = true
		}
		events = append(events, applyHits(state, openingHits)...)
	}
	leftHits, leftEvents := gunneryVolley(state, factions[left], factions[right], takeHalf(left))
	rightHits, rightEvents := gunneryVolley(state, factions[right], factions[left], takeHalf(right))
	events = append(events, leftEvents...)
	events = append(events, rightEvents...)
	for _, hit := range leftHits {
		hitTargets[right][hit.TargetID] = true
	}
	for _, hit := range rightHits {
		hitTargets[left][hit.TargetID] = true
	}
	events = append(events, applyHits(state, append(leftHits, rightHits...))...)
	for _, faction := range names {
		enemy := right
		if faction == right {
			enemy = left
		}
		delta := 1
		if faction == surprised {
			delta += 2
		}
		remaining := fleetMembers(state, FleetKey(&factions[faction][0]))
		enemyRemaining := fleetMembers(state, FleetKey(&factions[enemy][0]))
		if len(remaining)*2 < len(enemyRemaining) {
			delta++
		}
		if len(hitTargets[faction])*2 > len(factions[faction]) {
			delta++
		}
		if len(enemyRemaining) == 0 {
			delta--
		}
		for _, unit := range remaining {
			unit.Confusion += delta
			if unit.Confusion < 0 {
				unit.Confusion = 0
			}
		}
		events = append(events, EngineEvent{Type: "confusion", Audience: faction, Text: fmt.Sprintf("%s交战结束，混乱值变化%+d", faction, delta)})
	}
	return events
}
