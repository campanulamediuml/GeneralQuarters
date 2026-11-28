package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
	"github.com/gorilla/websocket"
)

type testView struct {
	Turn       int                      `json:"turn"`
	Units      []game.Unit              `json:"units"`
	Fleets     []game.FleetGroup        `json:"fleets"`
	Aviation   game.AviationView        `json:"aviation"`
	AirCombats []game.AirCombatReport   `json:"air_combats"`
	FleetStays map[string]string        `json:"fleet_stays"`
	GroundAir  []game.AirfieldAirStatus `json:"ground_air"`
	You        store.Player             `json:"you"`
	Players    []store.Player           `json:"players"`
	Orders     []game.Order             `json:"orders"`
	JoinToken  string                   `json:"join_token"`
	Solo       bool                     `json:"solo"`
}

type testClient struct {
	client *http.Client
	base   string

	rpcMu     sync.Mutex
	rpcSeq    int
	roomConns map[string]*websocket.Conn
}

func newTestClient(t *testing.T, base string) *testClient {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{client: &http.Client{Jar: jar}, base: base}
}

// call 使用各接口唯一的传输方式：大厅走 HTTP，房间状态与操作走 WebSocket RPC。
func (c *testClient) call(t *testing.T, method, path string, input, output any) int {
	t.Helper()
	if strings.HasPrefix(path, "/api/v1/rooms/") {
		for _, suffix := range []string{"/state", "/events", "/commands", "/ready"} {
			if strings.HasSuffix(path, suffix) {
				return c.rpc(t, method, path, input, output)
			}
		}
	}
	return c.httpCall(t, method, path, input, output)
}

func (c *testClient) rpc(t *testing.T, method, path string, input, output any) int {
	t.Helper()
	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		t.Fatalf("RPC 房间路径无效：%s", path)
	}
	roomID := parts[4]
	conn := c.roomConns[roomID]
	if conn == nil {
		conn = dialTestWS(t, c, "/ws?room_id="+roomID)
		if c.roomConns == nil {
			c.roomConns = map[string]*websocket.Conn{}
		}
		c.roomConns[roomID] = conn
	}
	c.rpcSeq++
	id := "test-rpc-" + strconv.Itoa(c.rpcSeq)
	var body json.RawMessage
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = encoded
	}
	if err := conn.WriteJSON(wsRequest{Type: "rpc_request", ID: id, Method: method, Path: path, Body: body}); err != nil {
		t.Fatal(err)
	}
	response := readWSResponse(t, conn, id)
	if output != nil && len(response.Body) > 0 {
		if err := json.Unmarshal(response.Body, output); err != nil {
			t.Fatal(err)
		}
	}
	return response.Status
}

func (c *testClient) resetRPC() {
	c.rpcMu.Lock()
	defer c.rpcMu.Unlock()
	for _, conn := range c.roomConns {
		_ = conn.Close()
	}
	c.roomConns = nil
}

// httpCall 只用于大厅接口和"旧房间 HTTP 已删除"的协议断言。
func (c *testClient) httpCall(t *testing.T, method, path string, input, output any) int {
	t.Helper()
	var body bytes.Buffer
	if input != nil {
		_ = json.NewEncoder(&body).Encode(input)
	}
	request, err := http.NewRequest(method, c.base+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if output != nil {
		_ = json.NewDecoder(response.Body).Decode(output)
	}
	return response.StatusCode
}

func setupTestServer(t *testing.T, options ...func(*Server)) (*testClient, string, *store.Store, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := New(database, testContentRoot(t), false)
	for _, option := range options {
		option(app)
	}
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	httpServer := httptest.NewServer(mux)
	cleanup := func() { httpServer.Close(); database.Close() }
	return newTestClient(t, httpServer.URL), path, database, cleanup
}

func TestRoomHTTPInterfacesRemoved(t *testing.T) {
	client, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.httpCall(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/state"},
		{http.MethodGet, "/events"},
		{http.MethodPost, "/commands"},
		{http.MethodPost, "/ready"},
		{http.MethodGet, "/spectate?token=" + created.JoinToken},
	}
	for _, test := range tests {
		path := "/api/v1/rooms/" + created.RoomID + test.path
		if status := client.httpCall(t, test.method, path, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s %s 应返回 404，实际 %d", test.method, path, status)
		}
	}
}

func TestTwoPlayerCommandAndTurnFlow(t *testing.T) {
	a, dbPath, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	if len(created.JoinToken) != 14 {
		t.Fatalf("邀请token格式错误：%s", created.JoinToken)
	}

	var initial testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial); status != http.StatusOK {
		t.Fatal("读取A状态失败")
	}
	if initial.You.Faction != "A" || initial.JoinToken == "" {
		t.Fatal("房主信息不完整")
	}
	for _, unit := range initial.Units {
		if unit.Faction == "B" {
			t.Fatal("A初始状态泄露远端敌舰")
		}
	}

	b := newTestClient(t, a.base)
	var joined roomResponse
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"name": "B", "token": created.JoinToken}, &joined); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	var bState testView
	_ = b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &bState)
	if bState.You.Faction != "B" || bState.JoinToken != "" {
		t.Fatal("B阵营或token权限错误")
	}
	for _, unit := range bState.Units {
		if unit.Faction == "A" {
			t.Fatal("B初始状态泄露远端敌舰")
		}
	}

	invalid := map[string]any{"command_id": "invalid", "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0404"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", invalid, nil); status != http.StatusBadRequest {
		t.Fatalf("非法移动状态码%d", status)
	}
	valid := map[string]any{"command_id": "move-1", "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0203"}
	var moveView map[string]any
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", valid, &moveView); status != http.StatusOK {
		t.Fatalf("合法移动被拒绝：%v", moveView)
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", valid, nil); status != http.StatusOK {
		t.Fatal("幂等重试失败")
	}
	// 同一回合内对同一支舰队反复下命令是正常的：后写的顶掉先写的（订单按 unit 做主键）。
	// 以前这里靠 base_version 相等来卡，导致玩家点第二下就被拒——重复操作不该失败。
	// 0201 和 0203 都是 0202 的相邻格：先挪到 0201，验证后写覆盖先写。
	rerouted := map[string]any{"command_id": "move-2", "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0201"}
	var reroutedBody map[string]any
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", rerouted, &reroutedBody); status != http.StatusOK {
		t.Fatalf("同回合内的重复操作被拒绝：status=%d body=%v", status, reroutedBody)
	}
	var reroutedView testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &reroutedView)
	if len(reroutedView.Orders) != 1 || reroutedView.Orders[0].Coord != "0201" {
		t.Fatalf("后写的命令没有覆盖先写的：%+v", reroutedView.Orders)
	}
	// 改回去，后续断言仍然按 0203 结算。
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{"command_id": "move-3", "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0203"}, nil); status != http.StatusOK {
		t.Fatal("改回原目标失败")
	}
	var moved testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &moved)
	// 移动是"本回合规划"：提交后不立刻位移，只登记在待结算命令里。
	if len(moved.Orders) != 1 || moved.Orders[0].Type != "SET_MOVE" || moved.Orders[0].Coord != "0203" {
		t.Fatalf("移动规划未写入待结算命令：%+v", moved.Orders)
	}

	// ready 的响应只是回执：状态要另外拉。
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatal("单方ready不应结算")
	}
	var afterSingle testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &afterSingle)
	if afterSingle.Turn != 1 {
		t.Fatalf("单方ready就结算了：第%d回合", afterSingle.Turn)
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatal("双方ready结算失败")
	}
	var readyView testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &readyView); status != http.StatusOK {
		t.Fatal("结算后读状态失败")
	}
	if readyView.Turn != 2 {
		t.Fatalf("回合没推进：%+v", readyView)
	}
	// 结算完毕后进入新回合：位置应为规划目标，移动标记在新回合开始时清空。
	found := false
	for _, unit := range readyView.Units {
		if unit.ID == "a-bb-01" && unit.Coord == "0203" {
			found = true
		}
	}
	if !found {
		t.Fatal("回合结算没有执行规划的移动")
	}

	// 只有跨回合的过期提交必须拒绝：那条命令是基于上一回合的战场做的决定，
	// 落到新回合会错误地执行。这是 base_turn 唯一负责的事。
	stale := map[string]any{"command_id": "move-stale", "base_turn": 1, "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0205"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", stale, nil); status != http.StatusConflict {
		t.Fatal("过期回合的命令未被拒绝")
	}
	persisted, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer persisted.Close()
	room, err := persisted.Room(t.Context(), created.RoomID)
	if err != nil || room.Turn != 2 || room.State.Units["a-bb-01"].Coord != "0203" {
		t.Fatalf("持久化恢复失败：%+v %v", room, err)
	}

	third := newTestClient(t, a.base)
	if status := third.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken}, nil); status != http.StatusBadRequest {
		t.Fatal("满员房间允许第三人加入")
	}
}

func TestSoloReadyAndInvalidToken(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	_ = a.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created)
	var view testView
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, &view); status != http.StatusOK || view.Turn != 2 {
		t.Fatalf("单人沙盒未结算：%+v", view)
	}
	other := newTestClient(t, a.base)
	if status := other.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": "XXXX-XXXX-XXXX"}, nil); status != http.StatusBadRequest {
		t.Fatal("无效token未被拒绝")
	}
}

func TestRoomActorSerializesAllMessages(t *testing.T) {
	actor := newRoomActor(nil)
	const messageCount = 64
	var active atomic.Int32
	var maximum atomic.Int32
	var completed int
	actor.apply = func(msg roomMsg) roomReply {
		if msg.reply != nil { // 收尾用的探针消息，不参与计数
			return roomReply{}
		}
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		runtime.Gosched()
		completed++
		active.Add(-1)
		return roomReply{}
	}
	actor.start.Do(func() { go actor.run() })
	start := make(chan struct{})
	var workers sync.WaitGroup
	for index := 0; index < messageCount; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			actor.msgs <- roomMsg{kind: msgConnect, roomID: "same-room"}
		}()
	}
	close(start)
	workers.Wait()
	// 队列是 FIFO：这条带应答的消息回来时，前面 64 条都已经处理完了。
	reply := make(chan roomReply, 1)
	actor.msgs <- roomMsg{kind: msgConnect, roomID: "same-room", reply: reply}
	<-reply
	if completed != messageCount {
		t.Fatalf("房间消息丢失：%d/%d", completed, messageCount)
	}
	if maximum.Load() != 1 {
		t.Fatalf("房间消息并发处理，最大并发数：%d", maximum.Load())
	}
}

func TestRoomActorContinuesAfterPanic(t *testing.T) {
	actor := newRoomActor(nil)
	actor.apply = func(roomMsg) roomReply { panic("test panic") }
	actor.start.Do(func() { go actor.run() })
	first := make(chan roomReply, 1)
	actor.msgs <- roomMsg{kind: msgConnect, roomID: "same-room", reply: first}
	if reply := <-first; reply.status != http.StatusInternalServerError {
		t.Fatalf("panic 之后没有回错误应答：%+v", reply)
	}
	actor.apply = func(roomMsg) roomReply { return roomReply{status: http.StatusOK} }
	completed := make(chan roomReply, 1)
	actor.msgs <- roomMsg{kind: msgConnect, roomID: "same-room", reply: completed}
	if reply := <-completed; reply.status != http.StatusOK {
		t.Fatal("房间actor在panic后停止处理后续消息")
	}
}

func TestConcurrentCommandsHaveSingleWinner(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatal("创建房间失败")
	}
	// 同一版本并发命令只能有一个成功：用两条会立即改变权威状态的命令（给同一个机库算子分配任务）来验证。
	commands := []map[string]any{
		{"command_id": "concurrent-flag", "type": "LAUNCH_CAP", "aircraft_id": "a2-cv-01-fighter-x1", "aircraft_ids": []string{"a2-cv-01-fighter-x1"}},
		{"command_id": "concurrent-screen", "type": "LAUNCH_STRIKE", "aircraft_id": "a2-cv-01-fighter-x1", "aircraft_ids": []string{"a2-cv-01-fighter-x1"}},
	}
	statuses := make(chan int, len(commands))
	transports := []*testClient{a, {client: a.client, base: a.base}}
	for index, command := range commands {
		client := transports[index]
		go func() {
			var response map[string]any
			status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, &response)
			t.Logf("并发命令响应：status=%d body=%v", status, response)
			statuses <- status
		}()
	}
	// 并发下必须只有一个成功，但"输的那个"不一定是版本冲突：
	// 房间请求是串行化执行的，后到的那条会看到前一条已经落库的权威状态，
	// 于是由 ValidateOrder 用语义错误拒掉（"算子已经属于其他任务"）。
	// 这比版本号更精确：拒绝理由是真实的业务冲突，而不是"你的快照旧了"。
	accepted, rejected := 0, 0
	for range commands {
		status := <-statuses
		switch status {
		case http.StatusOK:
			accepted++
		case http.StatusConflict, http.StatusBadRequest:
			rejected++
		default:
			t.Fatalf("并发命令返回意外状态码：%d", status)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("并发命令应只有一个成功：accepted=%d rejected=%d", accepted, rejected)
	}
	// 赢家只能有一个任务：两条命令都是"给同一个算子分配任务"，
	// 所以权威状态里应该只有一条任务，不是两条叠加。
	var view testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatalf("读取权威状态失败：%d", status)
	}
	if len(view.Aviation.Tasks) != 1 {
		t.Fatalf("并发命令后权威任务数错误：%d", len(view.Aviation.Tasks))
	}
}

func TestSectorReconCommandPersistsAndAdvances(t *testing.T) {
	a, dbPath, database, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatal("创建房间失败")
	}
	b := newTestClient(t, a.base)
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken}, nil); status != http.StatusOK {
		t.Fatal("加入房间失败")
	}
	command := map[string]any{
		"command_id": "recon-sector-1", "type": "LAUNCH_RECON", "aircraft_id": "b-cv-04-dive_bomber-x1",
		"heading": "W", "sector_width": 60,
	}
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, nil); status != http.StatusOK {
		t.Fatalf("机库算子分配侦察任务被拒绝")
	}
	var assigned testView
	if status := b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &assigned); status != http.StatusOK {
		t.Fatal("读取预备侦察任务失败")
	}
	// 侦察半径不可设置：无论命令里写什么，都按机型的最大搜索半径执行。
	wantRadius := game.ReconRadiusLimit(assigned.Aviation.Tasks[0].Aircraft[0])
	if len(assigned.Aviation.Tasks) != 1 || assigned.Aviation.Tasks[0].Phase != "servicing" || assigned.Aviation.Tasks[0].ReconRadius != wantRadius {
		t.Fatalf("侦察任务未立即进入整备：%+v（期望半径 %d）", assigned.Aviation.Tasks, wantRadius)
	}
	room, err := database.Room(t.Context(), created.RoomID)
	if err != nil || room.State.Aircraft["b-cv-04-dive_bomber-x1"].Status != "service" || room.State.Aircraft["b-cv-04-dive_bomber-x1"].TaskKind != "recon" || room.State.Aircraft["b-cv-04-dive_bomber-x1"].ReconRadius != game.ReconRadiusLimit(*room.State.Aircraft["b-cv-04-dive_bomber-x1"]) {
		t.Fatalf("预备侦察任务未持久化：%+v %v", room.State.Aircraft["b-cv-04-dive_bomber-x1"], err)
	}

	for _, expected := range []struct {
		turn   int
		phase  string
		status string
	}{{2, "servicing", "service"}, {3, "lifting", "elevator"}, {4, "launching", "recon"}} {
		if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
			t.Fatal("A ready失败")
		}
		if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
			t.Fatal("B ready失败")
		}
		var ready testView
		if status := b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &ready); status != http.StatusOK {
			t.Fatal("读取结算后的状态失败")
		}
		if ready.Turn != expected.turn || len(ready.Aviation.Tasks) != 1 || ready.Aviation.Tasks[0].Phase != expected.phase || ready.Aviation.Tasks[0].Aircraft[0].Status != expected.status {
			t.Fatalf("第%d回合航空阶段错误：%+v", expected.turn, ready.Aviation.Tasks)
		}
	}
	launched := assigned
	if status := b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &launched); status != http.StatusOK {
		t.Fatal("读取自动起飞任务失败")
	}
	task := launched.Aviation.Tasks[0]
	if task.Kind != "recon" || task.Heading != "W" || task.SectorWidth != 60 || task.ReconDistance != 0 || len(task.SectorHexes) == 0 {
		t.Fatalf("自动起飞扇面投影错误：%+v", task)
	}

	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatal("A ready失败")
	}
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatal("B ready失败")
	}
	var ready testView
	if status := b.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &ready); status != http.StatusOK {
		t.Fatal("读取结算后的状态失败")
	}
	if ready.Turn != 5 || len(ready.Aviation.Tasks) != 1 || ready.Aviation.Tasks[0].Phase != "active" || ready.Aviation.Tasks[0].ReconDistance != 2 || ready.Aviation.Tasks[0].CurrentCoord != "0206" {
		t.Fatalf("自动起飞后没有推进侦察航线：%+v", ready.Aviation.Tasks)
	}
	persisted, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer persisted.Close()
	restored, err := persisted.Room(t.Context(), created.RoomID)
	restoredRecon := restored.State.Aircraft["b-cv-04-dive_bomber-x1"]
	if restoredRecon == nil {
		t.Fatalf("重启后侦察算子丢失：%v", err)
	}
	// 落盘后航程不应变化：主燃油 = 基础航程 × 副油箱系数，减去已飞的 2 格。
	wantRange := game.SortieBaseRange(restoredRecon.BaseRange, restoredRecon.Loadout) - 2
	if err != nil || restoredRecon.Coord != "0206" || restoredRecon.Range() != wantRange {
		t.Fatalf("重启读取的侦察状态错误：%+v %v", restored.State.Aircraft["b-cv-04-dive_bomber-x1"], err)
	}
}
