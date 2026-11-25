package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"

	"generalquarters/internal/game"
)

//go:embed watch.html
var watchHTML []byte

// spectate 是"观战"视角：用房间 join_token 换一份不按阵营裁剪的完整快照。
// 对局本身仍然是两边私有情报各看各的；这个接口只给拿得到邀请 token 的人（房主/旁观者）用，
// 页面是 /watch/{roomID}?token=XXXX。它会合并 A/B 两边的合法视图——
// 每艘船取自己阵营那份（含完整卡面），敌方视图里的情报投影直接丢掉。
func (s *Server) spectate(w http.ResponseWriter, r *http.Request, roomID string) {
	room, err := s.store.Room(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "房间不存在")
		return
	}
	if game.NormalizeToken(r.URL.Query().Get("token")) != room.JoinToken {
		writeError(w, http.StatusForbidden, "观战 token 不正确")
		return
	}
	players, err := s.store.Players(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取玩家失败")
		return
	}
	events, err := s.store.AllEvents(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取战报失败")
		return
	}
	state := room.State
	units := []game.Unit{}
	fleets := []game.FleetGroup{}
	airfields := []game.AirfieldContact{}
	groundAir := []game.AirfieldAirStatus{}
	stays := map[string]string{}
	carriers := []game.CarrierAirStatus{}
	tasks := []game.AirTask{}
	combats := []game.AirCombatReport{}
	seenCombat := map[string]bool{}
	losses := map[string]game.LossLedger{}
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
		for side, ledger := range game.VisibleLosses(state, faction) {
			losses[side] = ledger
		}
	}
	response := map[string]any{
		"room_id": room.ID, "status": room.Status, "turn": room.Turn, "version": room.Version,
		"scenario":       map[string]any{"id": state.ScenarioID, "name": state.ScenarioName, "description": state.Description},
		"map":            state.Map,
		"units":          units,
		"fleets":         fleets,
		"aviation":       map[string]any{"carriers": carriers, "tasks": tasks},
		"airfields":      airfields,
		"ground_air":     groundAir,
		"air_combats":    combats,
		"fleet_stays":    stays,
		"losses":         losses,
		"scores":         game.VictoryPoints(state),
		"objectives":     state.Objectives,
		"shore_supplies": state.ShoreSupplies,
		"outcome":        state.Outcome,
		"night":          state.Night,
		"clock":          game.Clock(state),
		"wind":           state.Wind,
		"players":        players,
		"events":         events,
		"spectator":      true,
	}
	writeJSON(w, http.StatusOK, response)
}

// watchPage 是给旁观者看的只读海图，不依赖前端构建产物，随服务端一起发布。
func (s *Server) watchPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	page := watchHTML
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
