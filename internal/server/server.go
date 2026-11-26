package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

// roomKind 是房间 worker 能受理的消息种类。
//
// 房间的规则是串行的：一条消息读到的就是上一条消息留下的状态，胜负、油量、
// 侦察、待结算命令都在同一个 goroutine 里一环扣一环地变。所以这里不需要
// 版本号、锁、合并窗口——需要的只是"按到达顺序一条一条处理"，
// 以及一份只有 worker 自己能碰的视图缓存（补丁的顺序就是这么来的）。
type roomKind int

const (
	msgState         roomKind = iota // 拉全量：同时记成这名玩家这一侧的底座
	msgEvents                        // 拉战报
	msgCommand                       // 提交命令
	msgReady                         // 完成部署
	msgSpectate                      // 观战视图（join_token 认证）
	msgJoin                          // 加入房间
	msgConnect                       // 连接上线 / 离线
	msgSpectatorJoin                 // 观战连接接入：推首帧全量
	msgSettle                        // 巡检补结算
)

func (kind roomKind) String() string {
	switch kind {
	case msgState:
		return "state"
	case msgEvents:
		return "events"
	case msgCommand:
		return "command"
	case msgReady:
		return "ready"
	case msgSpectate:
		return "spectate"
	case msgJoin:
		return "join"
	case msgConnect:
		return "connect"
	case msgSpectatorJoin:
		return "spectator"
	case msgSettle:
		return "settle"
	}
	return "unknown"
}

// roomMsg 是一条房间消息：它带够处理这条消息所需的全部输入，worker 按 kind 分派。
// 会产出应答的几种把结果送回 reply（HTTP）或直接入到发起连接的 send（WebSocket）——
// 两者都由 worker 自己做，所以"先入队的全量、后入队的补丁"这个顺序天然成立。
type roomMsg struct {
	kind   roomKind
	roomID string
	// player 是这条消息的身份（已经过 HTTP / WebSocket 鉴权）。
	player store.Player
	// body 是 POST 的请求体原文；命令和 ready 用。
	body []byte
	// token / name / playerID / session 是加入房间用的：加入也必须串行，
	// 否则两个人能同时挤进一个满员房。
	token    string
	name     string
	playerID string
	session  string
	// connected 是这条消息要求玩家变成上线（true）还是离线。
	connected bool
	// client + rpcID：WebSocket 请求。应答由 worker 写进这条连接的 send。
	client *wsClient
	rpcID  string
	// reply：HTTP 请求。worker 处理完把结果放进来就关闭它。
	reply chan roomReply
}

// roomReply 是一条房间消息的结果：状态码 + 已经编码好的 JSON。
type roomReply struct {
	status int
	body   []byte
	// session 只在加入房间时用：HTTP 侧要拿它去种 Cookie。
	session string
}

// wantsReply 说明这条消息要不要给发起方一个答复。
// 连接上下线、观战首帧、巡检结算都是单向的：它们的结果本身就是推送。
func (msg roomMsg) wantsReply() bool {
	return msg.reply != nil || (msg.client != nil && msg.rpcID != "")
}

// answer 把结果送回发起方。只有 worker 会调用它。
func (msg roomMsg) answer(reply roomReply) {
	if msg.reply != nil {
		msg.reply <- reply
		close(msg.reply)
		return
	}
	if msg.client == nil || msg.rpcID == "" {
		return
	}
	frame, err := json.Marshal(wsResponse{Type: "rpc_response", ID: msg.rpcID, Status: reply.status, Body: reply.body})
	if err != nil {
		return
	}
	if !msg.client.enqueue(frame) {
		_ = msg.client.conn.Close()
	}
}

// roomJSON / roomError 是房间消息的两种应答。
func roomJSON(status int, value any) roomReply {
	body, err := json.Marshal(value)
	if err != nil {
		return roomReply{status: http.StatusInternalServerError, body: []byte("{\"error\":\"响应编码失败\"}")}
	}
	return roomReply{status: status, body: body}
}

func roomError(status int, message string) roomReply {
	return roomJSON(status, errorResponse{Error: message})
}

type roomActor struct {
	msgs  chan roomMsg
	start sync.Once
	// apply 处理一条消息。生产里它就是 Server.applyRoomMsg；
	// 单元测试会换成自己的实现，用来验证"串行"和"panic 之后还能继续"。
	apply func(roomMsg) roomReply
	// views 是这个房间上一次推给每个玩家、以及观战者的视图。
	// 只由 worker 自己读写，所以不需要锁：这也是补丁顺序天然正确的原因。
	views roomViews
}

// roomViews 按玩家 / 观战分别缓存上一份推出去的视图，用来算下一次的补丁。
// 同一玩家开两个窗口共享一份：补丁相对"他这一侧上次看到的"，而不是相对每条连接。
type roomViews struct {
	players  map[string]map[string]any
	spectate map[string]any
}

func newRoomActor(server *Server) *roomActor {
	actor := &roomActor{
		msgs:  make(chan roomMsg, 128),
		views: roomViews{players: map[string]map[string]any{}},
	}
	if server != nil {
		actor.apply = server.applyRoomMsg
	}
	return actor
}

// run 是房间唯一的一条处理循环：取消息、处理、应答。所有对权威状态的修改、
// 所有推送都发生在这一个 goroutine 里，所以两条消息之间不可能交错。
func (a *roomActor) run() {
	for msg := range a.msgs {
		reply := a.applySafely(msg)
		if msg.wantsReply() {
			msg.answer(reply)
		}
	}
}

// applySafely 让一条消息的 panic 只作废它自己：记录日志、回 500，
// 房间继续处理后面的消息。否则一次越界就能把整个房间永久卡死。
func (a *roomActor) applySafely(msg roomMsg) (reply roomReply) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("房间 %s 处理 %s 异常：%v\n%s", msg.roomID, msg.kind, recovered, debug.Stack())
			reply = roomError(http.StatusInternalServerError, "房间处理异常")
		}
	}()
	if a.apply == nil {
		return roomReply{}
	}
	return a.apply(msg)
}

// applyRoomMsg 是房间 worker 的分派表：一个房间能发生的事就这几种，全在这里。
func (s *Server) applyRoomMsg(msg roomMsg) roomReply {
	switch msg.kind {
	case msgState:
		return s.doState(msg)
	case msgEvents:
		return s.doEvents(msg)
	case msgCommand:
		return s.doCommand(msg)
	case msgReady:
		return s.doReady(msg)
	case msgSpectate:
		return s.doSpectate(msg)
	case msgJoin:
		return s.doJoin(msg)
	case msgConnect:
		s.doConnect(msg)
	case msgSpectatorJoin:
		s.doSpectatorJoin(msg)
	case msgSettle:
		s.doSettle(msg)
	}
	return roomReply{}
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

// roomRoutes 是房间的 HTTP 入口：只做鉴权和参数粗筛，真正的事交给房间 worker。
// 每个分支都变成一条消息丢进同一个 worker，所以 HTTP 和 WebSocket 打进来的命令
// 在房间里是同一条串行队列。
func (s *Server) roomRoutes(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "rooms" {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}
	defer r.Body.Close()
	roomID, action := parts[3], parts[4]
	// 观战接口不走玩家 Cookie：它用房间 join_token 认证，返回合并后的双方视图。
	if r.Method == http.MethodGet && action == "spectate" {
		writeRoomReply(w, s.askRoom(roomMsg{kind: msgSpectate, roomID: roomID, token: r.URL.Query().Get("token")}))
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
	msg := roomMsg{roomID: roomID, player: player}
	switch {
	case r.Method == http.MethodGet && action == "state":
		msg.kind = msgState
	case r.Method == http.MethodGet && action == "events":
		msg.kind = msgEvents
	case r.Method == http.MethodPost && action == "commands":
		msg.kind = msgCommand
	case r.Method == http.MethodPost && action == "ready":
		msg.kind = msgReady
	default:
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}
	if msg.kind == msgCommand || msg.kind == msgReady {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			writeError(w, http.StatusBadRequest, "请求JSON无效")
			return
		}
		msg.body = body
	}
	writeRoomReply(w, s.askRoom(msg))
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
	// 加入也走房间队列：满员判断和落库必须原子，否则两个人能同时挤进一个双人房。
	reply := s.askRoom(roomMsg{kind: msgJoin, roomID: roomID, token: token, name: request.Name, playerID: playerID, session: session})
	if reply.status != http.StatusOK {
		writeRoomReply(w, reply)
		return
	}
	s.setSessionCookie(w, session)
	writeRoomReply(w, reply)
}

// doJoin 在房间 worker 里加入：满员判断和落库是同一个串行动作。
func (s *Server) doJoin(msg roomMsg) roomReply {
	player, err := s.store.JoinRoom(context.Background(), msg.token, strings.TrimSpace(msg.name), msg.playerID, hashToken(msg.session))
	if err != nil {
		return roomError(http.StatusBadRequest, err.Error())
	}
	s.publishRoom(msg.roomID)
	body, err := json.Marshal(roomResponse{RoomID: player.RoomID, PlayerID: player.ID})
	if err != nil {
		return roomError(http.StatusInternalServerError, "响应编码失败")
	}
	return roomReply{status: http.StatusOK, body: body, session: msg.session}
}

// doConnect 记录玩家上下线，并推给对手（双方的 players 里都带在线状态）。
func (s *Server) doConnect(msg roomMsg) {
	_ = s.store.SetConnected(context.Background(), msg.playerID, msg.connected)
	s.publishRoom(msg.roomID)
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

// doCommand 在房间 worker 里处理一条命令：校验、落库、必要时立刻改权威状态，
// 最后把这次变更推出去。因为它跑在 worker 里，"两条并发命令谁先生效"
// 和"先落库再推送"都是这一条队列说了算。
func (s *Server) doCommand(msg roomMsg) roomReply {
	ctx := context.Background()
	roomID, player := msg.roomID, msg.player
	var request commandRequest
	if err := decodeBody(msg.body, &request); err != nil {
		return roomError(http.StatusBadRequest, err.Error())
	}
	if request.CommandID == "" || len(request.CommandID) > 128 {
		return roomError(http.StatusBadRequest, "command_id无效")
	}
	fingerprint := commandFingerprint(request)
	if response, stored, found, err := s.store.CommandResponse(ctx, player.ID, request.CommandID); err == nil && found {
		// 同一个 command_id 必须是同一条命令：客户端重启后从 1 重新计数的情况下，
		// 沿用旧 ID 提交新命令以前会静默返回旧回执（HTTP 200），新命令被吞掉。
		if stored != "" && stored != fingerprint {
			return roomError(http.StatusBadRequest, "command_id已被其他命令使用，请换一个新的ID")
		}
		return roomReply{status: http.StatusOK, body: []byte(response)}
	}
	room, err := s.store.Room(ctx, roomID)
	if err != nil {
		return roomError(http.StatusNotFound, "房间不存在")
	}
	// 身份用库里最新的记录：连接是几分钟前建的，用连接上那份旧的 ready
	// 会绕过"就绪即冻结"，换过阵营之后也会按老阵营校验命令。
	players, err := s.store.Players(ctx, roomID)
	if err != nil {
		return roomError(http.StatusInternalServerError, "读取玩家失败")
	}
	fresh, found := findPlayer(players, player.ID)
	if !found {
		return roomError(http.StatusNotFound, "玩家不存在")
	}
	player = fresh
	// 就绪即冻结：点"完成部署"那一刻本回合的操作就锁死了，之后服务端拒绝任何修改。
	// 以前这里会自动把 ready 撤回再接受命令，等于"准备"完全没有约束力：
	// 玩家点了准备还能继续改，对手以为局面定了，结算出来的却是改过的版本。
	if player.Ready {
		return roomError(http.StatusConflict, "本回合已就绪，操作已锁定：双方就绪后由服务器统一结算，下一回合才能再操作")
	}
	// 同回合内的操作本来就该互相覆盖：订单按 (room, turn, player, unit) 落库，后写的顶掉先写的。
	// 唯一必须拒绝的是"回合已经结算完"——那说明这条命令是基于上一回合的战场做的决定，
	// 落到新回合会错误地执行（比如把已经开出去的舰再挪一次）。
	if request.BaseTurn > 0 && request.BaseTurn != room.Turn {
		return roomError(http.StatusConflict, "本回合已经结算：请基于当前回合重新操作")
	}
	order := game.Order{Type: request.Type, UnitID: request.UnitID, TargetUnitID: request.TargetUnitID, AircraftID: request.AircraftID, AircraftIDs: request.AircraftIDs, TaskID: request.TaskID, Coord: game.NormalizeToken(request.Coord), TargetFleetID: request.TargetFleetID, Heading: strings.ToUpper(request.Heading), SectorWidth: request.SectorWidth, Assignments: request.Assignments, TaskKind: strings.ToLower(request.TaskKind), Loadout: strings.ToLower(request.Loadout), Disperse: request.Disperse, Engagement: strings.ToLower(request.Engagement), Cycle: request.Cycle}
	if err := game.ValidateOrder(room.State, player.Faction, order); err != nil {
		return roomError(http.StatusBadRequest, err.Error())
	}
	// 无线电机会在提交时就要扣：同一回合里重复给同一个编队发命令必须立刻被拒绝，
	if room.State.Outcome != nil {
		return roomError(http.StatusBadRequest, "对局已经结束："+room.State.Outcome.Reason)
	}
	// 不能等到结算阶段才丢弃（那样玩家会以为第二次也发出去了）。
	if game.IsRadioOrder(order.Type) {
		if used := pendingRadioTask(ctx, s.store, roomID, room.Turn, player.ID, order.TaskID); used {
			return roomError(http.StatusBadRequest, "该编队本回合已经发过无线电，不能再通信")
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
			return s.savePlannedOrder(ctx, room, msg, request, order)
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
		room, err = s.store.ApplyImmediate(ctx, room, player.ID, request.CommandID, fingerprint, string(response), room.State, events)
		if err != nil {
			return roomError(http.StatusConflict, "命令结算失败："+err.Error())
		}
		s.publishRoom(roomID)
		return roomReply{status: http.StatusOK, body: response}
	}
	response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID})
	if err := s.store.SaveOrder(ctx, roomID, room.Turn, player.ID, request.CommandID, order, string(response), fingerprint); err != nil {
		return roomError(http.StatusInternalServerError, "保存命令失败")
	}
	s.publishRoom(roomID)
	return roomReply{status: http.StatusOK, body: response}
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
func (s *Server) savePlannedOrder(ctx context.Context, room store.Room, msg roomMsg, request commandRequest, order game.Order) roomReply {
	response, _ := json.Marshal(map[string]any{"accepted": true, "command_id": request.CommandID, "turn": room.Turn})
	// 先推进版本（commands 表在这里写幂等响应），再落待结算命令；顺序反了会让幂等重试撞上主键冲突。
	room, err := s.store.BumpVersion(ctx, room, msg.player.ID, request.CommandID, commandFingerprint(request), string(response))
	if err != nil {
		return roomError(http.StatusConflict, "命令结算失败："+err.Error())
	}
	if err := s.store.SaveOrder(ctx, msg.roomID, room.Turn, msg.player.ID, request.CommandID, order, string(response), commandFingerprint(request)); err != nil {
		return roomError(http.StatusInternalServerError, "保存命令失败")
	}
	s.publishRoom(msg.roomID)
	return roomReply{status: http.StatusOK, body: response}
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
		// 单向消息：补结算的结果本身就是推送，不需要应答。
		s.postRoom(roomMsg{kind: msgSettle, roomID: room.ID})
	}
}

// doSettle 在房间 worker 里补一次结算：双方都已 ready 但没人再触发结算
// （进程重启后就是这样），由巡检捡起来。
func (s *Server) doSettle(msg roomMsg) {
	ctx := context.Background()
	roomID := msg.roomID
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

// doReady 处理"完成部署"。它可能顺手结算整个回合：双方都 ready 时，
// 命令在同一条队列里取出来、按顺序执行，然后统一推送。
func (s *Server) doReady(msg roomMsg) roomReply {
	ctx := context.Background()
	roomID, player := msg.roomID, msg.player
	var request readyRequest
	if err := decodeBody(msg.body, &request); err != nil {
		return roomError(http.StatusBadRequest, err.Error())
	}
	room, err := s.store.Room(ctx, roomID)
	if err != nil {
		return roomError(http.StatusNotFound, "房间不存在")
	}
	if request.BaseTurn > 0 && request.BaseTurn != room.Turn {
		return roomError(http.StatusConflict, "本回合已经结算：请基于当前回合重新操作")
	}
	// 就绪必须不可逆：ready 之后不能再改主意，也不能用 ready:false 解锁。
	// 需要改部署就等本回合结算。前端已经没有撤销按钮，脚本也必须遵守同一规则。
	if !request.Ready {
		return roomError(http.StatusBadRequest, "ready 一旦提交不可撤销；请等待本回合结算")
	}
	if room.State.Outcome != nil {
		return roomError(http.StatusBadRequest, "对局已经结束："+room.State.Outcome.Reason)
	}
	// 双人对局不能一个人自己跟自己跑：对手没进房就直接拒绝 ready，
	// 而不是"单人 ready 就结算"把十几个回合跑掉（那样回合、油量、侦察全白给）。
	if !room.Solo {
		players, err := s.store.Players(ctx, roomID)
		if err != nil {
			return roomError(http.StatusInternalServerError, "读取玩家失败")
		}
		if len(players) < 2 {
			return roomError(http.StatusBadRequest, "这是双人对局，对手还没进房：把房间 token 发给他，两名玩家都到齐后回合才会推进")
		}
	}
	if err := s.store.SetReady(ctx, player.ID, request.Ready); err != nil {
		return roomError(http.StatusInternalServerError, "准备状态保存失败")
	}
	players, _ := s.store.Players(ctx, roomID)
	allReady := len(players) > 0
	for _, item := range players {
		allReady = allReady && item.Ready
	}
	if allReady {
		records, err := s.store.Orders(ctx, roomID, room.Turn, "")
		if err != nil {
			return roomError(http.StatusInternalServerError, "读取命令失败")
		}
		orders := make([]game.Order, 0, len(records))
		for _, record := range records {
			orders = append(orders, record.Order)
		}
		// 引擎内部的回合号与服务端的房间回合保持一致，胜利条件的回合上限要用它。
		room.State.Turn = room.Turn
		events := resolveRoomTurn(&room.State, orders)
		room.State.Turn = room.Turn + 1
		resolved, err := s.store.Resolve(ctx, room, room.State, events)
		if err != nil {
			return roomError(http.StatusConflict, err.Error())
		}
		room = resolved
	}
	s.publishRoom(roomID)
	// ready 的响应只是一条回执：状态由推送下发，不用为改一个标记回 150 KB 全量。
	return roomJSON(http.StatusOK, map[string]any{"accepted": true, "turn": room.Turn})
}

// doState 投影某个玩家的私有视图，并把它记成这名玩家这一侧的底座。
// 这份全量和之后的补丁由同一个 goroutine 先后入队，所以补丁不可能排在它前面。
func (s *Server) doState(msg roomMsg) roomReply {
	ctx := context.Background()
	room, err := s.store.Room(ctx, msg.roomID)
	if err != nil {
		return roomError(http.StatusNotFound, "房间不存在")
	}
	players, err := s.store.Players(ctx, msg.roomID)
	if err != nil {
		return roomError(http.StatusInternalServerError, "读取玩家失败")
	}
	player, found := findPlayer(players, msg.player.ID)
	if !found {
		return roomError(http.StatusNotFound, "玩家不存在")
	}
	view, err := s.privateView(ctx, room, players, player)
	if err != nil {
		return roomError(http.StatusInternalServerError, err.Error())
	}
	s.roomActor(msg.roomID).views.players[player.ID] = view
	return roomJSON(http.StatusOK, view)
}

// findPlayer 按 ID 取玩家：投影一律用库里最新的记录，不用连接上缓存的那份
// （改名、换阵营之后不该继续按老记录投影）。
func findPlayer(players []store.Player, playerID string) (store.Player, bool) {
	for _, player := range players {
		if player.ID == playerID {
			return player, true
		}
	}
	return store.Player{}, false
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

func (s *Server) doEvents(msg roomMsg) roomReply {
	events, err := s.store.Events(context.Background(), msg.roomID, msg.player.Faction)
	if err != nil {
		return roomError(http.StatusInternalServerError, "读取战报失败")
	}
	return roomJSON(http.StatusOK, map[string]any{"events": events})
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
// roomActor 取（必要时创建）房间的 worker。房间状态只由它的 goroutine 改。
func (s *Server) roomActor(roomID string) *roomActor {
	value, ok := s.actors.Load(roomID)
	if !ok {
		value, _ = s.actors.LoadOrStore(roomID, newRoomActor(s))
	}
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

// askRoom 把一条消息交给房间 worker 并等它的应答。HTTP 入口用它；
// WebSocket 不等——应答由 worker 直接写进那条连接的 send。
func (s *Server) askRoom(msg roomMsg) roomReply {
	reply := make(chan roomReply, 1)
	msg.reply = reply
	s.roomActor(msg.roomID).msgs <- msg
	return <-reply
}

// postRoom 把一条不需要应答的消息交给房间 worker。队列满了就丢掉这条：
// 在线状态这类消息会被下一次连接或推送纠正，不值得为它阻塞一个 socket goroutine。
func (s *Server) postRoom(msg roomMsg) {
	select {
	case s.roomActor(msg.roomID).msgs <- msg:
	default:
		log.Printf("房间 %s 的消息队列已满，丢弃 %s", msg.roomID, msg.kind)
	}
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

// decodeBody 解一条已经读成字节的请求体（WebSocket 的 body、HTTP 读出来的 body）。
// 规矩和 decodeJSON 一样：拒未知字段。
func decodeBody(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
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

// writeRoomReply 把房间 worker 的应答写回 HTTP。
func writeRoomReply(w http.ResponseWriter, reply roomReply) {
	status := reply.status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(reply.body)
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
// readLoop 只管把消息丢进房间 worker，writeLoop 只管把 chan 里的帧写出去，
// 中间只有这个 chan。
type wsClient struct {
	conn *websocket.Conn
	// send 里放要写到这条连接的帧：RPC 应答、补丁、错误。
	// 顺序由房间 worker 的先后决定；塞不进去或写失败就断开这条连接，让他重连拉全量。
	send      chan []byte
	roomID    string
	playerID  string
	spectator bool
}

// enqueue 把一帧塞进连接的出口。塞不进去说明对端已经读不动了：直接断开，
// 让他重连重新拉全量，比收一半补丁、拿着错的底座继续跑要好。
func (c *wsClient) enqueue(frame []byte) bool {
	select {
	case c.send <- frame:
		return true
	default:
		return false
	}
}

// fail 推一条错误帧再断开。
func (c *wsClient) fail(message string) {
	if frame, err := json.Marshal(map[string]any{"type": "error", "error": message}); err == nil {
		c.enqueue(frame)
	}
	_ = c.conn.Close()
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
// deliver 把一帧丢给房间里某类连接（某个玩家的连接 / 全部观战连接）。
//
// 塞不进去说明这条连接已经跟不上（对端不读了），直接断开：它重连时会重新拉一份
// 全量，比让它收一半补丁、拿着错的底座继续跑要好。绝不能在这里阻塞——worker
// 一阻塞，整个房间（另一个玩家的命令、结算、巡检）就全停了。
func (h *hub) deliver(roomID, playerID string, spectator bool, frame []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients[roomID] {
		if client.spectator != spectator {
			continue
		}
		if playerID != "" && client.playerID != playerID {
			continue
		}
		if !client.enqueue(frame) {
			_ = client.conn.Close()
		}
	}
}

// deliverPlayer 只给某个玩家的连接；deliverSpectators 只给观战连接。
func (h *hub) deliverPlayer(roomID, playerID string, frame []byte) {
	h.deliver(roomID, playerID, false, frame)
}

func (h *hub) deliverSpectators(roomID string, frame []byte) {
	h.deliver(roomID, "", true, frame)
}

// rpcFailure 是 WebSocket RPC 在校验阶段就被拒掉的原因。
type rpcFailure struct {
	status  int
	message string
}

// roomMsgOfRPC 把一条 WebSocket RPC 翻译成房间消息。校验都在这里做；
// 通过之后它就只是队列里的一条消息——读状态、提命令、点 ready 和 HTTP
// 打进来的是同一种东西，走同一张分派表。
func (s *Server) roomMsgOfRPC(client *wsClient, player store.Player, request wsRequest) (roomMsg, *rpcFailure) {
	if request.ID == "" || len(request.ID) > 128 || request.Method == "" || len(request.Path) > 512 || len(request.Body) > 1<<20 {
		return roomMsg{}, &rpcFailure{status: http.StatusBadRequest, message: "无效的 WebSocket 请求"}
	}
	parsed, err := url.ParseRequestURI(request.Path)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return roomMsg{}, &rpcFailure{status: http.StatusBadRequest, message: "不允许的 WebSocket 路径"}
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "rooms" || parts[3] != player.RoomID {
		return roomMsg{}, &rpcFailure{status: http.StatusForbidden, message: "WebSocket 请求必须属于当前房间"}
	}
	msg := roomMsg{roomID: player.RoomID, player: player, client: client, rpcID: request.ID}
	switch {
	case request.Method == http.MethodGet && parts[4] == "state":
		msg.kind = msgState
	case request.Method == http.MethodGet && parts[4] == "events":
		msg.kind = msgEvents
	case request.Method == http.MethodPost && parts[4] == "commands":
		msg.kind, msg.body = msgCommand, request.Body
	case request.Method == http.MethodPost && parts[4] == "ready":
		msg.kind, msg.body = msgReady, request.Body
	default:
		return roomMsg{}, &rpcFailure{status: http.StatusForbidden, message: "不允许通过 WebSocket 调用此接口"}
	}
	return msg, nil
}

// replyRPCError 把校验失败回给客户端。它不碰房间状态，readLoop 自己就能答。
func replyRPCError(client *wsClient, id string, failure *rpcFailure) {
	body, err := json.Marshal(errorResponse{Error: failure.message})
	if err != nil {
		return
	}
	frame, err := json.Marshal(wsResponse{Type: "rpc_response", ID: id, Status: failure.status, Body: body})
	if err != nil {
		return
	}
	if !client.enqueue(frame) {
		_ = client.conn.Close()
	}
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
	if client.spectator {
		// 观战不走 RPC 拉取：首帧全量由 worker 直接入到这条连接。
		s.roomActor(client.roomID).msgs <- roomMsg{kind: msgSpectatorJoin, roomID: client.roomID, client: client}
	} else {
		// 玩家：在线状态变了要推给对手；自己的底座等他发 GET state 时再记。
		s.postRoom(roomMsg{kind: msgConnect, roomID: client.roomID, playerID: client.playerID, connected: true})
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
			// readLoop 只入队，不等结果：应答和补丁都由 worker 按处理顺序
			// 写进 send，所以补丁不可能跑到它的底座前面。
			if msg, failure := s.roomMsgOfRPC(client, player, request); failure != nil {
				replyRPCError(client, request.ID, failure)
			} else {
				s.roomActor(client.roomID).msgs <- msg
			}
		}
	}()
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		s.hub.remove(client)
		if !client.spectator {
			s.postRoom(roomMsg{kind: msgConnect, roomID: client.roomID, playerID: client.playerID, connected: false})
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

// doSpectatorJoin 给一个新接入的观战连接推一份完整快照，并把它记成观战的底座。
// 观战不走 RPC 拉取，所以首帧必须由服务端给。
func (s *Server) doSpectatorJoin(msg roomMsg) {
	ctx := context.Background()
	room, err := s.store.Room(ctx, msg.roomID)
	if err != nil {
		msg.client.fail(err.Error())
		return
	}
	view, err := s.spectateView(ctx, room)
	if err != nil {
		msg.client.fail("读取观战视图失败")
		return
	}
	frame, err := json.Marshal(map[string]any{"type": "snapshot", "view": view})
	if err != nil {
		msg.client.fail("观战视图编码失败")
		return
	}
	s.roomActor(msg.roomID).views.spectate = view
	if !msg.client.enqueue(frame) {
		_ = msg.client.conn.Close()
	}
}

func (s *Server) webSocketPlayer(r *http.Request) (store.Player, bool) {
	cookie, err := r.Cookie("gq_session")
	if err != nil {
		return store.Player{}, false
	}
	player, err := s.store.PlayerBySession(r.Context(), hashToken(cookie.Value))
	return player, err == nil
}
