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
	events := []EngineEvent{}
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

func hasSurfaceRadar(members []Unit) bool {
	for _, unit := range members {
		if unit.Card.SurfaceRadar && !EnsureDamage(&unit).SurfaceRadarDisabled {
			return true
		}
	}
	return false
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
	doctrine := state.Doctrines[faction]
	if doctrine == "japan" && state.Year > 0 && state.Year <= 1942 {
		score++
		modifiers = append(modifiers, "精锐+1")
	}
	if doctrine == "japan" && state.Night {
		score++
		modifiers = append(modifiers, "夜战+1")
	}
	if doctrine == "japan" {
		torpedoFleet := true
		for _, unit := range members {
			switch unit.Type {
			case "TRS", "BB", "BC", "CV", "CVE", "CVL":
				torpedoFleet = false
			}
		}
		if torpedoFleet {
			score++
			modifiers = append(modifiers, "雷击队+1")
		}
	}
	if doctrine == "us" && hasSurfaceRadar(members) {
		score++
		modifiers = append(modifiers, "对海雷达+1")
	}
	if doctrine == "us" && state.Year >= 1943 {
		score++
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
	if attacker == "CVL" || attacker == "CVE" {
		attacker = "CV"
	}
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
	if attacker.Card.SurfaceRadar && !EnsureDamage(attacker).SurfaceRadarDisabled {
		chance += 10
	}
	if seaplaneSpotting(state, attacker) {
		chance += 10
	}
	if EnsureDamage(target).Fire == "burning" {
		chance += 10
	}
	difference := targetRow(attacker.Type) - targetRow(target.Type)
	switch {
	case difference == 1:
		chance -= 5
	case difference >= 2 && difference <= 3:
		chance -= 10
	case difference > 3:
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

func gunneryVolley(state *State, attackers, enemies []Unit) ([]ShellHit, []EngineEvent) {
	hits := []ShellHit{}
	events := []EngineEvent{}
	for _, attacker := range attackers {
		weapon := state.Units[attacker.ID]
		if weapon == nil || weapon.Card.Gunnery <= 0 {
			continue
		}
		target := selectWeightedTarget(state, weapon, enemies)
		if target == nil {
			continue
		}
		chance := gunneryChance(*state, weapon, target, enemies)
		count := 0
		for shot := 0; shot < weapon.Card.Gunnery; shot++ {
			if probabilityRoll(&state.RNGState) < chance {
				hits = append(hits, ShellHit{AttackerID: weapon.ID, TargetID: target.ID})
				count++
			}
		}
		events = append(events, EngineEvent{Type: "gunnery_volley", Audience: "all", Text: fmt.Sprintf("%s炮击%s：%d次判定，命中率%.1f%%，命中%d次", weapon.Name, target.Name, weapon.Card.Gunnery, chance, count)})
	}
	return hits, events
}

func torpedoVolley(state *State, attackers, enemies []Unit) ([]ShellHit, []EngineEvent) {
	hits := []ShellHit{}
	events := []EngineEvent{}
	for _, attacker := range attackers {
		weapon := state.Units[attacker.ID]
		if weapon == nil || weapon.Card.TorpedoSalvos <= 0 || EnsureDamage(weapon).TorpedoDisabled {
			continue
		}
		target := enemies[nextUint64(&state.RNGState)%uint64(len(enemies))]
		chance := 25.0
		switch target.Card.SpeedClass {
		case "slow":
			chance += 10
		case "fast":
			chance -= 10
		}
		if EnsureDamage(&target).Disabled {
			chance += 20
		}
		if target.Type == "DD" {
			chance -= 10
		}
		if state.Night {
			chance += 10
		}
		if state.Doctrines[weapon.Faction] == "us" && state.Year > 0 && state.Year < 1944 {
			chance -= 20
		}
		chance -= 5 * float64(maxConfusion(fleetMembers(state, FleetKey(weapon))))
		chance = math.Max(0, math.Min(100, chance))
		weapon.Card.TorpedoSalvos--
		count := 0
		for shot := 0; shot < 4; shot++ {
			if probabilityRoll(&state.RNGState) < chance {
				hits = append(hits, ShellHit{AttackerID: weapon.ID, TargetID: target.ID, Torpedo: true})
				count++
			}
		}
		events = append(events, EngineEvent{Type: "torpedo_volley", Audience: "all", Text: fmt.Sprintf("%s向%s齐射4枚鱼雷：命中率%.1f%%，命中%d次", weapon.Name, target.Name, chance, count)})
	}
	return hits, events
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
	if winner != "" {
		events = append(events, EngineEvent{Type: "surprise", Audience: "all", Text: winner + "取得突袭优势"})
		torpedoHits, torpedoEvents := torpedoVolley(state, factions[winner], factions[surprised])
		events = append(events, torpedoEvents...)
		for _, hit := range torpedoHits {
			hitTargets[surprised][hit.TargetID] = true
		}
		events = append(events, applyHits(state, torpedoHits)...)
		openingHits, openingEvents := gunneryVolley(state, factions[winner], factions[surprised])
		events = append(events, openingEvents...)
		for _, hit := range openingHits {
			hitTargets[surprised][hit.TargetID] = true
		}
		events = append(events, applyHits(state, openingHits)...)
	}
	leftHits, leftEvents := gunneryVolley(state, factions[left], factions[right])
	rightHits, rightEvents := gunneryVolley(state, factions[right], factions[left])
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
