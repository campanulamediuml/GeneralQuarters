package game

import (
	"fmt"
	"strconv"
)

type MapCell struct {
	Coord   string `json:"coord"`
	Terrain string `json:"terrain"`
	Weather string `json:"weather,omitempty"`
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
	ID             string `json:"id"`
	Unit           string `json:"unit"`
	Faction        string `json:"faction"`
	Name           string `json:"name"`
	Coord          string `json:"coord"`
	FleetID        string `json:"fleet_id"`
	FleetName      string `json:"fleet_name"`
	HangarCapacity int    `json:"hangar_capacity,omitempty"`
}

type AirDeployment struct {
	ID      string `json:"id"`
	Unit    string `json:"unit"`
	Carrier string `json:"carrier"`
}

type ScenarioDefinition struct {
	ID             string          `json:"id"`
	Version        int             `json:"version"`
	Name           string          `json:"name"`
	Map            string          `json:"map"`
	Description    string          `json:"description"`
	Deployments    []Deployment    `json:"deployments"`
	AirDeployments []AirDeployment `json:"air_deployments"`
}

type UnitDefinition struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Strength int      `json:"strength"`
	Card     ShipCard `json:"card"`
}

type ShipCard struct {
	Gunnery        int     `json:"gunnery"`
	DamageControl  int     `json:"damage_control"`
	MainGunCount   int     `json:"main_gun_count"`
	MainGunCaliber float64 `json:"main_gun_caliber"`
	HeavyAA        float64 `json:"heavy_aa"`
	LightAA        float64 `json:"light_aa"`
	SurfaceRadar   bool    `json:"surface_radar"`
	AirRadar       bool    `json:"air_radar"`
	SpeedClass     string  `json:"speed_class"`
	TorpedoSalvos  int     `json:"torpedo_salvos"`
	Aircraft       int     `json:"aircraft"`
	HangarCapacity int     `json:"hangar_capacity"`
	DeckCapacity   int     `json:"deck_capacity"`
	ElevatorCount  int     `json:"elevator_count"`
	DepthCharges   int     `json:"depth_charges"`
	Cargo          int     `json:"cargo"`
	ArmoredCarrier bool    `json:"armored_carrier"`
	HistoricalLoss string  `json:"historical_loss,omitempty"`
}

type AircraftDefinition struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Model      string `json:"model"`
	Kind       string `json:"kind"`
	Strength   int    `json:"strength"`
	Range      int    `json:"range"`
	AirCombat  int    `json:"air_combat"`
	AntiShip   int    `json:"anti_ship"`
	AntiGround int    `json:"anti_ground"`
	Seaplane   bool   `json:"seaplane"`
}

type UnitCatalog struct {
	ID      string               `json:"id"`
	Version int                  `json:"version"`
	Units   []UnitDefinition     `json:"units"`
	Air     []AircraftDefinition `json:"aircraft"`
}

type Unit struct {
	ID           string   `json:"id"`
	DefinitionID string   `json:"definition_id"`
	FleetID      string   `json:"fleet_id"`
	FleetName    string   `json:"fleet_name"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Faction      string   `json:"faction"`
	Coord        string   `json:"coord"`
	Heading      string   `json:"heading"`
	Strength     int      `json:"strength"`
	MaxStrength  int      `json:"max_strength"`
	Moved        bool     `json:"moved"`
	Card         ShipCard `json:"card"`
	LegalMoves   []string `json:"legal_moves,omitempty"`
}

type DistanceHex struct {
	Coord    string `json:"coord"`
	Distance int    `json:"distance"`
}

type DistanceReference struct {
	Origin      string        `json:"origin"`
	MaxDistance int           `json:"max_distance"`
	Cells       []DistanceHex `json:"cells"`
}

type FleetGroup struct {
	ID                string             `json:"id"`
	FleetID           string             `json:"fleet_id"`
	FleetName         string             `json:"fleet_name"`
	Name              string             `json:"name"`
	Type              string             `json:"type"`
	Faction           string             `json:"faction"`
	Coord             string             `json:"coord"`
	Heading           string             `json:"heading"`
	Strength          int                `json:"strength"`
	MaxStrength       int                `json:"max_strength"`
	Moved             bool               `json:"moved"`
	LegalMoves        []string           `json:"legal_moves,omitempty"`
	Count             int                `json:"count"`
	Members           []Unit             `json:"members"`
	DistanceReference *DistanceReference `json:"distance_reference,omitempty"`
}

type AirUnit struct {
	ID               string `json:"id"`
	DefinitionID     string `json:"definition_id"`
	CarrierID        string `json:"carrier_id"`
	Faction          string `json:"faction"`
	Name             string `json:"name"`
	Model            string `json:"model"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	Strength         int    `json:"strength"`
	MaxRange         int    `json:"max_range"`
	RemainingRange   int    `json:"remaining_range"`
	AirCombat        int    `json:"air_combat"`
	AntiShip         int    `json:"anti_ship"`
	AntiGround       int    `json:"anti_ground"`
	Seaplane         bool   `json:"seaplane"`
	Launched         bool   `json:"launched"`
	Priority         int    `json:"priority"`
	TaskID           string `json:"task_id,omitempty"`
	OriginCoord      string `json:"origin_coord,omitempty"`
	Coord            string `json:"coord,omitempty"`
	Heading          string `json:"heading,omitempty"`
	SectorWidth      int    `json:"sector_width,omitempty"`
	ReconRadius      int    `json:"recon_radius,omitempty"`
	ReconDistance    int    `json:"recon_distance,omitempty"`
	ReconRadiusLimit int    `json:"recon_radius_limit,omitempty"`
	Returning        bool   `json:"returning,omitempty"`
	TrackingFleetID  string `json:"tracking_fleet_id,omitempty"`
}

type AirTask struct {
	ID                string             `json:"id"`
	Kind              string             `json:"kind"`
	Name              string             `json:"name"`
	CarrierID         string             `json:"carrier_id"`
	AircraftIDs       []string           `json:"aircraft_ids"`
	Aircraft          []AirUnit          `json:"aircraft"`
	Priority          int                `json:"priority"`
	OriginCoord       string             `json:"origin_coord,omitempty"`
	CurrentCoord      string             `json:"current_coord,omitempty"`
	Heading           string             `json:"heading,omitempty"`
	SectorWidth       int                `json:"sector_width,omitempty"`
	ReconRadius       int                `json:"recon_radius,omitempty"`
	ReconDistance     int                `json:"recon_distance,omitempty"`
	Returning         bool               `json:"returning,omitempty"`
	TrackingFleetID   string             `json:"tracking_fleet_id,omitempty"`
	SectorHexes       []string           `json:"sector_hexes,omitempty"`
	CurrentRingHexes  []string           `json:"current_ring_hexes,omitempty"`
	DistanceReference *DistanceReference `json:"distance_reference,omitempty"`
}

type AviationView struct {
	Carriers []CarrierAirStatus `json:"carriers"`
	Tasks    []AirTask          `json:"tasks"`
}

type CarrierAirStatus struct {
	UnitID         string    `json:"unit_id"`
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	HangarUsed     int       `json:"hangar_used"`
	HangarCapacity int       `json:"hangar_capacity"`
	DeckCapacity   int       `json:"deck_capacity"`
	DeckUsed       int       `json:"deck_used"`
	LaunchUsed     int       `json:"launch_used"`
	Operation      string    `json:"operation"`
	Aircraft       []AirUnit `json:"aircraft"`
}

type FleetContact struct {
	FleetID string `json:"fleet_id"`
	Faction string `json:"faction"`
	Level   int    `json:"level"`
	Coord   string `json:"coord"`
	Count   int    `json:"count"`
	Members []Unit `json:"members,omitempty"`
}

type State struct {
	ScenarioID        string                              `json:"scenario_id"`
	ScenarioName      string                              `json:"scenario_name"`
	Description       string                              `json:"description"`
	Map               MapDefinition                       `json:"map"`
	Units             map[string]*Unit                    `json:"units"`
	Aircraft          map[string]*AirUnit                 `json:"aircraft,omitempty"`
	CarrierOperations map[string]string                   `json:"carrier_operations,omitempty"`
	Contacts          map[string]map[string]*FleetContact `json:"contacts,omitempty"`
	RNGState          uint64                              `json:"rng_state"`
	Definitions       map[string]string                   `json:"definition_versions"`
}

type Order struct {
	Type         string            `json:"type"`
	UnitID       string            `json:"unit_id,omitempty"`
	TargetUnitID string            `json:"target_unit_id,omitempty"`
	AircraftID   string            `json:"aircraft_id,omitempty"`
	AircraftIDs  []string          `json:"aircraft_ids,omitempty"`
	Coord        string            `json:"coord,omitempty"`
	Heading      string            `json:"heading,omitempty"`
	SectorWidth  int               `json:"sector_width,omitempty"`
	ReconRadius  int               `json:"recon_radius,omitempty"`
	Assignments  map[string]string `json:"assignments,omitempty"`
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
	x := zeroCol - (zeroRow+(zeroRow&1))/2
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

func BuildDistanceReference(gameMap MapDefinition, origin string, maxDistance int) *DistanceReference {
	if _, _, err := ParseCoord(origin); err != nil || maxDistance < 1 {
		return nil
	}
	reference := &DistanceReference{Origin: origin, MaxDistance: maxDistance, Cells: []DistanceHex{}}
	for row := 1; row <= gameMap.Rows; row++ {
		for col := 1; col <= gameMap.Cols; col++ {
			coord := Coord(row, col)
			distance := Distance(origin, coord)
			if distance <= maxDistance {
				reference.Cells = append(reference.Cells, DistanceHex{Coord: coord, Distance: distance})
			}
		}
	}
	return reference
}
