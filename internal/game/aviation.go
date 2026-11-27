package game

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

func syncEmbarkedAircraft(state *State) {
	for _, aircraft := range state.Aircraft {
		if airborne(aircraft) {
			continue
		}
		if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
			aircraft.Coord = carrier.Coord
		}
	}
}

func EnsureAviation(state *State) {
	for _, unit := range state.Units {
		if unit.Card.HangarCapacity <= 0 {
			unit.Deck = nil
			continue
		}
		length := unit.Card.DeckCapacity
		if length <= 0 {
			length = 1
		}
		count := unit.Card.ElevatorCount
		if count <= 0 {
			count = 1
		}
		if unit.Deck == nil {
			unit.Deck = &FlightDeck{Elevators: []*AircraftElevator{}}
		}
		if unit.Deck.Length <= 0 {
			unit.Deck.Length = length
		}
		// 基础容量单独存一份，实际甲板长度由风向与朝向推算（见 RefreshDeckLengths）。
		if unit.Deck.BaseLength <= 0 {
			unit.Deck.BaseLength = length
		}
		// Card.DeckCapacity 是卡面印刷的基础值，不跟着本回合风向改写；
		// 当前可用容量看 Deck.Length（航空面板/作业判定都用它）。
		// 每部可用升降机提供 4 算子的"提升+下降合计容量"（规则书 205、458 行）；
		// 非航母用吊车，一次 1 个。
		kind := "elevator"
		capacity := 4
		if !IsCarrier(unit) {
			kind = "crane"
			capacity = 1
		}
		damage := EnsureDamage(unit)
		for index := 0; index < count; index++ {
			id := fmt.Sprintf("%s-elevator-%d", unit.ID, index+1)
			if index >= len(unit.Deck.Elevators) {
				unit.Deck.Elevators = append(unit.Deck.Elevators, &AircraftElevator{ID: id, AircraftIDs: []string{}})
			}
			elevator := unit.Deck.Elevators[index]
			elevator.ID = id
			elevator.Kind = kind
			// 容量是设计值，不随战况变：航母每部 4 个算子、吊车 1 个。
			// 老存档里可能还是旧的 2，这里统一按规则纠正，否则"坏一部少 4 个名额"对不上。
			elevator.Capacity = capacity
			if elevator.Status == "" {
				elevator.Status = "idle"
			}
			if elevator.AircraftIDs == nil {
				elevator.AircraftIDs = []string{}
			}
			if index < damage.ElevatorDamage {
				elevator.Status = "damaged"
				elevator.TaskID = ""
				elevator.AircraftIDs = []string{}
			} else if elevator.Status == "damaged" {
				elevator.Status = "idle"
			}
		}
		unit.Card.ElevatorCount = len(unit.Deck.Elevators)
	}
	capPriorities := map[string]int{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskKind != "cap" {
			continue
		}
		carrier := state.Units[aircraft.CarrierID]
		if carrier == nil {
			continue
		}
		taskID := "cap-" + FleetKey(carrier)
		if priority := capPriorities[taskID]; priority == 0 || aircraft.Priority < priority {
			capPriorities[taskID] = aircraft.Priority
		}
	}
	for _, aircraft := range state.Aircraft {
		if aircraft.Status != "elevator" {
			aircraft.ElevatorID = ""
		}
		if aircraft.TaskKind == "cap" {
			carrier := state.Units[aircraft.CarrierID]
			if carrier != nil {
				taskID := "cap-" + FleetKey(carrier)
				aircraft.TaskID = taskID
				aircraft.Priority = capPriorities[taskID]
			}
		}
	}
	syncEmbarkedAircraft(state)
}

func AviationAvailable(unit *Unit) bool {
	if unit == nil || unit.Deck == nil {
		return false
	}
	damage := EnsureDamage(unit)
	// 甲板损伤只阻止"起飞/降落"本身，不阻止升降机调度：否则一次中弹就会让整艘航母永远无法整备，
	// 甲板修复永远等不到，机体还会卡在 service/elevator 状态里。
	// 火焰要分两种（本作规则把它们当两件事）：
	//   - 小型火灾：规则书 432 行"本回合无法进行航空作业"，整备也要停；
	//   - 起火燃烧：规则书 219 行只禁止"在上面起降"，整备和升降机调度照常。
	// 起降分别由 launchBlockReason / landingBlockReason 判定。
	if damage.Disabled || damage.Tilt || damage.DeckDestroyed || damage.Fire == "small" {
		return false
	}
	available := 0
	for _, elevator := range unit.Deck.Elevators {
		if elevator.Status != "damaged" {
			available++
		}
	}
	return available > 0
}

func airOrderIDs(order Order) []string {
	ids := append([]string(nil), order.AircraftIDs...)
	if len(ids) == 0 && order.AircraftID != "" {
		ids = []string{order.AircraftID}
	}
	sort.Strings(ids)
	return ids
}

func aircraftForOrder(state State, faction string, order Order) ([]*AirUnit, *Unit, error) {
	EnsureAviation(&state)
	ids := airOrderIDs(order)
	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("航空命令至少需要一个航空算子")
	}
	seen := map[string]bool{}
	aircraftList := make([]*AirUnit, 0, len(ids))
	carrierID := ""
	for _, id := range ids {
		if seen[id] {
			return nil, nil, fmt.Errorf("航空命令包含重复算子")
		}
		seen[id] = true
		aircraft := state.Aircraft[id]
		// 按任务下达的命令（改目标/改攻法/调优先级）：任务 ID 常常与首架飞机的 ID 相同，
		// 所以必须先按"任务"解释这个 ID，否则只会命中那一架飞机，命令对其他成员静默失效。
		if len(ids) == 1 && (order.Type == "PRIORITIZE_AIR_TASK" || order.Type == "SET_STRIKE_TARGET" || order.Type == "SET_STRIKE_DISPERSION") {
			for _, candidate := range sortedAirUnitIDs(state) {
				member := state.Aircraft[candidate]
				if member.TaskID == id && member.Faction == faction {
					aircraft = member
					break
				}
			}
		}
		if aircraft == nil || aircraft.Faction != faction {
			return nil, nil, fmt.Errorf("航空算子不存在或不属于当前玩家")
		}
		if carrierID == "" {
			carrierID = aircraft.CarrierID
		} else if order.Type != "LAUNCH_STRIKE" && order.Type != "LAUNCH_CAP" && order.Type != "LAUNCH_COVER" && order.Type != "RECOVER_AIR" && order.Type != "SET_AIR_CYCLE" && aircraft.CarrierID != carrierID {
			// 任务循环开关只改标志位，跨母舰一起点没有任何副作用；
			// 空战任务栏里同一型 CAP 本来就横跨翔鹤、瑞鹤两艘母舰，
			// 不放开这条的话"点一下取消整队循环"会被拒绝成"点了没反应"。
			return nil, nil, fmt.Errorf("该航空命令必须使用同一母舰的算子")
		}
		aircraftList = append(aircraftList, aircraft)
	}
	carrier := state.Units[carrierID]
	if carrier == nil || carrier.Faction != faction {
		return nil, nil, fmt.Errorf("母舰不存在")
	}
	// 编队级命令（改目标、改攻法、调优先级、开关循环、无线电召回）只改编队参数，
	// 不要求这条船现在能做航空作业：编队里可能有一条船甲板被打穿，但另一条船完好在天上。
	// 以前这里一刀切，结果是"翔鹤甲板被摧毁 → 连给瑞鹤的编队选目标都选不了"。
	// 同理，"收回机库"只是把甲板/待发的算子交给升降机，甲板炸了也能收。
	relaxed := groupLevelOrder(order.Type) || order.Type == "RETURN_TO_HANGAR"
	if !relaxed && !AviationAvailable(carrier) {
		// 把具体原因说出来：倾斜/起火/甲板摧毁以前一律报"损伤状态无法执行航空作业"，
		// 玩家不知道该等火灭还是这艘船已经废了。
		if reason := aviationBlockReason(state, carrier); reason != "" {
			return nil, nil, fmt.Errorf("%s%s", carrier.Name, reason)
		}
		return nil, nil, fmt.Errorf("%s当前无法执行航空作业", carrier.Name)
	}
	return aircraftList, carrier, nil
}

// groupLevelOrder 判断这条命令是不是"只改编队参数"的编队级命令。
func groupLevelOrder(orderType string) bool {
	switch orderType {
	case "SET_STRIKE_TARGET", "SET_STRIKE_DISPERSION", "PRIORITIZE_AIR_TASK",
		"SET_AIR_CYCLE", "RECALL_AIR", "RETASK_AIR", "RETARGET_AIR":
		return true
	}
	return false
}

func sameTaskAssignment(aircraftList []*AirUnit, order Order) bool {
	if order.TaskID == "" || (order.Type != "LAUNCH_RECON" && order.Type != "LAUNCH_STRIKE") {
		return false
	}
	taskKind := "recon"
	if order.Type == "LAUNCH_STRIKE" {
		taskKind = "attack"
	}
	for _, aircraft := range aircraftList {
		if aircraft.TaskID != order.TaskID || aircraft.TaskKind != taskKind {
			return false
		}
	}
	return len(aircraftList) > 0
}

// nightBlockedAirOrder 判断这条命令是不是"入夜后安排航空任务"。
// 19:40 回收截止到次日 04:00 整备开始之间，所有航空算子都应该在机库里，
// 04:00 之后（包括 04:00–05:20 这段整备期）才允许重新分配，所以这里按整备窗口判定，
// 不能按"能不能起飞"判——那样 04:00 到 05:20 之间会被误拦。
func nightBlockedAirOrder(orderType string) bool {
	switch orderType {
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_COVER", "LAUNCH_TRANSFER", "RETASK_AIR", "LIFT_TO_DECK":
		return true
	}
	return false
}

// validateCarrierTransferRange 校验舰载机转场的目的地距离。
//
// 规则只要求"起飞那一刻够得着"：这里按当时的母舰位置查一次，
// 已经在天上的算子直接跳过——目标舰队之后跑远了，飞机照样按原目标飞，
// 燃油照烧，不再复检。
func validateCarrierTransferRange(state State, aircraftList []*AirUnit, carrier *Unit, coord string) error {
	if carrier == nil || coord == "" || !inBounds(state.Map, coord) {
		return nil
	}
	// 转场到机场也走同一套容量检查（机场默认不限，只有显式配了上限的小机场会拒绝）。
	if dest := friendlyAirfieldAtCoord(state, carrier.Faction, coord); dest != nil {
		incoming := 0
		for _, aircraft := range aircraftList {
			if aircraft != nil && !airborne(aircraft) {
				incoming++
			}
		}
		if err := airfieldHangarFull(state, dest, incoming); err != nil {
			return err
		}
	}
	need := Distance(carrier.Coord, coord)
	for _, aircraft := range aircraftList {
		if aircraft == nil || airborne(aircraft) {
			continue
		}
		limit := SortieBaseRange(aircraft.BaseRange, LoadoutTransfer)
		if need > limit {
			return fmt.Errorf("%s转场航程%d hex（副油箱+转场加成），到%s需要%d hex", aircraft.Model, limit, coord, need)
		}
	}
	return nil
}

func ValidateAirOrder(state State, faction string, order Order) error {
	if !state.ServiceWindowOpen() && nightBlockedAirOrder(order.Type) {
		return fmt.Errorf("夜间无法执行航空任务")
	}
	aircraftList, carrier, err := aircraftForOrder(state, faction, order)
	if err != nil {
		return err
	}
	switch order.Type {
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_COVER", "LAUNCH_TRANSFER":
		if sameTaskAssignment(aircraftList, order) {
			return nil
		}
		status := aircraftList[0].Status
		if status != "hangar" && status != "service" && status != "deck" && status != "ready" {
			return fmt.Errorf("只有机库、整备中或已完成提升的算子可以分配任务")
		}
		definitionID := aircraftList[0].DefinitionID
		fleetID := FleetKey(carrier)
		for _, aircraft := range aircraftList {
			if aircraft.Status != status {
				return fmt.Errorf("同批分配的算子必须处于相同作业阶段")
			}
			if aircraft.Damaged || aircraft.Maintenance > 0 {
				return fmt.Errorf("损伤或检修中的算子必须完成修复后才能分配任务")
			}
			if aircraft.TaskID != "" {
				return fmt.Errorf("算子已经属于其他任务，请先取消原任务")
			}
			ship := state.Units[aircraft.CarrierID]
			if ship == nil || ship.Faction != faction || !AviationAvailable(ship) {
				return fmt.Errorf("航空算子的母舰损伤状态无法执行航空作业")
			}
			if order.Type != "LAUNCH_STRIKE" && order.Type != "LAUNCH_TRANSFER" && aircraft.DefinitionID != definitionID {
				return fmt.Errorf("CAP和侦察任务必须使用同型算子")
			}
			if order.Type == "LAUNCH_CAP" && aircraft.Kind != "fighter" {
				return fmt.Errorf("只有战斗机可以执行CAP")
			}
			if order.Type == "LAUNCH_COVER" && aircraft.Kind != "fighter" {
				return fmt.Errorf("空中掩护只能由战斗机执行")
			}
			if order.Type == "LAUNCH_TRANSFER" && aircraft.Seaplane {
				return fmt.Errorf("水上侦察机不能转场到其他航母")
			}
			if order.Type == "LAUNCH_RECON" && !canSearch(*aircraft) {
				return fmt.Errorf("战斗机不能执行扇面搜索，TBF、SB2C和侦察机可以执行搜索")
			}
			if (order.Type == "LAUNCH_CAP" || order.Type == "LAUNCH_COVER") && FleetKey(ship) != fleetID {
				return fmt.Errorf("CAP和空中掩护的母舰必须属于同一舰队")
			}
			if order.Type == "LAUNCH_STRIKE" {
				if aircraft.Seaplane || (aircraft.Kind != "dive_bomber" && aircraft.Kind != "torpedo_bomber" && aircraft.Kind != "fighter") {
					return fmt.Errorf("攻击编队只能由舰载轰炸机和护航战斗机组成")
				}
				if FleetKey(ship) != fleetID {
					return fmt.Errorf("攻击编队的母舰必须属于同一舰队")
				}
			}
		}
		if order.Type == "LAUNCH_CAP" && !IsCarrier(carrier) {
			return fmt.Errorf("CAP舰载机只能从航母起飞")
		}
		if order.Type == "LAUNCH_COVER" && !IsCarrier(carrier) {
			return fmt.Errorf("空中掩护只能由航母舰载战斗机执行")
		}
		// 掩护 hex / 转场目标可以先空着：编队先在甲板整备，等玩家在地图上点目标。
		if order.Coord != "" && !inBounds(state.Map, order.Coord) {
			return fmt.Errorf("目标 hex 不在地图内")
		}
		if order.Type == "LAUNCH_RECON" {
			// 加入已有侦察组时航向/扇面/半径全部继承原组（API.md 的约定），
			// 客户端只提交 task_id 即可，不必重复抄一遍参数。
			joiningExisting := order.TaskID != "" && len(taskAircraft(state, order.TaskID)) > 0
			if !joiningExisting {
				order.Heading = strings.ToUpper(order.Heading)
				if !ValidReconHeading(order.Heading) {
					return fmt.Errorf("侦察航向必须是E、SE、SW、W、NW或NE")
				}
				if order.SectorWidth != 60 && order.SectorWidth != 120 {
					return fmt.Errorf("侦察扇面只能选择60度或120度")
				}
			}
			// 侦察半径由机型航程决定：命令里没有这个字段，客户端多传也只是被 json 解码忽略。
			if !aircraftList[0].Seaplane && !IsCarrier(carrier) {
				return fmt.Errorf("舰载机只能从航母起飞")
			}
		}
		if order.TaskID != "" {
			members := taskAircraft(state, order.TaskID)
			if len(members) == 0 || members[0].Faction != faction {
				return fmt.Errorf("要加入的航空任务不存在")
			}
			if members[0].TaskKind != "recon" && members[0].TaskKind != "attack" {
				return fmt.Errorf("只有侦察组和攻击编队可以接受新算子")
			}
			if order.Type == "LAUNCH_RECON" && members[0].TaskKind != "recon" {
				return fmt.Errorf("要加入的侦察组不存在")
			}
			if order.Type == "LAUNCH_RECON" && FleetKey(state.Units[members[0].CarrierID]) != fleetID {
				return fmt.Errorf("只能加入同一舰队的侦察组")
			}
			if order.Type == "LAUNCH_STRIKE" {
				if members[0].TaskKind != "attack" {
					return fmt.Errorf("要加入的攻击编队不存在")
				}
				if FleetKey(state.Units[members[0].CarrierID]) != fleetID {
					return fmt.Errorf("只能加入同一舰队的攻击编队")
				}
				for _, member := range members {
					if member.Status == "attack" {
						return fmt.Errorf("攻击编队已经离场，不能继续加入飞机")
					}
				}
			}
			if order.Type == "LAUNCH_CAP" {
				return fmt.Errorf("CAP算子各自独立，不能加入侦察组或攻击编队")
			}
		} else if order.Type == "LAUNCH_RECON" {
			// 只数"还在用"的组：已经全部落地/下降中的旧组不该继续占名额，
			// 否则打完一轮就再也开不了新组，玩家会觉得"侦察任务不能换了"。
			active := map[string][]*AirUnit{}
			for _, aircraft := range state.Aircraft {
				ship := state.Units[aircraft.CarrierID]
				if aircraft.Faction == faction && ship != nil && FleetKey(ship) == fleetID && aircraft.TaskKind == "recon" && aircraft.TaskID != "" {
					active[aircraft.TaskID] = append(active[aircraft.TaskID], aircraft)
				}
			}
			groups := 0
			for _, members := range active {
				if reconGroupActive(members) {
					groups++
				}
			}
			if groups >= 2 {
				return fmt.Errorf("最多同时保留两个侦察组，请把飞机加入已有任务栏")
			}
		}
		if order.Type == "LAUNCH_STRIKE" && order.TaskID == "" {
			if order.TargetFleetID != "" {
				if err := ValidateStrikeFleetTarget(state, faction, order.TargetFleetID, order.Coord); err != nil {
					return err
				}
			} else if order.Coord != "" {
				if err := ValidateStrikeTarget(state, faction, order.Coord); err != nil {
					return err
				}
			}
		}
		if order.Type == "LAUNCH_TRANSFER" {
			if err := validateCarrierTransferRange(state, aircraftList, carrier, order.Coord); err != nil {
				return err
			}
		}
		if status == "deck" || status == "ready" {
			// 甲板上还没整备完的算子（空载、或者刚换过挂载）不立刻放飞：
			// 它在甲板上原地整备一轮，整备完的下一回合才开始受放飞时间窗和容量限制。
			ready := true
			for _, aircraft := range aircraftList {
				if !aircraftReadyToLaunch(aircraft) {
					ready = false
					break
				}
			}
			if !ready {
				return nil
			}
			// 甲板/待发算子会立刻起飞，同样受剧本的放飞时间窗限制。
			if !state.LaunchWindowOpen() {
				return fmt.Errorf("%s", state.launchWindowHint())
			}
			return validateLaunchCapacity(state, aircraftList)
		}
		return nil
	case "SET_STRIKE_TARGET":
		if len(aircraftList) != 1 {
			return fmt.Errorf("只有编队可以设置目标")
		}
		switch aircraftList[0].TaskKind {
		case "attack":
			if order.TargetFleetID != "" {
				if err := ValidateStrikeFleetTarget(state, faction, order.TargetFleetID, order.Coord); err != nil {
					return err
				}
			} else if err := ValidateStrikeTarget(state, faction, order.Coord); err != nil {
				return err
			}
			// 改目标不自动换挂载：挂鱼雷去打机场就是对地值 0，投不出弹。要换弹走 SET_LOADOUT。
			return nil
		case "cover", "transfer":
			// 空中掩护改掩护 hex、转场改降落目标：只要求是地图内合法 hex。
			// 降不下去时玩家就是靠这条命令把编队改到另一支舰队。
			if order.Coord == "" || !inBounds(state.Map, order.Coord) {
				return fmt.Errorf("目标 hex 无效")
			}
			if aircraftList[0].TaskKind == "transfer" {
				// 还没起飞时按母舰位置查一次航程；已经在天上的不再复检。
				task := taskAircraft(state, aircraftList[0].TaskID)
				return validateCarrierTransferRange(state, task, carrier, order.Coord)
			}
			return nil
		default:
			return fmt.Errorf("只有攻击编队、空中掩护或转场可以设置目标")
		}
	case "SET_STRIKE_DISPERSION":
		// 任何一架编队成员都能代表整个编队设置集中/分散攻击（规则书 624 行）。
		for _, aircraft := range aircraftList {
			if aircraft.TaskKind != "attack" {
				return fmt.Errorf("只有攻击编队可以设置攻击方式")
			}
		}
		return nil
	case "LIFT_TO_DECK":
		// 机库 → 甲板栏：不是派任务，是把算子送上甲板待命（可以空载上去再甲板整备）。
		for _, aircraft := range aircraftList {
			if aircraft.DeckService {
				return fmt.Errorf("%s正在甲板上整备，等这一轮整备完成再操作", aircraft.Model)
			}
			switch aircraft.Status {
			case "hangar", "service":
			case "deck", "ready":
				return fmt.Errorf("%s已经在甲板上", aircraft.Model)
			case "elevator":
				return fmt.Errorf("%s正在升降机上", aircraft.Model)
			default:
				return fmt.Errorf("%s当前状态（%s）不能提升到甲板", aircraft.Model, aircraft.Status)
			}
			if aircraft.Damaged || aircraft.Maintenance > 0 {
				return fmt.Errorf("损伤或检修中的算子必须完成修复后才能提升到甲板")
			}
		}
		return nil
	case "PREPARE_AIR":
		return fmt.Errorf("整备和提升必须绑定航空任务，请把飞机拖入任务栏")
	case "RETURN_TO_HANGAR":
		for _, aircraft := range aircraftList {
			if airborne(aircraft) {
				return fmt.Errorf("%s正在执行任务，不能直接撤回：请等它完成任务返航，或对编队下达召回", aircraft.Model)
			}
			switch aircraft.Status {
			case "service", "deck", "ready":
			case "elevator":
				return fmt.Errorf("%s正在升降机上，不能中途撤回", aircraft.Model)
			case "recovery":
				return fmt.Errorf("%s刚降落、正在等待下降，请等这一轮回收完成", aircraft.Model)
			default:
				return fmt.Errorf("%s当前状态（%s）不能撤回机库", aircraft.Model, aircraft.Status)
			}
			// 甲板本回合在做起飞/回收，不影响把飞机"下降回机库"（那是升降机的活）。
			// 以前这里一刀切拒绝，表现就是"甲板上有飞机，但拖不回机库"。
		}
		return nil
	case "RECOVER_AIR":
		counts := map[string]int{}
		for _, aircraft := range aircraftList {
			if aircraft.Status != "cap" {
				return fmt.Errorf("只有空中的CAP算子可以主动回收，侦察和攻击算子需要完成任务后返航")
			}
			counts[aircraft.CarrierID]++
		}
		for carrierID, count := range counts {
			ship := state.Units[carrierID]
			if ship == nil || ship.Deck == nil || ship.Deck.Operation != "" {
				return fmt.Errorf("回收母舰甲板本回合已经进行其他作业，不能再降落")
			}
			if !AviationAvailable(ship) {
				return fmt.Errorf("回收母舰损伤状态无法降落")
			}
			if reason := landingBlockReason(state, ship); reason != "" && !strings.HasPrefix(reason, "甲板已满") {
				return fmt.Errorf("%s%s", ship.Name, reason)
			}
			if deckUsed(state, carrierID)+count > ship.Deck.Length {
				return fmt.Errorf("%s的飞行甲板没有足够空间接收降落算子", ship.Name)
			}
		}
		return nil
	case "PRIORITIZE_AIR_TASK":
		if len(aircraftList) != 1 || aircraftList[0].TaskID == "" {
			return fmt.Errorf("只有已分配的航空任务可以调整优先级")
		}
		return nil
	case "SET_AIR_CYCLE":
		// 规则书 191 行：点击算子开启/关闭这台（或这一批）飞机的自动任务循环。
		for _, aircraft := range aircraftList {
			kind := aircraft.TaskKind
			if kind == "" {
				kind = aircraft.CycleKind
			}
			if kind == "" && aircraft.Status == "cap" {
				kind = "cap"
			}
			if !order.Cycle {
				// 关循环任何时候都可以：在空中关掉就是"降落后不再自动再来一轮"。
				continue
			}
			if !cyclicTaskKind(kind) {
				return fmt.Errorf("%s没有可循环的任务（只有CAP、侦察和空中掩护支持自动循环）", aircraft.Model)
			}
		}
		return nil
	default:
		return fmt.Errorf("不支持的航空命令")
	}
}

func deckUsed(state State, carrierID string) int {
	used := 0
	for _, aircraft := range state.Aircraft {
		if aircraft.CarrierID != carrierID {
			continue
		}
		// 甲板上换弹/挂弹的算子（status service + DeckService）同样占甲板位：
		// 它就在甲板上作业，不能一边占位一边不计入容量。
		if aircraft.Status == "deck" || aircraft.Status == "ready" || aircraft.Status == "recovery" || (aircraft.Status == "service" && aircraft.DeckService) {
			used++
		}
	}
	return used
}

// launchBlockReason 返回这艘航空舰本回合不能放飞的原因（空串表示可以放飞）。
// 起火/甲板损伤只挡起降、不挡整备；玩家看到的提示必须和实际被拦下的动作一致。
func launchBlockReason(state State, carrier *Unit) string {
	if carrier == nil || carrier.Deck == nil || carrier.Deck.Length <= 0 {
		return "没有可用飞行甲板"
	}
	damage := EnsureDamage(carrier)
	switch {
	case damage.Disabled:
		return "失去动力，无法起飞"
	case damage.Tilt:
		return "舰体倾斜，暂停起降作业"
	case damage.Fire != "":
		return "舰上起火，暂停起降作业"
	case damage.DeckDestroyed:
		return "飞行甲板已被摧毁，无法起飞"
	case damage.DeckDamaged:
		return "飞行甲板受损，本回合无法起飞"
	case carrier.Deck.Operation == "recover":
		return "正在回收飞机，不能同时起飞"
	}
	return ""
}

// launchCapacityFor 返回某艘航母本回合还能放飞几架（已扣除本回合已放飞的和正在回收的）。
func launchCapacityFor(state State, carrierID string) int {
	carrier := state.Units[carrierID]
	if carrier == nil || carrier.Deck == nil || carrier.Deck.Length <= 0 {
		return 0
	}
	if !AviationAvailable(carrier) || launchBlockReason(state, carrier) != "" {
		return 0
	}
	used := 0
	for _, aircraft := range state.Aircraft {
		if aircraft.CarrierID == carrierID && aircraft.Launched {
			used++
		}
	}
	capacity := carrier.Deck.Length - used
	if capacity < 0 {
		return 0
	}
	return capacity
}

func validateLaunchCapacity(state State, aircraftList []*AirUnit) error {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range aircraftList {
		groups[aircraft.CarrierID] = append(groups[aircraft.CarrierID], aircraft)
	}
	for carrierID, group := range groups {
		carrier := state.Units[carrierID]
		if carrier == nil || carrier.Deck == nil || carrier.Deck.Length <= 0 || !AviationAvailable(carrier) {
			if reason := aviationBlockReason(state, carrier); reason != "" {
				return fmt.Errorf("%s%s", carrier.Name, reason)
			}
			return fmt.Errorf("母舰没有可用甲板或航空设施受损")
		}
		if reason := launchBlockReason(state, carrier); reason != "" {
			return fmt.Errorf("%s%s", carrier.Name, reason)
		}
		used := 0
		for _, aircraft := range state.Aircraft {
			// 只统计"本回合已经放飞"的算子：甲板长度限制的是单回合起飞架次，
			// 已经在空中的编队不占本回合的起飞位。
			if aircraft.CarrierID == carrierID && aircraft.Launched {
				used++
			}
		}
		if used+len(group) > carrier.Deck.Length {
			return fmt.Errorf("%s本回合甲板起飞容量不足", carrier.Name)
		}
	}
	return nil
}

// formationLabel 用各机型数量描述一支编队，例如"B5N2 九七舰攻×5、A6M2 零战×2"。
// 战报以前统一写领队机型（"放飞7个A6M2 零战"），实际是混合编队，按机型统计会直接算错。
func formationLabel(members []*AirUnit) string {
	counts := map[string]int{}
	order := []string{}
	for _, aircraft := range members {
		if aircraft == nil {
			continue
		}
		if _, seen := counts[aircraft.Model]; !seen {
			order = append(order, aircraft.Model)
		}
		counts[aircraft.Model]++
	}
	if len(order) == 0 {
		return "编队"
	}
	if len(order) == 1 {
		return order[0]
	}
	parts := make([]string, 0, len(order))
	for _, model := range order {
		parts = append(parts, fmt.Sprintf("%s×%d", model, counts[model]))
	}
	return strings.Join(parts, "、")
}

func nextAirTaskPriority(state *State, faction string) int {
	priority := 1
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == faction && aircraft.TaskID != "" && aircraft.Priority >= priority {
			priority = aircraft.Priority + 1
		}
	}
	return priority
}

// newFlightTag 生成一张编队牌的 tag（形如 air-9f3c1a7b2d4e）。
//
// 拆组不是"又开了一个任务"：一次 100 架的超远程打击，沿途航程不足的算子会陆续
// 掉队，一路能拆出几十上百张牌子。序号在这种规模下既不代表顺序、也不代表任务，
// 反过来还会让人以为"拆组 = 新开一次任务"。牌子只需要一个唯一标识——
// 服务端拿它当分组键，客户端拿它当 key——所以这里用随机 tag。
// 随机源用 crypto/rand，不碰战斗判定的 RNG 流，重放/回放的对局结果不受影响。
// CAP 是例外：它的 tag 按母舰/机场固定（cap-<舰队>、af-<机场>-cap），
// 因为"同一支舰队的 CAP"在规则上就是同一个持续任务，见 EnsureAviation。
func newFlightTag(state *State) string {
	for attempt := 0; attempt < 64; attempt++ {
		tag := randomFlightTag(attempt)
		if state == nil {
			return tag
		}
		if len(taskAircraft(*state, tag)) == 0 && len(groundTaskAircraft(*state, tag)) == 0 {
			return tag
		}
	}
	return randomFlightTag(64)
}

func randomFlightTag(attempt int) string {
	var buf [6]byte
	if _, err := crand.Read(buf[:]); err == nil {
		return "air-" + hex.EncodeToString(buf[:])
	}
	// crypto/rand 不可用（几乎只发生在极端环境）：退回时间戳，仍然保证唯一。
	return fmt.Sprintf("air-%x%02x", time.Now().UnixNano(), attempt)
}

func clearAirTask(aircraft *AirUnit) {
	aircraft.Priority = 0
	aircraft.TaskID = ""
	aircraft.TaskKind = ""
	// 栏位属于这次组队，任务结束就交还，下一支新编队可以重新拿回 I。
	aircraft.TaskLane = 0
	aircraft.OriginCoord = ""
	aircraft.Coord = ""
	aircraft.TargetCoord = ""
	aircraft.TargetFleetID = ""
	aircraft.Heading = ""
	aircraft.SectorWidth = 0
	aircraft.ReconRadius = 0
	aircraft.ReconDistance = 0
	aircraft.Returning = false
	aircraft.TrackingFleetID = ""
	// 接敌标记和搜索状态属于"上一轮任务"：换任务、落舰、重新编队之后必须清掉。
	// 否则重新出击的编队会带着 Engaged=true 离场，一旦进入 attack 状态就在
	// 十几格外直接对旧目标结算一次空袭（凭空打击），或者反过来永远不再接敌。
	aircraft.Engaged = false
	aircraft.SearchTurns = 0
	aircraft.SearchHeading = ""
}

// taskLaneCount 是面板里同类任务分栏的数量：打击任务 I、II（侦察也分两栏）。
const taskLaneCount = 2

// freeTaskLane 从占用表里挑栏位（1 起）：玩家指定了空栏就照他的来（拖进 II 就放 II），
// 没指定或者那一栏已经被占，就挑最小空位。
// "占用"只算还在组队的编队：编队全部升空以后栏位就空出来，下一支新编队重新拿到 I。
// 两栏都占着时退回 1——面板那一栏会并列列出两支编队，总比把新编队藏起来强。
func freeTaskLane(occupied map[int]bool, prefer int) int {
	if prefer >= 1 && prefer <= taskLaneCount && !occupied[prefer] {
		return prefer
	}
	for lane := 1; lane <= taskLaneCount; lane++ {
		if !occupied[lane] {
			return lane
		}
	}
	return 1
}

// aircraftFleetKey 取舰载机所在舰队的 key。
func aircraftFleetKey(state State, aircraft *AirUnit) string {
	if aircraft == nil {
		return ""
	}
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		return FleetKey(carrier)
	}
	return ""
}

// carrierTaskLanes 汇总同一支舰队里还在组队的同类任务占的栏位。
func carrierTaskLanes(state State, fleetID, kind string) map[int]bool {
	occupied := map[int]bool{}
	if fleetID == "" {
		return occupied
	}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskLane <= 0 || aircraft.TaskID == "" || aircraft.TaskKind != kind {
			continue
		}
		// 已经在天上的不占栏位：编队离场之后面板该把这一栏空出来。
		if airborne(aircraft) {
			continue
		}
		if aircraftFleetKey(state, aircraft) != fleetID {
			continue
		}
		occupied[aircraft.TaskLane] = true
	}
	return occupied
}

// assignCarrierTaskLane 给一组舰载机定栏位：新编队拿最小空位，加入已有编队沿用原栏位。
// 老存档里没记过栏位（0）的编队现场补一个并同步给全组，避免成员之间对不上。
func assignCarrierTaskLane(state *State, aircraftList []*AirUnit, existingTask []*AirUnit, kind string, prefer int) int {
	lane := 0
	if len(existingTask) > 0 && existingTask[0].TaskLane > 0 {
		lane = existingTask[0].TaskLane
	} else {
		scope := aircraftList[0]
		if len(existingTask) > 0 {
			scope = existingTask[0]
		}
		lane = freeTaskLane(carrierTaskLanes(*state, aircraftFleetKey(*state, scope), kind), prefer)
		for _, member := range existingTask {
			member.TaskLane = lane
		}
	}
	for _, aircraft := range aircraftList {
		aircraft.TaskLane = lane
	}
	return lane
}

func configureAirTask(state *State, aircraftList []*AirUnit, order Order) []EngineEvent {
	taskID := order.TaskID
	priority := 0
	var existingTask []*AirUnit
	kind := missionKindFromOrder(order.Type)
	if taskID == "" && order.Type == "LAUNCH_CAP" {
		// CAP 的 tag 按舰队固定：同一支舰队的 CAP 是一个持续任务，不能每次新建随机 tag。
		taskID = "cap-" + FleetKey(state.Units[aircraftList[0].CarrierID])
	}
	if taskID == "" {
		taskID = newFlightTag(state)
		priority = nextAirTaskPriority(state, aircraftList[0].Faction)
	} else {
		existingTask = taskAircraft(*state, taskID)
		if len(existingTask) > 0 {
			priority = existingTask[0].Priority
		} else {
			priority = nextAirTaskPriority(state, aircraftList[0].Faction)
		}
	}
	if kind == MissionRecon && len(existingTask) > 0 && reconGroupActive(existingTask) {
		// 只有"这组还在用"、且本次命令没写参数时才继承。
		// 玩家明确指定了航向/扇面就照他写的来——以前无条件覆盖，
		// 于是拖进已有组之后航向再也改不了，只能把整组取消重来。
		explicitHeading := ValidReconHeading(order.Heading)
		explicitSector := order.SectorWidth == 60 || order.SectorWidth == 120
		if !explicitHeading {
			order.Heading = existingTask[0].Heading
		}
		if !explicitSector {
			order.SectorWidth = existingTask[0].SectorWidth
		}
		// 玩家明确改方向时，整组还没起飞的成员一起改——否则组里两架飞机朝两个方向，
		// 界面写着"W"，实际有一半在往 E 飞。已经在天上的保持原航向（那一段已经飞出去了）。
		if explicitHeading || explicitSector {
			for _, member := range existingTask {
				if airborne(member) {
					continue
				}
				heading, sector := member.Heading, member.SectorWidth
				if explicitHeading {
					heading = order.Heading
				}
				if explicitSector {
					sector = order.SectorWidth
				}
				assignReconMission(member, heading, sector)
			}
		}
	}
	if kind == MissionStrike && len(existingTask) > 0 {
		order.Coord = existingTask[0].TargetCoord
	}

	mismatch := []string{}
	seenMismatch := map[string]bool{}
	for _, aircraft := range aircraftList {
		// 先在接单前记下"这架是不是已经整备好、挂的什么"：beginAirMission 会把整备进度清零，
		// 但玩家已经在机库里整备好的算子再拖进任务栏，不该又白等一轮。
		wasReady := serviceComplete(aircraft) && aircraft.Loadout != LoadoutNone
		prevLoadout := aircraft.Loadout
		prevServiceTurns, prevArmed := aircraft.ServiceTurns, aircraft.Armed
		beginAirMission(aircraft, taskID, kind, priority)
		switch kind {
		case MissionRecon:
			// 侦察距离不可设置：一律飞到这型飞机的最大搜索半径。
			// 玩家要决定的是"搜哪个方向、多宽的扇面"，不是"飞多远"。
			assignReconMission(aircraft, order.Heading, order.SectorWidth)
		case MissionStrike:
			assignStrikeMission(state, aircraft, order.Coord)
			if order.TargetFleetID != "" {
				aircraft.TargetFleetID = order.TargetFleetID
				if coord, ok := strikeFleetCoord(*state, aircraft.Faction, order.TargetFleetID); ok {
					aircraft.TargetCoord = coord
				}
			}
		case MissionCover:
			assignCoverMission(aircraft, order.Coord)
			aircraft.CoverUnitID = friendlyUnitIDAt(*state, aircraft.Faction, order.Coord)
		case MissionTransfer:
			assignTransferMission(aircraft, order.Coord)
			aircraft.TargetFleetID = ""
			if order.Coord != "" {
				// 目标是舰队时记下舰队 ID，飞行途中按舰队当前位置追。
				if id := order.TargetFleetID; id != "" {
					if _, ok := friendlyFleetCoord(*state, aircraft.Faction, id); ok {
						aircraft.TargetFleetID = id
					}
				} else {
					aircraft.TargetFleetID = friendlyFleetIDAt(*state, aircraft.Faction, order.Coord)
				}
			}
		default:
			// CAP：巡逻点留空表示"跟着母舰走"，capCoord 逐回合回落到母舰当前 hex。
			assignCapMission(aircraft, aircraft.PatrolCoord)
		}
		// 拖进任务栏先校验现有挂载：
		//   - 空载：按任务自动配弹并走整备流程（下面 applyLoadout）；
		//   - 已经有挂载、但干不了这个任务（鱼雷打岸上、挂炸弹做 CAP/转场）：保留原挂载和
		//     整备进度，只弹一条提示，命令照收。
		if kind == MissionTransfer {
			if notice := transferLoadoutNotice(aircraft.Loadout); notice != "" {
				entry := aircraft.Model + "：" + notice
				if !seenMismatch[entry] {
					seenMismatch[entry] = true
					mismatch = append(mismatch, entry)
				}
			}
		}
		if reason := loadoutTaskMismatch(aircraft.Kind, string(kind), taskTargetKind(*state, aircraft), aircraft.Loadout); reason != "" {
			aircraft.ServiceTurns = prevServiceTurns
			aircraft.Armed = prevArmed
			entry := aircraft.Model + "：" + reason
			if !seenMismatch[entry] {
				seenMismatch[entry] = true
				mismatch = append(mismatch, entry)
			}
			continue
		}
		// 按任务换挂载：转场加航程、鱼雷机挂鱼雷、水平轰炸挂炸弹（规则书 43–63 行）。
		// 目标 hex 上有敌方机场即视为对地攻击（影响默认挂载选择）。
		applyLoadout(*state, aircraft)
		// 挂载没被任务改掉、而且本来就整备完了：沿用它，直接从提升开始。
		if wasReady && aircraft.Loadout == prevLoadout && !aircraft.Damaged && aircraft.Maintenance == 0 {
			aircraft.ServiceTurns = 1
		}
	}
	// 栏位要在 beginAirMission 之后写：它内部会 clearAirTask，而清任务会把栏位交还。
	// 面板栏位（打击任务 I/II）跟着编队走，不按任务在列表里的位置轮排——
	// 位置会随"已出发的编队还挂在列表里"和随机 tag 的排序一起漂。
	assignCarrierTaskLane(state, aircraftList, existingTask, string(kind), order.Lane)
	if len(mismatch) == 0 {
		return nil
	}
	sort.Strings(mismatch)
	return []EngineEvent{{Type: "air_loadout_mismatch", Audience: aircraftList[0].Faction, Text: strikeLoadoutMismatchHint(mismatch)}}
}

// reconGroupActive 判断一个侦察组还在不在"用"：有飞机在天上，或者还在整备/甲板上等起飞。
// 全部落回机库、或者正在下降的组已经打完这一轮，不该再接管新的航向参数。
func reconGroupActive(members []*AirUnit) bool {
	for _, aircraft := range members {
		if aircraft == nil {
			continue
		}
		if airborne(aircraft) {
			return true
		}
		switch aircraft.Status {
		case "service", "deck", "ready":
			return true
		}
	}
	return false
}

func taskAircraft(state State, taskID string) []*AirUnit {
	members := []*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskID == taskID {
			members = append(members, aircraft)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members
}

func airborne(aircraft *AirUnit) bool {
	switch aircraft.Status {
	case "cap", "recon", "attack_holding", "attack", "cover", "transfer":
		return true
	}
	return false
}

// firstAirborneMember 挑出组里第一个还在空中的算子（列表已按 ID 排序）。
// 编队牌的位置、"返航中"状态都要以它为准：ID 最小的那一架可能已经落地/回到甲板，
// 拿它当坐标源会让整支还在天上的编队挂到地面格子上，甚至和返航队显示成同一种状态。
func firstAirborneMember(list []*AirUnit) *AirUnit {
	for _, aircraft := range list {
		if airborne(aircraft) {
			return aircraft
		}
	}
	return nil
}

func updateAttackFormation(state *State, taskID string) []EngineEvent {
	members := taskAircraft(*state, taskID)
	if len(members) == 0 {
		return nil
	}
	// 集结判定只认 attack_holding：一架已经在往目标飞的成员说明编队早就出发了，
	// 这时再有飞机在母舰上空等待，只能算另一波（等它自己落地/重新组队），
	// 不能反过来把等待的这架也推成 attack 让它单独飞过去——那是分批出击。
	ready := false
	airborneCount := 0
	for _, aircraft := range members {
		if airborne(aircraft) {
			airborneCount++
		}
		// 返航中的成员不参与"到齐没有"的判定：它们自己往母舰飞，编队照常出发。
		if aircraft.Returning {
			continue
		}
		if aircraft.Status != "attack_holding" {
			ready = false
			break
		}
		ready = true
	}
	if !ready {
		// 已经有成员在攻击状态（编队出发过了）时不再重新集结：剩下的等待成员
		// 属于"没赶上这一波"，等它们自己落地/整备，或者玩家另开编队。
		for _, aircraft := range members {
			if aircraft.Status == "attack" {
				return nil
			}
		}
		return []EngineEvent{{Type: "attack_assembling", Audience: members[0].Faction, Text: fmt.Sprintf("攻击编队%d/%d个算子已升空，先起飞的飞机在舰队上空等待", airborneCount, len(members))}}
	}
	if members[0].TargetCoord == "" {
		return []EngineEvent{{Type: "attack_waiting_target", Audience: members[0].Faction, Text: "攻击编队已全部升空，等待选择目标"}}
	}
	for _, aircraft := range members {
		if aircraft.Returning {
			continue
		}
		aircraft.Status = "attack"
		// 离场这一刻是"新一轮出击"的起点：整备期间攒下的接敌/搜索状态必须清掉。
		// 上一轮打击留下的 Engaged=true 如果带出去，编队会在还没飞到的位置就按目标 hex
		// 结算一次空袭（凭空打击），或者卡在"已接敌但不推进"。
		aircraft.Engaged = false
		aircraft.SearchTurns = 0
		aircraft.SearchHeading = ""
	}
	return []EngineEvent{{Type: "attack_departed", Audience: members[0].Faction, Text: fmt.Sprintf("攻击编队%d个算子全部到齐，编队离开舰队", len(members))}}
}

// strandedMembers 找编队里"永远上不了甲板"的成员：还在机库/整备，而母舰已经不能做航空作业。
func strandedMembers(state *State, members []*AirUnit) []*AirUnit {
	stranded := []*AirUnit{}
	for _, member := range members {
		if member == nil || airborne(member) {
			continue
		}
		switch member.Status {
		case "service", "hangar", "elevator", "deck", "ready":
		default:
			continue
		}
		carrier := state.Units[member.CarrierID]
		if carrier == nil || !AviationAvailable(carrier) || carrier.Deck == nil {
			stranded = append(stranded, member)
		}
	}
	return stranded
}

// departReadyAttacks 检查所有攻击编队：全员已升空（attack_holding）且有目标时立即离场。
func departReadyAttacks(state *State) []EngineEvent {
	events := []EngineEvent{}
	seen := map[string]bool{}
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.TaskID == "" || aircraft.TaskKind != "attack" {
			continue
		}
		if seen[aircraft.TaskID] {
			continue
		}
		seen[aircraft.TaskID] = true
		// 先把任务 ID 记下来：下面剔除"永远出不了场"的成员时，可能正好清掉当前这架的 TaskID，
		// 之后再读 aircraft.TaskID 就变成空串，编队永远发不出去。
		taskID := aircraft.TaskID
		members := taskAircraft(*state, taskID)
		if len(members) == 0 || members[0].TargetCoord == "" {
			continue
		}
		// 编队里如果有成员永远上不了甲板（母舰沉没、甲板被摧毁），
		// 整组会一直等它、永远不离场。这里把它们剔出编队并说明原因，
		// 让剩下的飞机照常出击——不然玩家看到的就是"攻击编队一直不出击"。
		if stranded := strandedMembers(state, members); len(stranded) > 0 {
			for _, member := range stranded {
				clearAirTask(member)
				member.Status = "hangar"
				member.Returning = false
				events = append(events, EngineEvent{Type: "attack_member_stranded", Audience: member.Faction, Text: fmt.Sprintf("%s 无法出动（母舰无法进行航空作业），已退出攻击编队", member.Model)})
			}
			members = taskAircraft(*state, taskID)
			if len(members) == 0 || members[0].TargetCoord == "" {
				continue
			}
		}
		// 已在返航的成员不再挡住编队出发：一架燃油不足掉头的飞机不能让整队留在舰队上空烧油。
		allHolding := false
		for _, member := range members {
			if member.Returning {
				continue
			}
			if member.Status != "attack_holding" {
				allHolding = false
				break
			}
			allHolding = true
		}
		if !allHolding {
			continue
		}
		events = append(events, updateAttackFormation(state, taskID)...)
	}
	return events
}

func launchAirTask(state *State, aircraftList []*AirUnit) []EngineEvent {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range aircraftList {
		groups[aircraft.CarrierID] = append(groups[aircraft.CarrierID], aircraft)
	}
	carrierIDs := make([]string, 0, len(groups))
	for carrierID := range groups {
		carrierIDs = append(carrierIDs, carrierID)
	}
	sort.Strings(carrierIDs)
	events := []EngineEvent{}
	for _, carrierID := range carrierIDs {
		group := groups[carrierID]
		carrier := state.Units[carrierID]
		for _, aircraft := range group {
			aircraft.Launched = true
			aircraft.NightWarned = false
			aircraft.Status = aircraft.TaskKind
			if aircraft.TaskKind == "attack" {
				aircraft.Status = "attack_holding"
			}
			aircraft.ElevatorID = ""
			carrier.Card.Aircraft--
			// 起飞顶点一律记下来：任务卡要显示"目标 / 起飞位置"，以前只有侦察和攻击记，
			// CAP、掩护、转场的任务卡就只剩一个当前坐标，玩家分不清它是从哪条船起飞的。
			aircraft.OriginCoord = carrier.Coord
			if aircraft.TaskKind == "recon" || aircraft.TaskKind == "attack" {
				aircraft.Coord = carrier.Coord
				aircraft.ReconDistance = 0
				aircraft.Returning = false
				aircraft.TrackingFleetID = ""
			}
		}
		carrier.Deck.Operation = "launch"
		events = append(events, EngineEvent{Type: "air_launched", Audience: carrier.Faction, Text: fmt.Sprintf("%s通过飞行甲板放飞%d个算子：%s", carrier.Name, len(group), formationLabel(group))})
	}
	if aircraftList[0].TaskKind == "attack" {
		events = append(events, updateAttackFormation(state, aircraftList[0].TaskID)...)
	}
	return events
}

func ApplyAirOrder(state *State, order Order) []EngineEvent {
	EnsureAviation(state)
	defer syncEmbarkedAircraft(state)
	aircraftList, carrier, err := aircraftForOrder(*state, orderAircraftFaction(*state, order), order)
	if err != nil || len(aircraftList) == 0 || carrier == nil {
		return nil
	}
	switch order.Type {
	case "RETURN_TO_HANGAR":
		lowered := 0
		cancelled := 0
		for _, aircraft := range aircraftList {
			if aircraft.Status == "service" && !aircraft.DeckService {
				aircraft.Status = "hangar"
				aircraft.ServiceTurns = 0
				aircraft.LiftRequested = false
				cancelled++
			} else {
				// 甲板算子（包括甲板上换弹的整备中算子）撤回机库一样要走升降机。
				// 机数只还一次：甲板整备的算子本来就在甲板上，没有"从天上落下来"这一笔。
				fromDeck := aircraft.Status != "service"
				aircraft.Status = "recovery"
				aircraft.DeckService = false
				aircraft.ServiceTurns = 0
				aircraft.LiftRequested = false
				if mother := state.Units[aircraft.CarrierID]; mother != nil && fromDeck {
					mother.Card.Aircraft++
				}
				lowered++
			}
			// 规则书 190 行：主动撤回的算子同时取消自动循环。
			disarmTaskCycle(aircraft)
			clearAirTask(aircraft)
		}
		if lowered > 0 {
			carrier.Deck.Operation = "recover"
		}
		return []EngineEvent{{Type: "air_returned_to_hangar", Audience: carrier.Faction, Text: fmt.Sprintf("%s取消%d个机库任务算子，%d个甲板算子等待升降机下降", carrier.Name, cancelled, lowered)}}
	case "LIFT_TO_DECK":
		// 只是排队提升：真正的提升动作在回合结束时由升降机执行（每部升降机每回合 4 个）。
		// 机库里的算子先转成"整备中/待提升"（service），空载的立刻就能上去，
		// 有挂载的要等这一轮整备走完。
		for _, aircraft := range aircraftList {
			aircraft.LiftRequested = true
			if aircraft.Status == "hangar" {
				aircraft.Status = "service"
				aircraft.DeckService = false
			}
		}
		return []EngineEvent{{Type: "air_lift_requested", Audience: carrier.Faction, Text: fmt.Sprintf("%s：%d个算子排队提升至甲板", carrier.Name, len(aircraftList))}}
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_COVER", "LAUNCH_TRANSFER":
		if sameTaskAssignment(aircraftList, order) {
			return []EngineEvent{{Type: "air_task_unchanged", Audience: carrier.Faction, Text: "算子已在当前任务中，无需重复加入"}}
		}
		status := aircraftList[0].Status
		taskEvents := configureAirTask(state, aircraftList, order)
		if status == "deck" || status == "ready" {
			// 甲板上已经整备好的算子直接拖进任务栏就是"整备完毕、直接出击"。
			// 空载（没有油弹）或者刚被任务换了挂载的算子还不能飞：在甲板上原地整备一轮。
			launchable := true
			for _, aircraft := range aircraftList {
				if !aircraftReadyToLaunch(aircraft) {
					launchable = false
					break
				}
			}
			if launchable {
				return append(taskEvents, launchAirTask(state, aircraftList)...)
			}
			for _, aircraft := range aircraftList {
				aircraft.Status = "service"
				aircraft.DeckService = true
			}
			return append(taskEvents, EngineEvent{Type: "air_task_assigned", Audience: carrier.Faction, Text: fmt.Sprintf("%d个甲板算子加入%s任务并开始甲板整备", len(aircraftList), aircraftList[0].TaskKind)})
		}
		for _, aircraft := range aircraftList {
			if aircraft.Status == "service" && aircraft.DeckService {
				// 甲板上正在整备的算子：任务记上了，整备完这一轮直接参加放飞。
				continue
			}
			aircraft.Status = "service"
			aircraft.DeckService = false
		}
		return append(taskEvents, EngineEvent{Type: "air_task_assigned", Audience: carrier.Faction, Text: fmt.Sprintf("%d个算子加入%s任务并开始机库整备，后续提升由母舰升降机执行", len(aircraftList), aircraftList[0].TaskKind)})
	case "SET_STRIKE_TARGET":
		taskID := aircraftList[0].TaskID
		kind := aircraftList[0].TaskKind
		for _, aircraft := range taskAircraft(*state, taskID) {
			if kind == "attack" {
				// 已经在返航的成员不再被改目标拉回来：它们已经在回家路上，再塞回打击编队
				// 只会让编队卡里混进"返航中"的飞机。
				if aircraft.Returning {
					continue
				}
				setStrikeTarget(state, aircraft, carrier.Faction, order.Coord)
				if order.TargetFleetID != "" {
					aircraft.TargetFleetID = order.TargetFleetID
					if coord, ok := strikeFleetCoord(*state, carrier.Faction, order.TargetFleetID); ok {
						aircraft.TargetCoord = coord
					}
				}
				continue
			}
			// 掩护/转场改目标：清掉"降不下去"标记，重新飞向新 hex。
			aircraft.TargetCoord = order.Coord
			aircraft.CannotLand = false
			aircraft.Returning = false
			aircraft.TargetFleetID = ""
			switch kind {
			case "cover":
				aircraft.CoverUnitID = friendlyUnitIDAt(*state, carrier.Faction, order.Coord)
			case "transfer":
				// 转场到舰队时记住舰队 ID：只认 hex 的话，舰队开出去一格，
				// 编队就停在空海面上刷"机库已满"，一路烧油到航程 -5 被销毁。
				if id := order.TargetFleetID; id != "" {
					if _, ok := friendlyFleetCoord(*state, carrier.Faction, id); ok {
						aircraft.TargetFleetID = id
					}
				} else {
					aircraft.TargetFleetID = friendlyFleetIDAt(*state, carrier.Faction, order.Coord)
				}
			}
		}
		events := []EngineEvent{{Type: "strike_target_set", Audience: carrier.Faction, Text: fmt.Sprintf("编队目标设为%s", order.Coord)}}
		if kind == "attack" {
			// 改目标不自动换挂载：把"有些挂载不适合这个任务"提示给玩家，别让他们拿着鱼雷去打机场。
			if models := strikeLoadoutMismatchModels(*state, taskAircraft(*state, taskID)); len(models) > 0 {
				events = append(events, EngineEvent{Type: "air_loadout_mismatch", Audience: carrier.Faction, Text: strikeLoadoutMismatchHint(models)})
			}
			return append(events, updateAttackFormation(state, taskID)...)
		}
		return events
	case "SET_STRIKE_DISPERSION":
		taskID := aircraftList[0].TaskID
		mode := "集中攻击"
		if order.Disperse {
			mode = "分散攻击"
		}
		for _, aircraft := range taskAircraft(*state, taskID) {
			aircraft.Disperse = order.Disperse
		}
		return []EngineEvent{{Type: "strike_dispersion_set", Audience: carrier.Faction, Text: "攻击编队改为" + mode}}
	case "RECOVER_AIR":
		for _, aircraft := range aircraftList {
			aircraft.Status = "recovery"
			aircraft.Launched = false
			// 规则书 190 行：主动回收的飞机会取消循环任务。
			disarmTaskCycle(aircraft)
			clearAirTask(aircraft)
			mother := state.Units[aircraft.CarrierID]
			if mother == nil {
				continue
			}
			mother.Card.Aircraft++
			mother.Deck.Operation = "recover"
		}
		return []EngineEvent{{Type: "air_recovered", Audience: carrier.Faction, Text: fmt.Sprintf("%d个CAP算子已降落在各自母舰甲板，等待升降机下降", len(aircraftList))}}
	case "SET_AIR_CYCLE":
		// 规则书 191 行：开关自动任务循环。开启时立刻保留当前任务参数，
		// 这样即使在空中的算子也可以"这一轮落回来之后继续"。
		toggled := 0
		for _, aircraft := range aircraftList {
			if order.Cycle {
				kind := aircraft.TaskKind
				if kind == "" {
					kind = aircraft.CycleKind
				}
				if kind == "" && aircraft.Status == "cap" {
					kind = "cap"
				}
				enableTaskCycle(aircraft, kind)
				toggled++
				continue
			}
			disarmTaskCycle(aircraft)
			toggled++
		}
		label := "已开启"
		if !order.Cycle {
			label = "已取消"
		}
		return []EngineEvent{{Type: "air_cycle_toggled", Audience: carrier.Faction, Text: fmt.Sprintf("%d个算子的自动任务循环%s", toggled, label)}}
	case "PRIORITIZE_AIR_TASK":
		selected := aircraftList[0]
		selectedTask := selected.TaskID
		seen := map[string]bool{}
		for _, aircraft := range state.Aircraft {
			if aircraft.Faction == selected.Faction && aircraft.TaskID != "" && aircraft.TaskID != selectedTask {
				seen[aircraft.TaskID] = true
			}
		}
		for _, aircraft := range state.Aircraft {
			if aircraft.Faction != selected.Faction || aircraft.TaskID == "" {
				continue
			}
			if aircraft.TaskID == selectedTask {
				aircraft.Priority = 1
			} else if seen[aircraft.TaskID] {
				aircraft.Priority++
			}
		}
		return []EngineEvent{{Type: "air_task_prioritized", Audience: selected.Faction, Text: selected.Model + "的航空任务已调整为最高优先级"}}
	}
	return nil
}

func orderAircraftFaction(state State, order Order) string {
	for _, id := range airOrderIDs(order) {
		if aircraft := state.Aircraft[id]; aircraft != nil {
			return aircraft.Faction
		}
	}
	if order.Type == "PRIORITIZE_AIR_TASK" || order.Type == "SET_STRIKE_TARGET" || order.Type == "SET_STRIKE_DISPERSION" {
		for _, aircraft := range state.Aircraft {
			if aircraft.TaskID == order.AircraftID {
				return aircraft.Faction
			}
		}
	}
	return ""
}

func completeElevatorCycles(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, carrierID := range sortedUnitIDs(state.Units) {
		carrier := state.Units[carrierID]
		if carrier.Deck == nil {
			continue
		}
		for _, elevator := range carrier.Deck.Elevators {
			if elevator.Status != "lifting" && elevator.Status != "lowering" {
				continue
			}
			for _, id := range elevator.AircraftIDs {
				aircraft := state.Aircraft[id]
				if aircraft == nil {
					continue
				}
				if elevator.Status == "lifting" {
					if aircraft.TaskKind == "" && !aircraft.LiftRequested {
						// 没有任务类型、也没人要求提升的算子上去也放飞不了：直接退回机库，
						// 让它重新变成"可分配"的算子，而不是卡在甲板上占位。
						clearAirTask(aircraft)
						aircraft.Status = "hangar"
						aircraft.ElevatorID = ""
						continue
					}
					// 待发算子（机库里选好挂载、或者空载直接拖上甲板）到甲板就是"甲板待命"：
					// 没有任务也可以停在甲板上，等玩家把它拖进任务栏再放飞。
					aircraft.Status = "deck"
					if aircraft.Seaplane {
						aircraft.Status = "ready"
					}
					aircraft.LiftRequested = false
					aircraft.DeckService = false
				} else {
					aircraft.Status = "hangar"
					aircraft.LiftRequested = false
					aircraft.DeckService = false
					aircraft.ServiceTurns = 0
				}
				aircraft.ElevatorID = ""
			}
			action := "提升至甲板"
			if elevator.Status == "lowering" {
				action = "下降至机库"
			}
			events = append(events, EngineEvent{Type: "elevator_completed", Audience: carrier.Faction, Text: fmt.Sprintf("%s的%s完成%s：%d个算子", carrier.Name, elevator.ID, action, len(elevator.AircraftIDs))})
			elevator.Status = "idle"
			elevator.TaskID = ""
			elevator.AircraftIDs = []string{}
		}
	}
	return events
}

func sortedTaskGroups(state State, filter func(*AirUnit) bool) [][]*AirUnit {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskID != "" && filter(aircraft) {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	taskIDs := make([]string, 0, len(groups))
	for taskID := range groups {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Slice(taskIDs, func(i, j int) bool {
		left, right := groups[taskIDs[i]][0], groups[taskIDs[j]][0]
		if left.Priority == right.Priority {
			return left.TaskID < right.TaskID
		}
		return left.Priority < right.Priority
	})
	result := make([][]*AirUnit, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		sort.Slice(groups[taskID], func(i, j int) bool { return groups[taskID][i].ID < groups[taskID][j].ID })
		result = append(result, groups[taskID])
	}
	return result
}

// taskWaitsForTarget 判断这个任务是不是"还没定目标、不能起飞"。
// 攻击/转场/掩护编队没有目标就不该被提升到甲板：以前它们照样占升降机、
// 占满甲板之后又不放飞，回收也被卡住，整条航空作业链就死在那里。
func taskWaitsForTarget(aircraft *AirUnit) bool {
	switch aircraft.TaskKind {
	case "attack", "transfer", "cover":
		return aircraft.TargetCoord == ""
	}
	return false
}

func launchReadyTasks(state *State) []EngineEvent {
	events := []EngineEvent{}
	groups := sortedTaskGroups(*state, func(aircraft *AirUnit) bool { return aircraft.Status == "deck" || aircraft.Status == "ready" })
	// 剧本时间线：launch_start 之前谁都不许起飞（珊瑚海 05:20 第一轮升空）。
	// 甲板上的算子原地等，到点自动放飞，不需要玩家重新下命令。
	if len(groups) > 0 && !state.LaunchWindowOpen() {
		return []EngineEvent{{Type: "air_launch_delayed", Audience: groups[0][0].Faction, Text: state.launchWindowHint()}}
	}
	for _, taskMembers := range groups {
		// 攻击/转场编队没有目标就不放飞：飞机留在甲板等目标，而不是升空到舰队上空
		// 每回合各烧 1 点航程干等（"没有目标连起飞都不该起飞"）。
		switch taskMembers[0].TaskKind {
		case "attack", "transfer", "cover":
			if taskMembers[0].TargetCoord == "" {
				continue
			}
		}
		byCarrier := map[string][]*AirUnit{}
		for _, aircraft := range taskMembers {
			byCarrier[aircraft.CarrierID] = append(byCarrier[aircraft.CarrierID], aircraft)
		}
		carrierIDs := make([]string, 0, len(byCarrier))
		for carrierID := range byCarrier {
			carrierIDs = append(carrierIDs, carrierID)
		}
		sort.Strings(carrierIDs)
		for _, carrierID := range carrierIDs {
			group := byCarrier[carrierID]
			// 规则书 238 行：超过当回合甲板容量的部分顺延到后续回合，而不是整组卡住。
			// 之前只要有一艘母舰放不下，整组都停在甲板，玩家必须手动退回飞机才能解开死锁。
			capacity := launchCapacityFor(*state, carrierID)
			if capacity <= 0 {
				name := carrierID
				if carrier := state.Units[carrierID]; carrier != nil {
					name = carrier.Name
				}
				reason := "甲板本回合没有起飞容量"
				if carrier := state.Units[carrierID]; carrier != nil {
					if blocked := launchBlockReason(*state, carrier); blocked != "" {
						reason = blocked
					}
				}
				events = append(events, EngineEvent{Type: "air_launch_delayed", Audience: group[0].Faction, Text: fmt.Sprintf("%s%s，%d 架在甲板等待", name, reason, len(group))})
				continue
			}
			deferred := 0
			if len(group) > capacity {
				deferred = len(group) - capacity
				group = group[:capacity]
			}
			if deferred > 0 {
				name := carrierID
				if carrier := state.Units[carrierID]; carrier != nil {
					name = carrier.Name
				}
				events = append(events, EngineEvent{Type: "air_launch_delayed", Audience: group[0].Faction, Text: fmt.Sprintf("%s甲板本回合只能放飞%d架，其余%d架顺延到后续回合", name, capacity, deferred)})
			}
			events = append(events, launchAirTask(state, group)...)
		}
	}
	return events
}

// advanceAircraftService 推进"机库内加油整备"这一阶段（规则书 231–236 行）：
// 普通整备要 3 回合——机库加油整备、提升到甲板、起飞，每个阶段各占 1 回合。
// 分配任务的当回合算机库整备，下一回合上升降机，再下一回合放飞；
// 转场/攻击这种要求整队一起走的任务，等到整组都整备完毕再一起提升。
//
// 这个计数此前完全不存在：任何"service"算子都会在同一回合被升降机抓走提升，
// 于是挂载检查、任务拥堵这些规则都没有立足点。
func advanceAircraftService(state *State) []EngineEvent {
	events := []EngineEvent{}
	// 剧本时间线：service_start 之前不进行机库整备（珊瑚海 04:00 才开始加油挂弹），
	// 回收截止之后同样不再开工。没配 timeline 的剧本不受影响。
	if !state.ServiceWindowOpen() {
		return events
	}
	// 逐架推进：机库里的空载算子只要选了挂载就开始整备，甲板上换弹的算子原地整备，
	// 有没有任务都不影响这件事——整备是"这架飞机"的状态，不是任务的一个阶段。
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Status != "service" || aircraft.ServiceTurns >= 1 {
			continue
		}
		// 整备只要 1 回合（规则书 240–256 行：整备、提升、起飞各 1 回合）。
		aircraft.ServiceTurns++
		if !serviceComplete(aircraft) {
			continue
		}
		if aircraft.DeckService {
			// 甲板整备的完成战报等它真的转成"甲板待发"时再报（见 AdvanceAviation），
			// 这里只推进计数，免得同一件事报两遍。
			continue
		}
		events = append(events, EngineEvent{Type: "air_service_complete", Audience: aircraft.Faction, Text: aircraft.Model + "完成机库加油整备，等待提升"})
	}
	return events
}

// taskNeedsWholeGroup 判断这类任务是不是"整队一起走"：攻击和转场必须全员到齐再提升，
// CAP／侦察／掩护是单机快反任务，谁整备好谁先走。
func taskNeedsWholeGroup(kind string) bool {
	return kind == string(MissionStrike) || kind == string(MissionTransfer)
}

// wholeTaskServiced 判断一个任务组是不是全员整备完毕（含没损伤、没在检修）。
func wholeTaskServiced(state State, taskID string) bool {
	for _, member := range taskAircraft(state, taskID) {
		if !serviceComplete(member) || member.Damaged || member.Maintenance > 0 {
			return false
		}
	}
	return true
}

// aircraftReadyToLaunch 判断甲板/跑道上的算子能不能直接放飞：整备完了、而且真带着油弹。
// 空载算子没有挂弹也没有副油箱，必须先在甲板整备一轮（规则书 273 行：换挂载要重新整备）。
func aircraftReadyToLaunch(aircraft *AirUnit) bool {
	if aircraft == nil || aircraft.Loadout == LoadoutNone {
		return false
	}
	return serviceComplete(aircraft)
}

// aircraftRepairChance 是损伤算子回机库后每回合的修复概率（规则书 387 行）：美军 10%，日军 3%。
// 具体数值在 FactionRules 里（按 doctrine 解析），这里只是给调用点一个短名字。
func aircraftRepairChance(state State, faction string) float64 {
	return rulesFor(state, faction).RepairChance
}

// serviceComplete 判断机库加油整备是否已完成：1 个整备回合就够，且没有待修/待检修状态。
// 整备、提升、起飞各占 1 回合，所以分配任务当回合是 servicing，下一回合才轮到升降机。
func serviceComplete(aircraft *AirUnit) bool {
	return aircraft != nil && aircraft.ServiceTurns >= 1 && !aircraft.Damaged && aircraft.Maintenance == 0
}

// elevatorBudget 是这艘船本回合"提升 + 下降"合计能处理的算子数：
// 可用升降机每部 4 个，损坏的整部不计入（规则书 205、458 行）。
func elevatorBudget(unit *Unit) int {
	if unit == nil || unit.Deck == nil {
		return 0
	}
	budget := 0
	for _, elevator := range unit.Deck.Elevators {
		if elevator != nil && elevator.Status != "damaged" {
			budget += elevator.Capacity
		}
	}
	return budget
}

func scheduleElevators(state *State) []EngineEvent {
	events := []EngineEvent{}
	// 先按"推进前就已经整备完"的算子挑提升对象，再推进整备计数：
	// 整备只要 1 回合，如果先推进计数，当回合完成整备的算子会立刻被升降机抓走，
	// "整备 / 提升 / 起飞"三个回合就压成两个了。整备完的这一回合仍算 servicing，
	// 下一回合才上甲板——和规则书 240–244 行的三段各 1 回合一致。
	taskGroups := sortedTaskGroups(*state, func(aircraft *AirUnit) bool {
		// TaskKind 为空的算子是"脱队残骸"（老存档里返航后类型被清掉留下的），
		// 不能再往甲板上提升：升上去也放飞不了，只会把甲板和升降机一起堵死。
		return aircraft.Status == "service" && !aircraft.DeckService && serviceComplete(aircraft) && aircraft.TaskKind != "" && !taskWaitsForTarget(aircraft)
	})
	// 攻击/转场是整队一起走的任务：只要同组还有一个成员没整备完，已经整备好的也得在机库等。
	// 先提上去又不放飞，只会占住甲板，玩家看到的就是"编队卡在甲板上一动不动"。
	wholeGroupLifts := taskGroups[:0]
	for _, members := range taskGroups {
		if taskNeedsWholeGroup(members[0].TaskKind) && !wholeTaskServiced(*state, members[0].TaskID) {
			continue
		}
		wholeGroupLifts = append(wholeGroupLifts, members)
	}
	taskGroups = wholeGroupLifts
	// 待发算子的提升：玩家在机库里整备好（或者干脆空载）之后拖进了甲板栏。
	// 空载算子没有可整备的东西，可以立刻提升；有挂载的要等这一轮整备走完。
	soloLifts := []*AirUnit{}
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Status != "service" || aircraft.DeckService || aircraft.TaskKind != "" {
			continue
		}
		if !aircraft.LiftRequested || aircraft.Damaged || aircraft.Maintenance > 0 {
			continue
		}
		if aircraft.Loadout != LoadoutNone && !serviceComplete(aircraft) {
			continue
		}
		soloLifts = append(soloLifts, aircraft)
	}
	// 挑选名单确定之后再推进整备计数/发整备完成战报。
	events = append(events, advanceAircraftService(state)...)
	for _, carrierID := range sortedUnitIDs(state.Units) {
		carrier := state.Units[carrierID]
		if !AviationAvailable(carrier) {
			continue
		}
		reserved := 0
		delayed := false
		budget := elevatorBudget(carrier)
		for _, elevator := range carrier.Deck.Elevators {
			if elevator.Status != "idle" || budget <= 0 {
				continue
			}
			recovery := []*AirUnit{}
			for _, aircraft := range state.Aircraft {
				if aircraft.CarrierID == carrierID && aircraft.Status == "recovery" {
					recovery = append(recovery, aircraft)
				}
			}
			sort.Slice(recovery, func(i, j int) bool { return recovery[i].ID < recovery[j].ID })
			if len(recovery) > 0 {
				count := len(recovery)
				if count > budget {
					count = budget
				}
				selected := recovery[:count]
				budget -= len(selected)
				elevator.Status = "lowering"
				elevator.TaskID = ""
				elevator.AircraftIDs = []string{}
				for _, aircraft := range selected {
					aircraft.Status = "elevator"
					aircraft.ElevatorID = elevator.ID
					elevator.AircraftIDs = append(elevator.AircraftIDs, aircraft.ID)
				}
				events = append(events, EngineEvent{Type: "elevator_started", Audience: carrier.Faction, Text: fmt.Sprintf("%s的%s开始下降%d个回收算子", carrier.Name, elevator.ID, len(selected))})
				continue
			}
			available := carrier.Deck.Length - deckUsed(*state, carrierID) - reserved
			if available <= 0 {
				delayed = true
				continue
			}
			var selectedTask []*AirUnit
			for _, taskMembers := range taskGroups {
				candidates := []*AirUnit{}
				for _, aircraft := range taskMembers {
					if aircraft.CarrierID == carrierID && aircraft.Status == "service" {
						candidates = append(candidates, aircraft)
					}
				}
				if len(candidates) > 0 {
					selectedTask = candidates
					break
				}
			}
			if len(selectedTask) == 0 {
				// 没有编队要提升时再提升"待发算子"：它们不属于任何任务，升上去就是甲板待命。
				for _, candidate := range soloLifts {
					if candidate.CarrierID == carrierID && candidate.Status == "service" {
						selectedTask = append(selectedTask, candidate)
					}
				}
			}
			if len(selectedTask) == 0 {
				continue
			}
			count := len(selectedTask)
			if count > budget {
				count = budget
			}
			if count > available {
				count = available
			}
			if count <= 0 {
				continue
			}
			selected := selectedTask[:count]
			budget -= len(selected)
			elevator.Status = "lifting"
			elevator.TaskID = selected[0].TaskID
			elevator.AircraftIDs = []string{}
			for _, aircraft := range selected {
				aircraft.Status = "elevator"
				aircraft.ElevatorID = elevator.ID
				elevator.AircraftIDs = append(elevator.AircraftIDs, aircraft.ID)
			}
			reserved += len(selected)
			events = append(events, EngineEvent{Type: "elevator_started", Audience: carrier.Faction, Text: fmt.Sprintf("%s的%s开始提升%d个算子", carrier.Name, elevator.ID, len(selected))})
		}
		if delayed {
			for _, taskMembers := range taskGroups {
				for _, aircraft := range taskMembers {
					if aircraft.CarrierID == carrierID && aircraft.Status == "service" {
						events = append(events, EngineEvent{Type: "air_preparation_delayed", Audience: carrier.Faction, Text: carrier.Name + "甲板长度不足，整备完成的算子等待下一次提升"})
						delayed = false
						break
					}
				}
				if !delayed {
					break
				}
			}
		}
	}
	return events
}

// closeFinishedTasks 关闭已经全员落回机库的任务组。
// 任务结束后残留的 TaskID 有两个后果：侦察组会一直占着"最多两个侦察组"的名额，
// 普通攻击编队则会让算子永久处于"已经属于其他任务"，玩家想重新分配会被拒绝。
func closeFinishedTasks(state *State) {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.TaskID != "" {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	for _, members := range groups {
		active := false
		for _, aircraft := range members {
			switch aircraft.Status {
			case "cap", "recon", "attack", "attack_holding", "cover", "service", "elevator", "deck", "ready", "recovery":
				active = true
			}
		}
		if active {
			continue
		}
		for _, aircraft := range members {
			// 还在自动循环里的算子要保留任务号：落地后它会按原参数重新整备再出动。
			if aircraft.OnCycle || aircraft.CycleTaskID != "" {
				continue
			}
			aircraft.TaskID = ""
			aircraft.TaskKind = ""
		}
	}
}

func AdvanceAviation(state *State) []EngineEvent {
	EnsureAviation(state)
	for _, aircraft := range state.Aircraft {
		// Launched 的语义是"本回合刚从甲板放飞"（taskPhase 用它区分 launching / active），
		// 所以每回合要重置；判断"现在是否在空中"请用 airborne(status)，不要用这个字段。
		aircraft.Launched = false
		// 前出拦截标记必须在这里统一清空，不能放到下面的逐机循环里：
		// 攻击编队排在 CAP 前面被处理时会当场触发空袭结算，此时后面那些 CAP 还带着
		// 上一回合的 true，会被 strikeCAPGroups 直接跳过——表现为"CAP 明明在目标 hex
		// 上空、却没进空战，防御完全靠防空炮"。
		aircraft.InterceptedThisTurn = false
	}
	events := completeElevatorCycles(state)
	ids := make([]string, 0, len(state.Aircraft))
	for id := range state.Aircraft {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	processedAttacks := map[string]bool{}
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		// ids 是回合开始时拍下的快照；同一回合的空袭结算可能已经摧毁了排在后面的算子
		// （applyAircraftHit 会 delete(state.Aircraft, id)），这里必须跳过，否则 nil 解引用
		// 会让回合结算 panic，整回合的事件全部丢失、而状态却已经写进数据库。
		if aircraft == nil {
			continue
		}
		if !airborne(aircraft) {
			// 甲板/机库里的算子不可能是"返航中"（落地时 clearAirTask 已经清过，老存档可能留着 true）。
			// 留着它会让编队卡把地面整备的飞机混进返航队，也会被"返航中不改目标"的判定挡住。
			aircraft.Returning = false
			// 升降机孤儿：算子标着 elevator，但没有任何升降机还认它（AircraftIDs 被后一次作业覆盖过）。
			// 这种算子会永远卡在升降机上：不上甲板、不进机库、任务组也散不掉。
			// 兜底直接送回机库，并留一条战报。
			if aircraft.Status == "elevator" {
				attached := false
				if carrier := state.Units[aircraft.CarrierID]; carrier != nil && carrier.Deck != nil {
					for _, elevator := range carrier.Deck.Elevators {
						if elevator.Status == "idle" {
							continue
						}
						for _, id := range elevator.AircraftIDs {
							if id == aircraft.ID {
								attached = true
							}
						}
					}
				}
				if !attached {
					aircraft.Status = "hangar"
					aircraft.ElevatorID = ""
					events = append(events, EngineEvent{Type: "elevator_orphan_recovered", Audience: aircraft.Faction, Text: aircraft.Model + "在升降机上失去操作方，已直接送回机库"})
				}
			}
			if aircraft.Status == "recovery" && state.Units[aircraft.CarrierID] == nil {
				events = append(events, loseCarrierAircraft(state, aircraft)...)
				continue
			}
			if aircraft.Maintenance > 0 {
				aircraft.Maintenance--
			}
			// 已经落到机库、又没有在整备的算子，整备进度归零。
			// 甲板上的算子不能归零：甲板待命的算子已经整备好了，归零会让它掉回"未整备"，
			// 再拖进任务栏就要多等一轮；回收中的算子本来就先走 recovery → hangar 这一趟。
			if !airborne(aircraft) && aircraft.Status == "hangar" {
				aircraft.ServiceTurns = 0
			}
			// 甲板上换弹/挂弹：整备走完就地转成"甲板待发"，这一步必须赶在本次放飞之前，
			// 否则甲板整备会比机库整备多拖一个回合。
			if aircraft.Status == "service" && aircraft.DeckService && serviceComplete(aircraft) {
				aircraft.Status = "deck"
				if aircraft.Seaplane {
					aircraft.Status = "ready"
				}
				aircraft.DeckService = false
				events = append(events, EngineEvent{Type: "air_service_complete", Audience: aircraft.Faction, Text: aircraft.Model + "完成甲板加油整备，可以出击"})
			}
			// 任务循环：CAP／空中掩护／侦察降落后自动重新整备再升空（规则书 188 行）。
			// 损伤、检修、时间窗未开都只是"等"，不算失败；母舰不可用或航程不够才结束循环。
			events = append(events, restartCycledTask(state, aircraft)...)
			if aircraft.Damaged && aircraft.Status == "hangar" {
				chance := aircraftRepairChance(*state, aircraft.Faction)
				if probabilityRoll(&state.RNGState) < chance {
					aircraft.Damaged = false
					events = append(events, EngineEvent{Type: "aircraft_repaired", Audience: aircraft.Faction, Text: aircraft.Model + "损伤修复完成，可以重新分配任务"})
				}
			}
			// 只有落回机库的算子才补满油；回收途中的算子必须保留真实剩余航程，
			// 否则备用油透支的着舰判定永远拿不到负数。
			if aircraft.Status == "hangar" {
				refillFuelForLoadout(aircraft)
			}
			continue
		}
		switch aircraft.Status {
		case "cap":
			if aircraft.Returning {
				events = append(events, advanceAircraftReturn(state, aircraft)...)
				continue
			}
			if state.Units[aircraft.CarrierID] == nil {
				// 母舰沉没：CAP 立即返航，下一回合尝试备降航母或机场
				aircraft.Returning = true
				aircraft.ForwardCoord = ""
				events = append(events, EngineEvent{Type: "cap_diversion", Audience: aircraft.Faction, Text: aircraft.Model + "失去母舰，转向最近的可用甲板或机场"})
				continue
			}
			// 前出拦截的 CAP：拦截结束后飞回巡逻点（来回距离实际消耗航程）
			if aircraft.ForwardCoord != "" {
				patrol := aircraft.PatrolCoord
				if patrol == "" {
					patrol = state.Units[aircraft.CarrierID].Coord
				}
				if Distance(aircraft.ForwardCoord, patrol) <= 2 {
					aircraft.Spend(Distance(aircraft.ForwardCoord, patrol))
					aircraft.ForwardCoord = ""
					events = append(events, EngineEvent{Type: "cap_return_patrol", Audience: aircraft.Faction, Text: aircraft.Model + "完成前出拦截，返回" + patrol + "巡逻"})
				} else {
					// 超出一个回合的返程距离时按 1 格/回合推进，最后落在巡逻点上
					aircraft.ForwardCoord = nextFlightCoord(*state, aircraft, patrol)
					aircraft.Spend(1)
				}
			} else {
				// CAP 是"跟着舰队走"的：母舰开到哪儿，巡逻空域就在哪儿。
				// 以前只更新了判定用的 capCoord()，算子自己的 Coord/PatrolCoord 停在放飞时那一格，
				// 地图上看起来就是"CAP 没跟着母舰"（实测差距能到 2 格以上）。
				carrier := state.Units[aircraft.CarrierID]
				aircraft.Coord = carrier.Coord
				aircraft.PatrolCoord = carrier.Coord
				// 规则书 210 行：CAP 燃料不足时自动返航，不能一直烧到航程 -5 被销毁。
				if hasRTBDestination(*state, aircraft) && aircraft.Range() <= rtbDistance(*state, aircraft) {
					aircraft.Returning = true
					events = append(events, EngineEvent{Type: "cap_rtb", Audience: aircraft.Faction, Text: fmt.Sprintf("%s剩余航程%d不足以往返，自动返航", aircraft.Model, aircraft.Range())})
					continue
				}
				aircraft.Spend(1)
			}
		case "attack_holding":
			// 被召回的等待编队必须真的往回飞。以前这里只扣油不移动，
			// 算子会永远悬在原地烧油，直到 Range<=-5 才"燃油耗尽"——表现就是卡在空中。
			if aircraft.Returning {
				events = append(events, advanceAircraftReturn(state, aircraft)...)
				continue
			}
			// 等集结也烧油：剩的航程只够回家了就自己脱队返航（规则书 618 行），
			// 不能让一架等编队的飞机在母舰上空烧到油尽坠毁。
			if shouldRTB(*state, aircraft) {
				aircraft.Returning = true
				events = append(events, EngineEvent{Type: "attack_rtb", Audience: aircraft.Faction, Text: fmt.Sprintf("%s剩余航程%d不足以往返，脱离攻击编队返航", aircraft.Model, aircraft.Range())})
				continue
			}
			aircraft.Spend(1)
		case "attack":
			if aircraft.Returning || state.Units[aircraft.CarrierID] == nil {
				aircraft.Returning = true
				events = append(events, advanceAircraftReturn(state, aircraft)...)
			} else if !processedAttacks[aircraft.TaskID] {
				events = append(events, advanceAttack(state, aircraft)...)
				processedAttacks[aircraft.TaskID] = true
			}
		case "recon":
			if state.Units[aircraft.CarrierID] == nil {
				aircraft.Returning = true
				events = append(events, advanceCarrierlessRecon(state, aircraft)...)
				continue
			}
			events = append(events, advanceRecon(state, aircraft)...)
		case "cover":
			if aircraft.Returning {
				events = append(events, advanceAircraftReturn(state, aircraft)...)
				continue
			}
			events = append(events, advanceCover(state, aircraft)...)
		case "transfer":
			events = append(events, advanceTransfer(state, aircraft)...)
		}
		if state.Aircraft[id] != nil && aircraft.Range() <= -5 {
			events = append(events, EngineEvent{Type: "air_destroyed", Audience: aircraft.Faction, Text: aircraft.Model + "燃油耗尽，航空算子被摧毁"})
			delete(state.Aircraft, id)
		}
	}
	events = append(events, launchReadyTasks(state)...)
	// 最后一批算子起飞的同一回合要重新评估编队：全员升空且已设目标就该当回合离场。
	// 之前只有玩家再发一次 SET_STRIKE_TARGET 才会触发 updateAttackFormation，
	// 编队白白在舰队上空停 1-2 回合，白白烧油。
	events = append(events, departReadyAttacks(state)...)
	// 已经掉头返航的成员从打击编队里拆出去单独成组：编队卡只列还在执行任务的飞机。
	events = append(events, splitCarrierReturningMembers(state)...)
	events = append(events, scheduleElevators(state)...)
	for _, unit := range state.Units {
		if unit.Deck != nil {
			unit.Deck.Operation = ""
		}
	}
	closeFinishedTasks(state)
	// 回收后的算子会一直挂着 TaskID（分组需要），但 TaskKind 已经被 clearAirTask 清空；
	// 分组关闭只认 TaskKind，于是这些算子在 ValidateAirOrder 里被当成"已经属于其他任务"，
	// 而 RETURN_TO_HANGAR 又不接受 hangar 状态——飞机降落之后就永久不能再出击了。
	clearLandedTaskResidue(state)
	syncEmbarkedAircraft(state)
	return events
}

// clearLandedTaskResidue 清掉"已经回到机库、却没有任务类型"的残留 TaskID。
// 合法的机库算子要么属于一个仍在进行的分组（TaskKind 非空），要么完全没有任务。
func clearLandedTaskResidue(state *State) {
	for _, aircraft := range state.Aircraft {
		if aircraft == nil || aircraft.Status != "hangar" {
			continue
		}
		if aircraft.TaskKind == "" && aircraft.TaskID != "" {
			aircraft.TaskID = ""
			aircraft.Priority = 0
		}
	}
}

// advanceCarrierlessRecon 母舰沉没后的侦察机返航：直接朝最近机场飞，抵达后降落。
// 机场在 3–4 hex 外时，一回合最多 2 格，所以抵达通常发生在第二回合；
// 测试与调试时从"失去母舰"开始跑，注意至少推进两回合。
func advanceCarrierlessRecon(state *State, aircraft *AirUnit) []EngineEvent {
	aircraft.Returning = true
	aircraft.TrackingFleetID = ""
	airfield := alternateAirfield(state, aircraft)
	if airfield == nil {
		return nil
	}
	for steps := 0; steps < 2; steps++ {
		if aircraft.Coord == airfield.Coord {
			break
		}
		next := nextFlightCoord(*state, aircraft, airfield.Coord)
		if next == "" {
			break
		}
		aircraft.Coord = next
		aircraft.Spend(1)
	}
	if aircraft.Coord != airfield.Coord {
		return nil
	}
	model := aircraft.Model
	if crashed := landAtAirfield(state, aircraft, airfield); crashed != nil {
		return crashed
	}
	return []EngineEvent{{Type: "air_landed_airfield", Audience: aircraft.Faction, Text: fmt.Sprintf("%s失去母舰，降落在%s", model, airfield.Name)}}
}

func CanLandAircraft(state State, carrier *Unit) bool {
	return landingBlockReason(state, carrier) == ""
}

// landingBlockReason 返回该航母现在不能接收降落的原因；可以降落时返回空串。
// 玩家在"飞机一直盘旋不降落"时看到的提示就来自这里，避免只能盯着看成败。
// aviationBlockReason 返回整艘航空舰当前无法进行航空作业的原因（空串表示可用）。
func aviationBlockReason(state State, unit *Unit) string {
	if unit == nil || unit.Deck == nil {
		return ""
	}
	damage := EnsureDamage(unit)
	switch {
	case damage.Disabled:
		return "失去动力，无法进行航空作业"
	case damage.DeckDestroyed:
		return "飞行甲板已被摧毁，无法起降"
	case damage.Tilt:
		return "舰体倾斜，暂停起降作业"
	case damage.Fire != "":
		return "舰上起火，暂停起降作业"
	}
	return ""
}

// aboardAircraft 统计已经在舰上的算子：机库、整备、甲板、待发、回收、升降机。
// 升空在外执行任务的算子不算占机库——它们返航时正是要落回这些位置。
func aboardAircraft(state State, carrierID string) int {
	used := 0
	for _, aircraft := range state.Aircraft {
		if aircraft == nil || aircraft.CarrierID != carrierID {
			continue
		}
		switch aircraft.Status {
		case "hangar", "service", "deck", "ready", "recovery", "elevator":
			used++
		}
	}
	return used
}

func landingBlockReason(state State, carrier *Unit) string {
	if carrier == nil || carrier.Deck == nil {
		return "母舰不存在"
	}
	damage := EnsureDamage(carrier)
	switch {
	case damage.DeckDestroyed:
		return "飞行甲板已被摧毁，无法降落"
	case damage.DeckDamaged:
		return "飞行甲板受损，本回合无法降落"
	case damage.Tilt:
		return "舰体倾斜，暂停起降作业"
	case damage.Fire != "":
		return "舰上起火，暂停起降作业"
	case damage.Disabled:
		return "失去动力，无法进行航空作业"
	case carrier.Deck.Operation == "launch":
		return "本回合正在放飞，甲板不能同时回收"
	case deckUsed(state, carrier.ID) >= carrier.Deck.Length:
		return fmt.Sprintf("甲板已满（%d/%d），需要先清出整备或回收位置", deckUsed(state, carrier.ID), carrier.Deck.Length)
	case carrier.Card.HangarCapacity > 0 && aboardAircraft(state, carrier.ID) >= carrier.Card.HangarCapacity:
		// 机库满了就收不回来：还空着的甲板只是作业位，飞机落下来没处整备。
		// 编队里先到的那批会逐架填满机库，剩下的只能挂在舰队头顶盘旋、照常耗油。
		return fmt.Sprintf("机库已满（%d/%d），无法继续回收", aboardAircraft(state, carrier.ID), carrier.Card.HangarCapacity)
	}
	return ""
}

func LandAircraft(state *State, aircraft *AirUnit, carrier *Unit) {
	if aircraft == nil || carrier == nil || aircraft.Status == "elevator" || aircraft.Status == "hangar" {
		return
	}
	aircraft.Status = "recovery"
	aircraft.Launched = false
	// 只清空任务参数，保留 TaskID/TaskKind：任务分组依赖它们，清掉会让还在任务栏里的
	// 算子（含同组其他飞机）整组消失；TaskKind 清掉则整组变成"无类型"，
	// 既不能放飞也不能重新编队——返航的轰炸机卡在"待分配"就是这么来的。
	taskID := aircraft.TaskID
	taskKind := aircraft.TaskKind
	clearAirTask(aircraft)
	// 打击编队落地就等于这一波结束：算子回甲板/机库由面板展示，任务组里只留还在
	// 空中的成员（编队牌跟着收缩），最后一个落地时整队消失。以前把 TaskID 留着，
	// 于是"返航中"的编队落地后又以"任务整备中"的样子冒回打击任务栏。
	if taskKind != "attack" {
		aircraft.TaskID = taskID
		aircraft.TaskKind = taskKind
	}
	// 母舰已经沉没时没有需要用"在舰机数"做容量校验的接收方，直接停在回收状态等待处理。
	if state == nil || state.Units[carrier.ID] != carrier {
		return
	}
	carrier.Card.Aircraft++
	carrier.Deck.Operation = "recover"
}

// ReconcileCarrierAircraftCounts 把 Card.Aircraft 拉回真实在舰数。
//
// Card.Aircraft 是"机库/甲板上的算子数"，放飞时减、着舰时加。旧版本在两种摔毁路径上
// 反向加了回去（备用油透支、夜间着舰失败），于是每摔一架机库就凭空多占一个位置，
// 计数一路涨到 21/18，还会让 transferHost 这类容量判断误判"机库已满"。
// 每回合对一次账：以实际在舰算子为准，历史脏数据自然被修回去。
func ReconcileCarrierAircraftCounts(state *State) {
	if state == nil {
		return
	}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Deck == nil || unit.Card.HangarCapacity <= 0 {
			continue
		}
		unit.Card.Aircraft = aboardAircraft(*state, unit.ID)
	}
}

// landIfCapacity 统一着舰入口：先确认甲板真的有空间，再落地，避免同一回合多架返航飞机叠加上限。
// loseCarrierAircraft 处理母舰已沉没的空中/待处置算子：能备降就转移到同阵营可用甲板整备，
// 否则按燃油耗尽处理移除。避免出现"航母已沉、机库指标还在"的幽灵算子。
func loseCarrierAircraft(state *State, aircraft *AirUnit) []EngineEvent {
	if host := altLanding(state, aircraft); host != nil {
		aircraft.CarrierID = host.ID
		aircraft.Status = "service"
		aircraft.Returning = false
		host.Card.Aircraft++
		return []EngineEvent{{Type: "air_ditched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s失去母舰，转场至%s甲板重新整备", aircraft.Model, host.Name)}}
	}
	if airfield := alternateAirfield(state, aircraft); airfield != nil {
		model := aircraft.Model
		if crashed := landAtAirfield(state, aircraft, airfield); crashed != nil {
			return crashed
		}
		return []EngineEvent{{Type: "air_landed_airfield", Audience: aircraft.Faction, Text: fmt.Sprintf("%s失去母舰，转移至%s机库", model, airfield.Name)}}
	}
	// 既没有可降落的航母也没有机场：不凭空消失，留在空中盘旋，
	// 照常消耗燃油，烧光之后才算损失（规则：挂到燃油耗尽）。
	if aircraft.Range() > 0 {
		aircraft.Status = "recovery"
		aircraft.Returning = true
		aircraft.TrackingFleetID = ""
		aircraft.Spend(1)
		return []EngineEvent{{Type: "air_loiter", Audience: aircraft.Faction, Text: fmt.Sprintf("%s失去母舰且无可用甲板，在空中盘旋（剩余航程%d）", aircraft.Model, aircraft.Range())}}
	}
	delete(state.Aircraft, aircraft.ID)
	return []EngineEvent{{Type: "air_destroyed", Audience: aircraft.Faction, Text: aircraft.Model + "燃油耗尽坠毁"}}
}

// altLanding 为空中的算子挑选备降母舰：同一舰队其他航母优先，其次同阵营其他航母。
// 母舰被击沉后算子不能凭空消失，规则要求"尽力飞往最近的可以备降的机场或舰队"。
func altLanding(state *State, aircraft *AirUnit) *Unit {
	// 水上飞机没有起落架，只能回自己的巡洋舰母舰或水上飞机基地，不能降落在航母甲板上。
	// 以前这里不分机型地把所有航母都当候选，于是巡洋舰的水侦返航后"落"到了航母上。
	if aircraft.Seaplane {
		return nil
	}
	origin := state.Units[aircraft.CarrierID]
	ownFleet := ""
	if origin != nil {
		ownFleet = FleetKey(origin)
	}
	best := []*Unit{}
	for _, unit := range state.Units {
		if !IsCarrier(unit) || unit.Faction != aircraft.Faction || unit.ID == aircraft.CarrierID || !CanLandAircraft(*state, unit) {
			continue
		}
		if FleetKey(unit) == ownFleet {
			best = append([]*Unit{unit}, best...)
			continue
		}
		best = append(best, unit)
	}
	if len(best) == 0 {
		return nil
	}
	sort.SliceStable(best, func(i, j int) bool {
		return Distance(aircraft.Coord, best[i].Coord) < Distance(aircraft.Coord, best[j].Coord)
	})
	return best[0]
}

// airfieldCanAccept 判断机场能否接收这架舰载机：有可用跑道（无跑道也能降，但要走迫降判定），
// 机场不是被摧毁状态，且机场类型支持该机型（野战机场只收战斗机）。
func airfieldCanAccept(state *State, airfield *Airfield, aircraft *AirUnit) bool {
	if airfield == nil || airfield.Faction != aircraft.Faction {
		return false
	}
	// 规则书：飞机不能在无跑道的机场起飞，但可以降落（降落时 60% 迫降失败进入损伤状态）。
	//
	// 备降点必须和飞机的起降方式对得上：水上飞机只能落水上飞机基地，
	// 陆基机不能落到水上基地。以前这里反着写——凡是"不是水上基地"的机场都收，
	// 结果莫尔兹比港这种陆地机场把整队 SOC 海鸥收了进去。
	kind := NormalizeAirfieldType(airfield.Type)
	if aircraft.Seaplane && kind != AirfieldSeaplane {
		return false
	}
	if !aircraft.Seaplane && kind == AirfieldSeaplane {
		return false
	}
	// 和航母机库同一套检查：机场默认 99999（等于不限），只有剧本显式写了上限才可能卡住。
	return airfieldHangarFull(*state, airfield, 1) == nil
}

// alternateAirfield 在母舰无法降落时寻找最近的可用机场（同阵营、能收该机型）。
func alternateAirfield(state *State, aircraft *AirUnit) *Airfield {
	candidates := []*Airfield{}
	for _, id := range sortedAirfieldIDs(*state) {
		airfield := state.Airfields[id]
		if airfieldCanAccept(state, airfield, aircraft) {
			candidates = append(candidates, airfield)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return Distance(aircraft.Coord, candidates[i].Coord) < Distance(aircraft.Coord, candidates[j].Coord)
	})
	return candidates[0]
}

// landIfCapacity 统一着舰入口：先确认甲板真的有空间，再落地，避免同一回合多架返航飞机叠加上限。
// 甲板满或母舰沉没时，按规则书"尽力飞往最近的可以备降的机场"转场到机场机库；都没有则返回 false 继续盘旋。
func landIfCapacity(state *State, aircraft *AirUnit, carrier *Unit) bool {
	if carrier != nil && CanLandAircraft(*state, carrier) {
		landOnCarrier(state, aircraft, carrier)
		return true
	}
	// 同一编队里另一艘航母还能收，就落到那艘上：两艘航母不是一条船，
	// 一艘甲板在抢修不代表整个编队没地方降落。只接受同格（或相邻一格）的航母，
	// 远处的航母不能凭空接住这架飞机。水上飞机只能回自己的母舰或机场（见 altLanding）。
	if host := altLanding(state, aircraft); host != nil && host.ID != aircraft.CarrierID &&
		Distance(aircraft.Coord, host.Coord) <= 1 {
		aircraft.CarrierID = host.ID
		landOnCarrier(state, aircraft, host)
		return true
	}
	// 甲板暂时占满/在抢修：盘旋等待，不转场、更不瞬移。
	// 只有母舰沉没或甲板被彻底摧毁（永久失去回收能力）才谈备降，
	// 而且备降是"自己飞过去"，到得了才落，到不了就继续飞。
	if permanentlyUnlandable(*state, aircraft) {
		if airfield := alternateAirfield(state, aircraft); airfield != nil && aircraft.DivertAirfield == "" {
			aircraft.DivertAirfield = airfield.ID
			aircraft.TargetCoord = airfield.Coord
		}
	}
	return false
}

// landOnCarrier 把算子上舰，并保留任务循环参数。
func landOnCarrier(state *State, aircraft *AirUnit, carrier *Unit) {
	// 任务循环：CAP／空中掩护／侦察降落后会自动重新整备再升空（规则书 188 行）。
	// 必须在 LandAircraft 之前抄循环参数——它内部会 clearAirTask 清空航向/扇面/半径。
	if kind := cycleKindForStatus(aircraft.Status); kind != "" {
		// 规则书 188 行：这些任务默认自动循环；玩家手动关掉（CycleOptOut）的例外。
		if !aircraft.CycleOptOut {
			aircraft.OnCycle = true
		}
		rememberCycleParams(aircraft, kind)
	}
	aircraft.DivertAirfield = ""
	LandAircraft(state, aircraft, carrier)
}

// permanentlyUnlandable 判断这个阵营是不是已经没有能回收的甲板：
// 所有航母不是沉了就是飞行甲板被彻底摧毁。只要还有一艘能修、能腾出甲板，
// 飞机就该在天上等，而不是被送去机场。
func permanentlyUnlandable(state State, aircraft *AirUnit) bool {
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Faction != aircraft.Faction || !IsCarrier(unit) {
			continue
		}
		if unit.Damage == nil || !unit.Damage.DeckDestroyed {
			return false
		}
	}
	// 一艘航母都没有，或者全都被打废了甲板：这时候机场才是唯一出路。
	return true
}

// advanceAirfieldDivert 让失去甲板的算子自己飞到备降机场，抵达之后才真正着陆。
// 以前 landIfCapacity 直接把坐标改成机场坐标：30 格外的拉包尔能一回合"接住"飞机，
// 既不合规则，也让损失台账把这批飞机算成被击毁。
func advanceAirfieldDivert(state *State, aircraft *AirUnit) []EngineEvent {
	airfield := state.Airfields[aircraft.DivertAirfield]
	if airfield == nil || airfield.Faction != aircraft.Faction {
		aircraft.DivertAirfield = ""
		return nil
	}
	model := aircraft.Model
	for steps := 0; steps < 2 && aircraft.Coord != airfield.Coord; steps++ {
		next := nextFlightCoord(*state, aircraft, airfield.Coord)
		if next == "" {
			break
		}
		aircraft.Coord = next
		aircraft.Spend(1)
	}
	if aircraft.Coord != airfield.Coord {
		return nil
	}
	// 到了才发现机场停满（只可能发生在剧本显式设了小上限的机场）：不硬塞，继续盘旋等位。
	if err := airfieldHangarFull(*state, airfield, 1); err != nil {
		aircraft.DivertAirfield = ""
		aircraft.TargetCoord = ""
		return []EngineEvent{{Type: "air_loiter", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s，但%s，继续盘旋等位", model, airfield.Name, err.Error())}}
	}
	aircraft.DivertAirfield = ""
	if crashed := landAtAirfield(state, aircraft, airfield); crashed != nil {
		return crashed
	}
	return []EngineEvent{{Type: "air_landed_airfield", Audience: aircraft.Faction, Text: fmt.Sprintf("%s母舰无法回收，转场降落在%s", model, airfield.Name)}}
}

// landAtAirfield 让舰载机在机场降落：转为机场地面算子（状态 recovery，下一回合进机库），
// 并从舰载机列表移除，避免同一算子同时存在于航母和机场两套数据里。
// 返回非空事件表示夜间降落摔毁，算子已经从 state.Aircraft 移除。
func landAtAirfield(state *State, aircraft *AirUnit, airfield *Airfield) []EngineEvent {
	if state.GroundAir == nil {
		state.GroundAir = map[string]*AirfieldAirUnit{}
	}
	if _, exists := state.GroundAir[aircraft.ID]; exists {
		delete(state.Aircraft, aircraft.ID)
		return nil
	}
	// 无跑道的机场按规则"可以降落但有 60% 迫降失败"，失败进入损伤状态
	damaged := false
	if RunwaysAvailable(airfield.Strength, airfield.Runways) == 0 {
		damaged = probabilityRoll(&state.RNGState) < 60
	}
	state.GroundAir[aircraft.ID] = &AirfieldAirUnit{
		ID: aircraft.ID, DefinitionID: aircraft.DefinitionID, AirfieldID: airfield.ID, Faction: aircraft.Faction,
		Model: aircraft.Model, Kind: aircraft.Kind, Skill: aircraft.Skill, Picture: aircraft.Picture, PictureMode: aircraft.PictureMode, Status: "recovery", Strength: aircraft.Strength,
		// 基础航程和挂载必须跟着算子一起转成机场侧的数据：
		// 以前只抄了 MaxRange，改用机场那套"按挂载重建航程"时 base 是 0，
		// 换一次挂载就会把航程算飞掉。
		BaseRange: aircraft.BaseRange, Loadout: aircraft.Loadout,
		MaxRange: aircraft.MaxRange, RemainingRange: aircraft.MaxRange,
		AirCombat: aircraft.AirCombat, AntiShip: aircraft.AntiShip, AntiGround: aircraft.AntiGround,
		Seaplane: aircraft.Seaplane, AirfieldOnly: aircraft.AirfieldOnly, TorpedoCapable: aircraft.TorpedoCapable, HighAltitudeBomber: aircraft.HighAltitudeBomber,
		Coord: airfield.Coord, Damaged: damaged, ReconRadiusLimit: max(1, aircraft.MaxRange/2),
	}
	delete(state.Aircraft, aircraft.ID)
	// 夜间着陆机场：每个算子额外 20% 摔毁（规则书 535 行）。
	if state.Night && probabilityRoll(&state.RNGState) < 20 {
		delete(state.GroundAir, aircraft.ID)
		return []EngineEvent{{Type: "night_landing_crash", Audience: aircraft.Faction, Text: aircraft.Model + "夜间降落失败，冲出跑道"}}
	}
	return nil
}

// canRestartCAP 判断 CAP 循环能否重新开始：母舰可用、航程够、本回合没有被打废。
func canRestartCAP(state State, aircraft *AirUnit) bool {
	carrier := state.Units[aircraft.CarrierID]
	return carrier != nil && AviationAvailable(carrier)
}

// elevatorLowering 判断这架算子所在的升降机是不是在下降（回收方向）。
// 只看算子自己的 status == "elevator" 分不出方向，编队卡的阶段就会把"正在下降"说成
// "升降机提升中"——回收的编队看起来像又要出击。
func elevatorLowering(state State, aircraft *AirUnit) bool {
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil || carrier.Deck == nil {
		return false
	}
	for _, elevator := range carrier.Deck.Elevators {
		if elevator.ID != aircraft.ElevatorID {
			continue
		}
		return elevator.Status == "lowering"
	}
	return false
}

func taskPhase(state State, members []*AirUnit) string {
	// 回收阶段：一部分成员已经落到甲板/升降机上，另一部分还在空中往回飞。
	// 这种混合状态以前会显示成 lifting 或 launching，玩家看不出编队正在回收。
	// 只把"已经落到甲板等下降"(recovery) 算作已回收：elevator 可能是提升中，
	// 也可能是下降中，不能一律当成回收，否则分批提升的任务会被误判。
	recovering, airborneMembers := 0, 0
	for _, aircraft := range members {
		if aircraft.Status == "recovery" {
			recovering++
		}
		if airborne(aircraft) {
			airborneMembers++
		}
	}
	if recovering > 0 && airborneMembers > 0 && members[0].TaskKind != "cap" {
		return "recovering"
	}
	if members[0].TaskKind == "attack" {
		allDeparted := true
		launched := false
		returning := 0
		for _, aircraft := range members {
			if aircraft.Status != "attack" {
				allDeparted = false
			}
			if aircraft.Launched {
				launched = true
			}
			if aircraft.Returning {
				returning++
			}
		}
		// 无线电召回会把目标清空：不认返航状态的话，已经离场的编队会被判成
		// "舰队上空等待编队"，从整编栏里又冒出来一张早就出发了的编队卡。
		if allDeparted && returning == len(members) {
			return "active"
		}
		if allDeparted && members[0].TargetCoord == "" {
			return "assembling"
		}
		if allDeparted {
			if launched {
				return "launching"
			}
			return "active"
		}
		for _, aircraft := range members {
			if aircraft.Status == "attack_holding" {
				return "assembling"
			}
		}
	}
	launched := false
	airborneOnly := true
	for _, aircraft := range members {
		if !airborne(aircraft) {
			airborneOnly = false
		}
		if aircraft.Launched {
			launched = true
		}
		if aircraft.Status == "elevator" {
			// 下降到机库是回收流程的后半段，不是又一次出击。
			if elevatorLowering(state, aircraft) {
				return "recovering"
			}
			return "lifting"
		}
	}
	if airborneOnly {
		if launched {
			return "launching"
		}
		return "active"
	}
	for _, aircraft := range members {
		if aircraft.Status == "deck" || aircraft.Status == "ready" {
			return "preparing"
		}
	}
	return "servicing"
}

func projectReconSector(state State, aircraft *AirUnit) ReconSector {
	origin := aircraft.OriginCoord
	current := aircraft.Coord
	if aircraft.Status != "recon" {
		if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
			current = carrier.Coord
			if origin == "" {
				origin = current
			}
		}
	}
	sector := ReconSector{
		AircraftID: aircraft.ID, Status: aircraft.Status, OriginCoord: origin, CurrentCoord: current,
		Heading: aircraft.Heading, SectorWidth: aircraft.SectorWidth, ReconRadius: aircraft.ReconRadius,
		ReconDistance: aircraft.ReconDistance, Returning: aircraft.Returning, TrackingFleetID: aircraft.TrackingFleetID,
	}
	if aircraft.Status == "recon" {
		// 显示 = 实际：搜索带是沿航线会判定的全部格子，当前圈是本回合那 2（120° 为 4）格。
		// 和实际判定一致：扇面圆心跟着母舰走，UI 画出来的也必须是同一个圆。
		if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
			origin = carrier.Coord
		}
		sector.SectorHexes = ReconScanBandHexes(state.Map, origin, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius)
		// 本回合高亮的必须是"实际会被判定的那一段弧"：当前距离圈 + 下一距离圈落在扇面里的全部格子。
		// 以前给的是"本格 + 正前方一格"，地图上就只亮一个点，玩家看不出这回合到底扫了哪条弧。
		sector.CurrentRingHexes = ReconScannedRingHexes(state.Map, origin, current, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius)
		if current != "" {
			sector.DistanceReference = BuildDistanceReference(state.Map, current, 8)
		}
	}
	return sector
}

func VisibleAviation(state State, faction string) AviationView {
	EnsureAviation(&state)
	view := AviationView{Carriers: []CarrierAirStatus{}, Tasks: []AirTask{}}
	for _, unit := range state.Units {
		if unit.Faction != faction || unit.Deck == nil {
			continue
		}
		carrier := CarrierAirStatus{UnitID: unit.ID, Faction: unit.Faction, Name: unit.Name, Type: unit.Type, HangarCapacity: unit.Card.HangarCapacity, DeckLength: unit.Deck.Length, DeckCapacity: unit.Deck.Length, Operation: unit.Deck.Operation, Elevators: []AircraftElevator{}, Aircraft: []AirUnit{}}
		// 把"为什么不能起降"直接投影给客户端：倾斜/起火/甲板摧毁这类状态以前只能通过
		// 命令被 400 拒绝才知道，玩家会以为飞机卡死了。
		carrier.BlockedReason = aviationBlockReason(state, unit)
		for _, elevator := range unit.Deck.Elevators {
			projection := *elevator
			projection.AircraftIDs = append([]string{}, elevator.AircraftIDs...)
			carrier.Elevators = append(carrier.Elevators, projection)
		}
		for _, aircraft := range state.Aircraft {
			if aircraft.CarrierID != unit.ID {
				continue
			}
			if aircraft.Launched {
				carrier.LaunchUsed++
			}
			switch {
			case aircraft.Status == "service" && aircraft.DeckService:
				// 甲板上整备的算子占甲板位、不占机库位，和它显示的栏位一致。
				carrier.DeckUsed++
			case aircraft.Status == "hangar", aircraft.Status == "service":
				carrier.HangarUsed++
			case aircraft.Status == "deck", aircraft.Status == "ready", aircraft.Status == "recovery":
				carrier.DeckUsed++
			}
			projection := *aircraft
			if canSearch(projection) {
				projection.ReconRadiusLimit = ReconRadiusLimit(projection)
			}
			carrier.Aircraft = append(carrier.Aircraft, withLoadoutAttack(projection))
		}
		sort.Slice(carrier.Aircraft, func(i, j int) bool { return carrier.Aircraft[i].ID < carrier.Aircraft[j].ID })
		// 甲板被占满时，指出是被"还没设目标的攻击编队"占住的：玩家需要把编队取消
		// 或退回机库，返航的 CAP 才能降落。否则只会看到"无法降落：甲板已满"，不知道该动谁。
		// 必须放在 DeckUsed 统计之后。
		if carrier.DeckUsed >= carrier.DeckLength && carrier.DeckLength > 0 {
			waiting := 0
			for _, aircraft := range state.Aircraft {
				if aircraft.CarrierID != unit.ID || aircraft.Faction != faction {
					continue
				}
				if aircraft.TaskKind == "attack" && aircraft.TargetCoord == "" {
					switch aircraft.Status {
					case "deck", "ready", "service":
						waiting++
					}
				}
			}
			if waiting > 0 {
				carrier.PreventedBy = fmt.Sprintf("%d 架攻击编队尚未设置目标、占着甲板，返航的 CAP 可能因此降不下来", waiting)
			}
		}
		view.Carriers = append(view.Carriers, carrier)
	}
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == faction && aircraft.TaskID != "" {
			groups[aircraft.TaskID] = append(groups[aircraft.TaskID], aircraft)
		}
	}
	for taskID, aircraftList := range groups {
		sort.Slice(aircraftList, func(i, j int) bool { return aircraftList[i].ID < aircraftList[j].ID })
		leader := aircraftList[0]
		// 队长自己可能已经着舰、任务类型被清空：类型取组里第一个还带类型的成员，
		// 否则整组在任务栏里显示成空类型，玩家看到的既不是攻击也不是侦察，
		// 点它也没有可用操作。
		kind := leader.TaskKind
		if kind == "" {
			for _, member := range aircraftList {
				if member.TaskKind != "" {
					kind = member.TaskKind
					break
				}
			}
		}
		phase := taskPhase(state, aircraftList)
		task := AirTask{ID: taskID, Kind: kind, Phase: phase, CarrierID: leader.CarrierID, CarrierIDs: []string{}, AircraftIDs: []string{}, Aircraft: []AirUnit{}, Priority: leader.Priority, Lane: leader.TaskLane, Faction: leader.Faction}
		carrierSet := map[string]bool{}
		for _, aircraft := range aircraftList {
			task.AircraftIDs = append(task.AircraftIDs, aircraft.ID)
			task.Aircraft = append(task.Aircraft, withLoadoutAttack(*aircraft))
			if !carrierSet[aircraft.CarrierID] {
				carrierSet[aircraft.CarrierID] = true
				task.CarrierIDs = append(task.CarrierIDs, aircraft.CarrierID)
			}
			if airborne(aircraft) {
				task.AirborneCount++
				// 空中的编队才算"回不回得来"：机库里的等天亮就行。
				if need := TurnsToLanded(state, aircraft); need > task.ReturnTurns {
					task.ReturnTurns = need
				}
			}
		}
		sort.Strings(task.CarrierIDs)
		carrier := state.Units[leader.CarrierID]
		if carrier != nil {
			task.FleetID = FleetKey(carrier)
		}
		// NightSafe：这支编队在天黑前落得回去。前端只认这个布尔值，
		// 不重复实现一遍"距离/速度/天黑"的折算。
		// TurnsToDark / NightMargin 是同一笔账的结构化版本：AI 拿不到事件文本，
		// 只靠 night_safe 的布尔值没法判断"再等一回合还来不来得及"。
		if state.TurnsPerDay > 0 {
			task.TurnsToDark = turnsUntilNight(state)
			task.NightMargin = task.TurnsToDark - task.ReturnTurns
			task.NightSafe = task.NightMargin > 0
		} else {
			// 没有昼夜循环的剧本（沙盒、单日剧本）：不存在"天黑前回不来"。
			task.TurnsToDark = -1
			task.NightMargin = 0
			task.NightSafe = true
		}
		switch kind {
		case "cap":
			task.Name = "CAP · " + leader.Model
			task.OriginCoord = leader.OriginCoord
			// CAP 的"目标"就是它值班的空域：随母舰走的写母舰位置，前出拦截的写前出点。
			task.TargetCoord = leader.PatrolCoord
			if task.TargetCoord == "" {
				task.TargetCoord = leader.ForwardCoord
			}
			if carrier != nil {
				task.CurrentCoord = carrier.Coord
				if task.OriginCoord == "" {
					task.OriginCoord = carrier.Coord
				}
				if task.TargetCoord == "" {
					task.TargetCoord = carrier.Coord
				}
			}
		case "attack":
			task.Name = "攻击编队 · " + leader.Model
			task.OriginCoord = leader.OriginCoord
			// 坐标和状态都取"还在空中的第一架"：同一任务里 ID 最小的可能已经落地，
			// 用它会把这支还在天上的编队画在地面格子上。
			source := leader
			if flying := firstAirborneMember(aircraftList); flying != nil {
				source = flying
			}
			task.CurrentCoord = source.Coord
			task.TargetCoord = source.TargetCoord
			if task.TargetCoord == "" {
				task.TargetCoord = leader.TargetCoord
			}
			task.Disperse = leader.Disperse
			// 返航中的打击编队要让地图上的编队牌写明"返航中"，
			// 否则召回到家之前它看起来还是"任务执行中"。
			task.Returning = source.Returning
			if task.TargetCoord != "" {
				toTarget, toLanded := EstimateCarrierStrike(state, aircraftList, carrier)
				fillStrikeETA(state, &task, toTarget, toLanded)
			}
		case "cover":
			task.Name = "空中掩护 · " + leader.Model
			task.OriginCoord = leader.OriginCoord
			if task.OriginCoord == "" && carrier != nil {
				task.OriginCoord = carrier.Coord
			}
			task.CurrentCoord = leader.Coord
			task.TargetCoord = leader.TargetCoord
			task.Returning = leader.Returning
		case "transfer":
			task.Name = "转场 · " + leader.Model
			task.OriginCoord = leader.OriginCoord
			if task.OriginCoord == "" && carrier != nil {
				task.OriginCoord = carrier.Coord
			}
			transferSource := leader
			if flying := firstAirborneMember(aircraftList); flying != nil {
				transferSource = flying
			}
			task.CurrentCoord = transferSource.Coord
			task.TargetCoord = transferSource.TargetCoord
			if task.TargetCoord == "" {
				task.TargetCoord = leader.TargetCoord
			}
			task.Returning = transferSource.Returning
			// 到了目标却降不下去：前端据此在机队上打红色感叹号。
			for _, aircraft := range aircraftList {
				if aircraft.CannotLand {
					task.CannotLand = true
					break
				}
			}
		default:
			task.ReconSectors = []ReconSector{}
			for _, aircraft := range aircraftList {
				task.ReconSectors = append(task.ReconSectors, projectReconSector(state, aircraft))
			}
			reconLeader := leader
			for _, aircraft := range aircraftList {
				if aircraft.Status == "recon" {
					reconLeader = aircraft
					break
				}
			}
			task.Name = "扇面侦察 · " + leader.Model
			task.OriginCoord = reconLeader.OriginCoord
			task.CurrentCoord = reconLeader.Coord
			task.Heading = reconLeader.Heading
			task.SectorWidth = reconLeader.SectorWidth
			task.ReconRadius = reconLeader.ReconRadius
			task.ReconDistance = reconLeader.ReconDistance
			task.Returning = reconLeader.Returning
			task.TrackingFleetID = reconLeader.TrackingFleetID
			if reconLeader.Status == "recon" {
				origin := reconLeader.OriginCoord
				if carrier := state.Units[reconLeader.CarrierID]; carrier != nil {
					origin = carrier.Coord
				}
				task.SectorHexes = ReconScanBandHexes(state.Map, origin, reconLeader.Heading, reconLeader.SectorWidth, reconLeader.ReconRadius)
				task.CurrentRingHexes = ReconScannedRingHexes(state.Map, origin, reconLeader.Coord, reconLeader.Heading, reconLeader.SectorWidth, reconLeader.ReconRadius)
			}
		}
		if len(aircraftList) > 1 {
			task.Name += fmt.Sprintf(" ×%d", len(aircraftList))
		}
		if task.AirborneCount > 0 && task.CurrentCoord != "" {
			task.DistanceReference = BuildDistanceReference(state.Map, task.CurrentCoord, 8)
		}
		view.Tasks = append(view.Tasks, task)
	}
	sort.Slice(view.Carriers, func(i, j int) bool {
		if shipTypeRank(view.Carriers[i].Type) == shipTypeRank(view.Carriers[j].Type) {
			return view.Carriers[i].UnitID < view.Carriers[j].UnitID
		}
		return shipTypeRank(view.Carriers[i].Type) < shipTypeRank(view.Carriers[j].Type)
	})
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
