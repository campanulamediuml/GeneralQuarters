package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
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
	// views 是这个房间上一次推给每个玩家、以及观战者的视图。
	// 只由 worker 自己读写（所有变更和推送都在同一个 goroutine 里先后发生），
	// 所以不需要锁：这也是补丁顺序天然正确的原因。
	views roomViews
}

// roomViews 按玩家 / 观战分别缓存上一份推出去的视图，用来算下一次的补丁。
// 同一玩家开两个窗口共享一份：补丁相对"他这一侧上次看到的"，而不是相对每条连接。
type roomViews struct {
	players  map[string]map[string]any
	spectate map[string]any
}

func newRoomActor() *roomActor {
	return &roomActor{jobs: make(chan roomJob, 128), views: roomViews{players: map[string]map[string]any{}}}
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
	CommandID string `json:"command_id"`
	// BaseTurn：客户端提交这条命令时看到的回合号。只用来拒绝"回合已经结算"的过期提交；
	// 同回合内的重复操作一律接受（后写的覆盖先写的），不需要也不应该比对版本号。
	// 0 表示没带（老客户端/脚本），不做回合校验。
	BaseTurn      int               `json:"base_turn"`
	Type          string            `json:"type"`
	UnitID        string            `json:"unit_id"`
	TargetUnitID  string            `json:"target_unit_id"`
	AircraftID    string            `json:"aircraft_id"`
	AircraftIDs   []string          `json:"aircraft_ids"`
	TaskID        string            `json:"task_id"`
	Coord         string            `json:"coord"`
	TargetFleetID string            `json:"target_fleet_id"`
	Heading       string            `json:"heading"`
	SectorWidth   int               `json:"sector_width"`
	Assignments   map[string]string `json:"assignments"`
	TaskKind      string            `json:"task_kind"`
	Loadout       string            `json:"loadout"`
	// Disperse：SET_STRIKE_DISPERSION 用（true=分散攻击）。
	Disperse bool `json:"disperse"`
	// Cycle：SET_AIR_CYCLE 用（true=开启自动任务循环，false=取消）。
	Cycle bool `json:"cycle"`
	// Engagement：SET_AUTO_NAV / SET_ENGAGEMENT 用（aggressive/advantage/avoid）。
	Engagement string `json:"engagement"`
}

type readyRequest struct {
	// BaseTurn 见 commandRequest：只拦"回合已结算"的过期 ready，不比对版本号。
	BaseTurn int  `json:"base_turn"`
	Ready    bool `json:"ready"`
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
	mux.HandleFunc("GET /watch/", s.watchPage)
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
	// 观战接口不走玩家 Cookie：它用房间 join_token 认证，返回合并后的双方视图。
	if r.Method == http.MethodGet && action == "spectate" {
		s.submitRoom(roomID, func() { s.spectate(w, r, roomID) })
		return
	}
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
		request.ScenarioID = "_coral_real"
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
	// 单人房必须显式声明；正式剧本默认双人对局。
	room := store.Room{ID: roomID, JoinToken: joinToken, Status: "running", Turn: 1, Version: 1, State: state, Solo: request.Solo}
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
	s.publishRoom(player.RoomID)
	writeJSON(w, http.StatusOK, roomResponse{RoomID: player.RoomID, PlayerID: player.ID})
}

// commandFingerprint 把一条命令的可识别字段拼成指纹：用来区分"同一命令重试"和
// "复用 command_id 的另一条命令"。只取业务字段，不含 command_id / base_turn。
func commandFingerprint(request commandRequest) string {
	parts := []string{
		request.Type, request.UnitID, request.TargetUnitID, request.AircraftID,
		strings.Join(request.AircraftIDs, ","), request.TaskID, request.Coord, request.TargetFleetID, request.Heading,
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
	// 就绪即冻结：点"完成部署"那一刻本回合的操作就锁死了，之后服务端拒绝任何修改。
	// 以前这里会自动把 ready 撤回再接受命令，等于"准备"完全没有约束力：
	// 玩家点了准备还能继续改，对手以为局面定了，结算出来的却是改过的版本。
	if player.Ready {
		writeError(w, http.StatusConflict, "本回合已就绪，操作已锁定：双方就绪后由服务器统一结算，下一回合才能再操作")
		return
	}
	// 同回合内的操作本来就该互相覆盖：订单按 (room, turn, player, unit) 落库，后写的顶掉先写的。
	// 唯一必须拒绝的是"回合已经结算完"——那说明这条命令是基于上一回合的战场做的决定，
	// 落到新回合会错误地执行（比如把已经开出去的舰再挪一次）。
	if request.BaseTurn > 0 && request.BaseTurn != room.Turn {
		writeError(w, http.StatusConflict, "本回合已经结算：请基于当前回合重新操作")
		return
	}
	order := game.Order{Type: request.Type, UnitID: request.UnitID, TargetUnitID: request.TargetUnitID, AircraftID: request.AircraftID, AircraftIDs: request.AircraftIDs, TaskID: request.TaskID, Coord: game.NormalizeToken(request.Coord), TargetFleetID: request.TargetFleetID, Heading: strings.ToUpper(request.Heading), SectorWidth: request.SectorWidth, Assignments: request.Assignments, TaskKind: strings.ToLower(request.TaskKind), Loadout: strings.ToLower(request.Loadout), Disperse: request.Disperse, Engagement: strings.ToLower(request.Engagement), Cycle: request.Cycle}
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
		response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID, "turn": room.Turn})
		room, err = s.store.ApplyImmediate(r.Context(), room, player.ID, request.CommandID, fingerprint, string(response), room.State, events)
		if err != nil {
			writeError(w, http.StatusConflict, "命令结算失败："+err.Error())
			return
		}
		s.publishRoom(roomID)
		w.Header().Set("Content-Type", "application/json")
		w.Write(response)
		return
	}
	response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID})
	if err := s.store.SaveOrder(r.Context(), roomID, room.Turn, player.ID, request.CommandID, order, string(response), fingerprint); err != nil {
		writeError(w, http.StatusInternalServerError, "保存命令失败")
		return
	}
	s.publishRoom(roomID)
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
	response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID, "turn": room.Turn})
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
	s.publishRoom(roomID)
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

// StartTurnReconciler 周期巡检"所有玩家都已 ready、房间却没动"的对局。
// 重启会把 ready 标记留在库里，但不会再有新的 ready 请求触发结算，
// handleReady 里的结算逻辑永远等不到第二次触发——T108 卡住就是这个原因。
func (s *Server) StartTurnReconciler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.settleStalledRooms(ctx)
			}
		}
	}()
}

// stalledReadyAfter 是允许的静止时间：正常结算就在 ready 请求里完成，
// 超过这个时间还没推进，说明触发结算的请求已经丢了。
const stalledReadyAfter = 45 * time.Second

func (s *Server) settleStalledRooms(ctx context.Context) {
	rooms, err := s.store.StalledReadyRooms(ctx, stalledReadyAfter)
	if err != nil {
		return
	}
	for _, room := range rooms {
		roomID := room.ID
		s.submitRoom(roomID, func() { s.resolveStalledRoom(ctx, roomID) })
	}
}

func (s *Server) resolveStalledRoom(ctx context.Context, roomID string) {
	room, err := s.store.Room(ctx, roomID)
	if err != nil || room.Status != "running" || room.State.Outcome != nil {
		return
	}
	players, err := s.store.Players(ctx, roomID)
	if err != nil || len(players) == 0 || (!room.Solo && len(players) < 2) {
		return
	}
	for _, player := range players {
		if !player.Ready {
			return
		}
	}
	records, err := s.store.Orders(ctx, roomID, room.Turn, "")
	if err != nil {
		return
	}
	orders := make([]game.Order, 0, len(records))
	for _, record := range records {
		orders = append(orders, record.Order)
	}
	room.State.Turn = room.Turn
	events := resolveRoomTurn(&room.State, orders)
	room.State.Turn = room.Turn + 1
	if _, err := s.store.Resolve(ctx, room, room.State, events); err != nil {
		return
	}
	log.Printf("房间 %s 双方已 ready 但停在第 %d 回合，巡检已自动结算", roomID, room.Turn)
	s.publishRoom(roomID)
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
	if request.BaseTurn > 0 && request.BaseTurn != room.Turn {
		writeError(w, http.StatusConflict, "本回合已经结算：请基于当前回合重新操作")
		return
	}
	// 就绪必须不可逆：ready 之后不能再改主意，也不能用 ready:false 解锁。
	// 需要改部署就等本回合结算。前端已经没有撤销按钮，脚本也必须遵守同一规则。
	if !request.Ready {
		writeError(w, http.StatusBadRequest, "ready 一旦提交不可撤销；请等待本回合结算")
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
		}
	}
	s.publishRoom(roomID)
	// ready 的响应只是一条回执：状态由推送下发，不用为改一个标记回 150 KB 全量。
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "turn": room.Turn})
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
	view, err := s.privateView(r.Context(), room, players, player)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// privateView 生成某个玩家的阵营私有视图。HTTP/WS 的 state 请求和服务端主动推送
// 共用这一条路径：两边投影不一致的话，玩家会看到"推一版、拉一版"的界面抖。
func (s *Server) privateView(ctx context.Context, room store.Room, players []store.Player, player store.Player) (map[string]any, error) {
	orders, err := s.store.Orders(ctx, room.ID, room.Turn, player.ID)
	if err != nil {
		return nil, fmt.Errorf("读取命令失败")
	}
	events, err := s.store.Events(ctx, room.ID, player.Faction)
	if err != nil {
		return nil, fmt.Errorf("读取战报失败")
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
		"room_id": room.ID, "status": room.Status, "turn": room.Turn,
		"scenario": map[string]any{"id": room.State.ScenarioID, "name": room.State.ScenarioName, "description": room.State.Description},
		"map":      room.State.Map, "units": game.VisibleUnits(room.State, player.Faction), "fleets": game.VisibleFleets(room.State, player.Faction),
		"aviation":             game.VisibleAviation(room.State, player.Faction),
		"airfields":            game.VisibleAirfields(room.State, player.Faction),
		"ground_air":           game.VisibleGroundAir(room.State, player.Faction),
		"ground_tasks":         game.GroundAviationView(room.State, player.Faction).Tasks,
		"ground_aircraft":      game.GroundAircraftView(room.State, player.Faction),
		"air_combats":          game.VisibleAirCombats(room.State, player.Faction),
		"fleet_stays":          game.VisibleFleetStays(room.State, player.Faction),
		"fleet_sessions":       game.VisibleFleetSessions(room.State, player.Faction),
		"shore_supplies":       game.VisibleShoreSupplies(room.State, player.Faction),
		"enemy_shore_supplies": game.VisibleEnemyShoreSupplies(room.State, player.Faction),
		"radio_used":           game.VisibleRadioUsage(room.State, player.Faction),
		"radio_tasks":          game.AirborneRadioTasks(room.State, player.Faction),
		"wind":                 game.VisibleWind(room.State, player.Faction),
		"night":                room.State.Night,
		"clock":                game.Clock(room.State),
		"enemy_air":            game.VisibleEnemyAir(room.State, player.Faction),
		"losses":               game.VisibleLosses(room.State, player.Faction),
		// scores 是战果分：击沉/击落 + 已完成的投送目标；剧本用 score 目标时按它判定。
		"scores":     game.VictoryPoints(room.State),
		"outcome":    room.State.Outcome,
		"objectives": room.State.Objectives,
		"you":        player, "players": players, "orders": ownOrders, "events": events,
		// solo 让前端把"完成部署"换成"等待对手进房"，别让玩家点了才发现结算不了。
		"solo": room.Solo,
		// 这里不要放 server_time 之类每次都变的字段：推送靠"和上一帧逐字节相同就不发"
		// 去掉空转，混进一个时间戳，判重就永远命中不了，等于白算一遍。
	}
	if player.IsHost {
		response["join_token"] = formatToken(room.JoinToken)
	}
	return response, nil
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

// roomActor 取（必要时创建）房间的 worker。房间状态只由它的 goroutine 改。
func (s *Server) roomActor(roomID string) *roomActor {
	value, _ := s.actors.LoadOrStore(roomID, newRoomActor())
	actor := value.(*roomActor)
	actor.start.Do(func() { go actor.run() })
	return actor
}

// publishRoom 在房间 worker 里调用：把这次变更投影成每个玩家 / 观战者的可见视图，
// 和上一次推出去的那份做 diff，只把变了的部分发出去。
//
// 必须在 worker 里跑——它读权威状态、写每个玩家/观战者的上一版缓存，和命令处理
// 在同一个 goroutine 里前后发生，所以"先入队的全量、后入队的补丁"这个顺序天然成立，
// 不需要版本号、不需要合并窗口、也不需要在推送侧再协调一次时序。
func (s *Server) publishRoom(roomID string) {
	ctx := context.Background()
	room, err := s.store.Room(ctx, roomID)
	if err != nil {
		return
	}
	players, err := s.store.Players(ctx, roomID)
	if err != nil {
		return
	}
	actor := s.roomActor(roomID)
	for _, player := range players {
		view, err := s.privateView(ctx, room, players, player)
		if err != nil {
			continue
		}
		previous, exists := actor.views.players[player.ID]
		if !exists {
			// 这个玩家还没有底座（客户端刚连上、还没拉全量）。那一拉会把它记下来。
			continue
		}
		if patch := diffView(previous, view); patch != any(patchNone) {
			if frame, err := json.Marshal(map[string]any{"type": "state", "patch": patch}); err == nil {
				s.hub.deliverPlayer(roomID, player.ID, frame)
			}
		}
		actor.views.players[player.ID] = view
	}
	view, err := s.spectateView(ctx, room)
	if err != nil {
		return
	}
	if previous := actor.views.spectate; previous != nil {
		if patch := diffView(previous, view); patch != any(patchNone) {
			if frame, err := json.Marshal(map[string]any{"type": "snapshot", "patch": patch}); err == nil {
				s.hub.deliverSpectators(roomID, frame)
			}
		}
	}
	actor.views.spectate = view
}

func (s *Server) submitRoom(roomID string, run func()) {
	actor := s.roomActor(roomID)
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

// wsClient 是一条连接的全部状态：一个出口 chan、一个身份。
// readLoop 只管读，writeLoop 只管写，中间只有这个 chan。
type wsClient struct {
	conn *websocket.Conn
	// send 里放要写到这条连接的帧：RPC 响应、补丁、错误。
	// 顺序由房间 worker 的先后决定；写失败或塞不进去就断开这条连接，让他重连拉全量。
	send      chan []byte
	roomID    string
	playerID  string
	spectator bool
}

type wsRequest struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type wsResponse struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
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

// deliver 把一帧丢给房间里符合条件的连接。
//
// 塞不进去说明这条连接已经跟不上（对端不读了），直接断开：它重连时会重新拉一份
// 全量，比让它收一半补丁、拿着错的底座继续跑要好。绝不能在这里阻塞——worker
// 一阻塞，整个房间（另一个玩家的命令、结算、巡检）就全停了。
func (h *hub) deliver(roomID string, match func(*wsClient) bool, frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[roomID] {
		if !match(client) {
			continue
		}
		select {
		case client.send <- frame:
		default:
			_ = client.conn.Close()
		}
	}
}

// deliverPlayer 只给某个玩家的连接；deliverSpectators 只给观战连接。
func (h *hub) deliverPlayer(roomID, playerID string, frame []byte) {
	h.deliver(roomID, func(client *wsClient) bool {
		return !client.spectator && client.playerID == playerID
	}, frame)
}

func (h *hub) deliverSpectators(roomID string, frame []byte) {
	h.deliver(roomID, func(client *wsClient) bool { return client.spectator }, frame)
}

func (s *Server) dispatchWebSocketRPC(client *wsClient, handshake *http.Request, player store.Player, request wsRequest) wsResponse {
	response := wsResponse{Type: "rpc_response", ID: request.ID, Status: http.StatusBadRequest}
	if request.ID == "" || len(request.ID) > 128 || request.Method == "" || len(request.Path) > 512 || len(request.Body) > 1<<20 {
		response.Body = json.RawMessage(`{"error":"无效的 WebSocket 请求"}`)
		return response
	}
	parsed, err := url.ParseRequestURI(request.Path)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		response.Body = json.RawMessage(`{"error":"不允许的 WebSocket 路径"}`)
		return response
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "rooms" || parts[3] != player.RoomID {
		response.Status = http.StatusForbidden
		response.Body = json.RawMessage(`{"error":"WebSocket 请求必须属于当前房间"}`)
		return response
	}
	action := parts[4]
	// GET state 顺带把这份视图记成这条连接的底座：客户端重连时只拉这一次全量，
	// 之后服务端推的都是"和它相比变了什么"。用同一份投影，免得两个来源对不上。
	if request.Method == http.MethodGet && action == "state" {
		view, err := s.stateViewOf(player.RoomID, player.ID)
		if err != nil {
			return wsResponse{Type: "rpc_response", ID: request.ID, Status: http.StatusInternalServerError, Body: json.RawMessage(`{"error":"读取状态失败"}`)}
		}
		encoded, err := json.Marshal(view)
		if err != nil {
			return wsResponse{Type: "rpc_response", ID: request.ID, Status: http.StatusInternalServerError, Body: json.RawMessage(`{"error":"状态编码失败"}`)}
		}
		return wsResponse{Type: "rpc_response", ID: request.ID, Status: http.StatusOK, Body: encoded}
	}
	allowed := (request.Method == http.MethodGet && action == "events") || (request.Method == http.MethodPost && (action == "commands" || action == "ready"))
	if !allowed {
		response.Status = http.StatusForbidden
		response.Body = json.RawMessage(`{"error":"不允许通过 WebSocket 调用此接口"}`)
		return response
	}
	clone := handshake.Clone(handshake.Context())
	clone.Method = request.Method
	clone.URL = parsed
	clone.Body = io.NopCloser(bytes.NewReader(request.Body))
	clone.ContentLength = int64(len(request.Body))
	clone.Header = handshake.Header.Clone()
	clone.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	s.roomRoutes(recorder, clone)
	response.Status = recorder.Code
	if response.Status == 0 {
		response.Status = http.StatusOK
	}
	body := bytes.TrimSpace(recorder.Body.Bytes())
	if len(body) == 0 {
		response.Status = http.StatusInternalServerError
		body = []byte(`{"error":"房间处理未返回响应"}`)
	}
	response.Body = append(json.RawMessage(nil), body...)
	return response
}

func (s *Server) webSocket(w http.ResponseWriter, r *http.Request) {
	player, playerAuthed := s.webSocketPlayer(r)
	if r.URL.Query().Get("mode") == "spectator" {
		playerAuthed = false
	}
	if playerAuthed && r.URL.Query().Get("room_id") != "" && r.URL.Query().Get("room_id") != player.RoomID {
		http.Error(w, "当前会话不属于此房间", http.StatusForbidden)
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
	conn.SetReadLimit(1 << 20)
	client := &wsClient{conn: conn, send: make(chan []byte, 32)}
	if playerAuthed {
		client.roomID, client.playerID = player.RoomID, player.ID
	} else {
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		// 无 Cookie 连接有两种意图：观战（第一条是 subscribe）和玩家会话过期
		// （客户端不假思索先发 rpc_request）。以前后者会被当成"观战订阅无效"直接关掉，
		// 前端只能看到断线重连，永远不知道该回首页重新加入。
		var first struct {
			Type   string `json:"type"`
			ID     string `json:"id"`
			RoomID string `json:"room_id"`
			Token  string `json:"token"`
		}
		if err := conn.ReadJSON(&first); err != nil {
			_ = conn.Close()
			return
		}
		if first.Type == "rpc_request" {
			body := json.RawMessage(`{"error":"请先创建或加入房间"}`)
			_ = conn.WriteJSON(wsResponse{Type: "rpc_response", ID: first.ID, Status: http.StatusUnauthorized, Body: body})
			_ = conn.Close()
			return
		}
		if first.Type != "subscribe" || first.RoomID == "" || len(first.RoomID) > 128 {
			_ = conn.WriteJSON(map[string]any{"type": "error", "error": "观战订阅请求无效"})
			_ = conn.Close()
			return
		}
		room, err := s.store.Room(r.Context(), first.RoomID)
		if err != nil || game.NormalizeToken(first.Token) != room.JoinToken {
			_ = conn.WriteJSON(map[string]any{"type": "error", "error": "观战 token 不正确或房间不存在"})
			_ = conn.Close()
			return
		}
		// token 只在这一步校验一次：连接建立之后它就是这条订阅的身份，
		// 后续快照由写泵自己按房间投影，不必每次都带着 token 重走一遍 HTTP 鉴权。
		client.roomID, client.spectator = first.RoomID, true
	}
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	s.hub.add(client)
	if !client.spectator {
		// 玩家：在线状态变了要推给对手；自己的底座等他发 GET state 时再记。
		s.submitRoom(client.roomID, func() {
			_ = s.store.SetConnected(context.Background(), client.playerID, true)
			s.publishRoom(client.roomID)
		})
	} else if err := s.sendSpectatorFull(client); err != nil {
		s.hub.remove(client)
		_ = conn.WriteJSON(map[string]any{"type": "error", "error": err.Error()})
		_ = conn.Close()
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if client.spectator || messageType != websocket.TextMessage {
				continue
			}
			var request wsRequest
			if err := json.Unmarshal(payload, &request); err != nil || request.Type != "rpc_request" {
				continue
			}
			response := s.dispatchWebSocketRPC(client, r, player, request)
			encoded, _ := json.Marshal(response)
			select {
			case client.send <- encoded:
			default:
				return
			}
		}
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		s.hub.remove(client)
		if !client.spectator {
			s.submitRoom(client.roomID, func() {
				_ = s.store.SetConnected(context.Background(), client.playerID, false)
				s.publishRoom(client.roomID)
			})
		}
		_ = conn.Close()
	}()
	// writeLoop 只做一件事：把 chan 里的帧按序写出去。
	// 写失败、连接断了，就直接返回——客户端会重连并重新拉一份全量，
	// 所以这里不需要任何"跳过这一帧继续"的分支。
	for {
		select {
		case message := <-client.send:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

// sendSpectatorFull 给一个新接入的观战连接推一份完整快照，并把它记成观战的底座。
// 观战不走 RPC 拉取，所以首帧必须由服务端给。
func (s *Server) sendSpectatorFull(client *wsClient) error {
	var failure error
	s.submitRoom(client.roomID, func() {
		ctx := context.Background()
		room, err := s.store.Room(ctx, client.roomID)
		if err != nil {
			failure = err
			return
		}
		view, err := s.spectateView(ctx, room)
		if err != nil {
			failure = err
			return
		}
		frame, err := json.Marshal(map[string]any{"type": "snapshot", "view": view})
		if err != nil {
			failure = err
			return
		}
		s.roomActor(client.roomID).views.spectate = view
		select {
		case client.send <- frame:
		default:
			_ = client.conn.Close()
		}
	})
	return failure
}

// stateViewOf 投影某个玩家的私有视图。
//
// 必须走房间 actor：房间、玩家、命令、战报要在同一个时间点上读，否则推出去
// 的内容会和权威状态对不上。玩家按 ID 现查，不用连接上缓存的记录——
// 改名、换阵营之后不该继续按老记录投影。
func (s *Server) stateViewOf(roomID, playerID string) (map[string]any, error) {
	var view map[string]any
	var failure error
	s.submitRoom(roomID, func() {
		ctx := context.Background()
		room, err := s.store.Room(ctx, roomID)
		if err != nil {
			failure = err
			return
		}
		players, err := s.store.Players(ctx, roomID)
		if err != nil {
			failure = err
			return
		}
		var player store.Player
		found := false
		for _, item := range players {
			if item.ID == playerID {
				player, found = item, true
			}
		}
		if !found {
			failure = errNotFound
			return
		}
		projected, err := s.privateView(ctx, room, players, player)
		if err != nil {
			failure = err
			return
		}
		view = projected
		// 这份全量就是这名玩家这一侧的底座，之后的变更相对它算补丁。
		// 和响应入队在同一个任务里先后发生，所以补丁不可能排在它前面。
		s.roomActor(roomID).views.players[playerID] = projected
	})
	if failure != nil {
		return nil, failure
	}
	return view, nil
}

func (s *Server) webSocketPlayer(r *http.Request) (store.Player, bool) {
	cookie, err := r.Cookie("gq_session")
	if err != nil {
		return store.Player{}, false
	}
	player, err := s.store.PlayerBySession(r.Context(), hashToken(cookie.Value))
	return player, err == nil
}

var errNotFound = errors.New("not found")
