package game

import (
	"encoding/json"
	"fmt"
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
		Map: gameMap, Units: map[string]*Unit{}, RNGState: seed,
		Definitions: map[string]string{"scenario": fmt.Sprint(scenario.Version), "map": fmt.Sprint(gameMap.Version)},
	}
	definitions := map[string]UnitDefinition{}
	for _, definition := range catalog.Units {
		definitions[definition.ID] = definition
	}
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
		if !inBounds(gameMap, deployment.Coord) || terrainAt(gameMap, deployment.Coord) == "island" || terrainAt(gameMap, deployment.Coord) == "reef" {
			return state, fmt.Errorf("部署%s坐标不可通行", deployment.ID)
		}
		state.Units[deployment.ID] = &Unit{
			ID: deployment.ID, Name: deployment.Name, Type: definition.Type, Faction: deployment.Faction,
			Coord: deployment.Coord, Strength: definition.Strength, MaxStrength: definition.Strength,
		}
	}
	return state, nil
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

func ValidateOrder(state State, faction string, order Order) error {
	unit, ok := state.Units[order.UnitID]
	if !ok || unit.Faction != faction {
		return fmt.Errorf("单位不存在或不属于当前玩家")
	}
	switch order.Type {
	case "CLEAR_MOVE":
		return nil
	case "SET_MOVE":
		if !inBounds(state.Map, order.Coord) {
			return fmt.Errorf("目标坐标不在地图内")
		}
		for _, coord := range LegalMoves(state, unit) {
			if coord == order.Coord {
				return nil
			}
		}
		return fmt.Errorf("目标必须与当前hex相邻且可通行")
	default:
		return fmt.Errorf("不支持的命令类型")
	}
}

func ResolveOrders(state *State, orders []Order) []EngineEvent {
	events := []EngineEvent{}
	moves := map[string]string{}
	for _, order := range orders {
		if order.Type == "SET_MOVE" {
			moves[order.UnitID] = order.Coord
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
			copyUnit.LegalMoves = nil
			if unit.Faction == faction {
				copyUnit.LegalMoves = LegalMoves(state, unit)
			}
			units = append(units, copyUnit)
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })
	return units
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
