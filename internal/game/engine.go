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
	if scenarioID == "" {
		scenarioID = "_coral_real"
	}
	// 剧本目录是开放的：目录名就是剧本 ID，地图和单位库由剧本自己指定，
	// 这样加一个测试剧本不需要改任何 Go 代码。
	if err := readJSON(filepath.Join(contentDir, "scenarios", scenarioID, "scenario.json"), &scenario); err != nil {
		return scenario, gameMap, catalog, fmt.Errorf("剧本%s不存在：%w", scenarioID, err)
	}
	if scenario.Map == "" {
		scenario.Map = scenarioID
	}
	if err := readJSON(filepath.Join(contentDir, "maps", scenario.Map, "map.json"), &gameMap); err != nil {
		return scenario, gameMap, catalog, err
	}
	unitsID := scenario.Units
	if unitsID == "" {
		unitsID = "sandbox"
	}
	if err := readJSON(filepath.Join(contentDir, "units", unitsID+".json"), &catalog); err != nil {
		return scenario, gameMap, catalog, err
	}
	needsShips := false
	for _, deployment := range scenario.Deployments {
		if deployment.Ship != "" {
			needsShips = true
			break
		}
	}
	if needsShips {
		if ships, err := loadShipCards(filepath.Join(contentDir, "units", "ships")); err == nil {
			catalog.Units = append(catalog.Units, ships...)
		}
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

// loadShipCards 读取 units/ships/*.json：每艘舰一份完整卡，id 是舰名标识（lexington、shokaku…）。
func loadShipCards(dir string) ([]UnitDefinition, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ships := []UnitDefinition{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var ship UnitDefinition
		if err := readJSON(filepath.Join(dir, entry.Name()), &ship); err != nil {
			return nil, err
		}
		ships = append(ships, ship)
	}
	return ships, nil
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
		Year: scenario.Year, Night: scenario.Night, Wind: scenario.Wind, Doctrines: scenario.Doctrines,
		Map: gameMap, Units: map[string]*Unit{}, Aircraft: map[string]*AirUnit{}, Contacts: map[string]map[string]*FleetContact{}, RNGState: seed,
		Definitions: map[string]string{"scenario": fmt.Sprint(scenario.Version), "map": fmt.Sprint(gameMap.Version), "units": fmt.Sprint(catalog.Version)},
		Objectives:  scenario.Objectives, MaxTurns: scenario.MaxTurns, Turn: 1, Timeline: scenario.Timeline,
		// 昼夜循环靠这两个字段推进：此前只写了 Timeline 却没把开局时刻/每天回合数带进状态，
		// 结果是剧本配了时间线，引擎里 TurnsPerDay 仍然是 0，永远停在剧本固定的 night 值。
		DayHours: scenario.DayHours, TurnsPerDay: scenario.TurnsPerDay,
	}
	// timeline.turn_minutes 是单回合时长的唯一真相：20 分钟一回合就是每天 72 回合。
	if scenario.Timeline != nil && scenario.Timeline.TurnMinutes > 0 {
		state.TurnsPerDay = 1440 / scenario.Timeline.TurnMinutes
	}
	// 开局天色必须按时钟算，不能沿用剧本里那个静态 night 字段。
	// 以前状态里 Night 初始是 false，而珊瑚海开局是 00:00——第一回合结算时
	// 引擎以为"刚入夜"，于是播了一条假的"夜幕降临"，顺手把玩家这一刻下的
	// CAP/侦察命令当成夜间待发任务收进机库（任务直接没了，05:20 也不会起飞）。
	if state.TurnsPerDay > 0 {
		state.Night = state.nightAt(clockAt(state))
	}
	definitions, err := indexUnitDefinitions(catalog)
	if err != nil {
		return state, err
	}
	airfieldCoords := make(map[string]bool, len(scenario.Airfields))
	for _, airfield := range scenario.Airfields {
		airfieldCoords[airfield.Coord] = true
	}
	// 同一舰队只要有一艘航母，整队的出生点就必须避开浅滩和机场格；
	// 否则第一艘是巡洋舰时选到浅滩，后面加入的航母会直接实例化失败。
	fleetCarrier := map[string]bool{}
	for _, deployment := range scenario.Deployments {
		fleetID := deployment.FleetID
		if fleetID == "" {
			fleetID = deployment.ID
		}
		lookupID := deployment.Unit
		if deployment.Ship != "" {
			lookupID = deployment.Ship
		}
		definition, ok := definitions.lookup(deployment.Faction, lookupID)
		if !ok {
			continue
		}
		if isCarrierType(definition.Type) {
			fleetCarrier[fleetID] = true
		}
	}
	fleetHomes := map[string]Deployment{}
	for _, deployment := range scenario.Deployments {
		if deployment.Faction != "A" && deployment.Faction != "B" {
			return state, fmt.Errorf("部署%s阵营无效", deployment.ID)
		}
		if _, exists := state.Units[deployment.ID]; exists {
			return state, fmt.Errorf("部署ID重复：%s", deployment.ID)
		}
		// 优先具名舰卡（ship: "lexington"），没有就回退到舰级模板（unit: "cv"）。
		lookupID := deployment.Unit
		if deployment.Ship != "" {
			lookupID = deployment.Ship
		}
		definition, ok := definitions.lookup(deployment.Faction, lookupID)
		if !ok {
			return state, fmt.Errorf("单位定义不存在：%s", lookupID)
		}
		switch definition.Type {
		case "BB", "BC", "CA", "CL", "DD", "CV", "CVL", "TRS", "CVE":
		default:
			return state, fmt.Errorf("单位%s舰船类型无效：%s", definition.ID, definition.Type)
		}
		if definition.Strength <= 0 {
			return state, fmt.Errorf("单位%s生命值必须大于0", definition.ID)
		}
		fleetID := deployment.FleetID
		if fleetID == "" {
			fleetID = deployment.ID
		}
		fleetName := deployment.FleetName
		if fleetName == "" {
			fleetName = fleetID
		}
		// 出生点：写了 coord 就用固定点位；写了 coords 就在候选区里按种子随机，
		// 同一舰队的所有舰船必须落在同一个 hex。
		carrierFleet := fleetCarrier[fleetID]
		coord := deployment.Coord
		if coord == "" {
			if home, exists := fleetHomes[fleetID]; exists && home.Coord != "" {
				coord = home.Coord
			} else if deployment.Near != nil {
				anchor, ok := fleetHomes[deployment.Near.FleetID]
				if !ok || anchor.Coord == "" {
					return state, fmt.Errorf("部署%s的邻近舰队%s尚未出生", deployment.ID, deployment.Near.FleetID)
				}
				coord = nearFleetCoord(gameMap, deployment.Near, anchor.Coord, carrierFleet, airfieldCoords, &state.RNGState)
			} else {
				candidates := deployment.Coords
				if len(candidates) == 0 && deployment.Box != nil {
					candidates = boxCoords(gameMap, *deployment.Box, carrierFleet, airfieldCoords)
				}
				if len(candidates) == 0 {
					return state, fmt.Errorf("部署%s没有指定坐标", deployment.ID)
				}
				if len(candidates) == 1 {
					coord = candidates[0]
				} else {
					coord = candidates[nextUint64(&state.RNGState)%uint64(len(candidates))]
				}
			}
		}
		if !fleetTerrainAllowsEntry(gameMap, coord, carrierFleet, airfieldCoords[coord]) {
			return state, fmt.Errorf("部署%s坐标不可通行：%s", deployment.ID, coord)
		}
		if home, exists := fleetHomes[fleetID]; exists && (home.Faction != deployment.Faction || home.Coord != coord) {
			return state, fmt.Errorf("舰队%s的初始成员必须属于同一阵营和hex", fleetID)
		}
		home := deployment
		home.Coord = coord
		fleetHomes[fleetID] = home
		card := definition.Card
		// 剪影路径随卡下发：前端不再靠舰名硬编码映射。
		if card.Picture == "" {
			card.Picture = definition.Picture
		}
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
		// 炮击值以单位库为准：JSON 里写了 gunnery 就用写的，没写才按口径×炮塔数折算，
		// 这样单舰（或整类）参数都能在 content/units/*.json 里直接调。
		if card.Gunnery == 0 {
			card.Gunnery = int(math.Round(card.MainGunCaliber * float64(card.MainGunCount) / 9))
		}
		if card.SpeedClass == "" {
			card.SpeedClass = "normal"
		}
		strength := definition.Strength
		if o := deployment.Overrides; o != nil {
			if o.Strength != nil {
				if *o.Strength < 1 {
					return state, fmt.Errorf("部署%s强度必须至少为1", deployment.ID)
				}
				strength = *o.Strength
			}
			if o.DeckLength != nil {
				if *o.DeckLength < 0 {
					return state, fmt.Errorf("部署%s甲板长度不能为负数", deployment.ID)
				}
				// 甲板长度是"基础值"，实际长度仍按风向与朝向修正（规则书 846–849 行）。
				card.DeckCapacity = *o.DeckLength
			}
			if o.HangarCapacity != nil {
				card.HangarCapacity = *o.HangarCapacity
			}
			if o.ElevatorCount != nil {
				card.ElevatorCount = *o.ElevatorCount
			}
			if o.HeavyAA != nil {
				card.HeavyAA = *o.HeavyAA
			}
			if o.LightAA != nil {
				card.LightAA = *o.LightAA
			}
			if o.MainGunCount != nil {
				card.MainGunCount = *o.MainGunCount
			}
			if o.MainGunCaliber != nil {
				card.MainGunCaliber = float64(*o.MainGunCaliber)
			}
			if o.DamageControl != nil {
				card.DamageControl = *o.DamageControl
			}
			if o.SpeedClass != nil {
				card.SpeedClass = *o.SpeedClass
			}
			if o.Gunnery != nil {
				card.Gunnery = *o.Gunnery
			}
			if o.ArmoredCarrier != nil {
				card.ArmoredCarrier = *o.ArmoredCarrier
			}
			if o.OpenHangar != nil {
				card.OpenHangar = *o.OpenHangar
			}
		}
		state.Units[deployment.ID] = &Unit{
			ID: deployment.ID, DefinitionID: definition.ID, FleetID: fleetID, FleetName: fleetName, Name: deployment.Name, Type: definition.Type, Faction: deployment.Faction,
			Coord: coord, Heading: "NE", Strength: strength, MaxStrength: strength, Card: card,
		}
		if state.InitialShipCounts == nil {
			state.InitialShipCounts = map[string]int{}
		}
		state.InitialShipCounts[deployment.Faction]++
	}
	airDefinitions, err := indexAircraftDefinitions(catalog)
	if err != nil {
		return state, err
	}
	for _, deployment := range scenario.AirDeployments {
		if _, exists := state.Aircraft[deployment.ID]; exists {
			return state, fmt.Errorf("航空部署ID重复：%s", deployment.ID)
		}
		carrier := state.Units[deployment.Carrier]
		if carrier == nil || carrier.Card.HangarCapacity <= 0 {
			return state, fmt.Errorf("航空部署%s的载机舰无效", deployment.ID)
		}
		definition, ok := airDefinitions.lookup(carrier.Faction, deployment.Unit)
		if !ok {
			return state, fmt.Errorf("航空单位定义不存在：%s", deployment.Unit)
		}
		if carrier.Card.Aircraft >= carrier.Card.HangarCapacity {
			return state, fmt.Errorf("载机舰%s的机库容量不足", carrier.ID)
		}
		// 挂副油箱是默认配置：航程 = 机内 + 副油箱（+30%），两部分分开记账，
		// 这样空战后抛弃副油箱就能准确扣掉它还没烧掉的那部分里程（规则书 48 行）。
		// 机库待命时按机种给一个合法的默认挂载（不是 air+tank），
		// 否则侦察机/轰炸机一开始就处于"挂不了的组合"；真正出击时再按任务换成对应挂载。
		initialLoadout := DefaultLoadoutFor(definition.Kind, definition.Seaplane, definition.AntiGround, definition.AntiShip, "cap", StrikeTargetShip, definition.TorpedoCapable)
		fuel := MountedFuel(definition.Range, initialLoadout)
		airCombat, antiShip := elitePilotBonus(state, carrier.Faction, definition.Kind, definition.AirCombat, definition.AntiShip)
		state.Aircraft[deployment.ID] = &AirUnit{
			ID: deployment.ID, DefinitionID: definition.ID, CarrierID: carrier.ID, Faction: carrier.Faction,
			Name: definition.Name, Model: definition.Model, Kind: definition.Kind, Picture: definition.Picture, PictureMode: definition.PictureMode, Status: "hangar", Strength: definition.Strength,
			BaseRange: definition.Range, Loadout: initialLoadout,
			MaxRange: fuel.Available(), Fuel: fuel, RemainingRange: fuel.Available(), AirCombat: airCombat, AntiShip: antiShip, AntiGround: definition.AntiGround, Seaplane: definition.Seaplane, AirfieldOnly: definition.AirfieldOnly, TorpedoCapable: definition.TorpedoCapable, HighAltitudeBomber: definition.HighAltitudeBomber,
		}
		carrier.Card.Aircraft++
	}
	if err := deployAirfields(&state, scenario, airDefinitions, gameMap); err != nil {
		return state, err
	}
	EnsureAviation(&state)
	// 开局就要按当前风向与朝向算一次甲板长度：否则第一回合快照里 heading 已经是
	// 初始朝向，deck_length 却还是没修正的 base，玩家按它规划会差 2 架。
	RefreshDeckLengths(&state)
	return state, nil
}

// elitePilotBonus 是"开战初期精锐飞行员"的机型修正：
// 1942 年及以前，日军 D3A/B5N 对舰攻击 +1，零战空战 +1。
// 这是飞行员的素质加成，不该写进机型档案（那样 1944 年的同一机型也会带着这 +1）。
func elitePilotBonus(state State, faction, kind string, airCombat, antiShip int) (int, int) {
	// 精锐机组是"国家 + 年份"的规则，不是机型属性：1944 年的同一机型不该还带着这 +1。
	if rulesFor(state, faction).ElitePilotBonus == 0 {
		return airCombat, antiShip
	}
	switch kind {
	case "fighter":
		airCombat++
	case "dive_bomber", "torpedo_bomber":
		antiShip++
	}
	return airCombat, antiShip
}

// deployAirfields 把剧本里的机场和地面飞机落到状态里，并做与舰船部署一致的内容校验。
func deployAirfields(state *State, scenario ScenarioDefinition, airDefinitions aircraftDefinitionIndex, gameMap MapDefinition) error {
	if len(scenario.Airfields) == 0 {
		return nil
	}
	if state.Airfields == nil {
		state.Airfields = map[string]*Airfield{}
	}
	if state.GroundAir == nil {
		state.GroundAir = map[string]*AirfieldAirUnit{}
	}
	for _, definition := range scenario.Airfields {
		if definition.Faction != "A" && definition.Faction != "B" {
			return fmt.Errorf("机场%s阵营无效", definition.ID)
		}
		if _, exists := state.Airfields[definition.ID]; exists {
			return fmt.Errorf("机场ID重复：%s", definition.ID)
		}
		if !inBounds(gameMap, definition.Coord) {
			return fmt.Errorf("机场%s坐标不在地图内", definition.ID)
		}
		// 常规机场必须在陆地（岛/浅滩）上；水上飞机基地本来就锚泊在海上，允许建在水面。
		if definition.Type != AirfieldSeaplane {
			switch terrainAt(gameMap, definition.Coord) {
			case TerrainIsland, TerrainShoal:
			default:
				return fmt.Errorf("机场%s必须部署在陆地hex上", definition.ID)
			}
		} else if terrainAt(gameMap, definition.Coord) == TerrainReef {
			return fmt.Errorf("水上飞机基地%s不能设在暗礁上", definition.ID)
		}
		if definition.Runways < 1 || definition.Runways > 9 {
			return fmt.Errorf("机场%s跑道数量必须在1到9之间", definition.ID)
		}
		switch NormalizeAirfieldType(definition.Type) {
		case AirfieldSmall, AirfieldMedium, AirfieldLarge, AirfieldSeaplane:
		default:
			return fmt.Errorf("机场%s类型无效：%s（应为 small / medium / large / seaplane）", definition.ID, definition.Type)
		}
		definition.Type = NormalizeAirfieldType(definition.Type)
		airfield := &Airfield{
			ID: definition.ID, Faction: definition.Faction, Name: definition.Name, Coord: definition.Coord,
			Type: definition.Type, Picture: AirfieldPicture(definition.Type, definition.Picture),
			Runways: definition.Runways, MaxStrength: definition.Runways * 5,
			HangarCapacity: definition.HangarCapacity,
			Radar:          definition.Radar, HeavyAA: definition.HeavyAA, LightAA: definition.LightAA, Coastal: definition.Coastal,
		}
		if airfield.HangarCapacity <= 0 {
			// 机场没有机库：默认不限制（99999），剧本要限制某个小机场时自己写数。
			airfield.HangarCapacity = AirfieldHangarUnlimited
		}
		airfield.Strength = airfield.MaxStrength
		state.Airfields[definition.ID] = airfield
		for _, aircraftID := range definition.Aircraft {
			if _, exists := state.GroundAir[aircraftID]; exists {
				return fmt.Errorf("地面航空部署ID重复：%s", aircraftID)
			}
			// 剧本里可写机库编号（如 af-a-fighter-1），也可直接写机种定义（如 fighter）。
			// 编号匹配方式：逐个 "-" 切段，从最后一��往前找一个机种定义名。
			aircraftDef, ok := airDefinitions.resolve(airfield.Faction, aircraftID)
			if !ok {
				return fmt.Errorf("地面航空单位定义不存在：%s", aircraftID)
			}
			if !airfield.HardensFor(aircraftDef.Kind, aircraftDef.Seaplane) {
				return fmt.Errorf("机场%s（%s）无法起降%s", definition.ID, definition.Type, aircraftDef.Model)
			}
			initialGroundLoadout := DefaultLoadoutFor(aircraftDef.Kind, false, aircraftDef.AntiGround, aircraftDef.AntiShip, "cap", StrikeTargetShip, aircraftDef.TorpedoCapable)
			fuel := MountedFuel(aircraftDef.Range, initialGroundLoadout)
			groundAirCombat, groundAntiShip := elitePilotBonus(*state, airfield.Faction, aircraftDef.Kind, aircraftDef.AirCombat, aircraftDef.AntiShip)
			state.GroundAir[aircraftID] = &AirfieldAirUnit{
				ID: aircraftID, DefinitionID: aircraftDef.ID, AirfieldID: airfield.ID, Faction: airfield.Faction,
				Model: aircraftDef.Model, Kind: aircraftDef.Kind, Picture: aircraftDef.Picture, PictureMode: aircraftDef.PictureMode, Status: "hangar", Strength: aircraftDef.Strength,
				BaseRange: aircraftDef.Range, Loadout: initialGroundLoadout,
				MaxRange: fuel.Available(), RemainingRange: fuel.Available(),
				AirCombat: groundAirCombat, AntiShip: groundAntiShip, AntiGround: aircraftDef.AntiGround,
				AirfieldOnly: aircraftDef.AirfieldOnly, Seaplane: aircraftDef.Seaplane, TorpedoCapable: aircraftDef.TorpedoCapable, HighAltitudeBomber: aircraftDef.HighAltitudeBomber,
				// 侦察半径按"不含备用油"的航程折半：备用油只够回家，不能用来搜索（规则书 69 行）。
				ReconRadiusLimit: max(1, fuel.Available()/2),
			}
		}
	}
	return nil
}

// unitDefinitionIndex 把舰船目录按阵营拆开。同一个 ID（如 cv）在美日两侧是
// 不同的角色卡，同 ID 会出现在多个阵营下，所以查定义必须带上阵营。
type unitDefinitionIndex struct {
	byFaction map[string]map[string]UnitDefinition
	generic   map[string]UnitDefinition
}

func indexUnitDefinitions(catalog UnitCatalog) (unitDefinitionIndex, error) {
	index := unitDefinitionIndex{byFaction: map[string]map[string]UnitDefinition{}, generic: map[string]UnitDefinition{}}
	for _, definition := range catalog.Units {
		if definition.Faction == "" {
			if _, exists := index.generic[definition.ID]; exists {
				return index, fmt.Errorf("单位定义重复：%s", definition.ID)
			}
			index.generic[definition.ID] = definition
			continue
		}
		byID := index.byFaction[definition.Faction]
		if byID == nil {
			byID = map[string]UnitDefinition{}
			index.byFaction[definition.Faction] = byID
		}
		if _, exists := byID[definition.ID]; exists {
			return index, fmt.Errorf("单位定义重复：%s/%s", definition.Faction, definition.ID)
		}
		byID[definition.ID] = definition
	}
	return index, nil
}

// lookup 先取阵营专用定义，再退回通用定义。
func (index unitDefinitionIndex) lookup(faction, id string) (UnitDefinition, bool) {
	if byID, ok := index.byFaction[faction]; ok {
		if definition, ok := byID[id]; ok {
			return definition, true
		}
	}
	definition, ok := index.generic[id]
	return definition, ok
}

// aircraftDefinitionIndex 是机型的阵营索引，查法与舰船一致。
type aircraftDefinitionIndex struct {
	byFaction map[string]map[string]AircraftDefinition
	generic   map[string]AircraftDefinition
}

func indexAircraftDefinitions(catalog UnitCatalog) (aircraftDefinitionIndex, error) {
	index := aircraftDefinitionIndex{byFaction: map[string]map[string]AircraftDefinition{}, generic: map[string]AircraftDefinition{}}
	for _, definition := range catalog.Air {
		if definition.Faction == "" {
			if _, exists := index.generic[definition.ID]; exists {
				return index, fmt.Errorf("航空单位定义重复：%s", definition.ID)
			}
			index.generic[definition.ID] = definition
			continue
		}
		byID := index.byFaction[definition.Faction]
		if byID == nil {
			byID = map[string]AircraftDefinition{}
			index.byFaction[definition.Faction] = byID
		}
		if _, exists := byID[definition.ID]; exists {
			return index, fmt.Errorf("航空单位定义重复：%s/%s", definition.Faction, definition.ID)
		}
		byID[definition.ID] = definition
	}
	return index, nil
}

func (index aircraftDefinitionIndex) lookup(faction, id string) (AircraftDefinition, bool) {
	if byID, ok := index.byFaction[faction]; ok {
		if definition, ok := byID[id]; ok {
			return definition, true
		}
	}
	definition, ok := index.generic[id]
	return definition, ok
}

// resolve 支持两种剧本写法："fighter" 直接命中机种定义；
// "af-a-fighter-1" 这类机库编号则把 "-" 之间任意一段当作机种去匹配。
func (index aircraftDefinitionIndex) resolve(faction, id string) (AircraftDefinition, bool) {
	if definition, ok := index.lookup(faction, id); ok {
		return definition, true
	}
	parts := strings.Split(id, "-")
	for position := len(parts) - 1; position >= 0; position-- {
		if definition, ok := index.lookup(faction, parts[position]); ok {
			return definition, true
		}
	}
	return AircraftDefinition{}, false
}

func isCarrierType(unitType string) bool {
	return unitType == "CV" || unitType == "CVL" || unitType == "CVE"
}

func IsCarrier(unit *Unit) bool {
	return unit != nil && isCarrierType(unit.Type)
}

// CarrierEquivalentType 把 CVL/CVE 归并为 CV（规则书 119 行）：除甲板容量等自身属性外，
// 目标选择、炮战和空袭结算都按 CV 处理。此前只有炮战的目标权重做了归一，
// 空袭目标权重和轻防空加成仍然直接比较 "CV"，导致轻航母被当成 DD/CL 级目标。
func CarrierEquivalentType(unitType string) string {
	if unitType == "CVL" || unitType == "CVE" {
		return "CV"
	}
	return unitType
}

func inBounds(gameMap MapDefinition, coord string) bool {
	hex, ok := HexOf(coord)
	return ok && hex.Row() <= gameMap.Rows && hex.Col() <= gameMap.Cols
}

// terrainAt 查一格的地形。坐标非法就是水面——和以前"扫不到这个坐标就当 water"一致。
func terrainAt(gameMap MapDefinition, coord string) Terrain {
	hex, ok := HexOf(coord)
	if !ok {
		return TerrainWater
	}
	return gameMap.terrain(hex)
}

// MovementBlocked 判断舰队本回合是否还能移动。
// 冷却每回合结算开始只减 1，减到 0 之前该舰队任何成员都不能移动；
// 因此 LegalMoves 与 ValidateOrder 必须共用这一判断，不能在两处各写一份。
func MovementBlocked(state State, unit *Unit) bool {
	if unit == nil {
		return true
	}
	for _, member := range state.Units {
		if FleetKey(member) == FleetKey(unit) && (member.Moved || member.MoveCooldown > 0) {
			return true
		}
	}
	return false
}

// MovesBlocked 只判断"这艘船自己还能不能动"（含冷却），不含地形限制。
func MovesBlocked(state State, unit *Unit) bool {
	return unit == nil || unit.MoveCooldown > 0
}

func LegalMoves(state State, unit *Unit) []string {
	if unit == nil {
		return nil
	}
	if MovementBlocked(state, unit) {
		return []string{}
	}
	carrierFleet := IsCarrier(unit) || FleetHasCarrier(state, FleetKey(unit))
	moves := []string{}
	for _, coord := range Neighbors(unit.Coord) {
		if !fleetCanEnter(state, coord, carrierFleet) {
			continue
		}
		moves = append(moves, coord)
	}
	return moves
}

// fleetCanEnter 统一手动移动、自动航行及岸上行动的通行规则。
func fleetCanEnter(state State, coord string, carrierFleet bool) bool {
	for _, airfield := range state.Airfields {
		if airfield.Coord == coord {
			return fleetTerrainAllowsEntry(state.Map, coord, carrierFleet, true)
		}
	}
	return fleetTerrainAllowsEntry(state.Map, coord, carrierFleet, false)
}

func fleetTerrainAllowsEntry(gameMap MapDefinition, coord string, carrierFleet, airfieldHex bool) bool {
	if !inBounds(gameMap, coord) {
		return false
	}
	terrain := terrainAt(gameMap, coord)
	if terrain == TerrainReef {
		return false
	}
	if airfieldHex {
		if carrierFleet {
			return false
		}
		for _, neighbor := range Neighbors(coord) {
			if inBounds(gameMap, neighbor) && terrainAt(gameMap, neighbor) == TerrainWater {
				return true
			}
		}
		return false
	}
	if terrain == TerrainIsland {
		return false
	}
	return !carrierFleet || terrain != TerrainShoal
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

// nearFleetCoord 在另一支舰队周围按方向取一个可用海格。
// 默认距离 4–10 hex，避免两支舰队叠在同一格，也保证前哨不会离航母太远。
func nearFleetCoord(gameMap MapDefinition, near *DeploymentNear, anchor string, carrier bool, airfieldCoords map[string]bool, rng *uint64) string {
	direction := strings.ToUpper(strings.TrimSpace(near.Direction))
	minDistance := near.MinDistance
	if minDistance < 1 {
		minDistance = 1
	}
	maxDistance := near.MaxDistance
	if maxDistance < minDistance {
		maxDistance = 10
	}
	candidates := []string{}
	for row := 1; row <= gameMap.Rows; row++ {
		for col := 1; col <= gameMap.Cols; col++ {
			coord := Coord(row, col)
			terrain := terrainAt(gameMap, coord)
			if !fleetTerrainAllowsEntry(gameMap, coord, carrier, airfieldCoords[coord]) || terrain == TerrainReef {
				continue
			}
			distance := Distance(anchor, coord)
			if distance < minDistance || distance > maxDistance {
				continue
			}
			if headingBetween(anchor, coord) != direction {
				continue
			}
			candidates = append(candidates, coord)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Strings(candidates)
	return candidates[nextUint64(rng)%uint64(len(candidates))]
}

// boxCoords 把矩形出生区展开成可用候选 hex，使用与移动相同的地形规则。
func boxCoords(gameMap MapDefinition, box DeployBox, carrier bool, airfieldCoords map[string]bool) []string {
	rowMin, rowMax := min(box.RowMin, box.RowMax), max(box.RowMin, box.RowMax)
	colMin, colMax := min(box.ColMin, box.ColMax), max(box.ColMin, box.ColMax)
	coords := []string{}
	for row := rowMin; row <= rowMax; row++ {
		for col := colMin; col <= colMax; col++ {
			coord := Coord(row, col)
			if fleetTerrainAllowsEntry(gameMap, coord, carrier, airfieldCoords[coord]) {
				coords = append(coords, coord)
			}
		}
	}
	return coords
}

// groundAirView 把舰载侦察算子投影成机场算子形态，用于复用机场发现逻辑。
func groundAirView(aircraft *AirUnit) *AirfieldAirUnit {
	return &AirfieldAirUnit{ID: aircraft.ID, Faction: aircraft.Faction, Model: aircraft.Model, Kind: aircraft.Kind, Heading: aircraft.Heading, SectorWidth: aircraft.SectorWidth, OriginCoord: aircraft.OriginCoord, TorpedoCapable: aircraft.TorpedoCapable, HighAltitudeBomber: aircraft.HighAltitudeBomber}
}

func ValidateOrder(state State, faction string, order Order) error {
	// 无线电命令按任务 ID 挂载、也带 TaskID，必须在航空命令分派之前拦下来，
	// 否则会掉进 ValidateAirOrder 的 default 分支。
	if isRadioOrder(order.Type) {
		return validateRadioOrder(state, faction, order)
	}
	if needsAirValidation(order.Type) {
		if isAirfieldTask(order.TaskID) || airOrderIsGround(state, order) {
			return ValidateGroundAirOrder(state, order)
		}
		return ValidateAirOrder(state, faction, order)
	}
	if isAirOrderType(order.Type) {
		return ValidateAirOrder(state, faction, order)
	}
	if order.Type == "REORGANIZE_HEX" {
		return validateReorganization(state, faction, order)
	}
	if order.Type == "SET_HEADING" {
		return validateHeadingOrder(state, faction, order)
	}
	if order.Type == "SET_LOADOUT" {
		return validateLoadoutOrder(state, faction, order)
	}
	if order.Type == "SET_AUTO_NAV" {
		return validateNavOrder(state, faction, order)
	}
	if order.Type == "SET_ENGAGEMENT" {
		unit := state.Units[order.UnitID]
		if unit == nil || unit.Faction != faction {
			return fmt.Errorf("舰队不存在或不属于当前玩家")
		}
		if FleetHasCarrier(state, FleetKey(unit)) {
			return fmt.Errorf("含航母的舰队固定为消极避战，不能改为主动交战")
		}
		switch EngagementSetting(order.Engagement) {
		case "", "aggressive", "advantage", "avoid":
			return nil
		}
		return fmt.Errorf("未知的交战设置")
	}
	unit, ok := state.Units[order.UnitID]
	if !ok || unit.Faction != faction {
		return fmt.Errorf("单位不存在或不属于当前玩家")
	}
	switch order.Type {
	case "CLEAR_MOVE":
		return nil
	case "SET_MOVE":
		if MovementBlocked(state, unit) {
			return fmt.Errorf("该舰队本回合已经移动或仍在冷却")
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
	// 立即下达的移动是"本回合规划"：冷却留到回合结算时再往下减，
	// 否则规划移动的那一回合就会把 3 回合冷却吃掉 1 回合。
	leader := state.Units[order.UnitID]
	if leader == nil {
		return nil
	}
	if MovesBlocked(*state, leader) || !fleetCanEnter(*state, order.Coord, FleetHasCarrier(*state, FleetKey(leader))) {
		return nil
	}
	// 没在冷却里走上面的分支。
	interval := FleetMoveInterval(*state, leader)
	qwe := FleetMoveCooldown(*state, FleetKey(leader))
	_ = qwe
	events := []EngineEvent{}
	for _, unit := range state.Units {
		if FleetKey(unit) != FleetKey(leader) {
			continue
		}
		from := unit.Coord
		unit.Coord = order.Coord
		unit.Heading = headingBetween(from, order.Coord)
		unit.Moved = true
		if cooldown := movementCooldownAfterMove(interval); cooldown > unit.MoveCooldown {
			unit.MoveCooldown = cooldown
		}
		events = append(events, EngineEvent{Type: "unit_moved", Audience: unit.Faction, Text: fmt.Sprintf("%s从%s移动至%s，每%d回合移动一次", unit.Name, from, order.Coord, interval)})
	}
	// 移动后岸轰停留计时重新开始
	clearFleetStay(state, FleetKey(leader))
	syncEmbarkedAircraft(state)
	events = append(events, detectContacts(state)...)
	events = append(events, advanceTowardEdge(state)...)
	trackFleetStay(state)
	return events
}

// applyReorganization 是整编的唯一实现：提交命令时和回合结算时都走它。
// 以前两份代码各写一遍，只有提交那一版会给新编队起名，结算那一版只改 FleetID，
// 于是拆分出来的舰队名字时有时无。
func applyReorganization(state *State, order Order) (string, bool) {
	faction := ""
	for unitID := range order.Assignments {
		if unit := state.Units[unitID]; unit != nil {
			faction = unit.Faction
		}
		break
	}
	if validateReorganization(*state, faction, order) != nil {
		return faction, false
	}
	fleetNames := map[string]string{}
	for _, unit := range state.Units {
		if unit.FleetName != "" {
			fleetNames[FleetKey(unit)] = unit.FleetName
		}
	}
	// 拆分出来的新编队没有历史名。以前直接拿 fleetID 当代号，地图和舰队面板上
	// 就显示成 split-mwjwyzw / split-mwwjw452 这种内部 id，玩家根本认不出谁是谁。
	// 这里改按"原舰队名·分队"派生，撞名时再挂上旗舰名；遍历顺序固定，
	// 免得同一次整编因为 map 顺序不同，两队分到不一样的名字。
	taken := map[string]bool{}
	for _, name := range fleetNames {
		taken[name] = true
	}
	unitIDs := make([]string, 0, len(order.Assignments))
	for unitID := range order.Assignments {
		unitIDs = append(unitIDs, unitID)
	}
	sort.Strings(unitIDs)
	for _, unitID := range unitIDs {
		fleetID := order.Assignments[unitID]
		unit := state.Units[unitID]
		if unit == nil {
			continue
		}
		name := fleetNames[fleetID]
		if name == "" {
			parent := fleetNames[FleetKey(unit)]
			if parent == "" {
				parent = unit.FleetName
			}
			name = fleetID
			if parent != "" && parent != fleetID {
				name = parent + "·分队"
				if taken[name] {
					name = parent + "·" + unit.Name
				}
			}
			fleetNames[fleetID] = name
			taken[name] = true
		}
		unit.FleetID = fleetID
		unit.FleetName = name
		LockFleetMovement(state, fleetID, 3)
	}
	return faction, true
}

// NormalizeFleetNames 修历史存档：早期拆分出来的编队直接把 split-<uuid> 当舰队名，
// 地图和舰队面板上就是一串内部 id。这里只改"名字等于自己舰队 id 的 split-* 编队"，
// 幂等，可以每次读档都跑。
func NormalizeFleetNames(state *State) {
	named := map[string]string{}
	// 按 unitID 排序取名字：map 遍历顺序不固定，同一支分队不能一下叫"惠普尔"
	// 一下叫"莫里斯"，否则每次读档舰队名都会变。
	ids := make([]string, 0, len(state.Units))
	for id := range state.Units {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		unit := state.Units[id]
		if unit == nil {
			continue
		}
		key := FleetKey(unit)
		if !strings.HasPrefix(key, "split-") || unit.FleetName != key {
			continue
		}
		name := named[key]
		if name == "" {
			name = unit.Name + "·分队"
			named[key] = name
		}
		unit.FleetName = name
	}
}

func ApplyImmediateOrganization(state *State, order Order) []EngineEvent {
	if order.Type == "REORGANIZE_HEX" {
		faction, ok := applyReorganization(state, order)
		if !ok {
			return nil
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

// isAirOrderType 是航母与机场共用的一批航空命令类型。
func isAirOrderType(orderType string) bool {
	switch orderType {
	case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_COVER", "LAUNCH_TRANSFER", "PREPARE_AIR", "RETURN_TO_HANGAR", "RECOVER_AIR", "PRIORITIZE_AIR_TASK", "SET_STRIKE_TARGET", "SET_STRIKE_DISPERSION", "ARM_AIR", "SET_AIR_CYCLE":
		return true
	}
	return false
}

// needsAirValidation 与 airOrderIsGround 配合：机场与航母共用同一批航空命令类型。
func needsAirValidation(orderType string) bool {
	return isAirOrderType(orderType)
}

// IsImmediateOrder 判断命令是否在提交时立刻生效（而不是等回合结算）。
// SET_MOVE 也走这里：提交时只登记规划，位移在回合结算里统一执行。
// 注意：RETARGET_AIR 不在这里——改目标同样是无线电通信，必须按编队消耗机会，因此走回合结算。
func IsImmediateOrder(orderType string) bool {
	switch orderType {
	case "SET_MOVE", "SET_AUTO_NAV", "SET_ENGAGEMENT":
		return true
	}
	return false
}

// IsAirOrder 从服务端转调：机场与航母共用同一批航空命令。
func IsAirOrder(orderType string) bool {
	return isAirOrderType(orderType)
}

// IsAirfieldOrder 从服务端转调：机场与航母共用航空命令，需要先判断走哪一套。
func IsAirfieldOrder(state State, order Order) bool {
	return isAirfieldTask(order.TaskID) || airOrderIsGround(state, order)
}

// airOrderIsGround 判断航空命令里的算子是否属于机场：只要有一个算子在机场机库里就走机场校验。
func airOrderIsGround(state State, order Order) bool {
	for _, id := range airOrderIDs(order) {
		if _, ok := state.GroundAir[id]; ok {
			return true
		}
	}
	if order.Type == "PRIORITIZE_AIR_TASK" || order.Type == "SET_STRIKE_TARGET" {
		for _, aircraft := range state.GroundAir {
			if aircraft.TaskID == order.AircraftID {
				return true
			}
		}
	}
	return false
}

func ResolveOrders(state *State, orders []Order) []EngineEvent {
	if state.DeferCooldown {
		state.DeferCooldown = false
	} else {
		AdvanceMovementCooldown(state)
	}
	// 先给在编算子拍个花名册，回合结束时靠它结算飞机损失。
	snapshotAircraftRoster(state)
	events := advanceTurnStartWithEvents(state)
	// 朝向已定，按风向重算甲板长度；本回合起飞/降落容量以此为基准（规则书 824 行）。
	RefreshDeckLengths(state)
	events = append(events, AdvanceShipConditions(state)...)
	events = syncArmedAircraft(state, events)
	events = append(events, AdvanceAviation(state)...)
	events = append(events, AdvanceAirfields(state)...)
	events = append(events, AdvanceGroundAir(state)...)
	for _, order := range orders {
		if order.Type != "REORGANIZE_HEX" {
			continue
		}
		faction, ok := applyReorganization(state, order)
		if !ok {
			continue
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
		if order.Type != "SET_MOVE" || strings.TrimSpace(order.Coord) == "" {
			continue
		}
		leader, ok := state.Units[order.UnitID]
		if !ok || !fleetCanEnter(*state, order.Coord, FleetHasCarrier(*state, FleetKey(leader))) {
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
		interval := FleetMoveInterval(*state, unit)
		if cooldown := movementCooldownAfterMove(interval); cooldown > unit.MoveCooldown {
			unit.MoveCooldown = cooldown
		}
		clearFleetStay(state, FleetKey(unit))
		events = append(events, EngineEvent{Type: "unit_moved", Audience: unit.Faction, Text: fmt.Sprintf("%s从%s移动至%s，每%d回合移动一次", unit.Name, from, coord, interval)})
	}
	// 自动航行（规则书 280–283 行）：手动移动与自动移动共用移动机会，
	// 所以放在手动位移之后统一结算——本回合手动动过的舰队自动航行自然不生效。
	events = append(events, resolveAutoNavigation(state)...)
	// 移动会改写舰艏朝向，甲板长度依赖朝向（规则书 817–824 行）：不重算的话，
	// 快照里会出现"朝向已经变 SW、甲板长度还按旧 NE 算"的自相矛盾状态，
	// 玩家按快照规划下一回合的起降会失败。
	RefreshDeckLengths(state)
	syncEmbarkedAircraft(state)
	// 舰队侦查（规则书 575 行）：放在机动之后、接敌之前——先看清对手，
	// 接敌时的"预先发现+1"才有来源。
	events = append(events, resolveSurfaceRecon(state)...)
	events = append(events, resolveContacts(state)...)
	// 无线电改令 / 召回（规则书 597–602 行）：放在编队移动之后再结算，命令只影响下一段航程，
	// 同时处于规则书 176–178 行的"飞行编队移动"之后、"水面交火/岸轰"之前。
	events = append(events, resolveRadioOrders(state, orders)...)
	events = append(events, resolveDetachments(state)...)
	// 岸轰和运输卸载（规则书 178 行）
	events = append(events, resolveShoreBombardment(state)...)
	events = append(events, resolveUnloading(state)...)
	events = append(events, resolvePortSupply(state)...)
	// 回合结束：跟踪中的侦察机可能被敌方发现并被 CAP 攻击（规则书 551–559 行）
	events = append(events, resolveReconExposure(state)...)
	// 回合结束：脱离接触的情报按昼夜节奏衰减（规则书 587–593 行）。
	events = append(events, decayContactIntel(state)...)
	// 战斗之后舰船强度/损伤已经变化，情报里的成员快照要同步刷新，
	// 否则 4 级情报会滞后一回合显示旧数据（战报说掉强度、舰队面板还是满的）。
	refreshContactMembers(state)
	// 先结算停留计数，再记录本回合位置：trackFleetSessions 需要拿"上一回合结束时的
	// FleetStays"和当前位置比较；顺序反了的话 FleetStays 已经被覆盖，移动永远不会清零。
	trackFleetSessions(state)
	trackFleetStay(state)
	events = append(events, advanceTowardEdge(state)...)
	// 敌方飞行编队的可见性：雷达/侦察/目视发现后对对手可见。
	events = append(events, resolveAirVisibility(state)...)
	// 天黑前召回自动循环的飞行任务（规则书 199 行）：必须在航空作业推进之前发出，
	// 否则这一回合刚起飞的飞机已经上天，来不及回家。
	events = append(events, recallLoopingAircraftBeforeNight(state)...)
	// 天色收尾：快照里的 night 对应下一回合（服务端紧接着会 turn+1），
	// 入夜那一刻还在天上的算子在回收截止时判坠海。
	events = append(events, settleDayNightAtTurnEnd(state)...)
	// 胜利条件（DESIGN.md 49 行）：每回合结算后统一判定一次。
	// 回合结束对账：花名册里有、现在不在编的算子记为损失（击落/油箱耗尽/随舰沉没/机库被炸）。
	settleAircraftLosses(state)
	return append(events, settleOutcome(state)...)
}

// syncArmedAircraft 维护"整装待发"标记：
// 与当前航空任务挂载不符的算子在机库整备时转为未武装（需要额外 1 回合换挂载，规则书 258 行）；
// 与任务相符、且已经整备完毕的算子标记为 Armed，被炸弹命中时按整装待发规则摧毁（规则书 421–423 行）。
func syncArmedAircraft(state *State, events []EngineEvent) []EngineEvent {
	for _, id := range sortedAirUnitIDs(*state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil {
			continue
		}
		if !armedTaskMatches(aircraft.TaskKind, aircraft) {
			if aircraft.Armed {
				aircraft.Armed = false
				events = append(events, EngineEvent{Type: "air_rearm", Audience: aircraft.Faction, Text: aircraft.Model + "任务挂载不符，需要额外1回合在机库更换挂载"})
			}
			continue
		}
		if !aircraft.Armed && (aircraft.Status == "hangar" || aircraft.Status == "service") {
			aircraft.Armed = true
		}
	}
	return events
}

// armedTaskMatches 判断机型能否直接执行某类任务：
// 战斗机只能挂 CAP/护航，轰炸/侦察机型可以承担侦察，攻击/转场任务需要轰炸能力。
// 挂弹战斗机是战斗轰炸型，攻击任务同样算"整装待发"。
func armedTaskMatches(taskKind string, aircraft *AirUnit) bool {
	if aircraft == nil {
		return false
	}
	switch taskKind {
	case "":
		return false
	case "cap", "cover":
		return aircraft.Kind == "fighter"
	case "recon":
		return canSearch(*aircraft)
	case "attack":
		return aircraft.Kind != "fighter" || aircraft.Loadout.Weapon() == WeaponBomb
	case "transfer":
		return aircraft.Kind != "fighter"
	}
	return false
}

// advanceTurnStart 对应规则书 169 行的"回合开始状态处理"：
// 上一回合的小型火灾已经由损管阶段扑灭（火灾只持续一回合），这里负责清掉本回合的无线电记录。
func advanceTurnStart(state *State) {
	// 每个空中编队每回合各有一次无线电机会，回合开始时清空"已发射"记录。
	state.RadioUsed = map[string]string{}
	// 每回合玩家操作阶段可以调整一次舰队朝向（规则书 825 行）。
	state.HeadingChanged = map[string]bool{}
	// 新回合开始，尚未获得任何新情报。
	resetIntelFresh(state)
	// 接敌判定每回合每编队一次。
	state.EngageTried = map[string]bool{}
}

// advanceTurnStartWithEvents 在 advanceTurnStart 的基础上返回昼夜切换等回合开始事件。
func advanceTurnStartWithEvents(state *State) []EngineEvent {
	advanceTurnStart(state)
	// 回合开始状态处理：按剧本的昼夜循环刷新白天/夜间（规则书 199、587–593 行）。
	events := UpdateDayNight(state)
	// 动态天气（规则书 313–314、815 行）：剧本开启天气系统时按间隔刷新。
	return append(events, StepWeather(state)...)
}

// AdvanceTurnStart 是服务端可见的回合开始钩子：刷新昼夜等回合级状态并返回事件。
func AdvanceTurnStart(state *State) []EngineEvent {
	return advanceTurnStartWithEvents(state)
}

// resolveRadioOrders 按指令顺序结算本回合的无线电通信：
// 每个空中编队每回合各有一次机会（规则书 601 行），发出即消耗，失败不能再发。
func resolveRadioOrders(state *State, orders []Order) []EngineEvent {
	if state.RadioUsed == nil {
		state.RadioUsed = map[string]string{}
	}
	events := []EngineEvent{}
	for _, order := range orders {
		if !isRadioOrder(order.Type) {
			continue
		}
		faction := radioOrderFaction(*state, order)
		if faction == "" {
			continue
		}
		if _, used := state.RadioUsed[order.TaskID]; used {
			events = append(events, EngineEvent{Type: "radio_exhausted", Audience: faction, Text: "该编队本回合已经发过无线电，命令被忽略"})
			continue
		}
		events = append(events, ApplyRadioOrder(state, faction, order)...)
	}
	return events
}

// radioOrderFaction 从任务里的算子反推阵营。
func radioOrderFaction(state State, order Order) string {
	for _, aircraft := range airTaskMembersEveryStatus(state, order.TaskID) {
		if aircraft.Faction != "" {
			return aircraft.Faction
		}
	}
	for _, aircraft := range groundTaskMembersEveryStatus(state, order.TaskID) {
		if aircraft.Faction != "" {
			return aircraft.Faction
		}
	}
	return ""
}

// cloneUnit 复制一份舰船快照给视图用。
//
// Damage/Deck 这些子对象引擎是原地改的（进水层数、甲板长度、升降机状态），
// 视图要是直接共享指针，"上一帧的投影"就会跟着房间状态一起变——补丁拿它比差异，
// 会算成"没变化"，玩家那边就停在旧画面上了。投影必须是值快照。
func cloneUnit(unit Unit) Unit {
	copied := unit
	if unit.Damage != nil {
		damage := *unit.Damage
		damage.Flooding = append([]int(nil), unit.Damage.Flooding...)
		copied.Damage = &damage
	}
	if unit.Deck != nil {
		deck := *unit.Deck
		deck.Elevators = make([]*AircraftElevator, len(unit.Deck.Elevators))
		for index, elevator := range unit.Deck.Elevators {
			if elevator == nil {
				continue
			}
			elevatorCopy := *elevator
			elevatorCopy.AircraftIDs = append([]string(nil), elevator.AircraftIDs...)
			deck.Elevators[index] = &elevatorCopy
		}
		copied.Deck = &deck
	}
	return copied
}

func VisibleUnits(state State, faction string) []Unit {
	units := []Unit{}
	visibleFleets := map[string]bool{}
	for _, unit := range state.Units {
		visible := unit.Faction == faction
		if !visible {
			for _, friendly := range state.Units {
				if friendly.Faction == faction && Distance(friendly.Coord, unit.Coord) <= visualContactRange {
					visible = true
					break
				}
			}
		}
		if visible {
			copyUnit := cloneUnit(*unit)
			copyUnit.FleetID = FleetKey(unit)
			visibleFleets[copyUnit.FleetID] = true
			if copyUnit.Heading == "" {
				copyUnit.Heading = "NE"
			}
			copyUnit.LegalMoves = nil
			if unit.Faction == faction {
				copyUnit.LegalMoves = LegalMoves(state, unit)
			}
			// 实际航速档案要随快照下发：进水每层降一档，玩家在兵牌上看到的"慢速"必须等于
			// 引擎真正用的档位，而不是卡面印着的设计航速。
			copyUnit.EffectiveSpeed = EffectiveSpeedClass(&copyUnit)
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
			// 自动航行信息只给自己看，敌方不该看到对方的航线规划。
			fleet.Destination = FleetDestination(state, id)
			fleet.Engagement = string(FleetEngagementSetting(state, id))
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
				fleet.ContactStaleTurns = unit.ContactStaleTurns
			}
		}
		if fleet.ContactLevel > 0 {
			// 情报未到 4 级时，规模要用情报里的真实数量算，而不是投影出来的一两个占位算子：
			// 1 级连规模都不显示，2-3 级只给"小/中/大"。
			if fleet.Faction != faction {
				if contact := state.Contacts[faction][id]; contact != nil && contact.Level < 4 {
					if contact.Level <= 1 {
						fleet.ContactSize = ""
					} else {
						fleet.ContactSize = contactSizeClass(contact.Count)
					}
				} else {
					fleet.ContactSize = contactSizeClass(fleet.Count)
				}
			} else {
				fleet.ContactSize = contactSizeClass(fleet.Count)
			}
			// 情报 3 级起能判断出有没有主力舰：只给类别，不给具体舰名和数量。
			if fleet.Faction != faction && fleet.ContactLevel >= 3 {
				members := fleet.Members
				if contact := state.Contacts[faction][id]; contact != nil && len(contact.Members) > 0 {
					members = contact.Members
				}
				hasCarrier, hasBattleship, hasHeavy := false, false, false
				for _, member := range members {
					switch member.Type {
					case "CV", "CVL", "CVE":
						hasCarrier = true
					case "BB", "BC":
						hasBattleship = true
					case "CA":
						hasHeavy = true
					}
				}
				if hasCarrier {
					fleet.ContactCapitals = append(fleet.ContactCapitals, "carrier")
				}
				if hasBattleship {
					fleet.ContactCapitals = append(fleet.ContactCapitals, "battleship")
				}
				if hasHeavy {
					fleet.ContactCapitals = append(fleet.ContactCapitals, "heavy_cruiser")
				}
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
