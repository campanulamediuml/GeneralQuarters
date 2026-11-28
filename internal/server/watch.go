package server

import (
	"context"
	"net/http"
	"strings"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

// spectateView 读取玩家和战报，再投影成 WebSocket 观战连接的首帧与补丁底座。
func (s *Server) spectateView(ctx context.Context, room store.Room, players []store.Player) (map[string]any, error) {
	events, err := s.store.AllEvents(ctx, room.ID)
	if err != nil {
		return nil, err
	}
	view := buildSpectateView(room, players, events)
	// 观战视图和玩家视图用同一份展示文案字典：前端靠它把状态枚举翻成中文。
	view["display"] = s.display
	return view, nil
}

// buildSpectateView 把房间状态整理成观战页要的那一份 JSON 视图。
// 抽出来是为了能单独量它的开销：一次观战投影的重活几乎都在这里。
func buildSpectateView(room store.Room, players []store.Player, events []store.Event) map[string]any {
	state := room.State
	units := []game.Unit{}
	fleets := []game.FleetGroup{}
	airfields := []game.AirfieldContact{}
	groundAir := []game.AirfieldAirStatus{}
	groundTasks := spectatorGroundTasks(state)
	stays := map[string]string{}
	carriers := []game.CarrierAirStatus{}
	tasks := []game.AirTask{}
	combats := []game.AirCombatReport{}
	seenCombat := map[string]bool{}
	for _, faction := range []string{"A", "B"} {
		for _, unit := range game.VisibleUnits(state, faction) {
			if unit.Faction == faction {
				units = append(units, unit)
			}
		}
		for _, fleet := range game.VisibleFleets(state, faction) {
			if fleet.Faction == faction {
				fleets = append(fleets, fleet)
			}
		}
		aviation := game.VisibleAviation(state, faction)
		carriers = append(carriers, aviation.Carriers...)
		// 观战页把双方编队画在同一张图上，任务要带阵营才能按阵营上色；玩家视图里这个字段为空。
		for _, task := range aviation.Tasks {
			task.Faction = faction
			tasks = append(tasks, task)
		}
		for _, airfield := range game.VisibleAirfields(state, faction) {
			if airfield.Faction == faction {
				airfields = append(airfields, airfield)
			}
		}
		groundAir = append(groundAir, game.VisibleGroundAir(state, faction)...)
		for _, report := range game.VisibleAirCombats(state, faction) {
			if !seenCombat[report.ID] {
				seenCombat[report.ID] = true
				combats = append(combats, report)
			}
		}
		for fleetID, coord := range game.VisibleFleetStays(state, faction) {
			stays[fleetID] = coord
		}
	}
	losses := spectatorLosses(state)
	response := map[string]any{
		"room_id": room.ID, "status": room.Status, "turn": room.Turn,
		"scenario":        map[string]any{"id": state.ScenarioID, "name": state.ScenarioName, "description": state.Description},
		"map":             game.MapForView(state.Map),
		"units":           units,
		"fleets":          fleets,
		"aviation":        map[string]any{"carriers": carriers, "tasks": tasks},
		"airfields":       airfields,
		"ground_air":      groundAir,
		"ground_tasks":    groundTasks,
		"air_combats":     combats,
		"fleet_stays":     stays,
		"losses":          losses,
		"scores":          game.VictoryPoints(state),
		"score_breakdown": game.ScoreBreakdown(state),
		"objectives":      state.Objectives,
		"shore_supplies":  intMapSnapshot(state.ShoreSupplies),
		// 观战页只给潜艇战报，不给潜艇实时位置：潜艇的隐蔽性对观战者也不开放。
		"sub_battles": game.VisibleSubBattles(state),
		"outcome":     state.Outcome,
		"night":       state.Night,
		"clock":       game.Clock(state),
		// 和玩家视图一样发对象：前端按 wind.cells 画逐格风向箭头，发纯字符串时观战页拿不到逐格风。
		"wind":          game.WindView{Wind: game.PrevailingWind(state), Cells: game.WindCells(state)},
		"weather_cells": game.CloudCells(state.Map),
		"players":       playersSnapshot(players),
		"events":        events,
		"spectator":     true,
	}
	return response
}

// intMapSnapshot 拷贝一份 map 给视图用：岸头物资会被卸载/岸轰改掉，
// 视图直接放房间内存里的那张表，"上一帧"就会跟着变，补丁算不出差异。
func intMapSnapshot(source map[string]int) map[string]int {
	if source == nil {
		return nil
	}
	copied := make(map[string]int, len(source))
	for key, value := range source {
		copied[key] = value
	}
	return copied
}

// spectatorLosses 合并双方的损失台账。
//
// VisibleLosses 会把"对方阵营的飞机损失"抹掉（盲棋：不能靠台账数出对手还剩多少飞机）。
// 观战要按阵营分别投影再合并，而且**自己那一份必须优先**：以前后处理的 B 视图
// 直接把 A 的飞机损失覆盖成空，观战页显示 A 一架飞机都没丢过。
func spectatorLosses(state game.State) map[string]game.LossLedger {
	merged := map[string]game.LossLedger{}
	for _, faction := range []string{"A", "B"} {
		for side, ledger := range game.VisibleLosses(state, faction) {
			if side == faction {
				merged[side] = ledger
				continue
			}
			if _, exists := merged[side]; !exists {
				merged[side] = ledger
			}
		}
	}
	return merged
}

// spectatorGroundTasks 合并双方机场航空任务。观战页的基地侦察扇面、CAP 牌都从这份数据取；
// 私有快照按阵营各自投影，观战要两边都看，所以这里逐阵营合并并补上 faction 供上色。
func spectatorGroundTasks(state game.State) []game.AirTask {
	tasks := []game.AirTask{}
	for _, faction := range []string{"A", "B"} {
		for _, task := range game.GroundAviationView(state, faction).Tasks {
			task.Faction = faction
			tasks = append(tasks, task)
		}
	}
	return tasks
}

// resolveWatchRoom 把观战链接里的 token 解析成房间 ID。
// 前端 /watch/{token} 是 SPA 路由（不再是服务端渲染的独立页面）：
// 它先调这个接口拿 room_id，再连 /ws?mode=spectator 并 subscribe。
// token 无效/不存在一律 404，页面据此显示"观战链接无效"。
func (s *Server) resolveWatchRoom(w http.ResponseWriter, r *http.Request) {
	token := game.NormalizeToken(strings.TrimPrefix(r.URL.Path, "/api/v1/watch/"))
	if len(token) != 12 {
		writeError(w, http.StatusNotFound, "房间 token 无效")
		return
	}
	roomID, err := s.store.RoomIDByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusNotFound, "房间 token 无效")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"room_id": roomID})
}
