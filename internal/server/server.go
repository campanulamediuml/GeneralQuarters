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
	ReconRadius  int               `json:"recon_radius"`
	Assignments  map[string]string `json:"assignments"`
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
	room := store.Room{ID: roomID, JoinToken: joinToken, Status: "running", Turn: 1, Version: 1, State: state}
	player := store.Player{ID: playerID, RoomID: roomID, Faction: "A", Name: name, IsHost: true, Ready: false, Connected: true}
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
	if response, found, err := s.store.CommandResponse(r.Context(), player.ID, request.CommandID); err == nil && found {
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
	order := game.Order{Type: request.Type, UnitID: request.UnitID, TargetUnitID: request.TargetUnitID, AircraftID: request.AircraftID, AircraftIDs: request.AircraftIDs, TaskID: request.TaskID, Coord: game.NormalizeToken(request.Coord), Heading: strings.ToUpper(request.Heading), SectorWidth: request.SectorWidth, ReconRadius: request.ReconRadius, Assignments: request.Assignments}
	if err := game.ValidateOrder(room.State, player.Faction, order); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if order.Type == "SET_MOVE" || order.Type == "MERGE_FLEET" || order.Type == "REORGANIZE_HEX" || order.Type == "LAUNCH_CAP" || order.Type == "LAUNCH_RECON" || order.Type == "LAUNCH_STRIKE" || order.Type == "PREPARE_AIR" || order.Type == "RETURN_TO_HANGAR" || order.Type == "RECOVER_AIR" || order.Type == "PRIORITIZE_AIR_TASK" || order.Type == "SET_STRIKE_TARGET" {
		var events []game.EngineEvent
		switch order.Type {
		case "SET_MOVE":
			events = game.ApplyImmediateMove(&room.State, order)
		case "LAUNCH_CAP", "LAUNCH_RECON", "LAUNCH_STRIKE", "PREPARE_AIR", "RETURN_TO_HANGAR", "RECOVER_AIR", "PRIORITIZE_AIR_TASK", "SET_STRIKE_TARGET":
			events = game.ApplyAirOrder(&room.State, order)
		default:
			events = game.ApplyImmediateOrganization(&room.State, order)
		}
		response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID, "version": room.Version + 1})
		room, err = s.store.ApplyImmediate(r.Context(), room, player.ID, request.CommandID, string(response), room.State, events)
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
	if err := s.store.SaveOrder(r.Context(), roomID, room.Turn, player.ID, request.CommandID, order, string(response)); err != nil {
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
			events := game.ResolveOrders(&room.State, orders)
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
		"aviation": game.VisibleAviation(room.State, player.Faction),
		"you":      player, "players": players, "orders": ownOrders, "events": events,
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
