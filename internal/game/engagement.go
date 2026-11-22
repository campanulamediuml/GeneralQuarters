package game

import "fmt"

// —— 空袭接敌（规则书 604–618 行）——

// engageChance 计算攻击编队搜索并接敌的成功率：
// 目标同 hex 基础 200%；目标在 1 hex 内基础 100%，再乘天气系数，并叠加舰队规模、漏油起火、航程与编队规模修正。
func engageChance(state State, faction, coord, target string, aircraft []*AirUnit) float64 {
	// 兼容入口：按"目标 hex 上有哪支舰队"换算。引擎实际走 engageChanceAgainstFleet，
	// 因为舰队常常已经离开最后已知 hex，这时要按真实位置判距离。
	return engageChanceAgainstFleet(state, faction, coord, fleetAtCoord(state, target), target, aircraft)
}

// engageChanceAgainstFleet 用舰队的真实位置计算接敌成功率：
// 位于同一 hex 基础 200%，相邻 1 hex 基础 100%×天气系数，再叠加漏油/起火/规模/编队规模修正。
// 距离必须按"编队当前位置 ↔ 舰队当前所在"算，而不是"编队 ↔ 最后已知坐标"——
// 否则舰队前脚刚挪到邻格，贴着旧坐标的编队也会被判 0%，只能对着空海面漂到没油。
func engageChanceAgainstFleet(state State, faction, coord, fleetID, fleetCoord string, aircraft []*AirUnit) float64 {
	enemy := []Unit{}
	for _, unit := range unitsAtFleet(state, fleetCoord) {
		if unit.Faction != faction {
			enemy = append(enemy, unit)
		}
	}
	// 目标位置根本没有敌方舰队：无论距离多近都没有可接敌的对象。
	// 这条必须在距离判定之前，否则同格（距离 0 → 基础 200%）会让盲打空海域变成 100% 成功。
	if fleetID == "" || len(enemy) == 0 {
		return 0
	}
	distance := Distance(coord, fleetCoord)
	chance := 0.0
	switch {
	case distance == 0:
		chance = 200
	case distance == 1:
		chance = 100 * weatherFactor(weatherAt(state.Map, fleetCoord))
	default:
		return 0
	}
	if chance <= 0 {
		return 0
	}
	for _, unit := range enemy {
		if EnsureDamage(&unit).OilLeak {
			chance += 50
		}
	}
	for _, unit := range enemy {
		if EnsureDamage(&unit).Fire != "" {
			return chance // 有起火舰船时直接接敌成功
		}
	}
	for _, unit := range enemy {
		if isCapitalShip(unit.Type) || IsCarrier(&unit) {
			chance += 2
		} else {
			chance++
		}
	}
	chance += 5 * float64(len(aircraft))
	for _, aircraft := range aircraft {
		// 日军攻击队剩余航程小于"距离返航目的地+2"时成功率 -50%
		if aircraft.Faction == "B" {
			if mother := state.Units[aircraft.CarrierID]; mother != nil && aircraft.Range() < Distance(aircraft.Coord, mother.Coord)+2 {
				chance -= 50
				break
			}
		}
	}
	if chance < 0 {
		return 0
	}
	if chance > 100 {
		chance = 100
	}
	return chance
}

// weatherFactor 天气系数：多云 ×0.5，降水 ×0.35，风暴 ×0.2。
func weatherFactor(weather string) float64 {
	switch weather {
	case "cloud":
		return 0.5
	case "rain":
		return 0.35
	case "storm":
		return 0.2
	}
	return 1
}

func fleetAtCoord(state State, coord string) string {
	for _, unit := range state.Units {
		if unit.Coord == coord {
			return FleetKey(unit)
		}
	}
	return ""
}

func unitsAtFleet(state State, coord string) []Unit {
	result := []Unit{}
	for _, unit := range state.Units {
		if unit.Coord == coord {
			result = append(result, *unit)
		}
	}
	return result
}

// engageFleetAt 找出攻击编队这次搜索真正面对的敌方舰队：
// 优先用下达命令时认领的 TargetFleetID（它多半已经离开最后已知 hex），
// 认领不到时退回"目标 hex 上的敌方舰队"。返回舰队 ID 和它当前所在 hex。
func engageFleetAt(state State, aircraft *AirUnit) (string, string) {
	candidates := []string{}
	if aircraft.TargetFleetID != "" {
		candidates = append(candidates, aircraft.TargetFleetID)
	}
	if fleetID := fleetAtCoord(state, aircraft.TargetCoord); fleetID != "" {
		candidates = append(candidates, fleetID)
	}
	for _, fleetID := range candidates {
		members := fleetMembers(&state, fleetID)
		if len(members) > 0 && members[0].Faction != aircraft.Faction {
			return fleetID, members[0].Coord
		}
	}
	return "", ""
}

// tryEngage 在攻击编队抵达目标 hex 前做接敌判定：
// 成功则标记"已接敌、下一回合才能发起空袭"；失败则本回合不结算，按规则继续搜索。
// 接敌成功即视为目视发现（规则书 606 行：接敌成功情报直接到 4），所以同时把目标
// 改到舰队的真实位置——下一回合从这里空袭，而不是对着几回合前的旧坐标丢炸弹。
func tryEngage(state *State, aircraft *AirUnit, coord string) []EngineEvent {
	attacks := []*AirUnit{aircraft}
	if aircraft.TaskID != "" {
		attacks = taskAircraft(*state, aircraft.TaskID)
	}
	if state.EngageTried == nil {
		state.EngageTried = map[string]bool{}
	}
	state.EngageTried[aircraft.TaskID] = true
	fleetID, fleetCoord := engageFleetAt(*state, aircraft)
	chance := engageChanceAgainstFleet(*state, aircraft.Faction, coord, fleetID, fleetCoord, attacks)
	roll := probabilityRoll(&state.RNGState)
	if fleetID == "" {
		// 目标情报已经失效、附近也没有敌舰：这种“0%”不是运气差，而是根本没人可打。
		return []EngineEvent{{Type: "attack_searching", Audience: aircraft.Faction, Text: fmt.Sprintf("%s在%s未发现敌军（最后已知位置已无目标），继续搜索", aircraft.Model, coord)}}
	}
	if roll > chance {
		// 不写真实距离：那是情报之外的信息，玩家不该从战报里推出敌舰确切位置。
		return []EngineEvent{{Type: "attack_searching", Audience: aircraft.Faction, Text: fmt.Sprintf("%s在%s搜索接敌失败（成功率%.0f%%），继续搜索", aircraft.Model, coord, chance)}}
	}
	// 接敌成功：情报直逼 4，并记录下一回合进入目标上空
	members := fleetMembers(state, fleetID)
	for level := recordContact(state, aircraft.Faction, fleetID, members); level < 4; level = recordContact(state, aircraft.Faction, fleetID, members) {
	}
	for _, attacker := range attacks {
		attacker.Engaged = true
		attacker.TargetFleetID = fleetID
		attacker.TargetCoord = fleetCoord
	}
	return []EngineEvent{{Type: "attack_engaged", Audience: aircraft.Faction, Text: fmt.Sprintf("%s编队在%s搜索并接敌成功（%.0f%%），敌方在%s，下回合进入目标上空发起空袭", formationLabel(attacks), coord, chance, fleetCoord)}}
}
