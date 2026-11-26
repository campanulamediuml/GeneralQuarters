package game

import (
	"fmt"
	"math"
)

// —— 攻击编队的"到点 / 落舰"时间预估 ——
//
// 只服务前端弹窗：玩家把飞机编进攻击编队、点完目标之后，弹窗要告诉他
// "几点能打到""几点能全部落回来"，而不是只靠卡片角上一行红字。
// 这是回合尺度的粗略模型，不参与结算：结算仍由各任务自己的推进代码负责。
//
// 估算链路（与引擎实际推进顺序一致）：
//   整备/提升（还没起飞的）→ 飞向目标（每回合 1–2 hex）→ 抵达后下一回合接敌并空袭
//   → 返航（每回合 1–2 hex）→ 着舰排队（甲板一次只收得下几个，升降机每回合再清几个）。

// etaClock 把"再过 turns 个回合"换算成当天时刻（"HH:MM"）。
// 剧本没有时间线（TurnsPerDay = 0）时返回空串：那种剧本没有"天黑"这回事。
func etaClock(state State, turns int) string {
	if state.TurnsPerDay <= 0 {
		return ""
	}
	if turns < 0 {
		turns = 0
	}
	minutesPerTurn := 1440 / state.TurnsPerDay
	total := int(math.Round(clockAt(state)*60)) + turns*minutesPerTurn
	total = ((total % 1440) + 1440) % 1440
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// carrierLaunchPrep 估算一架舰载机离舰还要几个回合：
// 甲板上的这回合就能走；机库里要 1 回合整备再上一次升降机；整备中的按剩余回合数算。
func carrierLaunchPrep(aircraft *AirUnit) int {
	if aircraft == nil || airborne(aircraft) {
		return 0
	}
	switch aircraft.Status {
	case "deck", "ready":
		return 0
	case "elevator":
		return 1
	case "recovery":
		// 刚落甲板：先下降一回合，再整备一回合，再提升一回合。
		return 3
	case "service":
		need := 1 - aircraft.ServiceTurns
		if need < 0 {
			need = 0
		}
		return need + 1
	}
	// 机库/待命：整备 1 回合 + 提升 1 回合。
	return 2
}

// EstimateCarrierStrike 返回舰载攻击编队"命中目标的回合数"和"最后一架落回母舰的回合数"，
// 都从当前回合算起（0 = 本回合结算就发生）。home 是回收母舰。
//
// 着舰排队按"甲板一次收得下几架 + 升降机每回合清几架"折算：一波 12 架回来、
// 甲板只剩 8 个空位、每回合能下降 4 架时，后 4 架要等下回合才落得下来。
func EstimateCarrierStrike(state State, members []*AirUnit, home *Unit) (int, int) {
	if home == nil || len(members) == 0 {
		return 0, 0
	}
	homeCoord := home.Coord
	deckLength, used := 0, 0
	if home.Deck != nil {
		deckLength = home.Deck.Length
	}
	used = deckUsed(state, home.ID)
	rate := elevatorBudget(home)
	if rate < 1 {
		rate = 1
	}
	free := deckLength - used
	toTarget, toLanded := 0, 0
	for index, member := range members {
		if member == nil {
			continue
		}
		prep := carrierLaunchPrep(member)
		speed := aircraftSpeed(member)
		if speed < 1 {
			speed = 1
		}
		target := member.TargetCoord
		strikeTurn := prep
		if !member.Engaged {
			fly := 0
			if target != "" && member.Coord != "" {
				fly = (Distance(member.Coord, target) + speed - 1) / speed
			}
			// 抵达目标上空的那一回合只做接敌判定，空袭要到下一回合才发起（规则书 609 行）。
			strikeTurn = prep + fly + 1
		}
		back := 0
		if target != "" && homeCoord != "" {
			back = (Distance(target, homeCoord) + speed - 1) / speed
		}
		queue := 0
		if index+1 > free {
			queue = (index + 1 - free + rate - 1) / rate
		}
		// +1：着舰本身还排一个回合的余量，和 TurnsToLanded 的口径一致。
		landed := strikeTurn + back + 1 + queue
		if strikeTurn > toTarget {
			toTarget = strikeTurn
		}
		if landed > toLanded {
			toLanded = landed
		}
	}
	return toTarget, toLanded
}

// EstimateGroundStrike 返回机场攻击编队的"命中目标的回合数"和"最后一架落回机场的回合数"。
// 机场没有甲板升降机的瓶颈，但跑道一次也只能收那么多：按跑道数折算排队。
func EstimateGroundStrike(state State, members []*AirfieldAirUnit, airfield *Airfield) (int, int) {
	if airfield == nil || len(members) == 0 {
		return 0, 0
	}
	homeCoord := airfield.Coord
	runways := RunwaysAvailable(airfield.Strength, airfield.Runways)
	if runways < 1 {
		runways = 1
	}
	toTarget, toLanded := 0, 0
	for index, member := range members {
		if member == nil {
			continue
		}
		prep := 0
		if !airfieldAirborne(member) {
			// 机场算子落地即在机库，出击要一轮甲板准备/滑跑。
			prep = 1
		}
		speed := groundAirSpeed(state, member)
		if speed < 1 {
			speed = 1
		}
		target := member.TargetCoord
		strikeTurn := prep
		if target == "" || member.Coord != target {
			fly := 0
			if target != "" && member.Coord != "" {
				fly = (Distance(member.Coord, target) + speed - 1) / speed
			}
			strikeTurn = prep + fly + 1
		}
		back := 0
		if target != "" && homeCoord != "" {
			back = (Distance(target, homeCoord) + speed - 1) / speed
		}
		queue := index / runways
		landed := strikeTurn + back + 1 + queue
		if strikeTurn > toTarget {
			toTarget = strikeTurn
		}
		if landed > toLanded {
			toLanded = landed
		}
	}
	return toTarget, toLanded
}

// fillStrikeETA 把预估回合数写成任务卡上的时刻字段。
// lands_at_night 用的是"落舰那一刻是不是夜间"，和 night_safe（有没有富余）分开：
// 前者回答"会不会摸黑着舰"，后者回答"要不要现在就返航"。
func fillStrikeETA(state State, task *AirTask, toTarget, toLanded int) {
	task.TurnsToTarget = toTarget
	task.TurnsToLanded = toLanded
	task.ETATarget = etaClock(state, toTarget)
	task.ETALanded = etaClock(state, toLanded)
	task.LandsAtNight = nightFallsWithinTurns(state, toLanded)
}
