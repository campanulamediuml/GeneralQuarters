package game

import "sort"

// —— 战果台账（损失情况）——
//
// 舰船沉没时直接记账；飞机损失用"回合结束对账"的方式统计：
// 每回合开始记下全部在编算子，结算后消失的算子记为损失。
// 这样不用在十几个 delete(state.Aircraft) 的地方各埋一次钩子，也不会漏。

// ShipLoss 是一艘沉没舰船的记录。
type ShipLoss struct {
	Turn  int    `json:"turn"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Fleet string `json:"fleet"`
	// Card 是沉没那一刻的舰船卡：战果页面要按原样画兵牌，
	// 不能拿一张全零的占位卡拉倒（会显示成 0 强度、0-0 防空）。
	Card ShipCard `json:"card"`
}

// CrippledCarrier 是一艘"丧失作战能力但还浮着"的航母：
// 飞行甲板被摧毁、结构损失超过 80%、或失去动力，三者任一成立即算瘫痪。
// 它不进沉船台账（船还在、还有分数价值），所以单独记一笔，并且只记第一次：
// 甲板摧毁不可逆，但失去动力和进水会被损管修回来，战果分不能跟着来回跳。
type CrippledCarrier struct {
	Turn   int    `json:"turn"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Fleet  string `json:"fleet"`
	Reason string `json:"reason"`
	// Card 是瘫痪那一刻的卡面快照：机库容量就是战果分的一半，后面改卡不能改历史分数。
	Card ShipCard `json:"card"`
}

// AircraftLoss 是按机型聚合的飞机损失。
type AircraftLoss struct {
	Model string `json:"model"`
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// LossLedger 是一个阵营的战果账。
type LossLedger struct {
	Ships    []ShipLoss     `json:"ships"`
	Aircraft []AircraftLoss `json:"aircraft"`
	// Crippled 是失去作战能力但没沉的航母（按机库容量的一半计战果分）。
	Crippled      []CrippledCarrier `json:"crippled"`
	ShipTotal     int               `json:"ship_total"`
	AircraftTotal int               `json:"aircraft_total"`
}

func ensureLosses(state *State) map[string]*LossLedger {
	if state.Losses == nil {
		state.Losses = map[string]*LossLedger{}
	}
	return state.Losses
}

func ledgerFor(state *State, faction string) *LossLedger {
	ledgers := ensureLosses(state)
	ledger := ledgers[faction]
	if ledger == nil {
		ledger = &LossLedger{}
		ledgers[faction] = ledger
	}
	return ledger
}

// RecordShipLoss 记录一艘沉没的舰船（由 sinkUnit 调用）。
func RecordShipLoss(state *State, unit *Unit) {
	if state == nil || unit == nil {
		return
	}
	turn := state.Turn
	for _, existing := range ledgerFor(state, unit.Faction).Ships {
		if existing.ID == unit.ID {
			return
		}
	}
	ledger := ledgerFor(state, unit.Faction)
	ledger.Ships = append(ledger.Ships, ShipLoss{Turn: turn, ID: unit.ID, Name: unit.Name, Type: unit.Type, Fleet: unit.FleetName, Card: unit.Card})
	ledger.ShipTotal = len(ledger.Ships)
}

// RecordCrippledCarrier 把一艘丧失作战能力的航母记进台账；已经记过的返回 false。
// 调用方负责给出原因（甲板摧毁 / 结构损失超过 80% / 失去动力）。
func RecordCrippledCarrier(state *State, unit *Unit, reason string) bool {
	if state == nil || unit == nil || reason == "" {
		return false
	}
	ledger := ledgerFor(state, unit.Faction)
	for _, existing := range ledger.Crippled {
		if existing.ID == unit.ID {
			return false
		}
	}
	ledger.Crippled = append(ledger.Crippled, CrippledCarrier{
		Turn: state.Turn, ID: unit.ID, Name: unit.Name, Type: unit.Type,
		Fleet: unit.FleetName, Reason: reason, Card: unit.Card,
	})
	return true
}

// recordAircraftLossEntry 按机型累加一架损失。
func recordAircraftLossEntry(state *State, aircraft *AirUnit) {
	if state == nil || aircraft == nil {
		return
	}
	ledger := ledgerFor(state, aircraft.Faction)
	for index := range ledger.Aircraft {
		if ledger.Aircraft[index].Model == aircraft.Model && ledger.Aircraft[index].Kind == aircraft.Kind {
			ledger.Aircraft[index].Count++
			ledger.AircraftTotal++
			return
		}
	}
	ledger.Aircraft = append(ledger.Aircraft, AircraftLoss{Model: aircraft.Model, Kind: aircraft.Kind, Count: 1})
	ledger.AircraftTotal++
}

// snapshotAircraftRoster 在回合开始时把当前在编算子记进花名册（只补新出现的，不删除）。
// 已有同名 ID 直接跳过，避免把上一回合的存活状态覆盖掉。
func snapshotAircraftRoster(state *State) {
	if state == nil {
		return
	}
	if state.AircraftRoster == nil {
		state.AircraftRoster = map[string]AirUnit{}
	}
	for id, aircraft := range state.Aircraft {
		if aircraft == nil {
			continue
		}
		if _, exists := state.AircraftRoster[id]; exists {
			continue
		}
		state.AircraftRoster[id] = *aircraft
	}
	// 陆基算子住在 GroundAir，不在 state.Aircraft 里。以前花名册漏了它们，
	// 机场飞机被击落或在地面被炸都不会进损失台账——赢了也拿不到那 0.25 分。
	for id, ground := range state.GroundAir {
		if ground == nil {
			continue
		}
		if _, exists := state.AircraftRoster[id]; exists {
			continue
		}
		state.AircraftRoster[id] = groundAirUnit(*state, ground)
	}
}

// settleAircraftLosses 在回合结束时对账：花名册里有、当前不在编的算子记为损失。
func settleAircraftLosses(state *State) {
	if state == nil || state.AircraftRoster == nil {
		return
	}
	ids := make([]string, 0, len(state.AircraftRoster))
	for id := range state.AircraftRoster {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, alive := state.Aircraft[id]; alive {
			continue
		}
		// 转场到机场的算子只是换了所在位置（舰载机 → 机场地面算子），
		// 飞机还在、还能再出动，不能算击毁。以前这里只查 state.Aircraft，
		// 一次备降就把整队飞机记成战损，还白送对方战果分。
		if _, onGround := state.GroundAir[id]; onGround {
			delete(state.AircraftRoster, id)
			continue
		}
		gone := state.AircraftRoster[id]
		recordAircraftLossEntry(state, &gone)
		delete(state.AircraftRoster, id)
	}
}

// VisibleLosses 返回按阵营分组的损失台账（双方都返回：空战战报本来就会公开双方损失）。
func VisibleLosses(state State, faction string) map[string]LossLedger {
	out := map[string]LossLedger{}
	// 终局之前，敌方的飞机损失不下发：沉船是公开事件（战报 audience=all），但
	// "随舰沉没/机库被炸/燃油耗尽"这些损失对方看不见，累计台账会直接暴露
	// 对方还剩多少航空力量，盲棋就破了。终局后一次性公开。
	finished := state.Outcome != nil
	for side, ledger := range state.Losses {
		if ledger == nil {
			continue
		}
		copied := LossLedger{ShipTotal: ledger.ShipTotal, AircraftTotal: ledger.AircraftTotal}
		copied.Ships = append(copied.Ships, ledger.Ships...)
		copied.Aircraft = append(copied.Aircraft, ledger.Aircraft...)
		// 瘫痪航母和沉船一样是公开战果（打瘫一艘对方航母瞒不住），敌方也能看见。
		copied.Crippled = append(copied.Crippled, ledger.Crippled...)
		if side != faction && !finished {
			copied.Aircraft = nil
			copied.AircraftTotal = 0
		}
		// 飞机按数量降序，方便一眼看出主力损失
		sort.SliceStable(copied.Aircraft, func(i, j int) bool {
			if copied.Aircraft[i].Count == copied.Aircraft[j].Count {
				return copied.Aircraft[i].Model < copied.Aircraft[j].Model
			}
			return copied.Aircraft[i].Count > copied.Aircraft[j].Count
		})
		out[side] = copied
	}
	// 没有损失记录的一方也返回空账，前端不用判空
	for _, side := range []string{"A", "B"} {
		if _, ok := out[side]; !ok {
			out[side] = LossLedger{}
		}
	}
	return out
}
