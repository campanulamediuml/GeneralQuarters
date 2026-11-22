package game

// —— 返航（RTB）距离与阈值（规则书 67–78、617–618 行）——
//
// 规则书 618 行原文："当剩余航程小于或等于返回母舰或预定备降目的地所需距离时，停止搜索并返航。"
// 也就是说返航阈值是按"剩余航程 = 到家所需距离"触发的，不是真的把燃油烧光。
// "所需距离"要按实际飞行路径算（要绕开岛屿和暗礁），不是直线距离 ——
// 用直线距离会让飞机在绕路时被判定"还能回家"，实际上飞不到。

// rtbDestination 返回编队应该返航的目的地坐标：优先后备降母舰，否则母舰。
func rtbDestination(state State, aircraft *AirUnit) string {
	if aircraft == nil {
		return ""
	}
	if aircraft.ReturnToAirfield != "" {
		if airfield := airfieldFor(&state, aircraft.ReturnToAirfield); airfield != nil {
			return airfield.Coord
		}
	}
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		return carrier.Coord
	}
	// 没有母舰时回到起飞点；起飞点为空则没有可返航的地方（视为不需要返航判定）。
	return aircraft.OriginCoord
}

// hasRTBDestination 判断编队是否有明确的返航目的地。
// 没有目的地（例如测试里手工构造的无母舰编队）时不做返航阈值判定，
// 否则会把"当前位置"当成家，导致每走一步都触发返航。
func hasRTBDestination(state State, aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	if aircraft.ReturnToAirfield != "" {
		return true
	}
	if state.Units[aircraft.CarrierID] != nil {
		return true
	}
	return aircraft.OriginCoord != ""
}

// rtbDistance 返回编队回到返航目的地所需的航程，按实际可飞行路径计算。
// 路径不可达时退化为直线距离（避免返回 0 让飞机误以为已经到家）。
func rtbDistance(state State, aircraft *AirUnit) int {
	dest := rtbDestination(state, aircraft)
	if dest == "" || aircraft == nil || aircraft.Coord == "" {
		return 0
	}
	if aircraft.Coord == dest {
		return 0
	}
	if steps, ok := flightPathLength(state, aircraft, aircraft.Coord, dest); ok {
		return steps
	}
	return Distance(aircraft.Coord, dest)
}

// hostageAirfieldRtb 是机场飞机版本的返航距离。
func airfieldRtbDistance(state State, aircraft *AirfieldAirUnit) int {
	if aircraft == nil {
		return 0
	}
	airfield := airfieldFor(&state, aircraft.AirfieldID)
	if airfield == nil || aircraft.Coord == "" || aircraft.Coord == airfield.Coord {
		return 0
	}
	if steps, ok := groundFlightPathLength(state, aircraft, aircraft.Coord, airfield.Coord); ok {
		return steps
	}
	return Distance(aircraft.Coord, airfield.Coord)
}

// shouldRTB 判断编队是否已经应该停止当前任务、转向返航。
// 触发条件：剩余航程不足以覆盖返航所需距离（规则书 618 行）。
func shouldRTB(state State, aircraft *AirUnit) bool {
	if aircraft == nil || aircraft.Returning {
		return false
	}
	return aircraft.Range() <= rtbDistance(state, aircraft)
}

// shouldGroundRTB 是机场飞机版本。
func shouldGroundRTB(state State, aircraft *AirfieldAirUnit) bool {
	if aircraft == nil || aircraft.Returning {
		return false
	}
	return aircraft.Range() <= airfieldRtbDistance(state, aircraft)
}

// flightPathLength 用 BFS 求两个 hex 之间的实际飞行步数（考虑岛屿与暗礁）。
func flightPathLength(state State, aircraft *AirUnit, from, to string) (int, bool) {
	if from == to {
		return 0, true
	}
	visited := map[string]bool{from: true}
	type node struct {
		coord string
		steps int
	}
	queue := []node{{coord: from}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range Neighbors(current.coord) {
			if visited[neighbor] || !inBounds(state.Map, neighbor) {
				continue
			}
			if !flightCanEnter(state, aircraft, neighbor) {
				continue
			}
			if neighbor == to {
				return current.steps + 1, true
			}
			visited[neighbor] = true
			queue = append(queue, node{coord: neighbor, steps: current.steps + 1})
		}
	}
	return 0, false
}

// groundFlightPathLength 是机场飞机版本的路径长度。
func groundFlightPathLength(state State, aircraft *AirfieldAirUnit, from, to string) (int, bool) {
	if from == to {
		return 0, true
	}
	visited := map[string]bool{from: true}
	type node struct {
		coord string
		steps int
	}
	queue := []node{{coord: from}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range Neighbors(current.coord) {
			if visited[neighbor] || !inBounds(state.Map, neighbor) {
				continue
			}
			visited[neighbor] = true
			if neighbor == to {
				return current.steps + 1, true
			}
			queue = append(queue, node{coord: neighbor, steps: current.steps + 1})
		}
	}
	return 0, false
}
