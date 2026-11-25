package game

import (
	"fmt"
	"log"
	"math"
	"os"
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

// ReconRadiusLimit 是侦察机的最远搜索半径：按"不含备用油"的航程折半，
// 备用油只够回家，不能拿来搜索（规则书 69 行）。
func ReconRadiusLimit(aircraft AirUnit) int {
	// 规则书 571 行：搜索半径按“转场挂载”的航程折半。
	// 必须用机种基础航程 BaseRange 换算，不能用当前挂载的 MaxRange——同一机型的飞机
	// 挂载/燃油状态不同，会算出两个不同的上限（快照显示 11、校验却拒 >10），玩家照着面板填就被拒。
	base := aircraft.BaseRange
	if base <= 0 {
		base = aircraft.MaxRange
	}
	// 按侦察任务实际会挂的那套算（recon + 副油箱），不是转场挂载：
	// 用转场航程 ÷2 算出来的半径，飞机根本飞不到——燃油会在半路把它叫回来，
	// 于是面板写着 12 hex、实际只能扫 10 hex。
	loadout := LoadoutForTaskFor(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, "recon", StrikeTargetShip, aircraft.TorpedoCapable)
	limit := SortieBaseRange(base, loadout) / 2
	if limit < 1 {
		return 1
	}
	return limit
}

// GroundReconRadius 是机场侦察机的固定搜索半径。
// 侦察半径不开放设置：命令里没有这个字段，一律取机型上限，
// 免得同一型飞机因为外部传入的数字不同而飞出两种距离。
func GroundReconRadius(aircraft *AirfieldAirUnit) int {
	if aircraft == nil {
		return 1
	}
	if aircraft.ReconRadiusLimit > 0 {
		return aircraft.ReconRadiusLimit
	}
	if aircraft.MaxRange > 1 {
		return aircraft.MaxRange / 2
	}
	return 1
}

type sectorPoint struct {
	x float64
	y float64
}

// ReconSectorHexes 返回以 origin 为顶点、heading 为轴、width 为张角的楔形区域。
// 这就是实际会被判定的搜索范围：searchReconStep 每走一步扫掉「当前距离圈 + 下一距离圈」
// 落在本扇面内的全部格子，所以扇面宽度是真的覆盖差异，不是只影响概率的装饰。
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
		// 母舰已沉没：侦察机转向最近的机场备降，而不是停在原任务里消耗燃料
		return advanceCarrierlessRecon(state, aircraft)
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
			if aircraft.Range() <= Distance(aircraft.Coord, carrier.Coord) {
				aircraft.TrackingFleetID = ""
				aircraft.Returning = true
				events = append(events, EngineEvent{Type: "recon_returning", Audience: aircraft.Faction, Text: aircraft.Model + "燃料达到返航余量，停止跟踪并返航"})
				// 就地掉头，本回合不再判定脚下这一格：敌舰就在当前位置，
				// 再判一次会立刻重新锁定，燃料不足的编队就永远回不了家。
				aircraft.Spend(1)
				return events
			} else {
				aircraft.Spend(1)
				return append(events, resolveTracking(state, aircraft, members)...)
			}
		}
	}

	steps := reconSpeed(aircraft)
	for steps > 0 {
		if aircraft.Returning && aircraft.Coord == carrier.Coord {
			crashed, riskEvents := resolveLandingRisk(state, aircraft)
			events = append(events, riskEvents...)
			if crashed {
				return events
			}
			if landIfCapacity(state, aircraft, carrier) {
				return append(events, reconRecoveredEvent(aircraft, carrier))
			}
			aircraft.Spend(1)
			return events
		}
		if !aircraft.Returning && (aircraft.ReconDistance >= aircraft.ReconRadius || aircraft.Range() <= Distance(aircraft.Coord, carrier.Coord)) {
			aircraft.Returning = true
		}
		// 前方一格：去程沿航向，返程朝母舰。每步都判"自己所在 + 正前方"，
		// 所以出去一趟扫两遍、回来再扫两遍。
		forward := ""
		if aircraft.Returning {
			forward = nextFlightCoord(*state, aircraft, carrier.Coord)
		} else {
			forward = nextReconCoord(state.Map, aircraft.Coord, aircraft.Heading)
		}
		events = append(events, searchReconStep(state, aircraft, forward)...)
		if aircraft.TrackingFleetID != "" {
			return events
		}
		if forward == "" {
			// 走到地图边缘或返航路径不可达：停止前进，耗一点油。
			aircraft.Returning = true
			aircraft.Spend(1)
			steps--
			continue
		}
		aircraft.Coord = forward
		aircraft.ReconDistance = Distance(aircraft.OriginCoord, forward)
		aircraft.Spend(1)
		steps--
	}
	if aircraft.Returning && aircraft.Coord == carrier.Coord {
		crashed, riskEvents := resolveLandingRisk(state, aircraft)
		events = append(events, riskEvents...)
		if !crashed && landIfCapacity(state, aircraft, carrier) {
			events = append(events, reconRecoveredEvent(aircraft, carrier))
		}
	}
	return events
}

func nextReconCoord(gameMap MapDefinition, coord, heading string) string {
	row, col, err := ParseCoord(coord)
	if err != nil {
		return ""
	}
	// 六向偏移必须和 Neighbors/hexCenter 的奇偶行约定一致：奇数行整体右移半格，
	// 所以同一组 (dr,dc) 在两种行上的朝向不同。此前这里把奇偶两套偏移写反了，
	// 侦察机实际飞的是偏 60° 的方向，甚至一次跨两列、跳过了中间距离圈。
	delta := map[string][2]int{"E": {0, 1}, "W": {0, -1}}
	if row%2 == 1 {
		delta["NE"] = [2]int{-1, 1}
		delta["NW"] = [2]int{-1, 0}
		delta["SE"] = [2]int{1, 1}
		delta["SW"] = [2]int{1, 0}
	} else {
		delta["NE"] = [2]int{-1, 0}
		delta["NW"] = [2]int{-1, -1}
		delta["SE"] = [2]int{1, 0}
		delta["SW"] = [2]int{1, -1}
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
	deltas := movementDeltas(from)
	best := ""
	bestDistance := Distance(from, target)
	// 遍历顺序：正前方优先（含"直飞、之后改切向"的两段式航向，见 advanceAircraftReturn），
	// 其次是转向 +60°，再次是转向 -60°，两点转向都把目标拉不开距离；最后才考虑需要折返的方向。
	// 这样在六向移动下，飞行编队要么直飞，要么切向绕飞，不会出现"一步朝反方向"的抖动。
	for _, delta := range deltas {
		row, col, err := ParseCoord(from)
		if err != nil {
			return ""
		}
		coord := Coord(row+delta[0], col+delta[1])
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

// movementDeltas 给出六向移动的 6 个相邻格偏移，按"正前 / +60 度 / -60 度 / 反向"排序。
// 之前方向只按字符串字典序打破平局，导致朝目标飞行时会先走一步反方向，显得来回横跳。
func movementDeltas(from string) [][2]int {
	row, col, err := ParseCoord(from)
	if err != nil {
		return nil
	}
	_ = col
	if row%2 == 0 {
		return [][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, -1}, {-1, 0}, {0, 1}}
	}
	return [][2]int{{1, 1}, {1, 0}, {0, 1}, {-1, 1}, {-1, 0}, {0, -1}}
}

func reconRecoveredEvent(aircraft *AirUnit, carrier *Unit) EngineEvent {
	return EngineEvent{Type: "recon_recovered", Audience: aircraft.Faction, Text: fmt.Sprintf("%s完成扇面侦察并降落在%s甲板，等待升降机下降", aircraft.Model, carrier.Name)}
}

// ReconScanBandHexes 返回这次出击实际会被判定的所有格子。
//
// 判定是"每一步扫自己所在格 + 正前方一格"（120° 再加前方左右斜格），所以真正覆盖的是
// 沿航线的一条带，不是以起点为顶点的楔形。UI 必须画这条带：
// 玩家按楔形罩过去会以为扫到了，实际航迹没踩上就永远判定不到。
func ReconScanBandHexes(gameMap MapDefinition, origin, heading string, width, radius int) []string {
	if radius < 1 || origin == "" {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	// 搜索带 = 扇面内每一圈整段弧的并集：飞机每飞到一圈就判一次那一圈，
	// 所以整趟侦察覆盖的就是整块扇形，而不是一条航迹线。
	for ring := 1; ring <= radius; ring++ {
		for _, coord := range ReconRingHexes(gameMap, origin, heading, width, ring) {
			if seen[coord] {
				continue
			}
			seen[coord] = true
			out = append(out, coord)
		}
	}
	sort.Strings(out)
	return out
}

// ReconScannedRingHexes 返回编队此刻所在的这一"扫描带"：当前距离圈 + 下一距离圈里属于本扇面的格子。
// 供 UI 高亮本回合真正判定的范围，避免再出现"画的是一块、判定的是一条线"。
func ReconScannedRingHexes(gameMap MapDefinition, origin, coord, heading string, width, radius int) []string {
	if origin == "" || coord == "" {
		return []string{}
	}
	if radius <= 0 {
		radius = 99
	}
	seen := map[string]bool{}
	out := []string{}
	ring := Distance(origin, coord)
	for _, r := range []int{ring, ring + 1} {
		if r < 1 || r > radius {
			continue
		}
		for _, item := range ReconRingHexes(gameMap, origin, heading, width, r) {
			if seen[item] {
				continue
			}
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

// searchReconStep 判定编队本步的搜索扇面。
//
// 规则书 571 行要求"飞出和返航时经过敌方舰队上空均可进行侦查判定"，这里按扇面展开做：
// 每走一步扫掉以起飞点为圆心、落在本机扇面内的「当前距离圈 + 下一距离圈」全部格子。
// 60° 是窄扇面、120° 是宽扇面（覆盖翻倍但发现率减半），宽度是真的覆盖差异，
// 而不是只影响概率的装饰；去程和返程都会重扫一遍。
func searchReconStep(state *State, aircraft *AirUnit, forward string) []EngineEvent {
	// 每步扫掉"当前距离圈 + 下一距离圈"里属于本扇面的整段弧。
	// 以前只判"本格 + 正前方一格"（120° 再加左右），等于沿航迹的一条 2–4 格窄带：
	// 战斗机从舰队旁边 2 格飞过去都发现不了，12 hex 距离上几乎永远接不了敌。
	// 每一圈只在飞越时判一次（去程一次、返程一次），同一格不会在相邻两步被反复掷骰。
	// 扇面圆心跟着舰队走：每回合用母舰当前 hex 当圆心，舰队挪一格就等于
	// 把整个搜索圈往前推了一格，相当于白扫一遍新的海域。
	// 以前圆心固定在被放飞时的位置，舰队推进后侦察机还在按老圆心扫老家。
	origin := aircraft.OriginCoord
	if carrier := state.Units[aircraft.CarrierID]; carrier != nil {
		origin = carrier.Coord
	}
	coords := ReconScannedRingHexes(state.Map, origin, aircraft.Coord, aircraft.Heading, aircraft.SectorWidth, aircraft.ReconRadius)
	// 侦察诊断日志：为什么会扫不到（圆心/方位/半径圈/最近敌舰距离）。
	if os.Getenv("GQ_RECON_LOG") != "" {
		nearest, inside := 999, false
		for _, unit := range state.Units {
			if unit.Faction == aircraft.Faction {
				continue
			}
			if d := Distance(origin, unit.Coord); d < nearest {
				nearest = d
			}
		}
		for _, c := range coords {
			for _, unit := range state.Units {
				if unit.Faction != aircraft.Faction && unit.Coord == c {
					inside = true
				}
			}
		}
		log.Printf("recon %s faction=%s origin=%s plane=%s ring=%d sector=%d scanned=%d nearestEnemy=%d inArc=%v heading=%s",
			aircraft.Model, aircraft.Faction, origin, aircraft.Coord, Distance(origin, aircraft.Coord), aircraft.SectorWidth, len(coords), nearest, inside, aircraft.Heading)
	}
	if len(coords) == 0 {
		return nil
	}
	sort.Strings(coords)
	inside := make(map[string]bool, len(coords))
	for _, coord := range coords {
		inside[coord] = true
	}
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
	// 地图边缘时前方可能不可达，这时不要把空串写进战报。
	// 扇面全展开后一次可能覆盖十几格，战报只列前 10 格，避免刷屏。
	listed := coords
	if len(listed) > 24 {
		listed = listed[:24]
	}
	detail := strings.Join(listed, "、")
	if len(coords) > len(listed) {
		detail += fmt.Sprintf("等%d格", len(coords))
	}
	events := []EngineEvent{{Type: "recon_step_searched", Audience: aircraft.Faction, Text: fmt.Sprintf("%s扫描%s", aircraft.Model, detail)}}
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
		events = append(events, EngineEvent{Type: "recon_contact", Audience: aircraft.Faction, Text: fmt.Sprintf("%s发现敌方舰队：%d艘，情报等级%d（发现率%.1f%%，判定%.1f）", members[0].Coord, len(members), level, chance, roll)})
		// 正在返航的编队只记录情报、不重新转入跟踪：它的油已经不够再跟下去了，
		// 否则每经过一次敌舰格都会被重新锁定，永远回不了母舰。
		if !aircraft.Returning && bestChance < chance && !fleetTrackedByOther(state, aircraft, fleetID) {
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
	return append(events, discoverEnemyAirfieldsAt(state, groundAirView(aircraft), coords)...)
}

func containsCoord(list []string, coord string) bool {
	for _, item := range list {
		if item == coord {
			return true
		}
	}
	return false
}

// discoverEnemyAirfieldsAt 判定视野格内的敌方机场（机场固定不动，发现率固定 30%）。
func discoverEnemyAirfieldsAt(state *State, aircraft *AirfieldAirUnit, coords []string) []EngineEvent {
	inside := make(map[string]bool, len(coords))
	for _, coord := range coords {
		inside[coord] = true
	}
	events := []EngineEvent{}
	for _, id := range sortedAirfieldIDs(*state) {
		airfield := state.Airfields[id]
		if airfield == nil || airfield.Faction == aircraft.Faction || !inside[airfield.Coord] {
			continue
		}
		if probabilityRoll(&state.RNGState) >= 30 {
			continue
		}
		setAirfieldContact(state, aircraft.Faction, airfield)
		events = append(events, EngineEvent{Type: "airfield_discovered", Audience: aircraft.Faction, Text: fmt.Sprintf("在%s发现敌方机场%s（%d条跑道）", airfield.Coord, airfield.Name, airfield.Runways)})
	}
	return events
}
func reconDetectionChance(state State, aircraft *AirUnit, members []Unit) float64 {
	chance := 25.0
	// 开战初期日军精锐机组：基础发现率 +10%（1942 年及以前）。
	if state.Doctrines[aircraft.Faction] == "japan" && state.Year > 0 && state.Year <= 1942 {
		chance += 10
	}
	for _, unit := range members {
		if isCapitalShip(unit.Type) {
			chance += 5
		} else {
			chance++
		}
		// 规则书 542 行：漏油每艘 +10%、起火每艘 +20%。
		// 拖着油迹和黑烟的舰队在海上非常显眼，之前完全没实现这两项修正。
		if unit.Damage != nil {
			if unit.Damage.OilLeak {
				chance += 10
			}
			if unit.Damage.Fire != "" {
				chance += 20
			}
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
		// 规则书 543 行原值是每 hex -2%，实测太狠：8 格外基本搜不到，
		// 而整备 + 飞行 + 空袭这条链路根本来不及。这里下调为每 hex -1%，
		// 有效搜索半径从 6–8 格放宽到 12–15 格。
		// 母舰一直在动，用起飞点算会让航母一边前进一边给侦察机白送距离加成。
		origin := aircraft.OriginCoord
		if mother := state.Units[aircraft.CarrierID]; mother != nil {
			origin = mother.Coord
		}
		chance -= float64(Distance(origin, members[0].Coord))
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
	// 本回合拿到新情报，情报过时计数清零（规则书 589 行）。
	if state.IntelFresh == nil {
		state.IntelFresh = map[string]bool{}
	}
	key := observer + "|" + fleetID
	state.IntelFresh[key] = true
	if state.IntelStaleTurns != nil {
		delete(state.IntelStaleTurns, key)
	}
	contact.Faction = members[0].Faction
	contact.Coord = members[0].Coord
	contact.Heading = members[0].Heading
	// 有新情报才重新落定显示位置：等级提高时向真实位置收敛，坐标更新时跟着最后一次目击走。
	contact.DisplayCoord = projectedIntelCoord(*contact)
	contact.Count = len(members)
	contact.Members = make([]Unit, len(members))
	copy(contact.Members, members)
	for index := range contact.Members {
		contact.Members[index].LegalMoves = nil
	}
	return contact.Level
}

// refreshContactMembers 用舰队当前真实成员刷新情报里的成员快照。
// 只更新成员与数量，不动 Level / Coord / DisplayCoord —— 位置和等级由侦察与衰减决定。
func refreshContactMembers(state *State) {
	for observer, fleets := range state.Contacts {
		_ = observer
		for fleetID, contact := range fleets {
			if contact == nil || contact.Level <= 0 {
				continue
			}
			members := fleetMembers(state, fleetID)
			if len(members) == 0 {
				continue
			}
			// 只刷新强度/损伤这类会变的字段，坐标必须钉在情报的最后发现位置上。
			// 之前直接把真实成员塞进去，4 级情报在地图上显示实时位置，而打击命令仍按
			// contact.Coord 校验——玩家点看得见的舰队被拒，只能盲点旧坐标。
			display := intelDisplayCoord(*contact)
			for index := range members {
				members[index].Coord = display
				members[index].LegalMoves = nil
			}
			contact.Count = len(members)
			contact.Members = members
			// 打击编队的目标要用"玩家看到的那一格"。情报从 3 级升到 4 级时显示位置会从
			// 带误差的推定位置跳到真实最后位置，如果编队还写着旧 hex，面板和地图就会对不上
			// （玩家点的是 2025，任务卡写着 2021）。这里让编队目标跟着情报标记走。
			for _, aircraft := range state.Aircraft {
				if aircraft.Faction == observer && aircraft.TargetFleetID == fleetID && aircraft.TargetCoord != display {
					aircraft.TargetCoord = display
					aircraft.SearchTurns = 0
				}
			}
			// 机场打击编队同样跟着情报走：它们住在 GroundAir 里，以前这段只扫舰载机，
			// 于是陆攻的目标 hex 永远停在出航时那一格，情报更新了也不掉头。
			for _, aircraft := range state.GroundAir {
				if aircraft == nil || aircraft.Faction != observer || aircraft.TargetFleetID != fleetID {
					continue
				}
				if aircraft.TargetCoord == display {
					continue
				}
				aircraft.TargetCoord = display
			}
		}
	}
}

func projectedContactUnits(contact FleetContact) []Unit {
	level := contact.Level
	heading := contact.Heading
	if heading == "" {
		heading = "NE"
	}
	if level >= 4 && len(contact.Members) > 0 {
		units := make([]Unit, len(contact.Members))
		copy(units, contact.Members)
		for index := range units {
			// 情报 4 级编成完全已知，舰队名也用真名（以前这里覆盖成"侦察接触 · 情报4"，
			// 于是地图和敌军面板上永远看不到对方到底叫 TF17 还是 MO 机动部队）。
			units[index].ContactLevel = 4
			units[index].LegalMoves = nil
		}
		return units
	}
	// 情报等级 < 4 时位置不可靠：按等级给出固定偏移（规则书 144–147 行）。
	display := intelDisplayCoord(contact)
	marker := Unit{FleetID: contact.FleetID, FleetName: fmt.Sprintf("侦察接触 · 情报%d", level), Faction: contact.Faction, Coord: display, Heading: heading, ContactLevel: level}
	if level <= 2 {
		// 1 级只显示一个算子（连规模都不给）；2 级只给编队大小。
		marker.ID = fmt.Sprintf("contact-%s-marker", contact.FleetID)
		marker.Name = "敌方舰队"
		// 2 级的"编队大小"由舰队投影的 contact_size 给出（VisibleFleets 按情报数量计算）。
		return []Unit{marker}
	}
	// 3 级：船只类型可见，但具体身份、属性和编成顺序仍然未知。
	units := make([]Unit, 0, contact.Count)
	for index := 0; index < contact.Count; index++ {
		unit := marker
		unit.ID = fmt.Sprintf("contact-%s-%02d", contact.FleetID, index+1)
		unit.Name = "未知舰船"
		if index < len(contact.Members) {
			unit.Type = contact.Members[index].Type
		}
		units = append(units, unit)
	}
	return units
}

// contactSizeClass 把舰队规模折算成地图上显示的"小/中/大"（规则书 145 行）。
// contactSizeClass 把舰队规模折算成地图上显示的"小/中/大"。
// 规则口径：1–2 艘小型、3–5 艘中型、6 艘以上大型。
func contactSizeClass(count int) string {
	switch {
	case count <= 2:
		return "small"
	case count <= 5:
		return "medium"
	}
	return "large"
}

// —— 侦察机的暴露与 CAP 反制（规则书 551–559 行）——

// reconInEnemyCAPRange 判断某坐标是否处于指定阵营 CAP 的掩护范围内（巡逻点/前出点及 1 hex）。
func reconInEnemyCAPRange(state State, coord string, capFaction string) bool {
	for _, aircraft := range state.Aircraft {
		if aircraft.Faction != capFaction || aircraft.Status != "cap" || aircraft.Returning {
			continue
		}
		if Distance(capCoord(state, aircraft), coord) <= 1 {
			return true
		}
	}
	return false
}

// reconExposureChance 侦察机被发现的概率：基础 30%，处于敌方 CAP 范围内 50%，敌方美军对空雷达额外 +20%。
func reconExposureChance(state State, recon *AirUnit) float64 {
	enemy := "B"
	if recon.Faction == "B" {
		enemy = "A"
	}
	chance := 30.0
	if reconInEnemyCAPRange(state, recon.Coord, enemy) {
		chance = 50
	}
	if state.Doctrines[enemy] == "us" {
		for _, unit := range state.Units {
			if unit.Faction == enemy && unit.Card.AirRadar && !EnsureDamage(unit).AirRadarDisabled {
				chance += 20
				break
			}
		}
	}
	if chance > 100 {
		chance = 100
	}
	return chance
}

// resolveReconExposure 回合末处理跟踪中的侦察机：暴露判定 + 敌方 CAP 尝试攻击。
func resolveReconExposure(state *State) []EngineEvent {
	events := []EngineEvent{}
	ids := make([]string, 0, len(state.Aircraft))
	for id, aircraft := range state.Aircraft {
		if aircraft.Status == "recon" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		recon := state.Aircraft[id]
		if recon == nil || recon.TrackingFleetID == "" {
			continue
		}
		enemy := "B"
		if recon.Faction == "B" {
			enemy = "A"
		}
		if probabilityRoll(&state.RNGState) <= reconExposureChance(*state, recon) {
			events = append(events, EngineEvent{Type: "recon_spotted", Audience: enemy, Text: "侦察机位置被雷达发现：" + recon.Coord})
		}
		if !reconInEnemyCAPRange(*state, recon.Coord, enemy) {
			continue
		}
		chance := 50.0
		switch weatherAt(state.Map, recon.Coord) {
		case "cloud":
			chance -= 10
		case "rain":
			chance -= 15
		case "storm":
			chance -= 20
		}
		if state.Doctrines[recon.Faction] == "us" {
			chance += 20
		}
		count := 0
		for _, cap := range state.Aircraft {
			if cap.Faction == enemy && cap.Status == "cap" && !cap.Returning && Distance(capCoord(*state, cap), recon.Coord) <= 1 {
				count++
			}
		}
		if count > 4 {
			count = 4
		}
		chance += float64(count * 5)
		if chance < 0 {
			chance = 0
		}
		if probabilityRoll(&state.RNGState) > chance {
			continue
		}
		outcome, hitEvents := applyAircraftHit(state, recon)
		events = append(events, hitEvents...)
		if outcome != "" {
			events = append(events, EngineEvent{Type: "recon_intercepted", Audience: enemy, Text: "CAP 攻击跟踪中的侦察机：" + outcome})
		}
	}
	return events
}
