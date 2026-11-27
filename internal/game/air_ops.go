package game

import "fmt"

// —— 空中掩护（cover）与转场（transfer） ——

// advanceCover 让"空中掩护"编队飞到目标 hex 上空盘旋：
// 到位后就是那个 hex 的 CAP（敌方空袭打过来时由 strikeCAPGroups 算作防守方），
// 燃料只够回家时自动撤出返航，不需要玩家手动召回。
// friendlyUnitIDAt 返回目标 hex 上第一艘己方舰船；空中掩护用它认领要跟随的舰队。
func friendlyUnitIDAt(state State, faction, coord string) string {
	if coord == "" {
		return ""
	}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit != nil && unit.Faction == faction && unit.Coord == coord {
			return unit.ID
		}
	}
	return ""
}

func advanceCover(state *State, aircraft *AirUnit) []EngineEvent {
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		aircraft.Returning = true
		return advanceAircraftReturn(state, aircraft)
	}
	// 空中掩护跟着被掩护的舰队走：目标 hex 里有己方舰队时，认领其中一艘，
	// 每回合把 TargetCoord 更新到它当前位置，而不是钉在起飞时的旧 hex。
	if aircraft.CoverUnitID != "" {
		if guarded := state.Units[aircraft.CoverUnitID]; guarded != nil && guarded.Faction == aircraft.Faction {
			aircraft.TargetCoord = guarded.Coord
		} else {
			aircraft.CoverUnitID = ""
		}
	}
	home := carrier.Coord
	if aircraft.Range() <= Distance(aircraft.Coord, home)+1 {
		aircraft.Returning = true
		return []EngineEvent{{Type: "cover_rtb", Audience: aircraft.Faction, Text: fmt.Sprintf("%s燃料只够返航，撤出%s的空中掩护", aircraft.Model, aircraft.TargetCoord)}}
	}
	if aircraft.Coord != aircraft.TargetCoord {
		for steps := 0; steps < 2 && aircraft.Coord != aircraft.TargetCoord; steps++ {
			next := nextFlightCoord(*state, aircraft, aircraft.TargetCoord)
			if next == "" {
				break
			}
			aircraft.Coord = next
			aircraft.Spend(1)
		}
		if aircraft.Coord == aircraft.TargetCoord {
			return []EngineEvent{{Type: "cover_on_station", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s上空，开始执行空中掩护", aircraft.Model, aircraft.TargetCoord)}}
		}
		return nil
	}
	// 在目标上空值班：每回合消耗 1 点航程。
	aircraft.Spend(1)
	return nil
}

// advanceASW 让反潜巡逻编队跟着母舰走：母舰在哪，巡逻空域就在哪。
// 反潜巡逻只给潜艇战提供航空支援（规则书〈猎杀潜航〉），不参加防空拦截。
func advanceASW(state *State, aircraft *AirUnit) []EngineEvent {
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		aircraft.Returning = true
		return advanceAircraftReturn(state, aircraft)
	}
	aircraft.Coord = carrier.Coord
	aircraft.TargetCoord = carrier.Coord
	aircraft.PatrolCoord = carrier.Coord
	// 和 CAP 一样：燃料只够回家就自动返航，不能一直烧到航程 -5 被销毁。
	if hasRTBDestination(*state, aircraft) && aircraft.Range() <= rtbDistance(*state, aircraft) {
		aircraft.Returning = true
		return []EngineEvent{{Type: "asw_rtb", Audience: aircraft.Faction, Text: fmt.Sprintf("%s剩余航程%d不足以往返，撤出反潜巡逻返航", aircraft.Model, aircraft.Range())}}
	}
	aircraft.Spend(1)
	return nil
}

// advanceTransfer 处理转场：飞到目标舰队，降落在那支舰队里有空位的航母上。
// 舰队机库放不下时算子挂在天上（CannotLand），由玩家手动改转场目标。
func advanceTransfer(state *State, aircraft *AirUnit) []EngineEvent {
	if aircraft.Returning {
		// 已经放弃转场、或者被夜里召回：走统一的返航逻辑，别再往目的地飞。
		return advanceAircraftReturn(state, aircraft)
	}
	// 转场到舰队时目标 hex 只是下命令那一刻舰队所在的位置，舰队一开走就只剩空海。
	// 认了舰队 ID 就每回合按它的当前位置校正，别让编队对着空 hex 烧油。
	if aircraft.TargetFleetID != "" {
		if coord, ok := friendlyFleetCoord(*state, aircraft.Faction, aircraft.TargetFleetID); ok {
			aircraft.TargetCoord = coord
		} else {
			aircraft.TargetFleetID = ""
			aircraft.Returning = true
			return []EngineEvent{{Type: "transfer_aborted", Audience: aircraft.Faction, Text: fmt.Sprintf("%s要转场去的舰队已经不在，编队返航母舰", aircraft.Model)}}
		}
	}
	if aircraft.Coord != aircraft.TargetCoord {
		// 追着舰队飞可能越追越远：剩的油连目标格都够不着就放弃，别烧到航程 -5 被销毁。
		if aircraft.Range() < Distance(aircraft.Coord, aircraft.TargetCoord) {
			aircraft.Returning = true
			return []EngineEvent{{Type: "transfer_rtb", Audience: aircraft.Faction, Text: fmt.Sprintf("%s剩余航程%d够不到%s，放弃转场返航母舰", aircraft.Model, aircraft.Range(), aircraft.TargetCoord)}}
		}
		// 规则书 209 行：直接转场的飞机每回合只移动 1 hex。
		for steps := 0; steps < 1 && aircraft.Coord != aircraft.TargetCoord; steps++ {
			next := nextFlightCoord(*state, aircraft, aircraft.TargetCoord)
			if next == "" {
				break
			}
			aircraft.Coord = next
			aircraft.Spend(1)
		}
		if aircraft.Coord != aircraft.TargetCoord {
			aircraft.CannotLand = false
			return nil
		}
	}
	host := transferHost(*state, aircraft)
	if host == nil {
		// 目标 hex 上连己方航母都没有：这不是"机库满"，是目的地没了（舰队沉了/开走了），
		// 再挂在天上等只会一路烧油到航程 -5 被销毁。
		if !friendlyCarrierAt(*state, aircraft.Faction, aircraft.TargetCoord) {
			aircraft.TargetFleetID = ""
			aircraft.Returning = true
			return []EngineEvent{{Type: "transfer_aborted", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s，但这里没有己方航母，转场中止返航母舰", aircraft.Model, aircraft.TargetCoord)}}
		}
		// 只在刚发现降不下去的那一回合报警，避免每回合刷同一条事件。
		firstTime := !aircraft.CannotLand
		aircraft.CannotLand = true
		aircraft.Spend(1)
		if !firstTime {
			return nil
		}
		return []EngineEvent{{Type: "transfer_cannot_land", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s，但该舰队机库已满，只能继续在空中等待", aircraft.Model, aircraft.TargetCoord)}}
	}
	aircraft.CannotLand = false
	aircraft.CarrierID = host.ID
	aircraft.Returning = true
	LandAircraft(state, aircraft, host)
	return []EngineEvent{{Type: "transfer_landed", Audience: aircraft.Faction, Text: fmt.Sprintf("%s转场降落在%s", aircraft.Model, host.Name)}}
}

// friendlyCarrierAt 判断指定 hex 上有没有己方航母（不看机库还有没有空位）。
func friendlyCarrierAt(state State, faction, coord string) bool {
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit != nil && unit.Faction == faction && unit.Coord == coord && IsCarrier(unit) {
			return true
		}
	}
	return false
}

// transferHost 在目标 hex 的友军舰队里挑一艘还有机库空位的航母。
func transferHost(state State, aircraft *AirUnit) *Unit {
	var best *Unit
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil || unit.Faction != aircraft.Faction || unit.Coord != aircraft.TargetCoord || !IsCarrier(unit) {
			continue
		}
		if unit.Card.Aircraft >= unit.Card.HangarCapacity {
			continue
		}
		best = unit
		break
	}
	return best
}
