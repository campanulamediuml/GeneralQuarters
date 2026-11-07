package game

import (
	"fmt"
	"strconv"
)

type MapCell struct {
	Coord   string `json:"coord"`
	Terrain string `json:"terrain"`
	Name    string `json:"name,omitempty"`
}

type MapDefinition struct {
	ID      string    `json:"id"`
	Version int       `json:"version"`
	Name    string    `json:"name"`
	Rows    int       `json:"rows"`
	Cols    int       `json:"cols"`
	Cells   []MapCell `json:"cells"`
}

type Deployment struct {
	ID      string `json:"id"`
	Unit    string `json:"unit"`
	Faction string `json:"faction"`
	Name    string `json:"name"`
	Coord   string `json:"coord"`
}

type ScenarioDefinition struct {
	ID          string       `json:"id"`
	Version     int          `json:"version"`
	Name        string       `json:"name"`
	Map         string       `json:"map"`
	Description string       `json:"description"`
	Deployments []Deployment `json:"deployments"`
}

type UnitDefinition struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Strength int    `json:"strength"`
}

type UnitCatalog struct {
	ID      string           `json:"id"`
	Version int              `json:"version"`
	Units   []UnitDefinition `json:"units"`
}

type Unit struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Faction     string   `json:"faction"`
	Coord       string   `json:"coord"`
	Strength    int      `json:"strength"`
	MaxStrength int      `json:"max_strength"`
	LegalMoves  []string `json:"legal_moves,omitempty"`
}

type State struct {
	ScenarioID   string            `json:"scenario_id"`
	ScenarioName string            `json:"scenario_name"`
	Description  string            `json:"description"`
	Map          MapDefinition     `json:"map"`
	Units        map[string]*Unit  `json:"units"`
	RNGState     uint64            `json:"rng_state"`
	Definitions  map[string]string `json:"definition_versions"`
}

type Order struct {
	Type   string `json:"type"`
	UnitID string `json:"unit_id,omitempty"`
	Coord  string `json:"coord,omitempty"`
}

type EngineEvent struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Audience string `json:"audience"`
}

func ParseCoord(coord string) (int, int, error) {
	if len(coord) != 4 {
		return 0, 0, fmt.Errorf("坐标必须为ABCD格式")
	}
	row, errRow := strconv.Atoi(coord[:2])
	col, errCol := strconv.Atoi(coord[2:])
	if errRow != nil || errCol != nil || row < 1 || col < 1 {
		return 0, 0, fmt.Errorf("无效hex坐标")
	}
	return row, col, nil
}

func Coord(row, col int) string {
	return fmt.Sprintf("%02d%02d", row, col)
}

func Neighbors(coord string) []string {
	row, col, err := ParseCoord(coord)
	if err != nil {
		return nil
	}
	deltas := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	if row%2 == 0 {
		deltas = append(deltas, [2]int{-1, -1}, [2]int{1, -1})
	} else {
		deltas = append(deltas, [2]int{-1, 1}, [2]int{1, 1})
	}
	result := make([]string, 0, 6)
	for _, delta := range deltas {
		r, c := row+delta[0], col+delta[1]
		if r > 0 && c > 0 {
			result = append(result, Coord(r, c))
		}
	}
	return result
}

func offsetCube(row, col int) (int, int, int) {
	zeroCol := col - 1
	zeroRow := row - 1
	x := zeroCol - (zeroRow-(zeroRow&1))/2
	z := zeroRow
	y := -x - z
	return x, y, z
}

func Distance(a, b string) int {
	ar, ac, errA := ParseCoord(a)
	br, bc, errB := ParseCoord(b)
	if errA != nil || errB != nil {
		return 999
	}
	ax, ay, az := offsetCube(ar, ac)
	bx, by, bz := offsetCube(br, bc)
	dx, dy, dz := ax-bx, ay-by, az-bz
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	if dz < 0 {
		dz = -dz
	}
	if dx > dy && dx > dz {
		return dx
	}
	if dy > dz {
		return dy
	}
	return dz
}
