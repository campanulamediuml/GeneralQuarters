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
	roll := probabilityRoll(&state.RNGState) * 100
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

func performAirAttack(state *State, attacker *AirUnit, targets []*AirUnit, attackerBonus int, hitUnits map[string]bool, report *AirCombatReport, attackerSide bool) []EngineEvent {
	if !operationalAircraft(state, attacker) {
		return nil
	}
	target := randomOperationalAircraft(state, targets)
	if target == nil {
		return nil
	}
	chance := airCombatChance(attacker.AirCombat+attackerBonus, target.AirCombat)
	if probabilityRoll(&state.RNGState)*100 >= chance {
		return nil
	}
	hitUnits[target.ID] = true
	outcome, events := applyAircraftHit(state, target)
	if outcome != "" && outcome != "无效果" {
		if attackerSide {
			report.DefenderLosses++
		} else {
			report.AttackerLosses++
		}
	}
	return events
}

func ammunitionDepletionChance(faction string, own, opposing int) float64 {
	if opposing < 1 {
		opposing = 1
	}
	ratio := float64(own) / float64(opposing)
	if faction == "B" {
		switch {
		case ratio > 5:
			return 10
		case ratio > 3:
			return 25
		case ratio >= 1:
			return 50
		default:
			return 75
		}
	}
	switch {
	case ratio > 4:
		return 10
	case ratio > 2:
		return 25
	case ratio > 0.5:
		return 50
	default:
		return 75
	}
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
			chance := ammunitionDepletionChance(aircraft.Faction, group.ownCount, group.otherCount)
			if probabilityRoll(&state.RNGState)*100 >= chance {
				continue
			}
			aircraft.Returning = true
			if group.ownSide {
				report.AttackerLosses++
			} else {
				report.DefenderLosses++
			}
			events = append(events, EngineEvent{Type: "air_ammunition_empty", Audience: aircraft.Faction, Text: aircraft.Model + "弹药耗尽，退出空战返航"})
		}
	}
	return events
}

func resolveDogfight(state *State, attackers, defenders []*AirUnit, report *AirCombatReport) []EngineEvent {
	events := []EngineEvent{}
	escorts := []*AirUnit{}
	bombers := []*AirUnit{}
	for _, aircraft := range attackers {
		if aircraft.Kind == "fighter" {
			escorts = append(escorts, aircraft)
		} else {
			bombers = append(bombers, aircraft)
		}
	}
	hitUnits := map[string]bool{}
	for _, aircraft := range escorts {
		events = append(events, performAirAttack(state, aircraft, defenders, 0, hitUnits, report, true)...)
	}
	for _, aircraft := range defenders {
		events = append(events, performAirAttack(state, aircraft, escorts, 0, hitUnits, report, false)...)
	}
	bomberPenalty := 0
	capBonus := 0
	if len(escorts) == 0 {
		bomberPenalty = -1
		capBonus = 1
	}
	for _, aircraft := range bombers {
		events = append(events, performAirAttack(state, aircraft, defenders, bomberPenalty, hitUnits, report, true)...)
	}
	for _, aircraft := range defenders {
		events = append(events, performAirAttack(state, aircraft, bombers, capBonus, hitUnits, report, false)...)
	}
	for _, aircraft := range bombers {
		if operationalAircraft(state, aircraft) && !hitUnits[aircraft.ID] && !aircraft.Confusion && probabilityRoll(&state.RNGState) < 25 {
			aircraft.Confusion = true
			events = append(events, EngineEvent{Type: "air_confusion", Audience: aircraft.Faction, Text: aircraft.Model + "在空战中被打散编队"})
		}
	}
	return append(events, resolveAmmunition(state, attackers, defenders, report)...)
}

func capCoord(state State, aircraft *AirUnit) string {
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		return carrier.Coord
	}
	return aircraft.Coord
}

func strikeCAPGroups(state State, defender, coord string) [][]*AirUnit {
	groups := map[string][]*AirUnit{}
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction == defender && aircraft.Status == "cap" && !aircraft.Returning && aircraft.TaskID != "" && Distance(capCoord(state, aircraft), coord) <= 1 {
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
	if heavy && state.Year >= 1943 && state.Doctrines[faction] == "us" {
		chance += 2
	}
	return chance
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
	for shots := int(heavy); shots > 0; shots-- {
		if probabilityRoll(&state.RNGState)*100 < chance {
			target := randomOperationalAircraft(state, attackers)
			if target == nil {
				break
			}
			hits++
			report.AAHits++
			outcome, hitEvents := applyAircraftHit(state, target)
			if outcome != "" && outcome != "无效果" {
				report.AttackerLosses++
			}
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

func lightAAValue(state State, attacker *AirUnit, target *Unit, mixed bool) float64 {
	value := target.Card.LightAA
	if attacker.Kind == "torpedo_bomber" {
		value *= 2
	}
	if mixed {
		value *= 0.5
	}
	if target.Type == "CV" && state.Doctrines[target.Faction] == "us" {
		highest := 0.0
		for _, unit := range state.Units {
			if unit.Faction == target.Faction && unit.Coord == target.Coord && unit.ID != target.ID && unit.Card.LightAA > highest {
				highest = unit.Card.LightAA
			}
		}
		value += highest
	}
	return value
}

func resolveLightAA(state *State, attacker *AirUnit, target *Unit, mixed bool, report *AirCombatReport) []EngineEvent {
	if !operationalAircraft(state, attacker) || target == nil {
		return nil
	}
	events := []EngineEvent{}
	value := lightAAValue(*state, attacker, target, mixed)
	chance := aaHitChance(*state, target.Coord, false, target.Faction)
	shots := int(value)
	if probabilityRoll(&state.RNGState) < (value-float64(shots))*100 {
		shots++
	}
	for ; shots > 0; shots-- {
		if !operationalAircraft(state, attacker) {
			break
		}
		if probabilityRoll(&state.RNGState)*100 < chance {
			report.AAHits++
			outcome, hitEvents := applyAircraftHit(state, attacker)
			if outcome != "" && outcome != "无效果" {
				report.AttackerLosses++
			}
			events = append(events, hitEvents...)
		} else {
			confuseRandomAircraft(state, []*AirUnit{attacker})
		}
	}
	return events
}

func strikeShipWeight(unitType string) int {
	switch unitType {
	case "BB", "BC":
		return 10
	case "CV", "CVL", "CVE":
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

func selectStrikeShip(state *State, ships []Unit) *Unit {
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
	switch EffectiveSpeedClass(target) {
	case "slow", "very_slow":
		chance += 5
	case "fast", "very_fast":
		chance -= 5
	}
	if EnsureDamage(target).Disabled {
		chance += 10
	}
	if target.Type == "DD" {
		chance -= 5
	}
	chance -= float64(int(target.Card.LightAA/7) * 5)
	if weatherAt(state.Map, target.Coord) == "rain" {
		chance -= 10
	}
	if attacker.Confusion {
		chance -= 5
	}
	if chance < 5 {
		return 5
	}
	if chance > 95 {
		return 95
	}
	return chance
}

func resolveBombing(state *State, attacker *AirUnit, ships []Unit, mixed, capRemaining bool, report *AirCombatReport) []EngineEvent {
	if !operationalAircraft(state, attacker) || attacker.Kind == "fighter" {
		return nil
	}
	target := selectStrikeShip(state, ships)
	if target == nil {
		return nil
	}
	events := resolveLightAA(state, attacker, target, mixed, report)
	if !operationalAircraft(state, attacker) {
		return events
	}
	antiShip := attacker.AntiShip
	if attacker.Confusion && antiShip > 0 {
		antiShip--
	}
	chance := strikeHitChance(*state, attacker, target, mixed, capRemaining)
	hits := 0
	for ; antiShip > 0; antiShip-- {
		if probabilityRoll(&state.RNGState)*100 >= chance {
			continue
		}
		hits++
		report.StrikeHits++
		if attacker.Kind == "torpedo_bomber" {
			events = append(events, applyTorpedoHit(state, ShellHit{AttackerID: attacker.ID, TargetID: target.ID, Torpedo: true})...)
		} else {
			events = append(events, applyShellHit(state, ShellHit{AttackerID: attacker.ID, TargetID: target.ID})...)
		}
	}
	events = append(events, EngineEvent{Type: "strike_bombing", Audience: "all", Text: fmt.Sprintf("%s攻击%s：命中率%.1f%%，命中%d次", attacker.Model, target.Name, chance, hits)})
	if state.Units[target.ID] != nil {
		events = append(events, resolveLightAA(state, attacker, target, mixed, report)...)
	}
	return events
}

func resolveAirStrike(state *State, taskID, coord string) []EngineEvent {
	members := taskAircraft(*state, taskID)
	attackers := []*AirUnit{}
	for _, aircraft := range members {
		if aircraft.Status == "attack" && !aircraft.Returning && aircraft.Coord == coord {
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
	groups := strikeCAPGroups(*state, ships[0].Faction, coord)
	report := AirCombatReport{Coord: coord, AttackerFaction: faction, DefenderFaction: ships[0].Faction, AttackerModels: aircraftModels(attackers), AttackerCount: len(attackers)}
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
	mixedKinds := map[string]bool{}
	for _, aircraft := range attackers {
		if aircraft.Kind != "fighter" {
			bombers = append(bombers, aircraft)
			mixedKinds[aircraft.Kind] = true
		}
	}
	mixed := len(mixedKinds) > 1
	events = append(events, resolveHeavyAA(state, bombers, ships, coord, &report)...)
	capRemaining := false
	for _, group := range groups {
		if len(activeAircraft(state, group)) > 0 {
			capRemaining = true
			break
		}
	}
	for _, aircraft := range bombers {
		events = append(events, resolveBombing(state, aircraft, ships, mixed, capRemaining, &report)...)
	}
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
	if report.DefenderCount > 0 {
		report.ID = fmt.Sprintf("air-combat-%x", nextUint64(&state.RNGState))
		report.Winner = report.AttackerFaction
		if attackersRemaining == 0 {
			report.Winner = report.DefenderFaction
		} else if !capRemaining {
			report.Winner = report.AttackerFaction
		} else if report.AttackerLosses > report.DefenderLosses {
			report.Winner = report.DefenderFaction
		} else if report.AttackerLosses == report.DefenderLosses {
			report.Winner = "draw"
		}
		report.Summary = fmt.Sprintf("%s空战：攻击方损失%d，CAP损失%d；防空命中%d，投弹命中%d", coord, report.AttackerLosses, report.DefenderLosses, report.AAHits, report.StrikeHits)
		state.AirCombats = append(state.AirCombats, report)
		if len(state.AirCombats) > 20 {
			state.AirCombats = state.AirCombats[len(state.AirCombats)-20:]
		}
		events = append(events, EngineEvent{Type: "air_combat_resolved", Audience: "all", Text: report.Summary})
	}
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

func advanceAircraftReturn(state *State, aircraft *AirUnit) []EngineEvent {
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		delete(state.Aircraft, aircraft.ID)
		return []EngineEvent{{Type: "air_destroyed", Audience: aircraft.Faction, Text: aircraft.Model + "失去母舰，航空算子被移除"}}
	}
	for steps := 0; steps < 2 && aircraft.Coord != carrier.Coord; steps++ {
		next := nextCoordToward(state.Map, aircraft.Coord, carrier.Coord)
		if next == "" {
			break
		}
		aircraft.Coord = next
		aircraft.RemainingRange--
	}
	if aircraft.Coord != carrier.Coord || !CanLandAircraft(*state, carrier) {
		return nil
	}
	LandAircraft(state, aircraft, carrier)
	aircraft.Confusion = false
	return []EngineEvent{{Type: "air_landed", Audience: aircraft.Faction, Text: strings.Join([]string{aircraft.Model, "返航", carrier.Name, "，等待升降机下降"}, "")}}
}

func VisibleAirCombats(state State, faction string) []AirCombatReport {
	reports := []AirCombatReport{}
	for _, report := range state.AirCombats {
		if report.AttackerFaction == faction || report.DefenderFaction == faction {
			reports = append(reports, report)
		}
	}
	return reports
}
