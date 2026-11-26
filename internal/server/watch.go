package server

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

//go:embed watch.html
var watchHTML []byte

// doSpectate 是"观战"视角：用房间 join_token 换一份不按阵营裁剪的完整快照。
// 对局本身仍然是两边私有情报各看各的；这个接口只给拿得到邀请 token 的人（房主/旁观者）用，
// 页面是 /watch/{roomID}?token=XXXX。它会合并 A/B 两边的合法视图——
// 每艘船取自己阵营那份（含完整卡面），敌方视图里的情报投影直接丢掉。
func (s *Server) doSpectate(msg roomMsg) roomReply {
	ctx := context.Background()
	actor, err := s.actorRoom(msg.roomID)
	if err != nil {
		return roomError(http.StatusNotFound, "房间不存在")
	}
	if game.NormalizeToken(msg.token) != actor.room.JoinToken {
		return roomError(http.StatusForbidden, "观战 token 不正确")
	}
	view, err := s.spectateView(ctx, actor.room, actor.players)
	if err != nil {
		return roomError(http.StatusInternalServerError, "读取观战视图失败")
	}
	return roomJSON(http.StatusOK, view)
}

// spectateView 读取玩家和战报再投影。HTTP 接口和 WebSocket 推送共用这条路径，
// 免得两边对同一份状态给出不一样的视图。
func (s *Server) spectateView(ctx context.Context, room store.Room, players []store.Player) (map[string]any, error) {
	events, err := s.store.AllEvents(ctx, room.ID)
	if err != nil {
		return nil, err
	}
	return buildSpectateView(room, players, events), nil
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
		tasks = append(tasks, aviation.Tasks...)
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
		"outcome":         state.Outcome,
		"night":           state.Night,
		"clock":           game.Clock(state),
		"wind":            game.PrevailingWind(state),
		// 逐格风向/天气：行优先字符串，观战页按格画箭头和云层。
		"wind_cells":    game.WindCells(state),
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

// watchPage 是给旁观者看的只读海图，不依赖前端构建产物，随服务端一起发布。
// SetWatchTemplate 让观战页改成从磁盘读模板。
//
// 观战页是 go:embed 进二进制的，默认改一行 HTML 就得重新编译再重启——和改 Vue 页面
// 的体验完全不一样。开发时把它指到 internal/server/watch.html 就回到"改完刷新"。
// 空字符串表示继续用内嵌版本（发布时就是这一份，单文件部署不受影响）。
func (s *Server) SetWatchTemplate(path string) {
	s.watchPath = path
	s.watchMu.Lock()
	s.watchBody, s.watchMtime = nil, time.Time{}
	s.watchMu.Unlock()
}

// watchDocument 返回当前观战页 HTML：没配路径就用内嵌版本，配了就读磁盘（按 mtime 缓存，
// 每帧请求都读盘没必要）。读失败退回内嵌版本——页面挂了总比整个观战打不开好。
func (s *Server) watchDocument() []byte {
	if s.watchPath == "" {
		return watchHTML
	}
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	info, err := os.Stat(s.watchPath)
	if err != nil {
		return watchHTML
	}
	if s.watchBody != nil && info.ModTime().Equal(s.watchMtime) && info.Size() == int64(len(s.watchBody)) {
		return s.watchBody
	}
	data, err := os.ReadFile(s.watchPath)
	if err != nil {
		return watchHTML
	}
	s.watchBody, s.watchMtime = data, info.ModTime()
	return data
}

func (s *Server) watchPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	page := s.watchDocument()
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 2 && r.URL.Query().Get("token") == "" {
		token := game.NormalizeToken(parts[1])
		roomID, err := s.store.RoomIDByToken(r.Context(), token)
		if err != nil || len(token) != 12 {
			http.Error(w, "房间 token 无效", http.StatusNotFound)
			return
		}
		encoded, _ := json.Marshal(roomID)
		page = bytes.Replace(page, []byte("var room = parts[1] || '';"), []byte("var room = "+string(encoded)+";"), 1)
	}
	_, _ = w.Write(page)
}
