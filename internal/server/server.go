package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
	"github.com/gorilla/websocket"
)

type Server struct {
	store      *store.Store
	contentDir string
	hub        *hub
	actors     sync.Map
	secure     bool
}

type roomJob struct {
	run  func()
	done chan struct{}
}

type roomActor struct {
	jobs  chan roomJob
	start sync.Once
}

func newRoomActor() *roomActor {
	return &roomActor{jobs: make(chan roomJob, 128)}
}

func (a *roomActor) run() {
	for job := range a.jobs {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("房间任务异常：%v", recovered)
				}
				close(job.done)
			}()
			job.run()
		}()
	}
}

type createRoomRequest struct {
	Name       string `json:"name"`
	ScenarioID string `json:"scenario_id"`
	// Faction 是房主自选的阵营（A/B），空值默认 A。
	// 以前房主固定是 A、加入者固定是 B，想坐日军就只能靠剧本对调，太别扭。
	Faction string `json:"faction,omitempty"`
	// Solo 是单人房：一名玩家 ready 就结算，用于沙盒自测。
	// 双人对局必须两个人都在、都 ready，否则房间会"自己跟自己跑"。
	Solo bool `json:"solo,omitempty"`
}

type joinRoomRequest struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

type commandRequest struct {
	CommandID    string            `json:"command_id"`
	BaseVersion  int64             `json:"base_version"`
	Type         string            `json:"type"`
	UnitID       string            `json:"unit_id"`
	TargetUnitID string            `json:"target_unit_id"`
	AircraftID   string            `json:"aircraft_id"`
	AircraftIDs  []string          `json:"aircraft_ids"`
	TaskID       string            `json:"task_id"`
	Coord        string            `json:"coord"`
	Heading      string            `json:"heading"`
	SectorWidth  int               `json:"sector_width"`
	Assignments  map[string]string `json:"assignments"`
	TaskKind     string            `json:"task_kind"`
	Loadout      string            `json:"loadout"`
	// Disperse：SET_STRIKE_DISPERSION 用（true=分散攻击）。
	Disperse bool `json:"disperse"`
	// Cycle：SET_AIR_CYCLE 用（true=开启自动任务循环，false=取消）。
	Cycle bool `json:"cycle"`
	// Engagement：SET_AUTO_NAV / SET_ENGAGEMENT 用（aggressive/advantage/avoid）。
	Engagement string `json:"engagement"`
}

type readyRequest struct {
	BaseVersion int64 `json:"base_version"`
	Ready       bool  `json:"ready"`
}

type roomResponse struct {
	RoomID    string `json:"room_id"`
	PlayerID  string `json:"player_id"`
	JoinToken string `json:"join_token,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(store *store.Store, contentDir string, secureCookie bool) *Server {
	s := &Server{store: store, contentDir: contentDir, secure: secureCookie}
	s.hub = newHub()
	return s
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/rooms", s.createRoom)
	mux.HandleFunc("POST /api/v1/rooms/join", s.joinRoom)
	mux.HandleFunc("GET /api/v1/rooms/", s.roomRoutes)
	mux.HandleFunc("POST /api/v1/rooms/", s.roomRoutes)
	mux.HandleFunc("GET /ws", s.webSocket)
	// content/ 下的静态素材（兵牌剪影等）：路径由舰船卡的 picture 字段给出，
	// 例如 /content/misc/image/ship/new_orleans.png。换图只要替换数据目录里的文件，
	// 既不用重新构建前端，也不用重启（FileServer 每次读盘）。
	mux.Handle("GET /content/", http.StripPrefix("/content/", http.FileServer(http.Dir(s.contentDir))))
}

func (s *Server) roomRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "rooms" {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}
	roomID, action := parts[3], parts[4]
	player, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	if player.RoomID != roomID {
		writeError(w, http.StatusForbidden, "当前会话不属于此房间")
		return
	}
	s.submitRoom(roomID, func() {
		switch {
		case r.Method == http.MethodGet && action == "state":
			s.writeState(w, r, roomID, player)
		case r.Method == http.MethodGet && action == "events":
			s.writeEvents(w, r, roomID, player)
		case r.Method == http.MethodPost && action == "commands":
			s.handleCommand(w, r, roomID, player)
		case r.Method == http.MethodPost && action == "ready":
			s.handleReady(w, r, roomID, player)
		default:
			writeError(w, http.StatusNotFound, "接口不存在")
		}
	})
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	var request createRoomRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.ScenarioID == "" {
		request.ScenarioID = "_sandbox"
	}
	scenario, gameMap, catalog, err := game.LoadScenario(s.contentDir, request.ScenarioID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "载入剧本失败："+err.Error())
		return
	}
	seed, err := randomUint64()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "随机数初始化失败")
		return
	}
	state, err := game.NewState(scenario, gameMap, catalog, seed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "剧本校验失败："+err.Error())
		return
	}
	roomID, _ := randomHex(8)
	joinToken, _ := randomToken(12)
	playerID, _ := randomHex(12)
	session, _ := randomHex(32)
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = "玩家A"
	}
	// 单人房要显式声明：沙盒剧本默认单人（自测用得最多），其余剧本默认双人对局。
	solo := request.Solo
	if request.ScenarioID == "_sandbox" {
		solo = true
	}
	room := store.Room{ID: roomID, JoinToken: joinToken, Status: "running", Turn: 1, Version: 1, State: state, Solo: solo}
	faction := strings.ToUpper(strings.TrimSpace(request.Faction))
	if faction != "A" && faction != "B" {
		faction = "A"
	}
	player := store.Player{ID: playerID, RoomID: roomID, Faction: faction, Name: name, IsHost: true, Ready: false, Connected: true}
	if err := s.store.CreateRoom(r.Context(), room, player, hashToken(session)); err != nil {
		writeError(w, http.StatusInternalServerError, "创建房间失败")
		return
	}
	s.setSessionCookie(w, session)
	writeJSON(w, http.StatusCreated, roomResponse{RoomID: roomID, PlayerID: playerID, JoinToken: formatToken(joinToken)})
}

func (s *Server) joinRoom(w http.ResponseWriter, r *http.Request) {
	var request joinRoomRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	token := game.NormalizeToken(request.Token)
	if len(token) != 12 {
		writeError(w, http.StatusBadRequest, "请输入12位房间token")
		return
	}
	roomID, err := s.store.RoomIDByToken(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusBadRequest, "房间token无效")
		return
	}
	playerID, _ := randomHex(12)
	session, _ := randomHex(32)
	var player store.Player
	s.submitRoom(roomID, func() {
		player, err = s.store.JoinRoom(r.Context(), token, strings.TrimSpace(request.Name), playerID, hashToken(session))
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.setSessionCookie(w, session)
	s.hub.broadcast(player.RoomID, "player_joined", 0)
	writeJSON(w, http.StatusOK, roomResponse{RoomID: player.RoomID, PlayerID: player.ID})
}

// commandFingerprint 把一条命令的可识别字段拼成指纹：用来区分"同一命令重试"和
// "复用 command_id 的另一条命令"。只取业务字段，不含 command_id / base_version。
func commandFingerprint(request commandRequest) string {
	parts := []string{
		request.Type, request.UnitID, request.TargetUnitID, request.AircraftID,
		strings.Join(request.AircraftIDs, ","), request.TaskID, request.Coord, request.Heading,
		strconv.Itoa(request.SectorWidth),
		request.TaskKind, request.Loadout, fmt.Sprintf("%t", request.Disperse), request.Engagement,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

func (s *Server) handleCommand(w http.ResponseWriter, r *http.Request, roomID string, player store.Player) {
	var request commandRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.CommandID == "" || len(request.CommandID) > 128 {
		writeError(w, http.StatusBadRequest, "command_id无效")
		return
	}
	fingerprint := commandFingerprint(request)
	if response, stored, found, err := s.store.CommandResponse(r.Context(), player.ID, request.CommandID); err == nil && found {
		// 同一个 command_id 必须是同一条命令：客户端重启后从 1 重新计数的情况下，
		// 沿用旧 ID 提交新命令以前会静默返回旧回执（HTTP 200），新命令被吞掉。
		if stored != "" && stored != fingerprint {
			writeError(w, http.StatusBadRequest, "command_id已被其他命令使用，请换一个新的ID")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
		return
	}
	room, err := s.store.Room(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "房间不存在")
		return
	}
	if room.Version != request.BaseVersion {
		writeError(w, http.StatusConflict, "状态已更新，请刷新后重试")
		return
	}
	order := game.Order{Type: request.Type, UnitID: request.UnitID, TargetUnitID: request.TargetUnitID, AircraftID: request.AircraftID, AircraftIDs: request.AircraftIDs, TaskID: request.TaskID, Coord: game.NormalizeToken(request.Coord), Heading: strings.ToUpper(request.Heading), SectorWidth: request.SectorWidth, Assignments: request.Assignments, TaskKind: strings.ToLower(request.TaskKind), Loadout: strings.ToLower(request.Loadout), Disperse: request.Disperse, Engagement: strings.ToLower(request.Engagement), Cycle: request.Cycle}
	if err := game.ValidateOrder(room.State, player.Faction, order); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 无线电机会在提交时就要扣：同一回合里重复给同一个编队发命令必须立刻被拒绝，
	if room.State.Outcome != nil {
		writeError(w, http.StatusBadRequest, "对局已经结束："+room.State.Outcome.Reason)
		return
	}
	// 不能等到结算阶段才丢弃（那样玩家会以为第二次也发出去了）。
	if game.IsRadioOrder(order.Type) {
		if used := pendingRadioTask(r.Context(), s.store, roomID, room.Turn, player.ID, order.TaskID); used {
			writeError(w, http.StatusBadRequest, "该编队本回合已经发过无线电，不能再通信")
			return
		}
	}
	if game.IsImmediateOrder(order.Type) || order.Type == "MERGE_FLEET" || order.Type == "REORGANIZE_HEX" || game.IsAirOrder(order.Type) || order.Type == "SET_HEADING" || order.Type == "SET_LOADOUT" || order.Type == "SET_AUTO_NAV" || order.Type == "SET_ENGAGEMENT" {
		var events []game.EngineEvent
		switch order.Type {
		case "SET_HEADING":
			events = game.SetFleetHeading(&room.State, game.FleetKey(room.State.Units[order.UnitID]), order.Heading)
		case "SET_LOADOUT":
			events = game.ApplyLoadoutOrder(&room.State, order)
		case "SET_AUTO_NAV":
			events = game.ApplyNavOrder(&room.State, order)
		case "SET_ENGAGEMENT":
			events = game.ApplyEngagementOrder(&room.State, order)
		case "SET_MOVE":
			// 移动命令走下面 SaveOrder 的"本回合规划"通道，真正的位移在回合结算里执行。
			s.savePlannedOrder(w, r, room, roomID, player, request, order)
			return
		case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "LAUNCH_COVER", "LAUNCH_TRANSFER", "PREPARE_AIR", "RETURN_TO_HANGAR", "RECOVER_AIR", "PRIORITIZE_AIR_TASK", "SET_STRIKE_TARGET", "SET_STRIKE_DISPERSION", "ARM_AIR", "SET_AIR_CYCLE":
			if game.IsAirfieldOrder(room.State, order) {
				events = game.ApplyGroundAirOrder(&room.State, order)
			} else {
				events = game.ApplyAirOrder(&room.State, order)
			}
			if order.Type == "SET_STRIKE_DISPERSION" {
				log.Printf("DEBUG dispersion order %+v events=%d", order, len(events))
			}
		default:
			events = game.ApplyImmediateOrganization(&room.State, order)
		}
		response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID, "version": room.Version + 1})
		room, err = s.store.ApplyImmediate(r.Context(), room, player.ID, request.CommandID, fingerprint, string(response), room.State, events)
		if err != nil {
			writeError(w, http.StatusConflict, "命令结算失败："+err.Error())
			return
		}
		s.hub.broadcast(roomID, "state_changed", room.Version)
		w.Header().Set("Content-Type", "application/json")
		w.Write(response)
		return
	}
	response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID})
	if err := s.store.SaveOrder(r.Context(), roomID, room.Turn, player.ID, request.CommandID, order, string(response), fingerprint); err != nil {
		writeError(w, http.StatusInternalServerError, "保存命令失败")
		return
	}
	if player.Ready {
		_ = s.store.SetReady(r.Context(), player.ID, false)
	}
	s.hub.broadcast(roomID, "orders_updated", room.Version)
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

// pendingRadioTask 检查本回合是否已经有无线电命令发给了同一个编队。
// 幂等重试在外层 CommandResponse 就命中了，走不到这里，所以不必区分 command_id。
func pendingRadioTask(ctx context.Context, s *store.Store, roomID string, turn int, playerID, taskID string) bool {
	records, err := s.Orders(ctx, roomID, turn, playerID)
	if err != nil {
		return false
	}
	for _, record := range records {
		if record.Order.TaskID == taskID && game.IsRadioOrder(record.Order.Type) {
			return true
		}
	}
	return false
}

// savePlannedOrder 把一条待结算命令存进 orders 表（重复提交同一条移动不会重复记账）。
func (s *Server) savePlannedOrder(w http.ResponseWriter, r *http.Request, room store.Room, roomID string, player store.Player, request commandRequest, order game.Order) {
	response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID, "version": room.Version + 1})
	// 先推进版本（commands 表在这里写幂等响应），再落待结算命令；顺序反了会让幂等重试撞上主键冲突。
	room, err := s.store.BumpVersion(r.Context(), room, player.ID, request.CommandID, commandFingerprint(request), string(response))
	if err != nil {
		writeError(w, http.StatusConflict, "命令结算失败："+err.Error())
		return
	}
	if err := s.store.SaveOrder(r.Context(), roomID, room.Turn, player.ID, request.CommandID, order, string(response), commandFingerprint(request)); err != nil {
		writeError(w, http.StatusInternalServerError, "保存命令失败")
		return
	}
	s.hub.broadcast(roomID, "state_changed", room.Version)
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

// resolveRoomTurn 包一层 panic 兜底：回合结算里任何一处越界都不应该让整个房间卡死，
// 出错时记录日志并返回 nil（回合作废但不崩溃），便于快速定位。
func resolveRoomTurn(state *game.State, orders []game.Order) (events []game.EngineEvent) {
	defer func() {
		if recovered := recover(); recovered != nil {
			// 带上调用栈，否则"nil pointer dereference"这种错误完全无法定位。
			log.Printf("回合结算异常：%v\n%s", recovered, debug.Stack())
			events = nil
		}
	}()
	return game.ResolveOrders(state, orders)
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request, roomID string, player store.Player) {
	var request readyRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	room, err := s.store.Room(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "房间不存在")
		return
	}
	if room.Version != request.BaseVersion {
		writeError(w, http.StatusConflict, "状态已更新，请刷新后重试")
		return
	}
	if room.State.Outcome != nil && request.Ready {
		writeError(w, http.StatusBadRequest, "对局已经结束："+room.State.Outcome.Reason)
		return
	}
	// 双人对局不能一个人自己跟自己跑：对手没进房就直接拒绝 ready，
	// 而不是"单人 ready 就结算"把十几个回合跑掉（那样回合、油量、侦察全白给）。
	if request.Ready && !room.Solo {
		players, err := s.store.Players(r.Context(), roomID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "读取玩家失败")
			return
		}
		if len(players) < 2 {
			writeError(w, http.StatusBadRequest, "这是双人对局，对手还没进房：把房间 token 发给他，两名玩家都到齐后回合才会推进")
			return
		}
	}
	if err := s.store.SetReady(r.Context(), player.ID, request.Ready); err != nil {
		writeError(w, http.StatusInternalServerError, "准备状态保存失败")
		return
	}
	resolved := false
	if request.Ready {
		players, _ := s.store.Players(r.Context(), roomID)
		allReady := len(players) > 0
		for _, item := range players {
			allReady = allReady && item.Ready
		}
		if allReady {
			records, err := s.store.Orders(r.Context(), roomID, room.Turn, "")
			if err != nil {
				writeError(w, http.StatusInternalServerError, "读取命令失败")
				return
			}
			orders := make([]game.Order, 0, len(records))
			for _, record := range records {
				orders = append(orders, record.Order)
			}
			// 引擎内部的回合号与服务端的房间回合保持一致，胜利条件的回合上限要用它。
			room.State.Turn = room.Turn
			events := resolveRoomTurn(&room.State, orders)
			room.State.Turn = room.Turn + 1
			room, err = s.store.Resolve(r.Context(), room, room.State, events)
			if err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			resolved = true
		}
	}
	if resolved {
		s.hub.broadcast(roomID, "turn_resolved", room.Version)
	} else {
		s.hub.broadcast(roomID, "ready_changed", room.Version)
	}
	s.writeState(w, r, roomID, player)
}

func (s *Server) writeState(w http.ResponseWriter, r *http.Request, roomID string, player store.Player) {
	room, err := s.store.Room(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "房间不存在")
		return
	}
	players, err := s.store.Players(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取玩家失败")
		return
	}
	orders, err := s.store.Orders(r.Context(), roomID, room.Turn, player.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取命令失败")
		return
	}
	events, err := s.store.Events(r.Context(), roomID, player.Faction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取战报失败")
		return
	}
	for _, item := range players {
		if item.ID == player.ID {
			player = item
		}
	}
	ownOrders := make([]game.Order, 0, len(orders))
	for _, record := range orders {
		ownOrders = append(ownOrders, record.Order)
	}
	response := map[string]any{
		"room_id": room.ID, "status": room.Status, "turn": room.Turn, "version": room.Version,
		"scenario": map[string]any{"id": room.State.ScenarioID, "name": room.State.ScenarioName, "description": room.State.Description},
		"map":      room.State.Map, "units": game.VisibleUnits(room.State, player.Faction), "fleets": game.VisibleFleets(room.State, player.Faction),
		"aviation":        game.VisibleAviation(room.State, player.Faction),
		"airfields":       game.VisibleAirfields(room.State, player.Faction),
		"ground_air":      game.VisibleGroundAir(room.State, player.Faction),
		"ground_tasks":    game.GroundAviationView(room.State, player.Faction).Tasks,
		"ground_aircraft": game.GroundAircraftView(room.State, player.Faction),
		"air_combats":     game.VisibleAirCombats(room.State, player.Faction),
		"fleet_stays":     game.VisibleFleetStays(room.State, player.Faction),
		"fleet_sessions":  game.VisibleFleetSessions(room.State, player.Faction),
		"shore_supplies":  game.VisibleShoreSupplies(room.State, player.Faction),
		"radio_used":      game.VisibleRadioUsage(room.State, player.Faction),
		"radio_tasks":     game.AirborneRadioTasks(room.State, player.Faction),
		"wind":            game.VisibleWind(room.State, player.Faction),
		"night":           room.State.Night,
		"clock":           game.Clock(room.State),
		"enemy_air":       game.VisibleEnemyAir(room.State, player.Faction),
		"losses":          game.VisibleLosses(room.State, player.Faction),
		"outcome":         room.State.Outcome,
		"objectives":      room.State.Objectives,
		"you": player, "players": players, "orders": ownOrders, "events": events,
		// solo 让前端把"完成部署"换成"等待对手进房"，别让玩家点了才发现结算不了。
		"solo": room.Solo,
		"server_time": time.Now().UTC(),
	}
	if player.IsHost {
		response["join_token"] = formatToken(room.JoinToken)
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) writeEvents(w http.ResponseWriter, r *http.Request, roomID string, player store.Player) {
	events, err := s.store.Events(r.Context(), roomID, player.Faction)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取战报失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (store.Player, bool) {
	cookie, err := r.Cookie("gq_session")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "请先创建或加入房间")
		return store.Player{}, false
	}
	player, err := s.store.PlayerBySession(r.Context(), hashToken(cookie.Value))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "会话已失效，请重新加入")
		return store.Player{}, false
	}
	return player, true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, session string) {
	http.SetCookie(w, &http.Cookie{Name: "gq_session", Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: s.secure, MaxAge: 30 * 24 * 3600})
}

func (s *Server) submitRoom(roomID string, run func()) {
	value, _ := s.actors.LoadOrStore(roomID, newRoomActor())
	actor := value.(*roomActor)
	actor.start.Do(func() { go actor.run() })
	job := roomJob{run: run, done: make(chan struct{})}
	actor.jobs <- job
	<-job.done
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求JSON无效")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func randomUint64() (uint64, error) {
	value, err := randomHex(8)
	if err != nil {
		return 0, err
	}
	var result uint64
	for _, char := range value {
		result = result*16 + uint64(strings.IndexRune("0123456789abcdef", char))
	}
	return result, nil
}

func randomToken(length int) (string, error) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
	result := make([]byte, length)
	data := make([]byte, length)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	for index := range result {
		result[index] = alphabet[int(data[index])%len(alphabet)]
	}
	return string(result), nil
}

func formatToken(token string) string {
	if len(token) != 12 {
		return token
	}
	return token[:4] + "-" + token[4:8] + "-" + token[8:]
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

type hub struct {
	mu      sync.RWMutex
	clients map[string]map[*wsClient]bool
}

type wsClient struct {
	conn     *websocket.Conn
	send     chan []byte
	roomID   string
	playerID string
}

func newHub() *hub { return &hub{clients: map[string]map[*wsClient]bool{}} }

func (h *hub) add(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.roomID] == nil {
		h.clients[client.roomID] = map[*wsClient]bool{}
	}
	h.clients[client.roomID][client] = true
}

func (h *hub) remove(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[client.roomID], client)
	if len(h.clients[client.roomID]) == 0 {
		delete(h.clients, client.roomID)
	}
}

func (h *hub) broadcast(roomID, eventType string, version int64) {
	message, _ := json.Marshal(map[string]any{"type": eventType, "version": version, "time": time.Now().UTC()})
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[roomID] {
		select {
		case client.send <- message:
		default:
		}
	}
}

func (s *Server) webSocket(w http.ResponseWriter, r *http.Request) {
	player, ok := s.authenticate(w, r)
	if !ok {
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		parsed, err := url.Parse(origin)
		return err == nil && strings.EqualFold(parsed.Host, r.Host)
	}}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	client := &wsClient{conn: conn, send: make(chan []byte, 32), roomID: player.RoomID, playerID: player.ID}
	s.hub.add(client)
	s.submitRoom(player.RoomID, func() {
		_ = s.store.SetConnected(context.Background(), player.ID, true)
	})
	s.hub.broadcast(player.RoomID, "presence_changed", 0)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		s.hub.remove(client)
		s.submitRoom(player.RoomID, func() {
			_ = s.store.SetConnected(context.Background(), player.ID, false)
		})
		s.hub.broadcast(player.RoomID, "presence_changed", 0)
		conn.Close()
	}()
	for {
		select {
		case message := <-client.send:
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

var errNotFound = errors.New("not found")
