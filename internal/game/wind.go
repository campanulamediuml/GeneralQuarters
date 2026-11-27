package game

import (
	"fmt"
	"sort"
	"strings"
)

// —— 风向与甲板长度（规则书 815–825 行）——
//
// 剧本里的风向是"来向"（气象习惯：SE = 风从东南吹来，吹向西北）。
// 甲板长度修正看舰艏与风的夹角：
//   para  逆风：舰艏朝着来风方向，甲板相对风速最大    +2
//   same  顺风：舰艏与风同向跑，几乎没有相对风速      -2
//   ortho 侧逆风：偏离来风 60°，仍有迎面分量         +1
//   meta  侧顺风：偏离来风 120°，带顺风分量           -1
// 修正后的甲板长度是本回合起飞/降落的基础容量，最低为 0；为 0 时无法进行相应航空作业。
// 每回合玩家操作阶段可以调整一次舰队朝向，本回合自动结算期间不再改变。

// windHeadings 是六个朝向在 hex 平面上的角度（与侦察扇面共用同一套角度定义）。
func windHeadingAngle(heading string) (float64, bool) {
	angle, ok := reconHeadings[strings.ToUpper(heading)]
	return angle, ok
}

// windRelation 计算风向与舰艏的关系：same / para / ortho / meta。
// 以 60° 为一档：0° 同向（same）、60°/300° 侧顺风（ortho）、
// 120°/240° 侧逆风（meta）、180° 逆风（para）。
func windRelation(wind, heading string) string {
	windAngle, okWind := windHeadingAngle(wind)
	// 无风（剧本没设 wind）不该被当成"侧向顺风 +1"：以前这里一律回落到 ortho，
	// 导致"没写风向"的剧本反而给所有航母白加一格甲板。没有风就没有修正。
	if !okWind {
		return "none"
	}
	headingAngle, okHeading := windHeadingAngle(heading)
	if !okHeading {
		return "ortho"
	}
	diff := windAngle - headingAngle
	for diff < 0 {
		diff += 360
	}
	for diff >= 360 {
		diff -= 360
	}
	// 风向按"来向"记（气象习惯：东南风 = 风从东南吹来）。
	// 因此舰艏朝着风向 = 顶着风跑 = 逆风（甲板更长），反向才是顺风。
	step := int((diff+30)/60) % 6
	switch step {
	case 0:
		return "para" // 顶着风：逆风 +2
	case 1, 5:
		return "ortho" // 侧向，仍带一点迎面分量 +1
	case 2, 4:
		return "meta" // 侧向，带一点顺风分量 -1
	default:
		return "same" // 与风同跑：顺风 -2
	}
}

// windDeckModifier 返回某种风向关系对甲板长度的修正值。
func windDeckModifier(wind, heading string) int {
	// 甲板是 4（护航航母）/6（轻航母）/8（舰队航母）这种小整数，+2 在 4 上就是 +50%、
	// 在 8 上只有 +25%，比例完全不一样；调成 +1/0/-1/-2 之后顶风收益收窄，
	// 顺风仍然是腰斩级别的代价，风更像约束而不是白送。
	switch windRelation(wind, heading) {
	case "same":
		return -2
	case "para":
		return 1
	case "ortho":
		return 0
	case "meta":
		return -1
	}
	return 0
}

// deckLengthFor 计算某艘舰本回合的修正后甲板长度：基础容量 + 风向修正，最低 0。
// 非航母（只有吊车或水上飞机）不参与风向修正。
func deckLengthFor(state State, unit *Unit) int {
	if unit == nil || unit.Deck == nil {
		return 0
	}
	base := unit.Deck.BaseLength
	if base <= 0 {
		base = unit.Deck.Length
	}
	if !IsCarrier(unit) {
		if base < 0 {
			return 0
		}
		return base
	}
	heading := unit.Heading
	if heading == "" {
		heading = "NE"
	}
	// 风向按船所在的 hex 取：同一支舰队在同一个格，但不同海区的风不一样。
	direction, hasWind := state.WindDirectionAt(unit.Coord)
	if !hasWind {
		if base < 0 {
			return 0
		}
		return base
	}
	length := base + windDeckModifier(direction, heading)
	if length < 0 {
		return 0
	}
	return length
}

// RefreshDeckLengths 每回合按当前风向与舰队朝向重算所有舰船的甲板长度。
// 规则书 825 行：朝向在自动结算期间不再改变，因此这里在结算开始时统一刷新一次即可。
func RefreshDeckLengths(state *State) {
	for _, unit := range state.Units {
		if unit.Deck == nil {
			continue
		}
		// 只改本回合可用长度；卡面的 DeckCapacity 保留基础值，供兵牌/角色卡显示。
		unit.Deck.Length = deckLengthFor(*state, unit)
	}
}

// SetFleetHeading 调整一条舰队（整队）的朝向，每回合每支舰队只能调整一次。
func SetFleetHeading(state *State, fleetID, heading string) []EngineEvent {
	heading = strings.ToUpper(strings.TrimSpace(heading))
	if _, ok := windHeadingAngle(heading); !ok {
		return nil
	}
	if state.HeadingChanged == nil {
		state.HeadingChanged = map[string]bool{}
	}
	if state.HeadingChanged[fleetID] {
		return nil
	}
	state.HeadingChanged[fleetID] = true
	names := []string{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if FleetKey(unit) != fleetID {
			continue
		}
		unit.Heading = heading
		names = append(names, unit.Name)
	}
	if len(names) == 0 {
		return nil
	}
	RefreshDeckLengths(state)
	faction := ""
	fleetCoord := ""
	for _, id := range sortedUnitIDs(state.Units) {
		if FleetKey(state.Units[id]) == fleetID {
			faction = state.Units[id].Faction
			fleetCoord = state.Units[id].Coord
			break
		}
	}
	// 风向按这支舰队所在的 hex 取，不再用全局值。
	direction, hasWind := state.WindDirectionAt(fleetCoord)
	relation := "none"
	if hasWind {
		relation = windRelation(direction, heading)
	}
	// 没有航母的舰队（巡洋舰水侦、驱逐舰）不受风力影响，事件里别提"甲板修正"，
	// 否则玩家会以为转个向能改变起飞能力。
	hasCarrier := false
	for _, id := range sortedUnitIDs(state.Units) {
		if unit := state.Units[id]; unit != nil && FleetKey(unit) == fleetID && IsCarrier(unit) {
			hasCarrier = true
			break
		}
	}
	note := windRelationName(relation)
	if hasCarrier && hasWind {
		note = fmt.Sprintf("%s，甲板长度修正%+d", note, windDeckModifier(direction, heading))
	}
	windLabel := "无风"
	if hasWind {
		windLabel = direction
	}
	return []EngineEvent{{
		Type: "fleet_heading", Audience: faction,
		Text: fmt.Sprintf("%s调整朝向为%s（本地风向%s，%s）", names[0], heading, windLabel, note),
	}}
}

func windRelationName(relation string) string {
	switch relation {
	case "none":
		return "无风"
	case "same":
		return "顺风"
	case "para":
		return "逆风"
	case "ortho":
		return "侧向顺风"
	case "meta":
		return "侧向逆风"
	}
	return relation
}

// validateHeadingOrder 校验朝向调整命令。
func validateHeadingOrder(state State, faction string, order Order) error {
	unit, ok := state.Units[order.UnitID]
	if !ok || unit.Faction != faction {
		return fmt.Errorf("单位不存在或不属于当前玩家")
	}
	if _, ok := windHeadingAngle(order.Heading); !ok {
		return fmt.Errorf("朝向必须是E、SE、SW、W、NW或NE")
	}
	if state.HeadingChanged[FleetKey(unit)] {
		return fmt.Errorf("该舰队本回合已经调整过朝向")
	}
	return nil
}

// VisibleWind 把风向和每个己方舰队的风向关系告诉前端。
// PrevailingWind 返回盛行风名（有风场取风场，没有就用剧本值）。
func PrevailingWind(state State) string {
	if state.WindField != nil {
		return windDirectionName(state.WindField.BaseDeg)
	}
	return strings.ToUpper(strings.TrimSpace(state.Wind))
}

func VisibleWind(state State, faction string) WindView {
	prevailing := PrevailingWind(state)
	view := WindView{Wind: prevailing, Cells: WindCells(state), Fleets: []FleetWind{}}
	fleetIDs := map[string]bool{}
	for _, unit := range state.Units {
		if unit.Faction == faction {
			fleetIDs[FleetKey(unit)] = true
		}
	}
	ordered := make([]string, 0, len(fleetIDs))
	for id := range fleetIDs {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, fleetID := range ordered {
		var leader *Unit
		for _, id := range sortedUnitIDs(state.Units) {
			if FleetKey(state.Units[id]) == fleetID {
				leader = state.Units[id]
				break
			}
		}
		if leader == nil {
			continue
		}
		heading := leader.Heading
		if heading == "" {
			heading = "NE"
		}
		// 风力只影响航母甲板：没有航母的舰队（巡洋舰水上飞机、驱逐舰）风向对他们
		// 只是"风从哪来"，甲板修正不适用，前端不该显示"+1 甲板"这种误导数字。
		hasCarrier := false
		for _, id := range sortedUnitIDs(state.Units) {
			unit := state.Units[id]
			if FleetKey(unit) == fleetID && IsCarrier(unit) {
				hasCarrier = true
				break
			}
		}
		direction, hasWind := state.WindDirectionAt(leader.Coord)
		relation := "none"
		deckModifier := 0
		if hasWind {
			relation = windRelation(direction, heading)
			if hasCarrier {
				deckModifier = windDeckModifier(direction, heading)
			}
		}
		view.Fleets = append(view.Fleets, FleetWind{
			FleetID: fleetID, Heading: heading, Relation: relation, Direction: direction,
			DeckModifier: deckModifier, HasCarrier: hasCarrier, Changed: state.HeadingChanged[fleetID],
		})
	}
	return view
}

// WindView 是前端用的风向投影。
type WindView struct {
	// Wind 是盛行风（六方位之一）；每格的实际风向见 Cells。
	Wind string `json:"wind"`
	// Cells 是逐格风向：行优先、每格一个字符，'0'..'5' 对应 NE/E/SE/SW/W/NW。
	// 没有风场（老存档、剧本没设风向）时为空串。
	Cells  string      `json:"cells"`
	Fleets []FleetWind `json:"fleets"`
}

// FleetWind 是单支舰队与风向的关系。
type FleetWind struct {
	FleetID  string `json:"fleet_id"`
	Heading  string `json:"heading"`
	Relation string `json:"relation"`
	// Direction 是这支舰队所在 hex 的风向（六方位）；无风时为空。
	Direction string `json:"direction"`
	// HasCarrier：这支舰队里有没有航母。风力只修正航母甲板，
	// 没有航母时 DeckModifier 恒为 0，前端只显示风向关系。
	HasCarrier   bool `json:"has_carrier"`
	DeckModifier int  `json:"deck_modifier"`
	Changed      bool `json:"changed"`
}
