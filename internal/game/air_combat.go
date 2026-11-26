package game

import (
	"fmt"
	"sort"
	"strings"
)

func airCombatChance(attacker, defender int) float64 {
	difference := attacker - defender
	switch {
	case difference < -3:
		return 5
	case difference == -3:
		return 10
	case difference == -2:
		return 15
	case difference == -1:
		return 20
	case difference == 0:
		return 25
	case difference == 1:
		return 35
	case difference == 2:
		return 45
	case difference == 3:
		return 55
	case difference == 4:
		return 65
	case difference == 5:
		return 75
	case difference == 6:
		return 85
	default:
		return 95
	}
}

func operationalAircraft(state *State, aircraft *AirUnit) bool {
	return aircraft != nil && state.Aircraft[aircraft.ID] != nil && !aircraft.Returning
}

func activeAircraft(state *State, aircraftList []*AirUnit) []*AirUnit {
	result := []*AirUnit{}
	for _, aircraft := range aircraftList {
		if operationalAircraft(state, aircraft) {
			result = append(result, aircraft)
		}
	}
	return result
}

// shipCards 生成报告的舰队舰船清单：敌方可见舰船对外只给类型，本级对接"动画至少能画出全部船只"。
func shipCards(ships []Unit) []AirShipCard {
	cards := make([]AirShipCard, 0, len(ships))
	for _, ship := range ships {
		cards = append(cards, AirShipCard{ID: ship.ID, Name: ship.Name, Type: ship.Type, Strength: ship.Strength, Picture: ship.Card.Picture})
	}
	return cards
}

// pictureCardsFor 只取图片路径，给那些已经有机型名列表的地方用。
func pictureCardsFor(aircraftList []*AirUnit) []string {
	_, pictures := aircraftModelCards(aircraftList)
	return pictures
}

// aircraftModelCards 返回去重、排序后的机型名，以及一一对应的算子图片路径。
// 前端空战动画按机型显示新算子图（content/misc/image/aircraft/*.png），
// 只给机型名的话前端只能退回内置矢量剪影。
func aircraftModelCards(aircraftList []*AirUnit) ([]string, []string) {
	type card struct {
		model   string
		picture string
	}
	seen := map[string]bool{}
	cards := []card{}
	for _, aircraft := range aircraftList {
		if aircraft == nil || seen[aircraft.Model] {
			continue
		}
		seen[aircraft.Model] = true
		cards = append(cards, card{aircraft.Model, aircraft.Picture})
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].model < cards[j].model })
	models := make([]string, 0, len(cards))
	pictures := make([]string, 0, len(cards))
	for _, item := range cards {
		models = append(models, item.model)
		pictures = append(pictures, item.picture)
	}
	return models, pictures
}

func aircraftModels(aircraftList []*AirUnit) []string {
	seen := map[string]bool{}
	models := []string{}
	for _, aircraft := range aircraftList {
		if !seen[aircraft.Model] {
			seen[aircraft.Model] = true
			models = append(models, aircraft.Model)
		}
	}
	sort.Strings(models)
	return models
}

func applyAircraftHit(state *State, aircraft *AirUnit) (string, []EngineEvent) {
	if !operationalAircraft(state, aircraft) {
		return "", nil
	}
	retreatChance := []int{0, 20, 30, 40, 40}
	damageChance := []int{0, 20, 30, 30, 20}
	destroyChance := []int{0, 60, 40, 30, 20}
	strength := aircraft.Strength
	if strength < 1 {
		strength = 1
	}
	if strength > 4 {
		strength = 4
	}
	roll := probabilityRoll(&state.RNGState)
	outcome := "无效果"
	switch {
	case roll < float64(retreatChance[strength]):
		outcome = "撤退"
		aircraft.Returning = true
		aircraft.Maintenance = 1
	case roll < float64(retreatChance[strength]+damageChance[strength]):
		outcome = "损伤"
		aircraft.Returning = true
		aircraft.Damaged = true
	case roll < float64(retreatChance[strength]+damageChance[strength]+destroyChance[strength]):
		outcome = "摧毁"
		delete(state.Aircraft, aircraft.ID)
	}
	if outcome == "无效果" {
		return outcome, nil
	}
	return outcome, []EngineEvent{{Type: "aircraft_hit", Audience: "all", Text: fmt.Sprintf("%s航空算子被命中：%s", aircraft.Model, outcome)}}
}

func randomOperationalAircraft(state *State, candidates []*AirUnit) *AirUnit {
	active := activeAircraft(state, candidates)
	if len(active) == 0 {
		return nil
	}
	return active[nextUint64(&state.RNGState)%uint64(len(active))]
}

// guestCAPPenalty 判断客串 CAP 的空战值惩罚（规则书 804–807 行）：
// SBD/D4Y 这类俯冲轰炸机可以客串 CAP，此时空战值 -1；但如果对方攻击编队没有护航战斗机，
// 且本方有真正的战斗机协同 CAP，客串 CAP 不受此惩罚。
func guestCAPPenalty(state State, interceptor *AirUnit, escorts, ownFighters int) int {
	if interceptor == nil || interceptor.Kind != "dive_bomber" {
		return 0
	}
	if escorts == 0 && ownFighters > 0 {
		return 0
	}
	return -1
}

func performAirAttack(state *State, attacker *AirUnit, targets []*AirUnit, attackerBonus int, hitUnits map[string]bool, report *AirCombatReport, attackerSide bool) []EngineEvent {
	if !operationalAircraft(state, attacker) {
		return nil
	}
	target := randomOperationalAircraft(state, targets)
	if target == nil {
		return nil
	}
	chance := airCombatChance(attacker.AirCombat+attackerBonus, target.AirCombat)
	if probabilityRoll(&state.RNGState) >= chance {
		return nil
	}
	hitUnits[target.ID] = true
	outcome, events := applyAircraftHit(state, target)
	// 只有"被摧毁"才算损失；撤退/损伤是退出战斗、不是被击落。
	// 统一走 recordAircraftLoss，这样同一架飞机的多次命中不会被重复计数。
	recordAircraftLoss(report, target, outcome, !attackerSide)
	return events
}

// ammunitionDepletionChance 是战斗机弹药耗尽的概率：兵力比越悬殊越容易打空。
// 各阵营的拐点在 FactionRules.AmmunitionBands 里（日军更早打空）。
func ammunitionDepletionChance(state State, faction string, own, opposing int) float64 {
	return rulesFor(state, faction).AmmunitionDepletionChance(own, opposing)
}

func resolveAmmunition(state *State, attackers, defenders []*AirUnit, report *AirCombatReport) []EngineEvent {
	events := []EngineEvent{}
	groups := []struct {
		own        []*AirUnit
		opposing   []*AirUnit
		ownSide    bool
		ownCount   int
		otherCount int
	}{
		{own: attackers, opposing: defenders, ownSide: true, ownCount: len(attackers), otherCount: len(defenders)},
		{own: defenders, opposing: attackers, ownSide: false, ownCount: len(defenders), otherCount: len(attackers)},
	}
	for _, group := range groups {
		for _, aircraft := range group.own {
			if aircraft.Kind != "fighter" || !operationalAircraft(state, aircraft) {
				continue
			}
			chance := ammunitionDepletionChance(*state, aircraft.Faction, group.ownCount, group.otherCount)
			if probabilityRoll(&state.RNGState) >= chance {
				continue
			}
			aircraft.Returning = true
			// 弹药耗尽返航不是战损：单独计数、按机去重，不进 Losses。
			recordAircraftLoss(report, aircraft, "弹药耗尽", group.ownSide)
			events = append(events, EngineEvent{Type: "air_ammunition_empty", Audience: aircraft.Faction, Text: aircraft.Model + "弹药耗尽，退出空战返航"})
		}
	}
	return events
}

// resolveDogfight 按规则书的"列"展开空战（规则书 725–767 行）：
// 第一行攻击机、第二行 CAP（CAP 多于攻击机时同列堆叠）、第三行护航（只挂在有 CAP 的列上）。
// 每列依次：护航打 CAP → CAP 打护航 → 轰炸机打 CAP → CAP 打轰炸机。
func resolveDogfight(state *State, attackers, defenders []*AirUnit, report *AirCombatReport) []EngineEvent {
	if report.AttackerHits == nil {
		report.AttackerHits = []AirHitDetail{}
	}
	events := []EngineEvent{}
	// 规则书 48 行：遭遇敌人并爆发空战后抛弃副油箱，扣除副油箱内剩余航程。
	for _, group := range [][]*AirUnit{attackers, defenders} {
		for _, aircraft := range group {
			if aircraft == nil || aircraft.Returning {
				continue
			}
			if wasted := dropTank(aircraft); wasted > 0 {
				events = append(events, EngineEvent{Type: "tank_dropped", Audience: aircraft.Faction, Text: fmt.Sprintf("%s爆发空战，抛弃副油箱（损失%d航程）", aircraft.Model, wasted)})
			}
		}
	}
	escorts := []*AirUnit{}
	bombers := []*AirUnit{}
	for _, aircraft := range attackers {
		if aircraft.Kind == "fighter" {
			escorts = append(escorts, aircraft)
			continue
		}
		bombers = append(bombers, aircraft)
	}
	// 约定已取消："没有护航就抛弹转护航"从规则书里删掉了。
	// 轰炸机只能带着炸弹去投，护航是编队自己的责任，不该在结算时被悄悄改派。
	// 第一行：攻击机（轰炸机排在最前，护航紧随其后），至少一列
	columns := len(bombers)
	if columns < len(attackers) {
		columns = len(attackers)
	}
	if columns < 1 {
		columns = 1
	}
	type column struct {
		cap    []*AirUnit
		escort []*AirUnit
		bomber []*AirUnit
	}
	cols := make([]column, columns)
	for index, aircraft := range bombers {
		cols[index%columns].bomber = append(cols[index%columns].bomber, aircraft)
	}
	for index, aircraft := range escorts {
		cols[index%columns].escort = append(cols[index%columns].escort, aircraft)
	}
	// 第二行：CAP 依次铺到各列，超过列数时从第一列开始堆叠
	for index, aircraft := range defenders {
		cols[index%columns].cap = append(cols[index%columns].cap, aircraft)
	}
	hitUnits := map[string]bool{}
	bomberPenalty, capBonus := 0, 0
	if len(escorts) == 0 {
		bomberPenalty = -1
		capBonus = 1
	}
	for _, col := range cols {
		// 1) 护航战斗机轮流攻击本列 CAP
		for _, escort := range col.escort {
			events = append(events, performAirAttack(state, escort, col.cap, 0, hitUnits, report, true)...)
		}
		// 2) CAP 轮流攻击本列护航；没有护航时攻击本列轰炸机
		targets := col.escort
		if len(targets) == 0 {
			targets = col.bomber
		}
		for _, cap := range col.cap {
			events = append(events, performAirAttack(state, cap, targets, 0, hitUnits, report, false)...)
		}
		// 3) 轰炸机与 CAP 轮流交战
		for _, bomber := range col.bomber {
			events = append(events, performAirAttack(state, bomber, col.cap, bomberPenalty, hitUnits, report, true)...)
		}
		for _, cap := range col.cap {
			events = append(events, performAirAttack(state, cap, col.bomber, capBonus, hitUnits, report, false)...)
		}
	}
	for _, aircraft := range bombers {
		if operationalAircraft(state, aircraft) && !hitUnits[aircraft.ID] && !aircraft.Confusion && probabilityRoll(&state.RNGState) < 25 {
			aircraft.Confusion = true
			events = append(events, EngineEvent{Type: "air_confusion", Audience: aircraft.Faction, Text: aircraft.Model + "在空战中被打散编队"})
		}
	}
	return append(events, resolveAmmunition(state, attackers, defenders, report)...)
}

// capCoord 是 CAP 当前实际所在空域：前出中优先用前出点，其次是巡逻点，最后回落到母舰。
func capCoord(state State, aircraft *AirUnit) string {
	if aircraft.ForwardCoord != "" {
		return aircraft.ForwardCoord
	}
	// CAP 待命永远守在母舰头上（覆盖周围 1 hex）：舰队机动后巡逻空域跟着走，
	// 否则舰队开出去几格，CAP 还留在放飞时的旧位置，等于舰队裸奔。
	// 只有雷达引导前出拦截（ForwardCoord）才会离开本队上空。
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		return carrier.Coord
	}
	if aircraft.PatrolCoord != "" {
		return aircraft.PatrolCoord
	}
	return aircraft.Coord
}

// capUnavailableReason 解释目标上空为什么没有可作战 CAP。
func capUnavailableReason(state State, defender, coord string) string {
	inAir, returning, far := 0, 0, 0
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != defender || aircraft.Status != "cap" {
			continue
		}
		inAir++
		if aircraft.Returning {
			returning++
			continue
		}
		if Distance(capCoord(state, aircraft), coord) > 1 {
			far++
		}
	}
	switch {
	case inAir == 0:
		return "目标上空没有 CAP：本舰队当前没有升空的战斗机在执勤"
	case returning == inAir:
		return fmt.Sprintf("目标上空没有 CAP：%d 架战斗机都在返航途中", inAir)
	case far > 0:
		return fmt.Sprintf("目标上空没有 CAP：%d 架战斗机距目标超过 1 hex", far)
	}
	return ""
}

func strikeCAPGroups(state State, defender, coord string) [][]*AirUnit {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		// 空中掩护（cover）到位后等同于该 hex 的 CAP，一起参与防守。
		// CAP 会一直守在目标上空打，直到被击中或弹药耗尽才退出——不按"本回合已经
		// 前出拦截过"排除，否则同一队战斗机一回合只肯打一次，第二波来了就干看着。
		if aircraft.Faction == defender && (aircraft.Status == "cap" || aircraft.Status == "cover") && !aircraft.Returning && aircraft.TaskID != "" && Distance(capCoord(state, aircraft), coord) <= 1 {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left := capCoord(state, groups[ids[i]][0])
		right := capCoord(state, groups[ids[j]][0])
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

func aaHitChance(state State, coord string, heavy bool, faction string) float64 {
	chance := 3.0
	switch weatherAt(state.Map, coord) {
	case "rain":
		chance--
	case "storm":
		chance -= 2
	}
	if heavy {
		chance += rulesFor(state, faction).HeavyAALateBonus
	}
	return chance
}

// recordAirHit 把一次命中明细写进报告（包含"无效果"），前端据此把撤退/损伤/击落画到具体飞机上。
// recordAircraftLoss 统一记航空战损：只有被摧毁算 Losses，损伤/撤退/弹药耗尽算 Disabled。
// airLossText 把战报口径写清楚："损失"只算被摧毁，退出战斗（损伤/撤退/弹药耗尽）单独说。
func airLossText(report *AirCombatReport, attacker bool) string {
	losses, disabled := report.DefenderLosses, report.DefenderDisabled
	if attacker {
		losses, disabled = report.AttackerLosses, report.AttackerDisabled
	}
	if disabled > 0 {
		return fmt.Sprintf("损失%d（另%d架退出战斗）", losses, disabled)
	}
	return fmt.Sprintf("损失%d", losses)
}

func recordAircraftLoss(report *AirCombatReport, aircraft *AirUnit, outcome string, attacker bool) {
	if report == nil || aircraft == nil {
		return
	}
	switch outcome {
	case "摧毁":
		if attacker {
			report.AttackerLosses++
		} else {
			report.DefenderLosses++
		}
	case "损伤", "撤退", "弹药耗尽":
		// 同一架飞机一次交战里可能挨 CAP、重防空、轻防空好几下，Disabled 按机去重，
		// 否则战报里"退出战斗"会比真实架数多一两架。
		if report.disabledSeen == nil {
			report.disabledSeen = map[string]bool{}
		}
		if report.disabledSeen[aircraft.ID] {
			return
		}
		report.disabledSeen[aircraft.ID] = true
		if attacker {
			report.AttackerDisabled++
		} else {
			report.DefenderDisabled++
		}
	}
}

func recordAirHit(report *AirCombatReport, aircraft *AirUnit, outcome string) {
	if report == nil || aircraft == nil || outcome == "" {
		return
	}
	report.AttackerHits = append(report.AttackerHits, AirHitDetail{Attacker: aircraft.ID, Model: aircraft.Model, Outcome: outcome})
}

func confuseRandomAircraft(state *State, candidates []*AirUnit) {
	eligible := []*AirUnit{}
	for _, aircraft := range candidates {
		if operationalAircraft(state, aircraft) && !aircraft.Confusion {
			eligible = append(eligible, aircraft)
		}
	}
	if len(eligible) > 0 && probabilityRoll(&state.RNGState) < 5 {
		eligible[nextUint64(&state.RNGState)%uint64(len(eligible))].Confusion = true
	}
}

func resolveHeavyAA(state *State, attackers []*AirUnit, defenders []Unit, coord string, report *AirCombatReport) []EngineEvent {
	events := []EngineEvent{}
	heavy := 0.0
	faction := ""
	for _, unit := range defenders {
		heavy += unit.Card.HeavyAA
		faction = unit.Faction
	}
	chance := aaHitChance(*state, coord, true, faction)
	hits := 0
	shots := int(heavy)
	// 规则书 692 行：整数部分逐点判定，小数部分作为追加一次判定的概率。
	if probabilityRoll(&state.RNGState) <= (heavy - float64(shots)) {
		shots++
	}
	for ; shots > 0; shots-- {
		if probabilityRoll(&state.RNGState) < chance {
			target := randomOperationalAircraft(state, attackers)
			if target == nil {
				break
			}
			hits++
			report.AAHits++
			outcome, hitEvents := applyAircraftHit(state, target)
			recordAirHit(report, target, outcome)
			recordAircraftLoss(report, target, outcome, true)
			events = append(events, hitEvents...)
		} else {
			confuseRandomAircraft(state, attackers)
		}
	}
	if heavy > 0 {
		events = append(events, EngineEvent{Type: "heavy_aa", Audience: "all", Text: fmt.Sprintf("舰队重型防空命中率%.1f%%，命中%d次", chance, hits)})
	}
	return events
}

// carrierLightAABoost 返回美军航母"并入舰队最高轻防空"的那部分加成（规则书 675 行）。
// 规则书 119 行：CVL/CVE 在空袭结算中视为 CV，所以轻航母同样享受。
func carrierLightAABoost(state State, target *Unit) float64 {
	if CarrierEquivalentType(target.Type) != "CV" || !rulesFor(state, target.Faction).FleetLightAAMerge {
		return 0
	}
	highest := 0.0
	for _, unit := range state.Units {
		if unit.Faction == target.Faction && unit.Coord == target.Coord && unit.ID != target.ID && unit.Card.LightAA > highest {
			highest = unit.Card.LightAA
		}
	}
	return highest
}

// lightAAValue 是"某一架飞机单独承受本舰轻防空"时的点数：鱼雷机翻倍。
// 实际结算不按单架飞机掷骰（那样一队飞机打一艘船会重复掷好几遍），
// 这个函数只保留给规则对照和测试用。
func lightAAValue(state State, attacker *AirUnit, target *Unit) float64 {
	value := target.Card.LightAA
	if isTorpedoAttack(attacker) {
		value *= 2
	}
	return value + carrierLightAABoost(state, target)
}

// lightAAPoolValue 是一艘舰对"正在攻击自己的整队飞机"结算时用的轻防空点数：
// 美军航母并入舰队最高轻防空。骰子数不因复合编队缩水——缩的是命中率。
func lightAAPoolValue(state State, target *Unit) float64 {
	return target.Card.LightAA + carrierLightAABoost(state, target)
}

// lightAAHitChance 是单次轻防空判定的命中率。
//
// 规则书 712 行：轻型防空面对复合编队时"最终结算值×0.5"，所以减半排在最后，
// 叠完天气、年份、鱼雷机翻倍之后再整体砍一半。鱼雷机等效点数翻倍在目标池
// 合并成一副骰子后落到命中率上，因此"鱼雷机 ×2、复合编队 ×0.5"互相抵消。
func lightAAHitChance(state State, target *Unit, plane *AirUnit, mixed bool) float64 {
	chance := aaHitChance(state, target.Coord, false, target.Faction)
	if isTorpedoAttack(plane) {
		chance *= 2
	}
	if mixed {
		chance *= 0.5
	}
	return chance
}

// resolveLightAAPool 结算一艘舰对"正在攻击自己的整队飞机"的轻型防空。
//
// 规则书 683 行：每一点防空值都是一次独立判定，命中后从"正在攻击本舰的算子"里随机取一架，
// 撤退/损伤/摧毁的算子立刻退出目标池，后面的骰子不再把它算进去（掷出无效果则留在池里继续挨判定）。
// 之前是按"每架攻击机各掷一轮本舰全值"结算的：五架飞机打一艘船，防空骰子会多出五倍。
func resolveLightAAPool(state *State, attackers []*AirUnit, target *Unit, mixed bool, report *AirCombatReport) []EngineEvent {
	if target == nil || len(attackers) == 0 {
		return nil
	}
	events := []EngineEvent{}
	value := lightAAPoolValue(*state, target)
	shots := int(value)
	// 小数防空：整数部分逐点判定，小数部分作为追加一次判定的概率（0–1 → 0–100%）
	if probabilityRoll(&state.RNGState) <= (value - float64(shots)) {
		shots++
	}
	for ; shots > 0; shots-- {
		pool := activeAircraft(state, attackers)
		if len(pool) == 0 {
			break
		}
		plane := pool[nextUint64(&state.RNGState)%uint64(len(pool))]
		// 规则书 719 行：轻型防空值不影响战略轰炸机——要求在很高高度投弹，小口径炮够不着。
		// B-26 挂炸弹时按高空轰炸结算，同样免疫；挂鱼雷时走普通鱼雷机判定。
		// 骰子选中它们时这一发作废（也不产生混乱判定），骰数本身不变。
		if isHighAltitudeBomber(plane) {
			continue
		}
		// 规则书 673、712 行：鱼雷机翻倍、复合编队减半，都落在这一颗骰子的命中率上。
		hitChance := lightAAHitChance(*state, target, plane, mixed)
		if probabilityRoll(&state.RNGState) < hitChance {
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

func strikeShipWeight(unitType string) int {
	switch CarrierEquivalentType(unitType) {
	case "BB", "BC":
		return 10
	case "CV":
		return 100
	case "CA":
		return 3
	case "CL", "DD":
		return 1
	case "TRS":
		return 11
	default:
		return 1
	}
}

// selectStrikeShip 按攻击编队的分配方式选目标（规则书 624–631 行）：
// 集中攻击每个算子按权重表独立选；分散攻击每个算子均匀随机选编队中任意一艘船。
func selectStrikeShip(state *State, attacker *AirUnit, ships []Unit) *Unit {
	if attacker != nil && attacker.Disperse {
		if len(ships) == 0 {
			return nil
		}
		pick := ships[nextUint64(&state.RNGState)%uint64(len(ships))]
		return state.Units[pick.ID]
	}
	total := 0
	for _, ship := range ships {
		total += strikeShipWeight(ship.Type)
	}
	if total == 0 {
		return nil
	}
	roll := int(nextUint64(&state.RNGState) % uint64(total))
	for index := range ships {
		roll -= strikeShipWeight(ships[index].Type)
		if roll < 0 {
			return state.Units[ships[index].ID]
		}
	}
	return nil
}

func strikeHitChance(state State, attacker *AirUnit, target *Unit, mixed, capRemaining bool) float64 {
	chance := 25.0
	if !capRemaining {
		chance += 5
	}
	if mixed {
		chance += 10
	}
	// 目标航速：慢船更好打、快船更难打。EffectiveSpeedClass 只返回 very_slow/slow/normal/fast，
	// 之前多写的 very_fast 分支不存在，且 fast 会落进 very_slow 的 +5 里，方向正好反了。
	switch EffectiveSpeedClass(target) {
	case "slow", "very_slow":
		chance += 5
	case "fast":
		chance -= 5
	}
	if EnsureDamage(target).Disabled {
		chance += 10
	}
	if target.Type == "DD" {
		chance -= 5
	}
	chance -= float64(int(target.Card.LightAA/7) * 5)
	// 规则书 697–699 行：多云只压战略轰炸机，降雨对所有机种 -10%；对舰攻击表里没有风暴那一档。
	switch weatherAt(state.Map, target.Coord) {
	case "cloud":
		if isStrategicBomber(attacker) {
			chance -= 5
		}
	case "rain":
		chance -= 10
	}
	if attacker.Confusion {
		chance -= 5
	}
	// 水平轰炸对舰惩罚：炸弹从高空砸机动目标，命中率按机种打折。
	// 普通中型轰炸机（B-25）×0.75；高空机（战略轰炸机、B-26 挂炸弹）÷2 且至少 5%。
	switch {
	case isHighAltitudeBomber(attacker):
		chance /= 2
		if chance < 5 {
			chance = 5
		}
	case isLevelBomber(attacker) && !isTorpedoAttack(attacker):
		chance *= levelBomberShipPenalty
	}
	if chance < 5 {
		return 5
	}
	if chance > 95 {
		return 95
	}
	return chance
}

// levelBomberShipPenalty 是普通中型轰炸机（B-25）做水平轰炸攻击舰船时的命中率系数。
// 规则书只给了战略轰炸机"除以 2"，中型机没有数值；这里取 0.75。
// 带 high_altitude_bomber 标记的 B-26 挂炸弹时不走这个系数，而是按高空机减半。
const levelBomberShipPenalty = 0.75

// isStrategicBomber 识别战略轰炸机：按机种常量判断，没有常量时退回机型/型号关键字。
// 注意这里不再按 "G4M" 命中：一式陆攻是鱼雷机，把它算成战略轰炸机会让它
// 所有对舰雷击命中率被腰斩，而它恰恰是日军陆基航空队的主力。
func isStrategicBomber(aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	return strategicBomberModel(aircraft.Kind, aircraft.Model)
}

// strategicBomberModel 是舰载/陆基两种算子共用的战略轰炸机判定：
// 海上算子和机场算子用同一套机种常量与机型名，所以天气修正不该各判一套。
func strategicBomberModel(kind, model string) bool {
	if kind == "strategic_bomber" {
		return true
	}
	upper := strings.ToUpper(model)
	return strings.Contains(upper, "B-17") || strings.Contains(upper, "B-24") || strings.Contains(upper, "B-29")
}

// isStrategicGroundBomber 是机场算子（对地攻击）版本的战略轰炸机判定。
func isStrategicGroundBomber(aircraft *AirfieldAirUnit) bool {
	if aircraft == nil {
		return false
	}
	return strategicBomberModel(aircraft.Kind, aircraft.Model)
}

// isTorpedoAttack 识别这一次出击是否按鱼雷机结算：
// 专用鱼雷机，或带鱼雷挂架的机型（B-26）真正挂了鱼雷。
func isTorpedoAttack(aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	if aircraft.Kind == "torpedo_bomber" {
		return true
	}
	return aircraft.TorpedoCapable && aircraft.Loadout.Weapon() == WeaponTorpedo
}

// isHighAltitudeBomber 识别高空投弹：战略轰炸机始终算；
// B-26 这类带 high_altitude_bomber 标记的机型只在挂炸弹时算，挂鱼雷时是鱼雷机。
// 高空机免疫轻防空，同时对舰命中率吃战略轰炸机的减半惩罚。
func isHighAltitudeBomber(aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	if isStrategicBomber(aircraft) {
		return true
	}
	return aircraft.HighAltitudeBomber && aircraft.Loadout.Weapon() == WeaponBomb
}

// isLevelBomber 识别普通水平轰炸中型/重型轰炸机（B-25 这一档）。
// B-26 挂炸弹时由 isHighAltitudeBomber 先行拦截，不落到这个 0.75 系数。
func isLevelBomber(aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	switch aircraft.Kind {
	case "level_bomber", "medium_bomber", "heavy_bomber":
		return true
	}
	return false
}

// selectStrikeTarget 按交战权重给一个算子挑目标舰，已经沉没的船不参与（规则书 637 行）。
func selectStrikeTarget(state *State, attacker *AirUnit, ships []Unit) *Unit {
	remaining := make([]Unit, 0, len(ships))
	for _, ship := range ships {
		if state.Units[ship.ID] != nil {
			remaining = append(remaining, ship)
		}
	}
	return selectStrikeShip(state, attacker, remaining)
}

// bombWavePriority 决定投弹波的顺序：水平轰炸 → 俯冲轰炸 → 鱼雷。
//
// 这个顺序本身就是战术：先到的波次打掉轻防空、点起火，后到的波次就打得更准
// （规则书：目标燃烧 +10%、轻防空值每被打中一次 -10%），鱼雷机负责收人头。
func bombWavePriority(aircraft *AirUnit) int {
	if aircraft == nil {
		return 1
	}
	if isTorpedoAttack(aircraft) {
		return 2
	}
	if isHighAltitudeBomber(aircraft) || isLevelBomber(aircraft) {
		return 0
	}
	// 挂弹战斗机按炸弹波处理，不是单独的"战斗机波"；不挂弹的不会进 bombers。
	return 1
}

// strikeBomber 判断这架飞机这次是否携带弹药参与投弹：
// 轰炸机按机种；战斗机只有挂炸弹时才算投弹，挂对空/侦察的是护航。
func strikeBomber(aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	if aircraft.Kind != "fighter" {
		return true
	}
	return aircraft.Loadout.Weapon() == WeaponBomb
}

// resolveBombingWave 结算一次对舰攻击，顺序是：
//
//	重防空（由调用方在空袭开始时对整支机队结算一次）
//	→ 机队按目标分配拆成"攻击各舰的小分队"
//	→ 每支小分队先吃该舰一轮轻防空（目标池就是这支小队）
//	→ 小队内部按 水平 → 俯冲 → 鱼雷 依次投弹
//	→ 投弹后这支小队穿过弹幕，再吃一轮轻防空
//
// 全程用的都是实时值：这一舰的轻防空被命中一次就 -10%，舰沉了立刻退出，
// 小队里被打掉/撤退/损伤的算子马上不再投弹，美军航母并入的是当时同格最高的轻防空。
// 复合编队缩的是这一轮判定的命中率，不是骰子数（规则书 712 行）。
func resolveBombingWave(state *State, bombers []*AirUnit, ships []Unit, mixed, capRemaining bool, report *AirCombatReport) []EngineEvent {
	events := []EngineEvent{}
	order := []string{}
	groups := map[string][]*AirUnit{}
	for _, aircraft := range bombers {
		if !operationalAircraft(state, aircraft) || !strikeBomber(aircraft) {
			continue
		}
		target := selectStrikeTarget(state, aircraft, ships)
		if target == nil {
			continue
		}
		if _, ok := groups[target.ID]; !ok {
			order = append(order, target.ID)
		}
		groups[target.ID] = append(groups[target.ID], aircraft)
		report.TargetAllocations = append(report.TargetAllocations, StrikeAllocation{Attacker: aircraft.ID, Model: aircraft.Model, ShipID: target.ID, ShipName: target.Name})
	}
	for _, shipID := range order {
		attackers := groups[shipID]
		ship := state.Units[shipID]
		if ship == nil {
			continue
		}
		// 进场轻防空：这一舰对本小分队掷一副骰子。
		events = append(events, resolveLightAAPool(state, attackers, ship, mixed, report)...)
		// 小队内部仍按波次投弹：水平轰炸 → 俯冲轰炸 → 鱼雷。
		wave := make([]*AirUnit, 0, len(attackers))
		wave = append(wave, attackers...)
		sort.SliceStable(wave, func(i, j int) bool {
			return bombWavePriority(wave[i]) < bombWavePriority(wave[j])
		})
		// 这一轮投弹里所有炸弹都会砸在同一艘舰上：半路打沉不打断本轮，
		// 沉没先挂起，等这支小队投完再处理（规则书"沉不沉影响的是第二轮轻防空"）。
		state.DeferredSink = true
		for _, attacker := range wave {
			if !operationalAircraft(state, attacker) {
				continue
			}
			unit := state.Units[shipID]
			if unit == nil {
				// 舰在本次攻击之前就不在了（例如被同一回合的炮战打沉）：改打还浮着的舰。
				if unit = selectStrikeTarget(state, attacker, ships); unit == nil {
					continue
				}
			}
			events = append(events, resolveBombingOn(state, attacker, unit, mixed, capRemaining, report)...)
		}
		events = append(events, flushDeferredSinks(state)...)
		// 撤离轻防空：幸存算子穿过弹幕再吃一轮；这一轮已经把它打沉了就没有第二轮了。
		if unit := state.Units[shipID]; unit != nil {
			events = append(events, resolveLightAAPool(state, attackers, unit, mixed, report)...)
		}
	}
	return events
}

// resolveBombingOn 结算单个算子对指定舰的投弹；轻防空由 resolveBombingWave 按舰统一掷。
func resolveBombingOn(state *State, attacker *AirUnit, target *Unit, mixed, capRemaining bool, report *AirCombatReport) []EngineEvent {
	if !operationalAircraft(state, attacker) || !strikeBomber(attacker) || target == nil {
		return nil
	}
	events := []EngineEvent{}
	antiShip := attacker.AntiShip + AttackBonus(attacker.Loadout)
	if attacker.Confusion && antiShip > 0 {
		antiShip--
	}
	chance := strikeHitChance(*state, attacker, target, mixed, capRemaining)
	hits := 0
	// 记录尝试次数，即使 0 命中也要让战报/动画知道"确实投弹了"，而不是看起来漏结算。
	report.BombAttempts += antiShip
	for index := range report.TargetAllocations {
		if report.TargetAllocations[index].Attacker == attacker.ID {
			report.TargetAllocations[index].Attempts = antiShip
		}
	}
	for ; antiShip > 0; antiShip-- {
		if probabilityRoll(&state.RNGState) >= chance {
			continue
		}
		hits++
		report.StrikeHits++
		report.ShipHits++
		// 被雷达截获的空袭：命中结算的 DCT 临时 +1（规则书 408 行）
		warned := report.Detected
		strengthBefore := 0
		if unit := state.Units[target.ID]; unit != nil {
			strengthBefore = unit.Strength
		}
		if isTorpedoAttack(attacker) {
			events = append(events, applyTorpedoHit(state, Hit{AttackerID: attacker.ID, TargetID: target.ID, Cause: HitTorpedo, Warned: warned})...)
		} else {
			events = append(events, applyGunOrBombHit(state, Hit{AttackerID: attacker.ID, TargetID: target.ID, Cause: HitBomb, Warned: warned})...)
		}
		// 命中 ≠ 损伤：没过损管判定的命中只削弱防空、砸甲板，不掉强度。
		if unit := state.Units[target.ID]; unit == nil || unit.Strength < strengthBefore {
			report.DamageHits++
		}
		// 目标沉没后剩余攻击值不再投弹，否则会继续对已移除的单位结算损伤和战报
		if state.Units[target.ID] == nil {
			break
		}
	}
	for index := range report.TargetAllocations {
		if report.TargetAllocations[index].Attacker == attacker.ID {
			report.TargetAllocations[index].Hits += hits
		}
	}
	events = append(events, EngineEvent{Type: "strike_bombing", Audience: "all", Text: fmt.Sprintf("%s攻击%s：命中率%.1f%%，命中%d次", attacker.Model, target.Name, chance, hits)})
	// 规则书 83 行：携带炸弹或鱼雷的飞机攻击后变为轻载，剩余航程 +20%。
	markBombed(attacker)
	return events
}

func resolveAirStrike(state *State, taskID, coord string) []EngineEvent {
	members := taskAircraft(*state, taskID)
	attackers := []*AirUnit{}
	for _, aircraft := range members {
		// 已接敌的编队直接按 TargetCoord 结算空袭：接敌判定是对着 TargetCoord 做的，
		// 但编队可能停在接敌时的格子上，不能再用 Coord == coord 过滤。
		onTarget := aircraft.Coord == coord || aircraft.Engaged
		if aircraft.Status == "attack" && !aircraft.Returning && onTarget {
			attackers = append(attackers, aircraft)
		}
	}
	if len(attackers) == 0 {
		return nil
	}
	faction := attackers[0].Faction
	ships := []Unit{}
	for _, unit := range unitsAt(*state, coord) {
		if unit.Faction != faction {
			ships = append(ships, unit)
		}
	}
	if len(ships) == 0 {
		return nil
	}
	fleetID := FleetKey(&ships[0])
	for level := recordContact(state, faction, fleetID, ships); level < 4; level = recordContact(state, faction, fleetID, ships) {
	}
	// 陆基 CAP / 空中掩护住在 state.GroundAir 里，防守判定只扫 state.Aircraft，
	// 不投影进来就会出现"机场战斗机看着舰队挨炸"。
	restoreDefenders := projectDefendingGroundAir(state, ships[0].Faction, coord, 1)
	defer restoreDefenders()
	groups := strikeCAPGroups(*state, ships[0].Faction, coord)
	// 防守方明明有 CAP 却没来参战时说明原因（在返航、距离太远、或者还没起飞），
	// 否则玩家只能看到 defender_count=0，以为防御失效了。
	var capNote []EngineEvent
	if len(groups) == 0 {
		if reason := capUnavailableReason(*state, ships[0].Faction, coord); reason != "" {
			capNote = append(capNote, EngineEvent{Type: "air_cap_unavailable", Audience: ships[0].Faction, Text: reason})
		}
	}
	attackerModels, attackerPictures := aircraftModelCards(attackers)
	report := AirCombatReport{Coord: coord, AttackerFaction: faction, DefenderFaction: ships[0].Faction, AttackerModels: attackerModels, AttackerPictures: attackerPictures, AttackerCount: len(attackers)}
	// 任一攻击机曾被雷达截获，即视为"来袭机群已被提前发现"
	for _, aircraft := range attackers {
		if aircraft.WarnedIntercept {
			report.Detected = true
			break
		}
	}
	// bombs 单独统计"参与投弹的算子"：挂弹战斗机也算，不挂弹的护航战斗机不计入
	for _, aircraft := range attackers {
		if strikeBomber(aircraft) && operationalAircraft(state, aircraft) {
			report.Bombers = append(report.Bombers, AirHitDetail{Attacker: aircraft.ID, Model: aircraft.Model})
		}
	}
	defenders := []*AirUnit{}
	for _, group := range groups {
		defenders = append(defenders, group...)
		report.DefenderCount += len(group)
	}
	report.DefenderModels, report.DefenderPictures = aircraftModelCards(defenders)
	report.Target = string(StrikeTargetGroup)
	report.TargetName = "混合舰队"
	if len(ships) == 1 {
		report.Target = string(StrikeTargetShip)
		report.TargetName = ships[0].Name
	}
	events := append([]EngineEvent{}, capNote...)
	for _, group := range groups {
		events = append(events, resolveDogfight(state, attackers, group, &report)...)
	}
	bombers := []*AirUnit{}
	mixedKinds := map[string]bool{}
	for _, aircraft := range attackers {
		if !strikeBomber(aircraft) {
			continue
		}
		bombers = append(bombers, aircraft)
		kind := aircraft.Kind
		if kind == "fighter" {
			// 挂弹战斗机走炸弹波，编队判定也按炸弹机算，不额外算一种"战斗机"编队。
			kind = "dive_bomber"
		}
		mixedKinds[kind] = true
	}
	mixed := len(mixedKinds) > 1
	// 规则书 689 行：重型防空从“当前参与空袭的算子”里随机选目标——护航战斗机同样会被打，
	// 而不是只有投弹的轰炸机挨打。
	events = append(events, resolveHeavyAA(state, attackers, ships, coord, &report)...)
	capRemaining := false
	for _, group := range groups {
		if len(activeAircraft(state, group)) > 0 {
			capRemaining = true
			break
		}
	}
	// 目标分配前先给每艘舰备好报告条目，让动画能画出"舰队全部舰船"（即使某艘没被炸到）
	report.Ships = shipCards(ships)
	events = append(events, resolveBombingWave(state, bombers, ships, mixed, capRemaining, &report)...)
	for _, group := range groups {
		events = append(events, resolveDogfight(state, attackers, group, &report)...)
	}
	attackersRemaining := len(activeAircraft(state, attackers))
	capRemaining = false
	for _, group := range groups {
		if len(activeAircraft(state, group)) > 0 {
			capRemaining = true
			break
		}
	}
	// 对舰攻击即使目标上空没有 CAP 也要生成报告：舰队防空 + 投弹同样是一场有结果的交战，
	// 前端据此播放对舰攻击动画。此前只有出现过 CAP 才出报告，真实对局里基本看不到。
	report.ID = fmt.Sprintf("air-combat-%x", nextUint64(&state.RNGState))
	// 胜负只看「攻击机是否全灭」和「CAP 是否仍能拦截」。攻击机打完必定返航，返航不等于被击落，
	// 不能因为幸存者被标记返航就判 CAP 获胜；损失数相同时记为脱离接触。
	switch {
	case attackersRemaining == 0:
		report.Winner = report.DefenderFaction
	case !capRemaining:
		report.Winner = report.AttackerFaction
	case report.AttackerLosses > report.DefenderLosses:
		report.Winner = report.DefenderFaction
	case report.AttackerLosses == report.DefenderLosses:
		report.Winner = "draw"
	default:
		report.Winner = report.AttackerFaction
	}
	fleetHits := 0
	for _, unit := range ships {
		if state.Units[unit.ID] == nil || state.Units[unit.ID].Strength < unit.Strength {
			fleetHits++
		}
	}
	report.ShipsHit = fleetHits
	if report.DefenderCount > 0 {
		report.Summary = fmt.Sprintf("%s空战：攻击方%s，CAP%s；防空命中%d，投弹命中%d", coord, airLossText(&report, true), airLossText(&report, false), report.AAHits, report.StrikeHits)
	} else if report.BombAttempts == 0 {
		report.Summary = fmt.Sprintf("%s空袭%s：防空命中%d，攻击机全部被压制未投弹，攻击方%s", coord, report.TargetName, report.AAHits, airLossText(&report, true))
	} else {
		report.Summary = fmt.Sprintf("%s空袭%s：防空命中%d，投弹%d次命中%d次、其中有效损伤%d次（%d艘舰掉强度），攻击方%s", coord, report.TargetName, report.AAHits, report.BombAttempts, report.StrikeHits, report.DamageHits, fleetHits, airLossText(&report, true))
	}
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
	events = append(events, EngineEvent{Type: "air_strike_resolved", Audience: "all", Text: fmt.Sprintf("%s空袭结算完成：防空命中%d，投弹命中%d，幸存攻击机开始返航", coord, report.AAHits, report.StrikeHits)})
	return events
}

func aircraftModelsFromNames(models []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, model := range models {
		if !seen[model] {
			seen[model] = true
			result = append(result, model)
		}
	}
	sort.Strings(result)
	return result
}

// landingFailureChance 把备用油透支程度换算成着舰失败概率（规则书 71–76 行）：
// 备用航程不属于正常航程，只在主油耗尽后动用，记账可以为负数；-1 起每多透支 1 格，
// 失败率按 10% → 30% → 50% → 70% → 100% 递增。
func landingFailureChance(reserve int) float64 {
	switch {
	case reserve >= 0:
		return 0
	case reserve == -1:
		return 10
	case reserve == -2:
		return 30
	case reserve == -3:
		return 50
	case reserve == -4:
		return 70
	default:
		return 100
	}
}

// resolveLandingRisk 在着舰前结算备用油透支。crashed 为 true 表示算子已经在着舰时损失，
// 调用方不能再继续走落地流程。
func resolveLandingRisk(state *State, aircraft *AirUnit) (bool, []EngineEvent) {
	if state == nil || aircraft == nil {
		return false, nil
	}
	reserve := aircraft.ReserveLeft()
	chance := landingFailureChance(reserve)
	if chance <= 0 {
		return false, nil
	}
	model := aircraft.Model
	if probabilityRoll(&state.RNGState) >= chance {
		return false, []EngineEvent{{Type: "air_landing_overdue", Audience: aircraft.Faction, Text: fmt.Sprintf("%s备用油透支（剩余%d），仍安全着舰（失败率%.0f%%）", model, reserve, chance)}}
	}
	// 摔毁的算子从来没有真正上舰：它在放飞时就已经从 Card.Aircraft 里扣掉了，
	// 这里再加回去等于凭空多出一架，机库计数会一路涨到"21/18"，还会挡住后续转场和回收。
	delete(state.Aircraft, aircraft.ID)
	return true, []EngineEvent{{Type: "air_landing_crash", Audience: aircraft.Faction, Text: fmt.Sprintf("%s备用油透支（剩余%d），着舰失败，算子损毁", model, reserve)}}
}

// resolveGroundLandingRisk 是机场飞机的同一套判定。
func resolveGroundLandingRisk(state *State, aircraft *AirfieldAirUnit) (bool, []EngineEvent) {
	if state == nil || aircraft == nil {
		return false, nil
	}
	reserve := aircraft.ReserveLeft()
	chance := landingFailureChance(reserve)
	if chance <= 0 {
		return false, nil
	}
	model := aircraft.Model
	if probabilityRoll(&state.RNGState) >= chance {
		return false, []EngineEvent{{Type: "air_landing_overdue", Audience: aircraft.Faction, Text: fmt.Sprintf("%s备用油透支（剩余%d），仍安全着舰（失败率%.0f%%）", model, reserve, chance)}}
	}
	delete(state.GroundAir, aircraft.ID)
	return true, []EngineEvent{{Type: "air_landing_crash", Audience: aircraft.Faction, Text: fmt.Sprintf("%s备用油透支（剩余%d），着陆失败，算子损毁", model, reserve)}}
}

// nightGroundLandingCrash 夜间降落机场：每个算子额外 20% 摔毁（规则书 535 行）。
// 结算顺序按规则书 80 行：航程不足 → 跑道状况 → 夜间降落，所以它排在最后。
func nightGroundLandingCrash(state *State, aircraft *AirfieldAirUnit) (bool, []EngineEvent) {
	if state == nil || aircraft == nil || !state.Night {
		return false, nil
	}
	if probabilityRoll(&state.RNGState) >= 20 {
		return false, nil
	}
	model := aircraft.Model
	delete(state.GroundAir, aircraft.ID)
	return true, []EngineEvent{{Type: "night_landing_crash", Audience: aircraft.Faction, Text: model + "夜间降落失败，冲出跑道"}}
}

func advanceAircraftReturn(state *State, aircraft *AirUnit) []EngineEvent {
	// 母舰彻底没了（沉没/甲板报废）之后选定的备降机场：自己飞过去，到了才降落。
	if aircraft.DivertAirfield != "" {
		return advanceAirfieldDivert(state, aircraft)
	}
	carrier := state.Units[aircraft.CarrierID]
	// 甲板被彻底摧毁等于这艘航母不再能回收飞机：换船，而不是让飞机对着废甲板盘旋。
	if carrier != nil && carrier.Damage != nil && carrier.Damage.DeckDestroyed {
		carrier = nil
	}
	if carrier == nil {
		// 先找同阵营还有空甲的航母：哪怕隔着一整片海也要自己飞过去。
		if host := altLanding(state, aircraft); host != nil {
			aircraft.CarrierID = host.ID
			carrier = host
		}
	}
	if carrier == nil {
		// 一艘能收的航母都没有了，才谈备降机场——而且是自己飞过去，到了才落地。
		if aircraft.DivertAirfield == "" {
			if airfield := alternateAirfield(state, aircraft); airfield != nil {
				aircraft.DivertAirfield = airfield.ID
				aircraft.TargetCoord = airfield.Coord
			}
		}
		if aircraft.DivertAirfield != "" {
			return advanceAirfieldDivert(state, aircraft)
		}
		delete(state.Aircraft, aircraft.ID)
		return []EngineEvent{{Type: "air_destroyed", Audience: aircraft.Faction, Text: aircraft.Model + "失去母舰且无可用甲板或机场，航空算子被移除"}}
	}
	for steps := 0; steps < 2 && aircraft.Coord != carrier.Coord; steps++ {
		next := nextFlightCoord(*state, aircraft, carrier.Coord)
		if next == "" {
			break
		}
		aircraft.Coord = next
		aircraft.Spend(1)
	}
	if aircraft.Coord != carrier.Coord {
		return nil
	}
	// 夜航摔毁判定（规则书 520–524 行）：夜间降落航母每个算子额外 70% 摔毁，
	// 机场 20%（机场落地在 landAtAirfield 里做）。摔毁先于降落判定，否则夜间返航永远是安全的。
	if state.Night && !aircraft.Seaplane && probabilityRoll(&state.RNGState) < 70 {
		model := aircraft.Model
		// 同 resolveLandingRisk：摔毁的算子不占机库位置，放飞时已经扣过了。
		delete(state.Aircraft, aircraft.ID)
		return []EngineEvent{{Type: "night_landing_crash", Audience: aircraft.Faction, Text: model + "夜间降落失败，撞上甲板"}}
	}
	crashed, riskEvents := resolveLandingRisk(state, aircraft)
	if crashed {
		return riskEvents
	}
	if !landIfCapacity(state, aircraft, carrier) {
		// 降不下去时必须说明原因：否则玩家只看到飞机在母舰上空盘旋、燃油一路掉。
		reason := landingBlockReason(*state, carrier)
		if reason == "" {
			reason = "甲板没有空位"
		}
		// 在母舰上空盘旋等甲板也要烧油：以前这里直接 return 不扣油，
		// 甲板一满，飞机就能永远挂在舰队头顶上不下来（夜里的"空中幽灵"就是这么来的）。
		riskEvents = append(riskEvents, EngineEvent{Type: "air_landing_blocked", Audience: aircraft.Faction, Text: fmt.Sprintf("%s无法降落%s：%s（剩余航程%d）", aircraft.Model, carrier.Name, reason, aircraft.Range())})
		aircraft.Spend(1)
		return riskEvents
	}
	aircraft.Confusion = false
	return append(riskEvents, EngineEvent{Type: "air_landed", Audience: aircraft.Faction, Text: strings.Join([]string{aircraft.Model, "返航", carrier.Name, "，等待升降机下降"}, "")})
}

func VisibleAirCombats(state State, faction string) []AirCombatReport {
	reports := []AirCombatReport{}
	for _, report := range state.AirCombats {
		if report.AttackerFaction == faction || report.DefenderFaction == faction {
			reports = append(reports, filterReportFor(report, faction))
		}
	}
	return reports
}

// filterReportFor 按受众裁剪面向动画的明细：
// 攻击方看不到"自己每架飞机被防空打成什么状态"以外的防御方视角，防守方看不到分配细节和舰船身份（情报等级不足时）。
func filterReportFor(report AirCombatReport, faction string) AirCombatReport {
	if report.DefenderFaction != faction || report.AttackerFaction == faction {
		return report
	}
	// 防守方视角：只要不泄露"敌方怎么分配目标"，舰船本身是防守方自己的船，名字/类型可以保留；
	// 舰船数量属于可见信息（舰上飞机着舰时已经暴露）。分配细节只给攻击方。
	report.TargetAllocations = nil
	return report
}
