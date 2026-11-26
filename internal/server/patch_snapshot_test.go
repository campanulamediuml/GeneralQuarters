package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 补丁协议的地基：客户端把补丁合并回底座之后，结果必须和"重新拉一份全量快照"逐字段相同。
//
// 补丁是"和上一帧的差异"，快照是同一份投影的完整序列化。两边一旦对不上，客户端本地
// 视图就会永远比服务端多出或少掉一些字段，直到强制重连才恢复。这不是理论问题：
// 事件数组以前在快照里是倒序、而补丁只能往数组尾部追加，每回合合并出来的事件列表
// 顺序都会漂一次；视图类型以前带 omitempty，归零的字段在快照里直接消失、在补丁里
// 却以 0 出现，两边形状永远不一致。
func TestPatchMergeMatchesSnapshot(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	// 新剧本开局在部署阶段：先把第 1 回合开起来，再测"回合结算"的补丁。
	settleDeployment(t, created.RoomID, host)
	conn := dialTestWS(t, host, "/ws?room_id="+created.RoomID)
	seedPlayerView(t, conn, created.RoomID)

	// 六个回合足够走过"移动 -> 冷却 -> 再移动"：legal_moves/move_cooldown 会反复
	// 出现又归零，正是补丁和快照最容易对不上的地方。
	for turn := 1; turn <= 6; turn++ {
		view := fetchStateView(t, host, created.RoomID)
		issueMoveCommand(t, conn, created.RoomID, view)
		merged := readyAndWaitTurn(t, conn, created.RoomID, view, turn+1)
		snapshot := fetchStateView(t, host, created.RoomID)
		compareViewJSON(t, fmt.Sprintf("turn %d", turn), merged, snapshot)
	}
}

// 双人房里两个玩家各拿自己那份投影，两边都必须满足同一条不变式。
// 单人房是"一方 ready 就结算"，走的是另一条分支；多人局两边都 ready 才结算，
// 补丁路径要靠这条用例守住。
func TestPatchMergeMatchesSnapshotTwoPlayers(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_coral_real"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	guest := newTestClient(t, host.base)
	var joined roomResponse
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]any{"token": created.JoinToken, "name": "B"}, &joined); status != http.StatusOK {
		t.Fatalf("join status %d", status)
	}
	settleDeployment(t, created.RoomID, host, guest)
	usConn := dialTestWS(t, host, "/ws?room_id="+created.RoomID)
	jpConn := dialTestWS(t, guest, "/ws?room_id="+created.RoomID)
	seedPlayerView(t, usConn, created.RoomID)
	seedPlayerView(t, jpConn, created.RoomID)

	for turn := 1; turn <= 6; turn++ {
		usView := fetchStateView(t, host, created.RoomID)
		jpView := fetchStateView(t, guest, created.RoomID)
		issueMoveCommandFor(t, usConn, created.RoomID, usView, "A")
		issueMoveCommandFor(t, jpConn, created.RoomID, jpView, "B")
		sendReady(t, usConn, created.RoomID, usView)
		sendReady(t, jpConn, created.RoomID, jpView)
		usMerged := waitForTurn(t, usConn, turn+1)
		jpMerged := waitForTurn(t, jpConn, turn+1)
		compareViewJSON(t, fmt.Sprintf("turn %d USA", turn), usMerged, fetchStateView(t, host, created.RoomID))
		compareViewJSON(t, fmt.Sprintf("turn %d Japan", turn), jpMerged, fetchStateView(t, guest, created.RoomID))
	}
}

// fetchStateView 走 HTTP 拉一份全量：它就是客户端重连时拿到的底座，
// 也是"合并结果应该长什么样"的权威答案。
func fetchStateView(t *testing.T, client *testClient, roomID string) map[string]any {
	t.Helper()
	var view map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatalf("state status %d", status)
	}
	return view
}

// issueMoveCommand 从当前视图里挑一支能动的己方舰队，让它走一格合法移动。
// 顺手验证 SET_MOVE 的 unit_id 字段填的是舰船 id（填 fleet_id 会被拒）。
func issueMoveCommand(t *testing.T, conn *websocket.Conn, roomID string, view map[string]any) {
	t.Helper()
	issueMoveCommandFor(t, conn, roomID, view, "A")
}

func issueMoveCommandFor(t *testing.T, conn *websocket.Conn, roomID string, view map[string]any, faction string) {
	t.Helper()
	command := moveCommandFor(view, faction)
	if command == nil {
		return
	}
	encoded, err := json.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	writeRoomCommand(t, conn, roomID, command["command_id"].(string), encoded)
}

func moveCommandFor(view map[string]any, faction string) map[string]any {
	fleets, _ := view["fleets"].([]any)
	for _, raw := range fleets {
		fleet, ok := raw.(map[string]any)
		if !ok || fleet["faction"] != faction {
			continue
		}
		if cooldown, ok := fleet["move_cooldown"].(float64); ok && cooldown > 0 {
			continue
		}
		moves := stringList(fleet["legal_moves"])
		if len(moves) == 0 {
			continue
		}
		unitID := ""
		members, _ := fleet["members"].([]any)
		for _, rawMember := range members {
			if member, ok := rawMember.(map[string]any); ok {
				if id, ok := member["id"].(string); ok {
					unitID = id
				}
			}
			if unitID != "" {
				break
			}
		}
		if unitID == "" {
			continue
		}
		turn := int(view["turn"].(float64))
		return map[string]any{
			"command_id": fmt.Sprintf("move-%d", turn),
			"base_turn":  turn,
			"type":       "SET_MOVE",
			"unit_id":    unitID,
			"coord":      moves[0],
		}
	}
	return nil
}

// readyAndWaitTurn 点完成部署，然后等到第 wantTurn 回合的推送被合并进本地视图。
// 结算时服务端会推一帧补丁，它和 rpc_response 可能任意先到：这里共用一个读循环，
// 保证推送帧一帧都不漏（漏一帧，本地底座就永远缺一块）。
func readyAndWaitTurn(t *testing.T, conn *websocket.Conn, roomID string, view map[string]any, wantTurn int) map[string]any {
	t.Helper()
	sendReady(t, conn, roomID, view)
	// 结算推送可能已经在这个读循环里合并过了。
	return waitForTurn(t, conn, wantTurn)
}

// sendReady 只点完成部署、不等结算：双人房里要两边都 ready 才会结算。
func sendReady(t *testing.T, conn *websocket.Conn, roomID string, view map[string]any) {
	t.Helper()
	turn := int(view["turn"].(float64))
	encoded, err := json.Marshal(map[string]any{"base_turn": turn, "ready": true})
	if err != nil {
		t.Fatal(err)
	}
	id := fmt.Sprintf("ready-%d", turn)
	response := callRoomRPC(t, conn, wsRequest{Type: "rpc_request", ID: id, Method: http.MethodPost, Path: "/api/v1/rooms/" + roomID + "/ready", Body: encoded})
	if response.Status != http.StatusOK {
		t.Fatalf("ready status=%d body=%s", response.Status, response.Body)
	}
}

func waitForTurn(t *testing.T, conn *websocket.Conn, turn int) map[string]any {
	t.Helper()
	reader := viewReader(conn)
	if reader.view != nil && reader.view["turn"] == float64(turn) {
		return reader.view
	}
	return reader.readWhere(t, conn, func(view map[string]any) bool { return view["turn"] == float64(turn) })
}

// callRoomRPC 发一条 RPC 并等它的响应；途中收到的推送帧先合并进本地视图，
// 不能像 readWSResponse 那样直接丢掉。
func callRoomRPC(t *testing.T, conn *websocket.Conn, request wsRequest) wsResponse {
	t.Helper()
	if err := conn.WriteJSON(request); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		var raw json.RawMessage
		if err := conn.ReadJSON(&raw); err != nil {
			t.Fatal(err)
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		var kind string
		_ = json.Unmarshal(envelope["type"], &kind)
		if kind == "rpc_response" {
			var response wsResponse
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.ID == request.ID {
				return response
			}
			continue
		}
		viewReader(conn).apply(t, envelope)
	}
}

func writeRoomCommand(t *testing.T, conn *websocket.Conn, roomID, id string, encoded json.RawMessage) {
	t.Helper()
	response := callRoomRPC(t, conn, wsRequest{Type: "rpc_request", ID: id, Method: http.MethodPost, Path: "/api/v1/rooms/" + roomID + "/commands", Body: encoded})
	if response.Status != http.StatusOK {
		t.Fatalf("command %s status=%d body=%s", id, response.Status, response.Body)
	}
}

// compareViewJSON 比较"客户端合并出来的视图"和"服务端全量快照"，逐字段报差异。
func compareViewJSON(t *testing.T, label string, merged, snapshot any) {
	t.Helper()
	diffs := []string{}
	collectViewDiffs(&diffs, "view", merged, snapshot)
	if len(diffs) == 0 {
		return
	}
	const limit = 8
	shown := diffs
	if len(shown) > limit {
		shown = shown[:limit]
	}
	for _, diff := range shown {
		t.Errorf("%s: %s", label, diff)
	}
	t.Fatalf("%s: 合并后的视图和全量快照有 %d 处不一致", label, len(diffs))
}

func collectViewDiffs(diffs *[]string, path string, merged, snapshot any) {
	switch want := snapshot.(type) {
	case map[string]any:
		got, ok := merged.(map[string]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: 快照是对象，合并结果是 %T", path, merged))
			return
		}
		keys := make([]string, 0, len(want)+len(got))
		seen := map[string]bool{}
		for key := range got {
			keys = append(keys, key)
			seen[key] = true
		}
		for key := range want {
			if !seen[key] {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			wantValue, inSnapshot := want[key]
			gotValue, inMerged := got[key]
			if !inSnapshot {
				*diffs = append(*diffs, fmt.Sprintf("%s.%s: 合并结果多出字段（快照里没有），值=%v", path, key, gotValue))
				continue
			}
			if !inMerged {
				*diffs = append(*diffs, fmt.Sprintf("%s.%s: 合并结果缺字段，快照值=%v", path, key, wantValue))
				continue
			}
			collectViewDiffs(diffs, path+"."+key, gotValue, wantValue)
		}
	case []any:
		got, ok := merged.([]any)
		if !ok {
			*diffs = append(*diffs, fmt.Sprintf("%s: 快照是数组，合并结果是 %T", path, merged))
			return
		}
		if len(got) != len(want) {
			*diffs = append(*diffs, fmt.Sprintf("%s: 长度 %d（合并）vs %d（快照），合并=%v 快照=%v", path, len(got), len(want), keysOfList(got), keysOfList(want)))
			return
		}
		for index := range want {
			collectViewDiffs(diffs, fmt.Sprintf("%s[%d]", path, index), got[index], want[index])
		}
	default:
		if fmt.Sprintf("%v|%T", merged, merged) != fmt.Sprintf("%v|%T", snapshot, snapshot) {
			*diffs = append(*diffs, fmt.Sprintf("%s: 值 %v（%T，合并）vs %v（%T，快照）", path, merged, merged, snapshot, snapshot))
		}
	}
}

// keysOfList 只打印每个元素的主键，用来指认"顺序漂了"还是"内容少了"。
func keysOfList(list []any) []string {
	keys := make([]string, 0, len(list))
	for _, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			keys = append(keys, fmt.Sprint(raw))
			continue
		}
		keys = append(keys, fmt.Sprint(entry["id"], entry["unit_id"], entry["task_id"]))
	}
	return keys
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	}
	return nil
}
