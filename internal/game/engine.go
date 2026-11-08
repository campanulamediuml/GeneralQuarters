package game

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadScenario(contentDir, scenarioID string) (ScenarioDefinition, MapDefinition, UnitCatalog, error) {
	var scenario ScenarioDefinition
	var gameMap MapDefinition
	var catalog UnitCatalog
	if scenarioID != "_sandbox" {
		return scenario, gameMap, catalog, fmt.Errorf("剧本不存在")
	}
	if err := readJSON(filepath.Join(contentDir, "scenarios", scenarioID, "scenario.json"), &scenario); err != nil {
		return scenario, gameMap, catalog, err
	}
	if err := readJSON(filepath.Join(contentDir, "maps", scenario.Map, "map.json"), &gameMap); err != nil {
		return scenario, gameMap, catalog, err
	}
	if err := readJSON(filepath.Join(contentDir, "units", "sandbox.json"), &catalog); err != nil {
		return scenario, gameMap, catalog, err
	}
	if gameMap.Rows < 1 || gameMap.Rows > 99 || gameMap.Cols < 1 || gameMap.Cols > 99 {
		return scenario, gameMap, catalog, fmt.Errorf("地图行列必须在1到99之间")
	}
	seenCells := map[string]bool{}
	for _, cell := range gameMap.Cells {
		row, col, err := ParseCoord(cell.Coord)
		if err != nil || row > gameMap.Rows || col > gameMap.Cols || seenCells[cell.Coord] {
			return scenario, gameMap, catalog, fmt.Errorf("地图包含无效或重复坐标：%s", cell.Coord)
		}
		seenCells[cell.Coord] = true
	}
	return scenario, gameMap, catalog, nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("解析%s失败：%w", filepath.Base(path), err)
	}
	return nil
}

func NewState(scenario ScenarioDefinition, gameMap MapDefinition, catalog UnitCatalog, seed uint64) (State, error) {
	state := State{
		ScenarioID: scenario.ID, ScenarioName: scenario.Name, Description: scenario.Description,
		Map: gameMap, Units: map[string]*Unit{}, Aircraft: map[string]*AirUnit{}, CarrierOperations: map[string]string{}, Contacts: map[string]map[string]*FleetContact{}, RNGState: seed,
		Definitions: map[string]string{"scenario": fmt.Sprint(scenario.Version), "map": fmt.Sprint(gameMap.Version), "units": fmt.Sprint(catalog.Version)},
	}
	definitions := map[string]UnitDefinition{}
	for _, definition := range catalog.Units {
		definitions[definition.ID] = definition
	}
	fleetHomes := map[string]Deployment{}
	for _, deployment := range scenario.Deployments {
		if deployment.Faction != "A" && deployment.Faction != "B" {
			return state, fmt.Errorf("部署%s阵营无效", deployment.ID)
		}
		if _, exists := state.Units[deployment.ID]; exists {
			return state, fmt.Errorf("部署ID重复：%s", deployment.ID)
		}
		definition, ok := definitions[deployment.Unit]
		if !ok {
			return state, fmt.Errorf("单位定义不存在：%s", deployment.Unit)
		}
		switch definition.Type {
		case "BB", "BC", "CA", "CL", "DD", "CV", "CVL", "TRS", "CVE":
		default:
			return state, fmt.Errorf("单位%s舰船类型无效：%s", definition.ID, definition.Type)
		}
		if definition.Strength <= 0 {
			return state, fmt.Errorf("单位%s生命值必须大于0", definition.ID)
		}
		if !inBounds(gameMap, deployment.Coord) || terrainAt(gameMap, deployment.Coord) == "island" || terrainAt(gameMap, deployment.Coord) == "reef" {
			return state, fmt.Errorf("部署%s坐标不可通行", deployment.ID)
		}
		fleetID := deployment.FleetID
		if fleetID == "" {
			fleetID = deployment.ID
		}
		fleetName := deployment.FleetName
		if fleetName == "" {
			fleetName = fleetID
		}
		if home, exists := fleetHomes[fleetID]; exists && (home.Faction != deployment.Faction || home.Coord != deployment.Coord) {
			return state, fmt.Errorf("舰队%s的初始成员必须属于同一阵营和hex", fleetID)
		}
		fleetHomes[fleetID] = deployment
		card := definition.Card
		card.Aircraft = 0
		if deployment.HangarCapacity < 0 {
			return state, fmt.Errorf("部署%s机库容量不能为负数", deployment.ID)
		}
		if card.HangarCapacity == 0 {
			switch definition.Type {
			case "CV":
				card.HangarCapacity = 24
			case "CVL", "CVE":
				card.HangarCapacity = 9
			}
		}
		if deployment.HangarCapacity > 0 {
			card.HangarCapacity = deployment.HangarCapacity
		}
		card.Gunnery = int(math.Round(card.MainGunCaliber * float64(card.MainGunCount) / 9))
		if card.SpeedClass == "" {
			card.SpeedClass = "normal"
		}
		state.Units[deployment.ID] = &Unit{
			ID: deployment.ID, DefinitionID: definition.ID, FleetID: fleetID, FleetName: fleetName, Name: deployment.Name, Type: definition.Type, Faction: deployment.Faction,
			Coord: deployment.Coord, Heading: "NE", Strength: definition.Strength, MaxStrength: definition.Strength, Card: card,
		}
	}
	airDefinitions := map[string]AircraftDefinition{}
	for _, definition := range catalog.Air {
		airDefinitions[definition.ID] = definition
	}
	for _, deployment := range scenario.AirDeployments {
		if _, exists := state.Aircraft[deployment.ID]; exists {
			return state, fmt.Errorf("航空部署ID重复：%s", deployment.ID)
		}
		carrier := state.Units[deployment.Carrier]
		if carrier == nil || carrier.Card.HangarCapacity <= 0 {
			return state, fmt.Errorf("航空部署%s的载机舰无效", deployment.ID)
		}
		definition, ok := airDefinitions[deployment.Unit]
		if !ok {
			return state, fmt.Errorf("航空单位定义不存在：%s", deployment.Unit)
		}
		if carrier.Card.Aircraft >= carrier.Card.HangarCapacity {
			return state, fmt.Errorf("载机舰%s的机库容量不足", carrier.ID)
		}
		maxRange := int(math.Round(float64(definition.Range) * 1.3))
		state.Aircraft[deployment.ID] = &AirUnit{
			ID: deployment.ID, DefinitionID: definition.ID, CarrierID: carrier.ID, Faction: carrier.Faction,
			Name: definition.Name, Model: definition.Model, Kind: definition.Kind, Status: "hangar", Strength: definition.Strength,
			MaxRange: maxRange, RemainingRange: maxRange, AirCombat: definition.AirCombat, AntiShip: definition.AntiShip, AntiGround: definition.AntiGround, Seaplane: definition.Seaplane,
		}
		carrier.Card.Aircraft++
	}
	return state, nil
}

func IsCarrier(unit *Unit) bool {
	return unit != nil && (unit.Type == "CV" || unit.Type == "CVL" || unit.Type == "CVE")
}

func inBounds(gameMap MapDefinition, coord string) bool {
	row, col, err := ParseCoord(coord)
	return err == nil && row <= gameMap.Rows && col <= gameMap.Cols
}

func terrainAt(gameMap MapDefinition, coord string) string {
	for _, cell := range gameMap.Cells {
		if cell.Coord == coord {
			return cell.Terrain
		}
	}
	return "water"
}

func LegalMoves(state State, unit *Unit) []string {
	if unit == nil {
		return nil
	}
	if unit.Moved {
		return []string{}
	}
	for _, member := range state.Units {
		if FleetKey(member) == FleetKey(unit) && member.Moved {
			return []string{}
		}
	}
	moves := []string{}
	for _, coord := range Neighbors(unit.Coord) {
		if !inBounds(state.Map, coord) {
			continue
		}
		terrain := terrainAt(state.Map, coord)
		if terrain != "island" && terrain != "reef" {
			moves = append(moves, coord)
		}
	}
	return moves
}

func FleetKey(unit *Unit) string {
	if unit.FleetID == "" {
		return unit.ID
	}
	return unit.FleetID
}

func headingBetween(from, to string) string {
	fromRow, fromCol, fromErr := ParseCoord(from)
	toRow, toCol, toErr := ParseCoord(to)
	if fromErr != nil || toErr != nil {
		return "NE"
	}
	if toRow == fromRow {
		if toCol > fromCol {
			return "E"
		}
		return "W"
	}
	if toRow < fromRow {
		if (fromRow%2 == 0 && toCol < fromCol) || (fromRow%2 == 1 && toCol == fromCol) {
			return "NW"
		}
		return "NE"
	}
	if (fromRow%2 == 0 && toCol < fromCol) || (fromRow%2 == 1 && toCol == fromCol) {
		return "SW"
	}
	return "SE"
}

func ValidateOrder(state State, faction string, order Order) error {
	if order.Type == "LAUNCH_CAP" || order.Type == "LAUNCH_RECON" || order.Type == "RECOVER_AIR" || order.Type == "PRIORITIZE_AIR_TASK" {
		return ValidateAirOrder(state, faction, order)
	}
	if order.Type == "REORGANIZE_HEX" {
		return validateReorganization(state, faction, order)
	}
	unit, ok := state.Units[order.UnitID]
	if !ok || unit.Faction != faction {
		return fmt.Errorf("单位不存在或不属于当前玩家")
	}
	switch order.Type {
	case "CLEAR_MOVE":
		return nil
	case "SET_MOVE":
		if unit.Moved {
			return fmt.Errorf("该舰队本回合已经移动")
		}
		if !inBounds(state.Map, order.Coord) {
			return fmt.Errorf("目标坐标不在地图内")
		}
		for _, coord := range LegalMoves(state, unit) {
			if coord == order.Coord {
				return nil
			}
		}
		return fmt.Errorf("目标必须与当前hex相邻且可通行")
	case "MERGE_FLEET":
		target, ok := state.Units[order.TargetUnitID]
		if !ok || target.Faction != faction {
			return fmt.Errorf("目标舰队不存在或不属于当前玩家")
		}
		if unit.Coord != target.Coord {
			return fmt.Errorf("只有同一hex内的舰队可以整编")
		}
		if FleetKey(unit) == FleetKey(target) {
			return fmt.Errorf("这两个单位已经属于同一舰队")
		}
		return nil
	default:
		return fmt.Errorf("不支持的命令类型")
	}
}

func validateReorganization(state State, faction string, order Order) error {
	expected := map[string]bool{}
	for _, unit := range state.Units {
		if unit.Faction == faction && unit.Coord == order.Coord {
			expected[unit.ID] = true
		}
	}
	if len(expected) == 0 || len(order.Assignments) != len(expected) {
		return fmt.Errorf("整编必须包含当前hex内的全部己方舰船")
	}
	fleetSizes := map[string]int{}
	for unitID, fleetID := range order.Assignments {
		if !expected[unitID] || strings.TrimSpace(fleetID) == "" || len(fleetID) > 64 {
			return fmt.Errorf("整编包含无效单位或舰队标识")
		}
		fleetSizes[fleetID]++
		if fleetSizes[fleetID] > 20 {
			return fmt.Errorf("一支舰队最多允许20艘船")
		}
	}
	return nil
}

func ApplyImmediateMove(state *State, order Order) []EngineEvent {
	leader := state.Units[order.UnitID]
	events := []EngineEvent{}
	for _, unit := range state.Units {
		if FleetKey(unit) != FleetKey(leader) {
			continue
		}
		from := unit.Coord
		unit.Coord = order.Coord
		unit.Heading = headingBetween(from, order.Coord)
		unit.Moved = true
		events = append(events, EngineEvent{Type: "unit_moved", Audience: unit.Faction, Text: fmt.Sprintf("%s从%s移动至%s", unit.Name, from, order.Coord)})
	}
	return append(events, resolveContacts(state)...)
}

func ApplyImmediateOrganization(state *State, order Order) []EngineEvent {
	if order.Type == "REORGANIZE_HEX" {
		faction := ""
		for unitID := range order.Assignments {
			if unit := state.Units[unitID]; unit != nil {
				faction = unit.Faction
			}
			break
		}
		if validateReorganization(*state, faction, order) != nil {
			return nil
		}
		fleetNames := map[string]string{}
		for _, unit := range state.Units {
			if unit.FleetName != "" {
				fleetNames[FleetKey(unit)] = unit.FleetName
			}
		}
		for unitID, fleetID := range order.Assignments {
			state.Units[unitID].FleetID = fleetID
			state.Units[unitID].FleetName = fleetNames[fleetID]
			if state.Units[unitID].FleetName == "" {
				state.Units[unitID].FleetName = fleetID
			}
		}
		return []EngineEvent{{Type: "fleet_reorganized", Audience: faction, Text: order.Coord + "内舰队编成已更新"}}
	}
	if order.Type == "MERGE_FLEET" {
		source := state.Units[order.UnitID]
		target := state.Units[order.TargetUnitID]
		fleetID := FleetKey(target)
		for _, unit := range state.Units {
			if unit.Faction == source.Faction && unit.Coord == source.Coord && FleetKey(unit) == FleetKey(source) {
				unit.FleetID = fleetID
			}
		}
		return []EngineEvent{{Type: "fleet_merged", Audience: source.Faction, Text: fmt.Sprintf("%s与%s在%s完成整编", source.Name, target.Name, source.Coord)}}
	}
	return nil
}

func ResolveOrders(state *State, orders []Order) []EngineEvent {
	for _, unit := range state.Units {
		unit.Moved = false
	}
	events := AdvanceAviation(state)
	for _, order := range orders {
		if order.Type != "REORGANIZE_HEX" {
			continue
		}
		faction := ""
		for unitID := range order.Assignments {
			if unit := state.Units[unitID]; unit != nil {
				faction = unit.Faction
			}
			break
		}
		if validateReorganization(*state, faction, order) != nil {
			continue
		}
		for unitID, fleetID := range order.Assignments {
			state.Units[unitID].FleetID = fleetID
		}
		events = append(events, EngineEvent{Type: "fleet_reorganized", Audience: faction, Text: order.Coord + "内舰队编成已更新"})
	}
	for _, order := range orders {
		if order.Type != "MERGE_FLEET" {
			continue
		}
		source, sourceOK := state.Units[order.UnitID]
		target, targetOK := state.Units[order.TargetUnitID]
		if !sourceOK || !targetOK || source.Coord != target.Coord || source.Faction != target.Faction {
			continue
		}
		fleetID := FleetKey(target)
		for _, unit := range state.Units {
			if unit.Faction == source.Faction && unit.Coord == source.Coord && FleetKey(unit) == FleetKey(source) {
				unit.FleetID = fleetID
			}
		}
		events = append(events, EngineEvent{Type: "fleet_merged", Audience: source.Faction, Text: fmt.Sprintf("%s与%s在%s完成整编", source.Name, target.Name, source.Coord)})
	}
	moves := map[string]string{}
	for _, order := range orders {
		if order.Type != "SET_MOVE" {
			continue
		}
		leader, ok := state.Units[order.UnitID]
		if !ok {
			continue
		}
		for _, unit := range state.Units {
			if FleetKey(unit) == FleetKey(leader) {
				moves[unit.ID] = order.Coord
			}
		}
	}
	ids := sortedUnitIDs(state.Units)
	for _, id := range ids {
		coord, ok := moves[id]
		if !ok {
			continue
		}
		unit := state.Units[id]
		from := unit.Coord
		unit.Coord = coord
		unit.Heading = headingBetween(from, coord)
		events = append(events, EngineEvent{Type: "unit_moved", Audience: unit.Faction, Text: fmt.Sprintf("%s从%s移动至%s", unit.Name, from, coord)})
	}
	return append(events, resolveContacts(state)...)
}

func resolveContacts(state *State) []EngineEvent {
	events := []EngineEvent{}
	coords := map[string]bool{}
	for _, unit := range state.Units {
		coords[unit.Coord] = true
	}
	coordList := make([]string, 0, len(coords))
	for coord := range coords {
		coordList = append(coordList, coord)
	}
	sort.Strings(coordList)
	for _, coord := range coordList {
		var combatants []*Unit
		factions := map[string]bool{}
		for _, unit := range state.Units {
			if unit.Coord == coord {
				combatants = append(combatants, unit)
				factions[unit.Faction] = true
			}
		}
		if len(factions) < 2 {
			continue
		}
		sort.Slice(combatants, func(i, j int) bool { return combatants[i].ID < combatants[j].ID })
		events = append(events, EngineEvent{Type: "contact", Audience: "all", Text: "双方在" + coord + "发生接触，服务器开始测试炮战"})
		damage := map[string]int{}
		for _, attacker := range combatants {
			var targets []*Unit
			for _, target := range combatants {
				if target.Faction != attacker.Faction {
					targets = append(targets, target)
				}
			}
			if len(targets) == 0 {
				continue
			}
			target := targets[nextUint64(&state.RNGState)%uint64(len(targets))]
			roll := int(nextUint64(&state.RNGState)%6) + 1
			if roll >= 4 {
				damage[target.ID]++
				events = append(events, EngineEvent{Type: "hit", Audience: "all", Text: fmt.Sprintf("%s炮击%s：d6=%d，命中", attacker.Name, target.Name, roll)})
			} else {
				events = append(events, EngineEvent{Type: "miss", Audience: "all", Text: fmt.Sprintf("%s炮击%s：d6=%d，未命中", attacker.Name, target.Name, roll)})
			}
		}
		for id, amount := range damage {
			unit := state.Units[id]
			unit.Strength -= amount
			if unit.Strength <= 0 {
				events = append(events, EngineEvent{Type: "unit_destroyed", Audience: "all", Text: unit.Name + "在测试炮战中被摧毁"})
				delete(state.Units, id)
			}
		}
	}
	return events
}

func VisibleUnits(state State, faction string) []Unit {
	units := []Unit{}
	visibleFleets := map[string]bool{}
	for _, unit := range state.Units {
		visible := unit.Faction == faction
		if !visible {
			for _, friendly := range state.Units {
				if friendly.Faction == faction && Distance(friendly.Coord, unit.Coord) <= 2 {
					visible = true
					break
				}
			}
		}
		if visible {
			copyUnit := *unit
			copyUnit.FleetID = FleetKey(unit)
			visibleFleets[copyUnit.FleetID] = true
			if copyUnit.Heading == "" {
				copyUnit.Heading = "NE"
			}
			copyUnit.LegalMoves = nil
			if unit.Faction == faction {
				copyUnit.LegalMoves = LegalMoves(state, unit)
			}
			units = append(units, copyUnit)
		}
	}
	for _, contact := range state.Contacts[faction] {
		if contact.Faction == faction || visibleFleets[contact.FleetID] {
			continue
		}
		units = append(units, projectedContactUnits(*contact)...)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })
	return units
}

func VisibleFleets(state State, faction string) []FleetGroup {
	groups := map[string][]Unit{}
	for _, unit := range VisibleUnits(state, faction) {
		id := FleetKey(&unit)
		groups[id] = append(groups[id], unit)
	}
	fleets := []FleetGroup{}
	for id, members := range groups {
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		leader := members[0]
		fleet := FleetGroup{ID: leader.ID, FleetID: id, FleetName: leader.FleetName, Name: leader.Name, Type: leader.Type, Faction: leader.Faction, Coord: leader.Coord, Heading: leader.Heading, Count: len(members), Members: members, LegalMoves: leader.LegalMoves}
		if fleet.Faction == faction {
			fleet.DistanceReference = BuildDistanceReference(state.Map, fleet.Coord, 8)
		}
		if leader.FleetName != "" {
			fleet.Name = leader.FleetName
		} else if len(members) > 1 {
			fleet.Name += "编队"
		}
		for _, unit := range members {
			fleet.Strength += unit.Strength
			fleet.MaxStrength += unit.MaxStrength
			fleet.Moved = fleet.Moved || unit.Moved
		}
		fleets = append(fleets, fleet)
	}
	sort.Slice(fleets, func(i, j int) bool { return fleets[i].FleetID < fleets[j].FleetID })
	return fleets
}

func sortedUnitIDs(units map[string]*Unit) []string {
	ids := make([]string, 0, len(units))
	for id := range units {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func nextUint64(state *uint64) uint64 {
	if *state == 0 {
		*state = 0x9e3779b97f4a7c15
	}
	x := *state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	*state = x
	return x * 2685821657736338717
}

func NormalizeToken(token string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(token), "-", ""))
}
