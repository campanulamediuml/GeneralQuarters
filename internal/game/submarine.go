package game

import (
	"fmt"
	"math"
	"sort"
)

// —— 潜艇（【猎杀潜航】模块）——
//
// 潜艇是独立行动的隐蔽单位：不进 state.Units，不参与水面炮战、航空攻击、岸轰和情报体系。
// 部署阶段每方在可潜海格自由摆 N 艘；另有 3 艘随机撒在地图里的预备潜艇，连本方也不知道位置。
// 开战之后潜艇从双方视图里消失，只有"被发现"时才对舰队方显示（潜艇方看不到自己的潜艇）。
//
// 时间线（规则书〈猎杀潜航〉）：
//  1. 每回合开始时先按 1d6 定的相位每 6 回合随机挪 1 格（当前格和周围 6 格都可能）；
//  2. 然后结算潜艇战：同格有敌方舰队 → 双方各掷 d10 加修正，差额归点数高的一方；
//  3. 拿到点数的一方只执行能负担的最高一档效果；
//  4. 潜艇被发现后，由触发遭遇战的那支舰队每回合投 1 次深水炸弹，潜艇每回合掷一次撤退。
//
// 设计边界：
//   - 潜艇不会被炮击、空袭、岸轰命中，只有深水炸弹能击沉它；
//   - 随机巡逻不写战报：写"某潜艇移动到某格"等于把隐蔽性送掉，只有潜艇战才进战报。

const (
	// DefaultSubmarineMoveInterval 是随机巡逻节奏：每 6 回合挪 1 格。
	DefaultSubmarineMoveInterval = 6
	// submarineReserveCount 是每方随机撒在地图里的预备潜艇数（规则书〈猎杀潜航〉）。
	submarineReserveCount = 3

	// 深水炸弹投放的基础命中率与修正（规则书〈反潜作业〉）。
	depthChargeBaseChance  = 5
	depthChargeShoalBonus  = 5
	depthChargeUSBonus     = 5
	depthChargeScreenBonus = 5

	// 潜艇撤退的基础概率与修正（规则书〈撤退〉）。
	submarineRetreatBase      = 50
	submarineRetreatJPBonus   = 20
	submarineRetreatUSPenalty = -10
	submarineRetreatScreen    = -20
)

// submarineTargetWeight 是鱼雷的目标权重（规则书〈鱼雷〉）：航母和运输船是头号目标。
func submarineTargetWeight(unitType string) int {
	switch upperType(unitType) {
	case "CV", "CVL", "CVE", "TRS":
		return 500
	case "BB", "BC":
		return 50
	case "CA", "CL":
		return 20
	case "DD":
		return 10
	}
	return 10
}

// deploySubmarines 按剧本配置展开运行期潜艇，并给每方撒下 3 艘预备潜艇。
func deploySubmarines(state *State, scenario ScenarioDefinition) error {
	reserveTemplates := map[string]SubmarineDeployment{}
	factions := []string{}
	for _, deployment := range scenario.Submarines {
		if deployment.Faction != "A" && deployment.Faction != "B" {
			return fmt.Errorf("潜艇部署阵营无效：%s", deployment.Faction)
		}
		count := deployment.Count
		if count <= 0 {
			count = 1
		}
		interval := deployment.MoveInterval
		if interval <= 0 {
			interval = DefaultSubmarineMoveInterval
		}
		baseName := deployment.Name
		if baseName == "" {
			baseName = "潜艇"
		}
		if state.Submarines == nil {
			state.Submarines = map[string]*Submarine{}
		}
		for index := 1; index <= count; index++ {
			id := fmt.Sprintf("sub-%s-%d", deployment.Faction, index)
			if _, exists := state.Submarines[id]; exists {
				return fmt.Errorf("潜艇ID重复：%s", id)
			}
			name := baseName
			if count > 1 {
				name = fmt.Sprintf("%s%d", baseName, index)
			}
			state.Submarines[id] = &Submarine{
				ID: id, Faction: deployment.Faction, Name: name, Picture: deployment.Picture,
				MoveInterval: interval,
			}
		}
		if _, seen := reserveTemplates[deployment.Faction]; !seen {
			reserveTemplates[deployment.Faction] = deployment
			factions = append(factions, deployment.Faction)
		}
	}
	if len(state.Submarines) == 0 {
		return nil
	}
	// 预备潜艇：开局随机撒在地图里，双方都不知道位置；连本方也看不到、不能重新摆位。
	sort.Strings(factions)
	for _, faction := range factions {
		template := reserveTemplates[faction]
		baseName := template.Name
		if baseName == "" {
			baseName = "潜艇"
		}
		interval := template.MoveInterval
		if interval <= 0 {
			interval = DefaultSubmarineMoveInterval
		}
		for index := 1; index <= submarineReserveCount; index++ {
			id := fmt.Sprintf("sub-%s-reserve-%d", faction, index)
			if _, exists := state.Submarines[id]; exists {
				return fmt.Errorf("潜艇ID重复：%s", id)
			}
			state.Submarines[id] = &Submarine{
				ID: id, Faction: faction, Name: fmt.Sprintf("%s预备艇%d", baseName, index), Picture: template.Picture,
				Coord: submarineSpawnCoord(state, faction), Deployed: true, Reserve: true, MoveInterval: interval,
			}
		}
	}
	return nil
}

// submarineCanEnter 是潜艇的通行规则：水面和浅滩都行；机场格只要周围 1 hex 有水面也能进。
func submarineCanEnter(state State, coord string) bool {
	if !inBounds(state.Map, coord) {
		return false
	}
	terrain := terrainAt(state.Map, coord)
	if terrain == TerrainWater || terrain == TerrainShoal {
		return true
	}
	if terrain != TerrainIsland {
		return false
	}
	if _, ok := state.Airfields[coord]; !ok {
		return false
	}
	for _, neighbor := range Neighbors(coord) {
		if inBounds(state.Map, neighbor) && terrainAt(state.Map, neighbor) == TerrainWater {
			return true
		}
	}
	return false
}

// submarineSpawnCoord 随机挑一个能潜伏的海格；优先避开敌方舰队当前所在格。
func submarineSpawnCoord(state *State, faction string) string {
	candidates := []string{}
	fallback := []string{}
	for row := 1; row <= state.Map.Rows; row++ {
		for col := 1; col <= state.Map.Cols; col++ {
			coord := Coord(row, col)
			if !submarineCanEnter(*state, coord) {
				continue
			}
			fallback = append(fallback, coord)
			if len(enemyShipsAt(*state, coord, faction)) == 0 {
				candidates = append(candidates, coord)
			}
		}
	}
	if len(candidates) == 0 {
		candidates = fallback
	}
	if len(candidates) == 0 {
		return ""
	}
	return candidates[nextUint64(&state.RNGState)%uint64(len(candidates))]
}

// sortedSubmarineIDs 固定遍历顺序：同一随机种子要能复现同一场仗。
func sortedSubmarineIDs(submarines map[string]*Submarine) []string {
	ids := make([]string, 0, len(submarines))
	for id := range submarines {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// enemyShipsAt 取某个 hex 上不属于 faction 的舰船，按 ID 排序（结算可复现）。
func enemyShipsAt(state State, coord, faction string) []Unit {
	result := []Unit{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit != nil && unit.Coord == coord && unit.Faction != faction && unit.Strength > 0 {
			result = append(result, *unit)
		}
	}
	return result
}

// enemyFleetIDsAt 取某个 hex 上敌方舰队的 ID 列表，按 ID 排序。
func enemyFleetIDsAt(state State, coord, faction string) []string {
	seen := map[string]bool{}
	fleets := []string{}
	for _, unit := range enemyShipsAt(state, coord, faction) {
		id := FleetKey(&unit)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		fleets = append(fleets, id)
	}
	sort.Strings(fleets)
	return fleets
}

// StartSubmarines 在开战那一刻给每艘潜艇掷 1d6 决定第一次巡逻的回合，并把没摆位的潜艇随机落位。
func StartSubmarines(state *State) []EngineEvent {
	if len(state.Submarines) == 0 {
		return nil
	}
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if sub == nil {
			continue
		}
		if !sub.Deployed {
			// 没配部署阶段的剧本（或玩家没摆完就开战）直接随机潜伏。
			sub.Coord = submarineSpawnCoord(state, sub.Faction)
			sub.Deployed = true
		}
		if sub.MoveInterval <= 0 {
			sub.MoveInterval = DefaultSubmarineMoveInterval
		}
		sub.FirstMoveTurn = d6(state)
	}
	return []EngineEvent{{Type: "submarines_hidden", Audience: "all", Text: fmt.Sprintf("潜艇部队进入战位：共%d艘，位置从双方视野中消失", len(state.Submarines))}}
}

// UndeployedSubmarines 数一下某阵营还有几艘可部署潜艇没摆位；部署完毕前要用它拦 ready。
func UndeployedSubmarines(state State, faction string) int {
	remaining := 0
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if sub != nil && sub.Faction == faction && !sub.Deployed && !sub.Reserve {
			remaining++
		}
	}
	return remaining
}

// validateSubDeploy 是部署阶段的潜艇摆位校验：只能摆在可潜海格，位置随便挑。
func validateSubDeploy(state State, faction string, order Order) error {
	if !InDeployPhase(state) {
		return fmt.Errorf("潜艇只能在部署阶段摆位")
	}
	sub := state.Submarines[order.UnitID]
	if sub == nil || sub.Faction != faction {
		return fmt.Errorf("潜艇不存在或不属于当前玩家")
	}
	if sub.Reserve {
		return fmt.Errorf("预备潜艇不受玩家控制")
	}
	if !inBounds(state.Map, order.Coord) {
		return fmt.Errorf("目标格不存在")
	}
	if !submarineCanEnter(state, order.Coord) {
		return fmt.Errorf("潜艇只能部署在海面、浅滩，或周围有水的机场格")
	}
	return nil
}

// ApplySubDeploy 把一艘潜艇放到指定海格（部署阶段立即生效）。
func ApplySubDeploy(state *State, order Order) []EngineEvent {
	sub := state.Submarines[order.UnitID]
	if sub == nil {
		return nil
	}
	sub.Coord = order.Coord
	sub.Deployed = true
	return []EngineEvent{{Type: "submarine_deployed", Audience: sub.Faction, Text: fmt.Sprintf("%s进入战位", sub.Name)}}
}

// AdvanceSubmarines 推进随机巡逻：每艘潜艇按自己 1d6 的相位，每 MoveInterval 回合挪 1 格。
// 被发现的潜艇被钉在原地，不巡逻。
func AdvanceSubmarines(state *State) {
	if len(state.Submarines) == 0 || state.Turn < 1 {
		return
	}
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if sub == nil || !sub.Deployed || sub.Discovered {
			continue
		}
		interval := sub.MoveInterval
		if interval <= 0 {
			interval = DefaultSubmarineMoveInterval
		}
		if sub.FirstMoveTurn <= 0 {
			sub.FirstMoveTurn = 1
		}
		if state.Turn < sub.FirstMoveTurn || (state.Turn-sub.FirstMoveTurn)%interval != 0 {
			continue
		}
		patrolSubmarine(state, sub)
	}
}

// patrolSubmarine 随机挪 1 格：当前格和周围 6 格都可能，能潜就潜，不能潜的格跳过。
func patrolSubmarine(state *State, sub *Submarine) {
	candidates := []string{}
	if submarineCanEnter(*state, sub.Coord) {
		candidates = append(candidates, sub.Coord)
	}
	for _, coord := range Neighbors(sub.Coord) {
		if submarineCanEnter(*state, coord) {
			candidates = append(candidates, coord)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sub.Coord = candidates[nextUint64(&state.RNGState)%uint64(len(candidates))]
}

// ResolveSubmarinePhase 是回合开始时的潜艇阶段：先巡逻，再由潜艇战/反潜作业结算同格遭遇。
func ResolveSubmarinePhase(state *State) []EngineEvent {
	if len(state.Submarines) == 0 {
		return nil
	}
	events := []EngineEvent{}
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if sub == nil || !sub.Deployed {
			continue
		}
		if sub.Discovered {
			events = append(events, resolveDepthChargeTracking(state, sub)...)
			continue
		}
		fleets := enemyFleetIDsAt(*state, sub.Coord, sub.Faction)
		if len(fleets) == 0 {
			continue
		}
		fleetID := fleets[nextUint64(&state.RNGState)%uint64(len(fleets))]
		events = append(events, resolveSubmarineBattle(state, sub, fleetID)...)
	}
	return events
}

// ———————————————— 潜艇战 ————————————————

// subBattleModifiers 是潜艇方 d10 的修正（规则书〈潜艇战〉）。
func subBattleModifiers(state State, sub *Submarine, fleet []Unit) int {
	bonus := 0
	if terrainAt(state.Map, sub.Coord) == TerrainShoal {
		bonus += 2
	}
	if fleetHasDistressedShip(state, fleet) {
		bonus += 2
	}
	if fleetHasSlowShip(fleet) {
		bonus += 1
	}
	if fleetHasTransport(fleet) {
		bonus += 1
	}
	if len(fleetASWShips(fleet)) == 0 {
		bonus += 2
	}
	if state.Night {
		bonus += 1
	}
	return bonus
}

// fleetBattleModifiers 是舰队方 d10 的修正（规则书〈潜艇战〉）。
func fleetBattleModifiers(state State, fleet []Unit) int {
	if len(fleet) == 0 {
		return 0
	}
	bonus := 0
	faction := fleet[0].Faction
	doctrine := doctrineOf(state, faction)
	if doctrine == DoctrineUS {
		bonus++
	}
	if state.Year >= 1944 {
		bonus += 2
	}
	if fleetHasSurfaceRadar(fleet) {
		bonus++
	}
	weather := weatherAt(state.Map, fleet[0].Coord)
	if weather == WeatherClear {
		bonus++
	}
	aswShips := len(fleetASWShips(fleet))
	if doctrine == DoctrineJapan && aswShips >= 3 {
		bonus++
	}
	if doctrine == DoctrineUS && aswShips >= 3 {
		bonus += aswShips / 3
	}
	if aswAirPatrolAt(state, faction, fleet[0].Coord) {
		bonus++
	}
	if weather == WeatherStorm {
		bonus--
	}
	return bonus
}

// resolveSubmarineBattle 结算一次潜艇战：双方掷 d10，差额归点数高的一方，
// 胜方只执行能负担的最高一档效果，不重复购买，也不在效果未奏效后改买较低档。
func resolveSubmarineBattle(state *State, sub *Submarine, fleetID string) []EngineEvent {
	fleet := fleetMembers(state, fleetID)
	if len(fleet) == 0 {
		return nil
	}
	report := SubmarineBattle{
		ID:          fmt.Sprintf("sub-battle-%d-%d", state.Turn, len(state.SubBattles)+1),
		Turn:        state.Turn,
		Coord:       sub.Coord,
		SubmarineID: sub.ID, SubmarineName: sub.Name, SubmarineFaction: sub.Faction,
		FleetID: fleetID, FleetName: fleetNameOf(fleet), FleetFaction: fleet[0].Faction,
	}
	events := []EngineEvent{}
	subRoll := d10(state) + subBattleModifiers(*state, sub, fleet)
	fleetRoll := d10(state) + fleetBattleModifiers(*state, fleet)
	report.SubRoll, report.FleetRoll = subRoll, fleetRoll
	side := ""
	points := 0
	switch {
	case subRoll > fleetRoll:
		side, points = sub.Faction, subRoll-fleetRoll
	case fleetRoll > subRoll:
		side, points = fleet[0].Faction, fleetRoll-subRoll
	}
	report.SubPoints, report.FleetPoints = 0, 0
	if side == sub.Faction {
		report.SubPoints = points
	} else if side != "" {
		report.FleetPoints = points
	}
	cost := 0
	switch {
	case points >= 4:
		cost = 4
	case points >= 2:
		cost = 2
	case points >= 1:
		cost = 1
	}
	if side != "" && cost > 0 {
		points -= cost
		if side == sub.Faction {
			events = append(events, buySubmarineEffect(state, sub, fleetID, cost, &report)...)
		} else {
			events = append(events, buyFleetEffect(state, sub, fleetID, cost, &report)...)
		}
	}
	report.PointsLeft = points
	report.Summary = submarineBattleSummary(*state, report)
	state.SubBattles = append(state.SubBattles, report)
	events = append(events, EngineEvent{Type: "submarine_battle", Audience: "all", Text: report.Summary})
	return events
}

// buySubmarineEffect 兑换潜艇方的效果：4 = 鱼雷 + 全身而退，2 = 近距离鱼雷 + 下潜逃避，1 = 发送目标位置。
func buySubmarineEffect(state *State, sub *Submarine, fleetID string, cost int, report *SubmarineBattle) []EngineEvent {
	switch cost {
	case 4:
		events := submarineTorpedoAttack(state, sub, fleetID, report, !state.Night)
		report.SubmarineEscaped = true
		return events
	case 2:
		// 近距离打完鱼雷再"下潜逃避"：这一潜等于自己暴露了——舰队看着它下去，
		// 从下回合起就进入被发现状态、被钉在原地挨深水炸弹。
		events := submarineTorpedoAttack(state, sub, fleetID, report, false)
		fleet := fleetMembers(state, fleetID)
		if len(fleet) > 0 {
			sub.Discovered = true
			sub.DiscoveredBy = fleet[0].Faction
			sub.TrackedFleetID = fleetID
			report.Discovered = true
			report.Effects = append(report.Effects, "下潜逃避：潜艇暴露，从下回合起被反潜作业锁定")
			events = append(events, EngineEvent{Type: "submarine_discovered", Audience: fleet[0].Faction, Text: fmt.Sprintf("在%s发现下潜中的敌方潜艇%s", sub.Coord, sub.Name)})
		}
		return events
	default:
		setFleetContactLevel(state, sub.Faction, fleetID, 3)
		report.Effects = append(report.Effects, "发送目标位置：潜艇方获得 3 级情报")
		return nil
	}
}

// buyFleetEffect 兑换舰队方的效果：4 = 概率击沉，2 = 发现潜艇，1 = 位置暴露。
func buyFleetEffect(state *State, sub *Submarine, fleetID string, cost int, report *SubmarineBattle) []EngineEvent {
	fleet := fleetMembers(state, fleetID)
	if len(fleet) == 0 {
		return nil
	}
	switch cost {
	case 4:
		chance := 10
		if aswAirPatrolAt(*state, fleet[0].Faction, sub.Coord) {
			chance = 20
		}
		if probabilityRoll(&state.RNGState) < float64(chance) {
			report.Effects = append(report.Effects, "反潜舰立刻击沉潜艇")
			sinkSubmarine(state, sub)
			return []EngineEvent{{Type: "submarine_sunk", Audience: "all", Text: fmt.Sprintf("潜艇%s被反潜舰深水炸弹击沉", sub.Name)}}
		}
		report.Effects = append(report.Effects, fmt.Sprintf("反潜舰尝试立刻击沉（%d%%）未成功", chance))
		return nil
	case 2:
		sub.Discovered = true
		sub.DiscoveredBy = fleet[0].Faction
		sub.TrackedFleetID = fleetID
		report.Discovered = true
		report.Effects = append(report.Effects, "潜艇被发现，下潜逃避")
		return []EngineEvent{{Type: "submarine_discovered", Audience: fleet[0].Faction, Text: fmt.Sprintf("在%s发现敌方潜艇%s：本回合起展开反潜作业", sub.Coord, sub.Name)}}
	default:
		setFleetContactLevel(state, sub.Faction, fleetID, 1)
		report.Effects = append(report.Effects, "位置暴露：潜艇方获得 1 级情报")
		return nil
	}
}

// submarineTorpedoAttack 发射 1 发鱼雷：按权重挑目标，掷命中率，命中后走通用鱼雷损伤结算。
func submarineTorpedoAttack(state *State, sub *Submarine, fleetID string, report *SubmarineBattle, longRange bool) []EngineEvent {
	fleet := fleetMembers(state, fleetID)
	if len(fleet) == 0 {
		return nil
	}
	target := selectSubmarineTarget(state, fleet)
	if target == nil {
		return nil
	}
	chance := submarineTorpedoChance(*state, sub, target, longRange)
	report.TorpedoShots++
	if probabilityRoll(&state.RNGState) < chance {
		report.TorpedoHits++
		report.TorpedoShip = target.Name
		report.TorpedoShipType = target.Type
		events := []EngineEvent{{Type: "submarine_torpedo", Audience: "all", Text: fmt.Sprintf("潜艇%s向%s发射鱼雷命中（命中率%.0f%%）", sub.Name, target.Name, chance)}}
		events = append(events, applyHits(state, []Hit{{AttackerID: sub.ID, TargetID: target.ID, Cause: HitTorpedo}})...)
		report.TorpedoShipSunk = state.Units[target.ID] == nil
		return events
	}
	return []EngineEvent{{Type: "submarine_torpedo", Audience: "all", Text: fmt.Sprintf("潜艇%s向%s发射鱼雷未命中（命中率%.0f%%）", sub.Name, target.Name, chance)}}
}

// submarineTorpedoChance 是鱼雷命中率（%）：
//   - 基础 25%（白天远距离偷袭就是这个数）；
//   - 夜间潜艇能贴近到近距离发射，+20；夜间再 +10，所以夜间偷袭是 55%；
//   - 再叠加日军/美军 1944 前/小型舰艇的修正，最低 5。
func submarineTorpedoChance(state State, sub *Submarine, target *Unit, longRange bool) float64 {
	chance := 25.0
	if !longRange {
		chance += 20 // 近距离
	}
	if state.Night {
		chance += 10
	}
	if doctrineOf(state, sub.Faction) == DoctrineJapan {
		chance += 10
	}
	if doctrineOf(state, sub.Faction) == DoctrineUS && state.Year < 1944 {
		chance -= 20
	}
	if upperType(target.Type) == "DD" {
		chance -= 10
	}
	return math.Max(5, chance)
}

// selectSubmarineTarget 按权重随机挑目标：航母/运输船 500，战列舰 50，巡洋舰 20，驱逐舰 10。
func selectSubmarineTarget(state *State, fleet []Unit) *Unit {
	total := 0
	for index := range fleet {
		total += submarineTargetWeight(fleet[index].Type)
	}
	if total <= 0 {
		return &fleet[0]
	}
	roll := int(nextUint64(&state.RNGState) % uint64(total))
	for index := range fleet {
		roll -= submarineTargetWeight(fleet[index].Type)
		if roll < 0 {
			return &fleet[index]
		}
	}
	return &fleet[len(fleet)-1]
}

// ———————————————— 反潜作业 ————————————————

// resolveDepthChargeTracking 结算被发现潜艇的每回合反潜作业：
// 触发舰队随机一艘反潜舰投 1 次深弹（深弹 -1），然后潜艇掷一次撤退。
func resolveDepthChargeTracking(state *State, sub *Submarine) []EngineEvent {
	events := []EngineEvent{}
	fleet := fleetMembers(state, sub.TrackedFleetID)
	// 触发舰队不在了，或者已经离开这一格：追踪直接失败。
	if len(fleet) == 0 || fleet[0].Coord != sub.Coord {
		return append(events, hideSubmarine(sub, "跟踪的舰队已经离开，潜艇从地图上消失")...)
	}
	asw := fleetASWShips(fleet)
	withCharges := []Unit{}
	for _, unit := range asw {
		if unit.Card.DepthCharges > 0 {
			withCharges = append(withCharges, unit)
		}
	}
	if len(withCharges) == 0 {
		return append(events, hideSubmarine(sub, "触发舰队的深水炸弹耗尽，潜艇从地图上消失")...)
	}
	escort := withCharges[nextUint64(&state.RNGState)%uint64(len(withCharges))]
	ship := state.Units[escort.ID]
	if ship == nil {
		return events
	}
	if ship.Card.DepthCharges > 0 {
		ship.Card.DepthCharges--
	}
	chance := float64(depthChargeBaseChance)
	if terrainAt(state.Map, sub.Coord) == TerrainShoal {
		chance += depthChargeShoalBonus
	}
	if len(fleet) > 0 && doctrineOf(*state, fleet[0].Faction) == DoctrineUS {
		chance += depthChargeUSBonus
	}
	if len(asw) >= 2 {
		chance += depthChargeScreenBonus
	}
	if probabilityRoll(&state.RNGState) < chance {
		sinkSubmarine(state, sub)
		return append(events, EngineEvent{Type: "submarine_sunk", Audience: "all", Text: fmt.Sprintf("%s投放深水炸弹，击沉潜艇%s", ship.Name, sub.Name)})
	}
	events = append(events, EngineEvent{Type: "depth_charge", Audience: sub.DiscoveredBy, Text: fmt.Sprintf("%s投放深水炸弹未命中（命中率%.0f%%，剩余深弹%d组）", ship.Name, chance, ship.Card.DepthCharges)})
	if submarineRetreatRoll(*state, sub, fleet) {
		return append(events, hideSubmarine(sub, fmt.Sprintf("潜艇%s成功脱离接触，从地图上消失", sub.Name))...)
	}
	return events
}

// submarineRetreatRoll 掷一次潜艇撤退判定：基础 50%，日军 +20，美军 1943+ 反潜 -10，触发舰队两艘以上反潜舰 -20。
func submarineRetreatRoll(state State, sub *Submarine, fleet []Unit) bool {
	chance := submarineRetreatBase
	if doctrineOf(state, sub.Faction) == DoctrineJapan {
		chance += submarineRetreatJPBonus
	}
	if len(fleet) > 0 && doctrineOf(state, fleet[0].Faction) == DoctrineUS && state.Year >= 1943 {
		chance += submarineRetreatUSPenalty
	}
	if len(fleetASWShips(fleet)) >= 2 {
		chance += submarineRetreatScreen
	}
	return probabilityRoll(&state.RNGState) < float64(chance)
}

// hideSubmarine 让潜艇重新消失（撤退成功或追踪失败），它不是被击沉。
func hideSubmarine(sub *Submarine, text string) []EngineEvent {
	audience := sub.DiscoveredBy
	sub.Discovered = false
	sub.DiscoveredBy = ""
	sub.TrackedFleetID = ""
	if audience == "" {
		audience = "all"
	}
	return []EngineEvent{{Type: "submarine_hidden", Audience: audience, Text: text}}
}

// sinkSubmarine 击沉潜艇：直接移出战场（潜艇没有耐久，深弹命中即沉）。
func sinkSubmarine(state *State, sub *Submarine) {
	delete(state.Submarines, sub.ID)
}

// ———————————————— 舰队特征 ————————————————

// fleetASWShips 挑出反潜舰：装备深水炸弹且剩余数量不为 0。
func fleetASWShips(fleet []Unit) []Unit {
	result := []Unit{}
	for _, unit := range fleet {
		if unit.Card.DepthCharges > 0 && unit.Strength > 0 {
			result = append(result, unit)
		}
	}
	return result
}

func fleetHasDistressedShip(state State, fleet []Unit) bool {
	for _, unit := range fleet {
		if unit.Strength <= 0 {
			continue
		}
		damage := EnsureDamage(&unit)
		if damage.Disabled || damage.Fire != "" {
			return true
		}
		if unitUnloading(state, &unit) != nil {
			return true
		}
	}
	return false
}

func fleetHasSlowShip(fleet []Unit) bool {
	for _, unit := range fleet {
		switch EffectiveSpeedClass(&unit) {
		case "slow", "very_slow", "disabled":
			return true
		}
	}
	return false
}

func fleetHasTransport(fleet []Unit) bool {
	for _, unit := range fleet {
		if upperType(unit.Type) == "TRS" {
			return true
		}
	}
	return false
}

func fleetHasSurfaceRadar(fleet []Unit) bool {
	for _, unit := range fleet {
		if unit.Card.SurfaceRadar && !EnsureDamage(&unit).SurfaceRadarDisabled {
			return true
		}
	}
	return false
}

// fleetNameOf 取舰队的显示名；没有编队名时退回首舰名。
func fleetNameOf(fleet []Unit) string {
	if len(fleet) == 0 {
		return "敌方舰队"
	}
	if fleet[0].FleetName != "" {
		return fleet[0].FleetName
	}
	return fleet[0].Name
}

// aswAirPatrolAt 判断某格附近有没有正在执行反潜任务的航空单位。
// 反潜巡逻任务还没实装（规则书〈航空任务概述〉），这里先把判定留好。
func aswAirPatrolAt(state State, faction, coord string) bool {
	// 只有真正挂着反潜深弹的巡逻机才算数：挂炸弹/鱼雷的飞机飞过去也只是看客。
	armed := func(loadout Loadout) bool { return loadout.Weapon() == WeaponDepthCharge }
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != faction || aircraft.TaskKind != "asw" || !airborne(aircraft) || !armed(aircraft.Loadout) {
			continue
		}
		if Distance(aircraft.Coord, coord) <= 1 {
			return true
		}
	}
	for _, aircraft := range state.GroundAir {
		if aircraft.Faction != faction || aircraft.TaskKind != "asw" || !airfieldAirborne(aircraft) || !armed(aircraft.Loadout) {
			continue
		}
		if Distance(aircraft.Coord, coord) <= 1 {
			return true
		}
	}
	return false
}

// setFleetContactLevel 把 observer 对某舰队的情报抬到指定等级（只升不降），并刷新显示位置。
func setFleetContactLevel(state *State, observer, fleetID string, level int) {
	members := fleetMembers(state, fleetID)
	if len(members) == 0 {
		return
	}
	contact := upsertContact(state, observer, fleetID, members)
	if contact.Level < level {
		contact.Level = level
	}
	contact.DisplayCoord = projectedIntelCoord(*contact)
}

// submarineBattleSummary 写一句能直接贴进战报的完整说明。
func submarineBattleSummary(state State, report SubmarineBattle) string {
	text := fmt.Sprintf("T%d %s 潜艇战：潜艇「%s」对 %s「%s」——潜艇骰 %d，舰队骰 %d",
		report.Turn, report.Coord, report.SubmarineName, factionLabel(state, report.FleetFaction), report.FleetName, report.SubRoll, report.FleetRoll)
	if len(report.Effects) > 0 {
		text += "；" + joinText(report.Effects, "；")
	} else {
		text += "；战斗点数相同，双方脱离"
	}
	if report.SubmarineSunk {
		text += "；潜艇被击沉"
	}
	return text
}

// factionLabel 是战报里的阵营称呼。
func factionLabel(state State, faction string) string {
	switch doctrineOf(state, faction) {
	case DoctrineUS:
		return "美军"
	case DoctrineJapan:
		return "日军"
	}
	return "阵营 " + faction
}

// joinText 用分隔符拼字符串（Go 1.24 没有 strings.Join 之外的花样，这里只包一层可读性）。
func joinText(items []string, separator string) string {
	out := ""
	for index, item := range items {
		if index > 0 {
			out += separator
		}
		out += item
	}
	return out
}

// VisibleSubmarines 是某阵营能看到的潜艇：
//   - 部署阶段：本方还没开战的可部署潜艇（预备潜艇连本方也不可见）；
//   - 开战后：被本方发现并正在追踪的敌方潜艇。
func VisibleSubmarines(state State, faction string) []Submarine {
	result := []Submarine{}
	if InDeployPhase(state) {
		for _, id := range sortedSubmarineIDs(state.Submarines) {
			sub := state.Submarines[id]
			if sub == nil || sub.Faction != faction || sub.Reserve {
				continue
			}
			result = append(result, *sub)
		}
		return result
	}
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		sub := state.Submarines[id]
		if sub == nil || !sub.Discovered || sub.DiscoveredBy != faction {
			continue
		}
		result = append(result, *sub)
	}
	return result
}

// SpectatorSubmarines 给持有观战 token 的只读全知视图返回双方全部潜艇。
// 玩家投影仍走 VisibleSubmarines，不会因为观战需要而暴露对方的隐蔽位置。
func SpectatorSubmarines(state State) []Submarine {
	result := []Submarine{}
	for _, id := range sortedSubmarineIDs(state.Submarines) {
		if sub := state.Submarines[id]; sub != nil {
			result = append(result, *sub)
		}
	}
	return result
}

// VisibleSubBattles 返回潜艇战战报：交战双方的舰队与潜艇已经同格遭遇，战报对两边公开。
func VisibleSubBattles(state State) []SubmarineBattle {
	result := make([]SubmarineBattle, len(state.SubBattles))
	copy(result, state.SubBattles)
	return result
}
