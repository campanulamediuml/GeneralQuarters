package game

import (
	"fmt"
)

// —— 开局部署阶段（deploy）——
//
// 剧本可以给每个阵营划一个部署区（锚点 hex + 半径）。开局时舰队在区内随机出生，
// 双方各自把舰队拖到满意的位置、整编/拆分完毕，都点"部署完毕"之后才进第 1 回合。
// 部署阶段不算回合：没有冷却、不触发搜索、也不消耗 RNG 之外的东西。

const (
	PhaseDeploy = "deploy"
	PhaseBattle = "battle"
)

// InDeployPhase 判断当前是否还在开局部署阶段。老存档没有 phase 字段，按正常对局处理。
func InDeployPhase(state State) bool { return state.Phase == PhaseDeploy }

// deployZoneOf 取某个阵营的部署区。
func deployZoneOf(zones []DeployZone, faction string) *DeployZone {
	for index := range zones {
		if zones[index].Faction == faction {
			return &zones[index]
		}
	}
	return nil
}

// DeployZoneFor 取某阵营的部署区（状态里存的那份）。
func DeployZoneFor(state State, faction string) *DeployZone {
	return deployZoneOf(state.DeployZones, faction)
}

// zoneCoords 把部署区展开成候选 hex：六边形距离 <= 半径，且按舰种规则可以进入。
// airfieldCoords 由调用方给：新开局时机场还没实例化，只能用剧本里的坐标表。
func zoneCoords(gameMap MapDefinition, zone DeployZone, carrierFleet bool, airfieldCoords map[string]bool) []string {
	if _, _, err := ParseCoord(zone.Anchor); err != nil {
		return nil
	}
	radius := zone.Radius
	if radius < 0 {
		radius = 0
	}
	candidates := []string{}
	for row := 1; row <= gameMap.Rows; row++ {
		for col := 1; col <= gameMap.Cols; col++ {
			coord := Coord(row, col)
			if Distance(zone.Anchor, coord) > radius {
				continue
			}
			if !fleetTerrainAllowsEntry(gameMap, coord, carrierFleet, airfieldCoords[coord]) {
				continue
			}
			candidates = append(candidates, coord)
		}
	}
	return candidates
}

// DeployZoneCoords 是运行期版本：机场已经实例化，直接从状态里取机场格。
func DeployZoneCoords(state State, zone DeployZone, carrierFleet bool) []string {
	airfieldCoords := make(map[string]bool, len(state.Airfields))
	for _, airfield := range state.Airfields {
		airfieldCoords[airfield.Coord] = true
	}
	return zoneCoords(state.Map, zone, carrierFleet, airfieldCoords)
}

// DeployMoves 是部署阶段的"可落点"：区内所有能进的格子，不要求相邻，也不看冷却。
// 前端把它当成 legal_moves 用——部署时整片部署区都是拖拽目标。
func DeployMoves(state State, unit *Unit) []string {
	if unit == nil {
		return nil
	}
	zone := DeployZoneFor(state, unit.Faction)
	if zone == nil {
		return nil
	}
	carrierFleet := IsCarrier(unit) || FleetHasCarrier(state, FleetKey(unit))
	return DeployZoneCoords(state, *zone, carrierFleet)
}

// ApplyDeployMove 是部署阶段的移动：整支舰队列瞬移到目标格，
// 不产生移动冷却、不触发搜索与停留计时（这些都是开战之后才有的东西）。
func ApplyDeployMove(state *State, order Order) []EngineEvent {
	leader := state.Units[order.UnitID]
	if leader == nil {
		return nil
	}
	if inBounds(state.Map, order.Coord) == false {
		return nil
	}
	target := ""
	for _, coord := range DeployMoves(*state, leader) {
		if coord == order.Coord {
			target = coord
			break
		}
	}
	if target == "" {
		return nil
	}
	fleetID := FleetKey(leader)
	moved := 0
	for _, unit := range state.Units {
		if unit.Faction != leader.Faction || FleetKey(unit) != fleetID {
			continue
		}
		unit.Heading = headingBetween(unit.Coord, target)
		unit.Coord = target
		unit.Moved = false
		unit.MoveCooldown = 0
		moved++
	}
	if moved == 0 {
		return nil
	}
	name := leader.FleetName
	if name == "" {
		name = leader.Name
	}
	return []EngineEvent{{Type: "unit_deployed", Audience: leader.Faction, Text: fmt.Sprintf("%s在%s完成部署", name, target)}}
}

// StartBattle 结束部署阶段：清掉部署期间留下的冷却与停留计时，进入第 1 回合。
// 部署时的整编会把舰队锁 3 回合，那些锁必须在这里抹掉，否则开局第一回合动弹不得。
func StartBattle(state *State) []EngineEvent {
	if !InDeployPhase(*state) {
		return nil
	}
	state.Phase = PhaseBattle
	for _, unit := range state.Units {
		unit.Moved = false
		unit.MoveCooldown = 0
	}
	state.FleetStays = map[string]string{}
	state.FleetSessions = map[string]int{}
	state.HeadingChanged = map[string]bool{}
	RefreshDeckLengths(state)
	return []EngineEvent{{Type: "battle_start", Audience: "all", Text: "双方部署完毕，第 1 回合开始"}}
}

// DeployZoneView 把部署区投影给前端：锚点、半径，以及行优先的格位串（'1' = 区内可下水的格）。
// 部署阶段之外返回空视图，但字段形状保持一致——补丁按字段 diff，字段忽有忽无会让客户端难合并。
func DeployZoneView(state State, faction string) map[string]any {
	view := map[string]any{"anchor": "", "radius": 0, "cells": ""}
	if !InDeployPhase(state) {
		return view
	}
	zone := DeployZoneFor(state, faction)
	if zone == nil {
		return view
	}
	rows, cols := state.Map.Rows, state.Map.Cols
	cells := make([]byte, rows*cols)
	for index := range cells {
		cells[index] = '0'
	}
	for _, coord := range DeployZoneCoords(state, *zone, false) {
		row, col, err := ParseCoord(coord)
		if err != nil || row < 1 || row > rows || col < 1 || col > cols {
			continue
		}
		cells[(row-1)*cols+(col-1)] = '1'
	}
	view["anchor"] = zone.Anchor
	view["radius"] = zone.Radius
	view["cells"] = string(cells)
	return view
}

// deployOrderAllowed 是部署阶段允许的命令：只摆船，不能开火、侦察、起降。
func deployOrderAllowed(orderType string) bool {
	switch orderType {
	case "SET_MOVE", "CLEAR_MOVE", "MERGE_FLEET", "REORGANIZE_HEX":
		return true
	}
	return false
}
