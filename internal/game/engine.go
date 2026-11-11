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
		Year: scenario.Year, Night: scenario.Night, Doctrines: scenario.Doctrines,
		Map: gameMap, Units: map[string]*Unit{}, Aircraft: map[string]*AirUnit{}, Contacts: map[string]map[string]*FleetContact{}, RNGState: seed,
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
		terrain := terrainAt(gameMap, deployment.Coord)
		if !inBounds(gameMap, deployment.Coord) || terrain == "island" || terrain == "reef" || (terrain == "shoal" && isCarrierType(definition.Type)) {
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
	EnsureAviation(&state)
	return state, nil
}

func isCarrierType(unitType string) bool {
	return unitType == "CV" || unitType == "CVL" || unitType == "CVE"
}

func IsCarrier(unit *Unit) bool {
	return unit != nil && isCarrierType(unit.Type)
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
	if unit.Moved || unit.MoveCooldown > 0 {
		return []string{}
	}
	carrierFleet := false
	for _, member := range state.Units {
		if FleetKey(member) != FleetKey(unit) {
			continue
		}
		if member.Moved || member.MoveCooldown > 0 {
			return []string{}
		}
		if IsCarrier(member) {
			carrierFleet = true
		}
	}
	moves := []string{}
	for _, coord := range Neighbors(unit.Coord) {
		if !inBounds(state.Map, coord) {
			continue
		}
		terrain := terrainAt(state.Map, coord)
		if terrain == "island" || terrain == "reef" || (terrain == "shoal" && carrierFleet) {
			continue
		}
		moves = append(moves, coord)
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
	if order.Type == "LAUNCH_CAP" || order.Type == "LAUNCH_RECON" || order.Type == "LAUNCH_STRIKE" || order.Type == "PREPARE_AIR" || order.Type == "RETURN_TO_HANGAR" || order.Type == "RECOVER_AIR" || order.Type == "PRIORITIZE_AIR_TASK" || order.Type == "SET_STRIKE_TARGET" {
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
		if cooldown := FleetMoveCooldown(state, FleetKey(unit)); cooldown > 0 {
			return fmt.Errorf("该舰队还需等待%d回合才能移动", cooldown)
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
		if FleetConfusion(state, FleetKey(unit)) > 0 || FleetConfusion(state, FleetKey(target)) > 0 {
			return fmt.Errorf("交战后混乱的舰队无法整编")
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
			if unit.Confusion > 0 {
				return fmt.Errorf("交战后混乱的舰队无法整编")
			}
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
	interval := FleetMoveInterval(*state, leader)
	events := []EngineEvent{}
	for _, unit := range state.Units {
		if FleetKey(unit) != FleetKey(leader) {
			continue
		}
		from := unit.Coord
		unit.Coord = order.Coord
		unit.Heading = headingBetween(from, order.Coord)
		unit.Moved = true
		unit.MoveCooldown = interval
		events = append(events, EngineEvent{Type: "unit_moved", Audience: unit.Faction, Text: fmt.Sprintf("%s从%s移动至%s，%d回合后可再次移动", unit.Name, from, order.Coord, interval)})
	}
	syncEmbarkedAircraft(state)
	return append(events, detectContacts(state)...)
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
			LockFleetMovement(state, fleetID, 3)
		}
		return []EngineEvent{{Type: "fleet_reorganized", Audience: faction, Text: order.Coord + "内舰队编成已更新，3回合内无法移动"}}
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
		LockFleetMovement(state, fleetID, 3)
		return []EngineEvent{{Type: "fleet_merged", Audience: source.Faction, Text: fmt.Sprintf("%s与%s在%s完成整编，3回合内无法移动", source.Name, target.Name, source.Coord)}}
	}
	return nil
}

func ResolveOrders(state *State, orders []Order) []EngineEvent {
	AdvanceMovementCooldown(state)
	events := AdvanceShipConditions(state)
	events = append(events, AdvanceAviation(state)...)
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
			LockFleetMovement(state, fleetID, 3)
		}
		events = append(events, EngineEvent{Type: "fleet_reorganized", Audience: faction, Text: order.Coord + "内舰队编成已更新，3回合内无法移动"})
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
		LockFleetMovement(state, fleetID, 3)
		events = append(events, EngineEvent{Type: "fleet_merged", Audience: source.Faction, Text: fmt.Sprintf("%s与%s在%s完成整编，3回合内无法移动", source.Name, target.Name, source.Coord)})
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
		if unit.MoveCooldown > 0 {
			events = append(events, EngineEvent{Type: "movement_delayed", Audience: unit.Faction, Text: fmt.Sprintf("%s还需等待%d回合才能移动", unit.Name, unit.MoveCooldown)})
			continue
		}
		from := unit.Coord
		unit.Coord = coord
		unit.Heading = headingBetween(from, coord)
		unit.Moved = true
		unit.MoveCooldown = FleetMoveInterval(*state, unit)
		events = append(events, EngineEvent{Type: "unit_moved", Audience: unit.Faction, Text: fmt.Sprintf("%s从%s移动至%s，%d回合后可再次移动", unit.Name, from, coord, unit.MoveCooldown)})
	}
	syncEmbarkedAircraft(state)
	return append(events, resolveContacts(state)...)
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

func shipTypeRank(unitType string) int {
	order := map[string]int{"CV": 0, "CVL": 1, "CVE": 2, "BB": 3, "BC": 4, "CA": 5, "CL": 6, "DD": 7, "TRS": 8}
	if rank, ok := order[unitType]; ok {
		return rank
	}
	return len(order)
}

func sortUnitCards(units []Unit) {
	sort.Slice(units, func(i, j int) bool {
		if shipTypeRank(units[i].Type) == shipTypeRank(units[j].Type) {
			return units[i].ID < units[j].ID
		}
		return shipTypeRank(units[i].Type) < shipTypeRank(units[j].Type)
	})
}

func VisibleFleets(state State, faction string) []FleetGroup {
	groups := map[string][]Unit{}
	for _, unit := range VisibleUnits(state, faction) {
		id := FleetKey(&unit)
		groups[id] = append(groups[id], unit)
	}
	fleets := []FleetGroup{}
	for id, members := range groups {
		sortUnitCards(members)
		leader := members[0]
		fleet := FleetGroup{ID: leader.ID, FleetID: id, FleetName: leader.FleetName, Name: leader.Name, Type: leader.Type, Faction: leader.Faction, Coord: leader.Coord, Heading: leader.Heading, Count: len(members), Members: members, LegalMoves: leader.LegalMoves}
		if fleet.Faction == faction {
			fleet.DistanceReference = BuildDistanceReference(state.Map, fleet.Coord, 8)
			fleet.MoveInterval = FleetMoveInterval(state, &leader)
			fleet.MoveCooldown = FleetMoveCooldown(state, id)
			fleet.Confusion = FleetConfusion(state, id)
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
			if unit.ContactLevel > fleet.ContactLevel {
				fleet.ContactLevel = unit.ContactLevel
			}
		}
		if fleet.ContactLevel > 0 {
			switch {
			case fleet.Count <= 4:
				fleet.ContactSize = "small"
			case fleet.Count <= 9:
				fleet.ContactSize = "medium"
			default:
				fleet.ContactSize = "large"
			}
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
