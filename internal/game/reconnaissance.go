package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

var reconHeadings = map[string]float64{
	"E": 0, "SE": 60, "SW": 120, "W": 180, "NW": 240, "NE": 300,
}

func ValidReconHeading(heading string) bool {
	_, ok := reconHeadings[strings.ToUpper(heading)]
	return ok
}

func canSearch(aircraft AirUnit) bool {
	return aircraft.Kind == "recon" || aircraft.Kind == "dive_bomber" || aircraft.Kind == "torpedo_bomber"
}

func ReconRadiusLimit(aircraft AirUnit) int {
	limit := aircraft.MaxRange / 2
	if limit < 1 {
		return 1
	}
	return limit
}

type sectorPoint struct {
	x float64
	y float64
}

func ReconSectorHexes(gameMap MapDefinition, origin, heading string, width, radius int) []string {
	heading = strings.ToUpper(heading)
	axis, ok := reconHeadings[heading]
	originRow, originCol, err := ParseCoord(origin)
	if !ok || err != nil || (width != 60 && width != 120) || radius < 1 {
		return []string{}
	}
	originX, originY := hexCenter(originRow, originCol)
	hexes := []string{}
	for row := 1; row <= gameMap.Rows; row++ {
		for col := 1; col <= gameMap.Cols; col++ {
			coord := Coord(row, col)
			if coord == origin || Distance(origin, coord) > radius {
				continue
			}
			x, y := hexCenter(row, col)
			if sectorIntersectsHex(sectorPoint{x - originX, y - originY}, axis, float64(width)) {
				hexes = append(hexes, coord)
			}
		}
	}
	return hexes
}

func ReconRingHexes(gameMap MapDefinition, origin, heading string, width, ring int) []string {
	all := ReconSectorHexes(gameMap, origin, heading, width, ring)
	ringHexes := make([]string, 0, len(all))
	for _, coord := range all {
		if Distance(origin, coord) == ring {
			ringHexes = append(ringHexes, coord)
		}
	}
	return ringHexes
}

func hexCenter(row, col int) (float64, float64) {
	x := math.Sqrt(3) * (float64(col) - 0.5)
	if row%2 == 1 {
		x += math.Sqrt(3) / 2
	}
	return x, 1.5 * float64(row-1)
}

func sectorIntersectsHex(center sectorPoint, axis, width float64) bool {
	vertices := make([]sectorPoint, 6)
	for index := range vertices {
		angle := float64(index*60-30) * math.Pi / 180
		vertices[index] = sectorPoint{center.x + math.Cos(angle), center.y + math.Sin(angle)}
	}
	radius := math.Hypot(center.x, center.y) + 1.000001
	for _, vertex := range vertices {
		if math.Hypot(vertex.x, vertex.y) <= radius && angleInSector(vertex, axis, width) {
			return true
		}
	}
	for _, edgeAngle := range []float64{axis - width/2, axis + width/2} {
		radians := edgeAngle * math.Pi / 180
		rayEnd := sectorPoint{radius * math.Cos(radians), radius * math.Sin(radians)}
		for index, start := range vertices {
			if segmentsIntersect(sectorPoint{}, rayEnd, start, vertices[(index+1)%len(vertices)]) {
				return true
			}
		}
	}
	for index, start := range vertices {
		end := vertices[(index+1)%len(vertices)]
		if edgeCrossesSectorArc(start, end, radius, axis, width) {
			return true
		}
	}
	return false
}

func angleInSector(point sectorPoint, axis, width float64) bool {
	if math.Hypot(point.x, point.y) < 0.000001 {
		return true
	}
	angle := math.Atan2(point.y, point.x) * 180 / math.Pi
	delta := math.Mod(angle-axis+540, 360) - 180
	return math.Abs(delta) <= width/2+0.000001
}

func crossProduct(a, b sectorPoint) float64 {
	return a.x*b.y - a.y*b.x
}

func subtractPoint(a, b sectorPoint) sectorPoint {
	return sectorPoint{a.x - b.x, a.y - b.y}
}

func segmentsIntersect(a, b, c, d sectorPoint) bool {
	r := subtractPoint(b, a)
	s := subtractPoint(d, c)
	denominator := crossProduct(r, s)
	if math.Abs(denominator) < 0.0000001 {
		return false
	}
	ca := subtractPoint(c, a)
	t := crossProduct(ca, s) / denominator
	u := crossProduct(ca, r) / denominator
	return t >= -0.0000001 && t <= 1.0000001 && u >= -0.0000001 && u <= 1.0000001
}

func edgeCrossesSectorArc(start, end sectorPoint, radius, axis, width float64) bool {
	delta := subtractPoint(end, start)
	a := delta.x*delta.x + delta.y*delta.y
	if a < 0.0000001 {
		return false
	}
	b := 2 * (start.x*delta.x + start.y*delta.y)
	c := start.x*start.x + start.y*start.y - radius*radius
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		return false
	}
	root := math.Sqrt(discriminant)
	for _, t := range []float64{(-b - root) / (2 * a), (-b + root) / (2 * a)} {
		if t < 0 || t > 1 {
			continue
		}
		point := sectorPoint{start.x + t*delta.x, start.y + t*delta.y}
		if angleInSector(point, axis, width) {
			return true
		}
	}
	return false
}

func reconSpeed(aircraft *AirUnit) int {
	if aircraft.Seaplane {
		return 1
	}
	return 2
}

func advanceRecon(state *State, aircraft *AirUnit) []EngineEvent {
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		return nil
	}
	if aircraft.OriginCoord == "" {
		aircraft.OriginCoord = carrier.Coord
	}
	if aircraft.Coord == "" {
		aircraft.Coord = carrier.Coord
	}
	events := []EngineEvent{}
	if aircraft.TrackingFleetID != "" {
		members := fleetMembers(state, aircraft.TrackingFleetID)
		if len(members) == 0 {
			aircraft.TrackingFleetID = ""
			aircraft.Returning = true
		} else {
			aircraft.Coord = members[0].Coord
			aircraft.ReconDistance = Distance(aircraft.OriginCoord, aircraft.Coord)
			if aircraft.RemainingRange <= Distance(aircraft.Coord, carrier.Coord) {
				aircraft.TrackingFleetID = ""
				aircraft.Returning = true
				events = append(events, EngineEvent{Type: "recon_returning", Audience: aircraft.Faction, Text: aircraft.Model + "燃料达到返航余量，停止跟踪并返航"})
			} else {
				aircraft.RemainingRange--
				return append(events, resolveTracking(state, aircraft, members)...)
			}
		}
	}

	steps := reconSpeed(aircraft)
	for steps > 0 {
		if aircraft.Returning && aircraft.Coord == carrier.Coord {
			if CanLandAircraft(*state, carrier) {
				return append(events, landRecon(state, aircraft, carrier))
			}
			aircraft.RemainingRange--
			return events
		}
		if !aircraft.Returning && (aircraft.ReconDistance >= aircraft.ReconRadius || aircraft.RemainingRange <= Distance(aircraft.Coord, carrier.Coord)) {
			aircraft.Returning = true
		}
		next := ""
		if aircraft.Returning {
			next = nextCoordToward(state.Map, aircraft.Coord, carrier.Coord)
		} else {
			next = nextReconCoord(state.Map, aircraft.Coord, aircraft.Heading)
			if next == "" {
				aircraft.Returning = true
				continue
			}
		}
		if next == "" {
			aircraft.Returning = true
			continue
		}
		previousRing := Distance(aircraft.OriginCoord, aircraft.Coord)
		aircraft.Coord = next
		aircraft.ReconDistance = Distance(aircraft.OriginCoord, next)
		aircraft.RemainingRange--
		steps--
		if aircraft.ReconDistance > 0 && aircraft.ReconDistance != previousRing {
			events = append(events, searchReconRing(state, aircraft, aircraft.ReconDistance)...)
			if aircraft.TrackingFleetID != "" {
				return events
			}
		}
	}
	if aircraft.Returning && aircraft.Coord == carrier.Coord && CanLandAircraft(*state, carrier) {
		events = append(events, landRecon(state, aircraft, carrier))
	}
	return events
}

func nextReconCoord(gameMap MapDefinition, coord, heading string) string {
	row, col, err := ParseCoord(coord)
	if err != nil {
		return ""
	}
	delta := map[string][2]int{"E": {0, 1}, "W": {0, -1}}
	if row%2 == 1 {
		delta["NE"] = [2]int{-1, 0}
		delta["NW"] = [2]int{-1, 1}
		delta["SE"] = [2]int{1, 0}
		delta["SW"] = [2]int{1, 1}
	} else {
		delta["NE"] = [2]int{-1, 1}
		delta["NW"] = [2]int{-1, 0}
		delta["SE"] = [2]int{1, 1}
		delta["SW"] = [2]int{1, 0}
	}
	offset, ok := delta[strings.ToUpper(heading)]
	if !ok {
		return ""
	}
	next := Coord(row+offset[0], col+offset[1])
	if !inBounds(gameMap, next) {
		return ""
	}
	return next
}

func nextCoordToward(gameMap MapDefinition, from, target string) string {
	if from == target {
		return ""
	}
	best := ""
	bestDistance := Distance(from, target)
	for _, coord := range Neighbors(from) {
		if !inBounds(gameMap, coord) {
			continue
		}
		distance := Distance(coord, target)
		if distance < bestDistance || (distance == bestDistance && best != "" && coord < best) {
			best = coord
			bestDistance = distance
		}
	}
	return best
}

func landRecon(state *State, aircraft *AirUnit, carrier *Unit) EngineEvent {
	LandAircraft(state, aircraft, carrier)
	return EngineEvent{Type: "recon_recovered", Audience: aircraft.Faction, Text: fmt.Sprintf("%s完成扇面侦察并降落在%s甲板，等待升降机下降", aircraft.Model, carrier.Name)}
}

func searchReconRing(state *State, aircraft *AirUnit, ring int) []EngineEvent {
	hexes := ReconRingHexes(state.Map, aircraft.OriginCoord, aircraft.Heading, aircraft.SectorWidth, ring)
	inside := make(map[string]bool, len(hexes))
	for _, coord := range hexes {
		inside[coord] = true
	}
	events := []EngineEvent{{Type: "recon_ring_searched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s搜索第%d距离圈：%d° %s，共%d个hex", aircraft.Model, ring, aircraft.SectorWidth, aircraft.Heading, len(hexes))}}
	groups := map[string][]Unit{}
	for _, unit := range state.Units {
		if unit.Faction != aircraft.Faction && inside[unit.Coord] {
			key := FleetKey(unit)
			groups[key] = append(groups[key], *unit)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bestChance := -1.0
	bestFleet := ""
	for _, fleetID := range keys {
		members := groups[fleetID]
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		chance := reconDetectionChance(*state, aircraft, members)
		roll := probabilityRoll(&state.RNGState)
		if roll >= chance {
			continue
		}
		level := recordContact(state, aircraft.Faction, fleetID, members)
		events = append(events, EngineEvent{Type: "recon_contact", Audience: aircraft.Faction, Text: fmt.Sprintf("第%d圈%s发现敌方舰队：%d艘，情报等级%d（发现率%.1f%%，判定%.1f）", ring, members[0].Coord, len(members), level, chance, roll)})
		if bestChance < chance && !fleetTrackedByOther(state, aircraft, fleetID) {
			bestChance = chance
			bestFleet = fleetID
		}
	}
	if bestFleet != "" {
		aircraft.TrackingFleetID = bestFleet
		aircraft.Returning = false
		members := fleetMembers(state, bestFleet)
		if len(members) > 0 {
			aircraft.Coord = members[0].Coord
			aircraft.ReconDistance = Distance(aircraft.OriginCoord, aircraft.Coord)
		}
		events = append(events, EngineEvent{Type: "recon_tracking", Audience: aircraft.Faction, Text: fmt.Sprintf("%s开始持续跟踪%s的敌方舰队", aircraft.Model, aircraft.Coord)})
	}
	return events
}

func reconDetectionChance(state State, aircraft *AirUnit, members []Unit) float64 {
	chance := 25.0
	for _, unit := range members {
		if isCapitalShip(unit.Type) {
			chance += 5
		} else {
			chance++
		}
	}
	if len(members) > 0 {
		switch weatherAt(state.Map, members[0].Coord) {
		case "cloud":
			chance -= 5
		case "rain":
			chance -= 10
		case "storm":
			chance -= 15
		}
		chance -= 2 * float64(Distance(aircraft.OriginCoord, members[0].Coord))
		if contact := state.Contacts[aircraft.Faction][FleetKey(&members[0])]; contact != nil && contact.Level >= 1 {
			chance += 5
		}
	}
	if aircraft.SectorWidth == 120 {
		chance /= 2
	}
	return math.Max(0, math.Min(100, chance))
}

func resolveTracking(state *State, aircraft *AirUnit, members []Unit) []EngineEvent {
	chance := 50.0
	for _, unit := range members {
		if isCapitalShip(unit.Type) {
			chance += 2
		} else {
			chance++
		}
	}
	switch weatherAt(state.Map, members[0].Coord) {
	case "cloud":
		chance -= 10
	case "rain":
		chance -= 15
	case "storm":
		chance -= 20
	}
	chance = math.Max(0, math.Min(100, chance))
	roll := probabilityRoll(&state.RNGState)
	if roll >= chance {
		return []EngineEvent{{Type: "recon_tracking", Audience: aircraft.Faction, Text: aircraft.Model + "持续跟踪目标，本回合未能提升情报"}}
	}
	level := recordContact(state, aircraft.Faction, aircraft.TrackingFleetID, members)
	return []EngineEvent{{Type: "recon_tracking", Audience: aircraft.Faction, Text: fmt.Sprintf("%s持续跟踪%s的敌方舰队，情报等级提升至%d（成功率%.1f%%，判定%.1f）", aircraft.Model, members[0].Coord, level, chance, roll)}}
}

func isCapitalShip(shipType string) bool {
	return shipType == "BB" || shipType == "BC" || shipType == "CV" || shipType == "CVL" || shipType == "CVE"
}

func weatherAt(gameMap MapDefinition, coord string) string {
	for _, cell := range gameMap.Cells {
		if cell.Coord == coord {
			return strings.ToLower(cell.Weather)
		}
	}
	return ""
}

func probabilityRoll(rng *uint64) float64 {
	return float64(nextUint64(rng)>>11) / float64(uint64(1)<<53) * 100
}

func fleetMembers(state *State, fleetID string) []Unit {
	members := []Unit{}
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID {
			members = append(members, *unit)
		}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	return members
}

func fleetTrackedByOther(state *State, aircraft *AirUnit, fleetID string) bool {
	for _, other := range state.Aircraft {
		if other.ID != aircraft.ID && other.Faction == aircraft.Faction && other.Status == "recon" && other.TrackingFleetID == fleetID {
			return true
		}
	}
	return false
}

func recordContact(state *State, observer, fleetID string, members []Unit) int {
	if state.Contacts == nil {
		state.Contacts = map[string]map[string]*FleetContact{}
	}
	if state.Contacts[observer] == nil {
		state.Contacts[observer] = map[string]*FleetContact{}
	}
	contact := state.Contacts[observer][fleetID]
	if contact == nil {
		contact = &FleetContact{FleetID: fleetID}
		state.Contacts[observer][fleetID] = contact
	}
	if contact.Level < 4 {
		contact.Level++
	}
	contact.Faction = members[0].Faction
	contact.Coord = members[0].Coord
	contact.Count = len(members)
	contact.Members = make([]Unit, len(members))
	copy(contact.Members, members)
	for index := range contact.Members {
		contact.Members[index].LegalMoves = nil
	}
	return contact.Level
}

func projectedContactUnits(contact FleetContact) []Unit {
	level := contact.Level
	if level >= 3 && len(contact.Members) > 0 {
		units := make([]Unit, len(contact.Members))
		copy(units, contact.Members)
		for index := range units {
			units[index].FleetName = fmt.Sprintf("侦察接触 · 情报%d", level)
			units[index].LegalMoves = nil
		}
		return units
	}
	units := make([]Unit, 0, contact.Count)
	for index := 0; index < contact.Count; index++ {
		unit := Unit{
			ID: fmt.Sprintf("contact-%s-%02d", contact.FleetID, index+1), FleetID: contact.FleetID,
			FleetName: fmt.Sprintf("侦察接触 · 情报%d", level), Name: "未知舰船", Faction: contact.Faction, Coord: contact.Coord, Heading: "NE",
		}
		if level >= 2 && index < len(contact.Members) {
			unit.Type = contact.Members[index].Type
		}
		units = append(units, unit)
	}
	return units
}
