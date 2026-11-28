package server

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

func TestRoomActorRejectsSendWhenShutdownStarts(t *testing.T) {
	actor := newRoomActor(nil)
	actor.lifecycle.RLock()
	stopped := make(chan struct{})
	go func() {
		actor.shutdown()
		close(stopped)
	}()
	<-actor.stopping
	if actor.sendLocked(roomMsg{kind: msgState}) {
		t.Fatal("关闭开始后不得再接受消息")
	}
	actor.lifecycle.RUnlock()
	<-stopped
}

func TestRoomActorDrainsAcceptedMessagesOnShutdown(t *testing.T) {
	actor := newRoomActor(nil)
	started := make(chan struct{})
	release := make(chan struct{})
	actor.apply = func(msg roomMsg) roomReply {
		if msg.roomID == "block" {
			close(started)
			<-release
		}
		return roomReply{status: http.StatusOK}
	}
	actor.start.Do(func() { go actor.run() })
	if !actor.send(roomMsg{kind: msgState, roomID: "block"}) {
		t.Fatal("阻塞消息投递失败")
	}
	<-started
	reply := make(chan roomReply, 1)
	if !actor.send(roomMsg{kind: msgState, roomID: "queued", reply: reply}) {
		t.Fatal("排队消息投递失败")
	}
	actor.shutdown()
	close(release)
	select {
	case result := <-reply:
		if result.status != http.StatusOK {
			t.Fatalf("排队消息应答状态 %d", result.status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("关闭 actor 时丢失了已接受的消息")
	}
	if actor.send(roomMsg{kind: msgState}) {
		t.Fatal("关闭完成后仍接受了消息")
	}
}

func TestCloneRoomPreservesRuntimeOnlyState(t *testing.T) {
	original := store.Room{State: game.State{
		AircraftRoster: map[string]game.AirUnit{"air-a": {ID: "air-a"}},
		DeferredSink:   true,
		PendingSinks:   []*game.Unit{{ID: "ship-a"}},
	}}
	cloned, err := cloneRoom(original)
	if err != nil {
		t.Fatal(err)
	}
	if !cloned.State.DeferredSink || cloned.State.AircraftRoster["air-a"].ID != "air-a" || len(cloned.State.PendingSinks) != 1 {
		t.Fatalf("运行时字段在复制时丢失：%+v", cloned.State)
	}
	rosterEntry := cloned.State.AircraftRoster["air-a"]
	rosterEntry.ID = "changed"
	cloned.State.AircraftRoster["air-a"] = rosterEntry
	cloned.State.PendingSinks[0].ID = "changed"
	if original.State.AircraftRoster["air-a"].ID != "air-a" || original.State.PendingSinks[0].ID != "ship-a" {
		t.Fatal("状态副本仍与 actor 共享运行时 map 或指针")
	}
}

func TestCommandFingerprintIncludesBusinessPayload(t *testing.T) {
	tests := []struct {
		name   string
		base   commandRequest
		change func(*commandRequest)
	}{
		{
			name: "reorganization assignments",
			base: commandRequest{Type: "REORGANIZE_HEX", UnitID: "hex-0202", Coord: "0202", Assignments: map[string]string{"a": "fleet-a"}},
			change: func(request *commandRequest) {
				request.Assignments = map[string]string{"a": "fleet-b"}
			},
		},
		{
			name: "air task lane",
			base: commandRequest{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"air-a"}},
			change: func(request *commandRequest) {
				request.Lane = 2
			},
		},
		{
			name: "air cycle flag",
			base: commandRequest{Type: "SET_AIR_CYCLE", AircraftIDs: []string{"air-a"}},
			change: func(request *commandRequest) {
				request.Cycle = true
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := test.base
			test.change(&changed)
			if commandFingerprint(test.base) == commandFingerprint(changed) {
				t.Fatal("业务内容不同的命令不能使用相同指纹")
			}
		})
	}
}

func TestFailedImmediateCommandDoesNotMutateActorState(t *testing.T) {
	client, _, database, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码 %d", status)
	}
	var before testView
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &before); status != http.StatusOK {
		t.Fatal("读取初始状态失败")
	}
	originalHeading := ""
	for _, unit := range before.Units {
		if unit.ID == "a-bb-01" {
			originalHeading = unit.Heading
		}
	}
	if originalHeading == "" {
		t.Fatal("测试舰船不存在或没有朝向")
	}
	if _, err := database.DB.Exec(`CREATE TRIGGER reject_room_update BEFORE UPDATE ON rooms BEGIN SELECT RAISE(FAIL, 'forced room update failure'); END;`); err != nil {
		t.Fatal(err)
	}
	command := map[string]any{"command_id": "heading-db-failure", "type": "SET_HEADING", "unit_id": "a-bb-01", "heading": "E"}
	if originalHeading == "E" {
		command["heading"] = "W"
	}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, nil); status != http.StatusConflict {
		t.Fatalf("写库失败应返回 409，实际 %d", status)
	}
	var after testView
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &after); status != http.StatusOK {
		t.Fatal("读取失败后状态失败")
	}
	for _, unit := range after.Units {
		if unit.ID == "a-bb-01" && unit.Heading != originalHeading {
			t.Fatalf("写库失败的命令污染了内存状态：朝向从 %s 变为 %s", originalHeading, unit.Heading)
		}
	}
}

func TestPanickedTurnResolutionDoesNotCommitPartialState(t *testing.T) {
	client, _, database, cleanup := setupTestServer(t, func(app *Server) {
		app.resolveOrders = func(*game.State, []game.Order) []game.EngineEvent {
			panic("forced resolution panic")
		}
	})
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码 %d", status)
	}
	settleDeployment(t, created.RoomID, client)
	before, err := database.Room(t.Context(), created.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusInternalServerError {
		t.Fatalf("结算 panic 应返回 500，实际 %d", status)
	}
	after, err := database.Room(t.Context(), created.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Turn != before.Turn || after.Version != before.Version || !reflect.DeepEqual(after.State, before.State) {
		t.Fatal("结算 panic 后不得推进回合、版本或提交部分状态")
	}
}

func TestRadioOrderCanBeClearedByTaskID(t *testing.T) {
	client, _, database, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码 %d", status)
	}
	room, err := database.Room(t.Context(), created.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	taskID := "air-http-clear"
	aircraft := room.State.Aircraft["a2-cv-01-dive_bomber-x1"]
	aircraft.Status = "attack"
	aircraft.TaskID = taskID
	aircraft.TaskKind = "attack"
	aircraft.Coord = room.State.Units[aircraft.CarrierID].Coord
	if err := database.ReplaceState(t.Context(), created.RoomID, room.State); err != nil {
		t.Fatal(err)
	}
	reloaded, err := database.Room(t.Context(), created.RoomID)
	if err != nil || reloaded.State.Aircraft[aircraft.ID].Status != "attack" || reloaded.State.Aircraft[aircraft.ID].TaskID != taskID {
		t.Fatalf("测试空中编队未正确写入：%+v, %v", reloaded.State.Aircraft[aircraft.ID], err)
	}
	var view testView
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatal("读取航空任务失败")
	}
	var radioResponse map[string]any
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{"command_id": "radio-clear-recall", "type": "RECALL_AIR", "task_id": taskID}, &radioResponse); status != http.StatusOK {
		t.Fatalf("提交无线电召回状态码 %d：%+v", status, radioResponse)
	}
	orders, err := database.Orders(t.Context(), created.RoomID, view.Turn, created.PlayerID)
	if err != nil || len(orders) != 1 || orders[0].Order.TaskID != taskID {
		t.Fatalf("无线电订单未按任务保存：%+v, %v", orders, err)
	}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", map[string]any{"command_id": "radio-clear-undo", "type": "CLEAR_MOVE", "task_id": taskID}, nil); status != http.StatusOK {
		t.Fatalf("按 task_id 撤销无线电命令失败")
	}
	orders, err = database.Orders(t.Context(), created.RoomID, view.Turn, created.PlayerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 0 {
		t.Fatalf("撤销后无线电订单仍存在：%+v", orders)
	}
}

func TestFailedPlannedOrderCanBeRetried(t *testing.T) {
	client, _, database, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码 %d", status)
	}
	var initial testView
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial); status != http.StatusOK {
		t.Fatal("读取初始状态失败")
	}
	unitID, destination := "", ""
	for _, unit := range initial.Units {
		if unit.Faction == initial.You.Faction && len(unit.LegalMoves) > 0 {
			unitID, destination = unit.ID, unit.LegalMoves[0]
			break
		}
	}
	if unitID == "" {
		t.Fatal("找不到可移动的测试舰队")
	}
	if _, err := database.DB.Exec(`CREATE TRIGGER reject_order_insert BEFORE INSERT ON orders BEGIN SELECT RAISE(FAIL, 'forced order insert failure'); END;`); err != nil {
		t.Fatal(err)
	}
	command := map[string]any{"command_id": "move-db-failure", "type": "SET_MOVE", "unit_id": unitID, "coord": destination}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, nil); status != http.StatusInternalServerError {
		t.Fatalf("订单写库失败应返回 500，实际 %d", status)
	}
	if _, err := database.DB.Exec(`DROP TRIGGER reject_order_insert`); err != nil {
		t.Fatal(err)
	}
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, nil); status != http.StatusOK {
		t.Fatalf("故障恢复后重试应成功，实际 %d", status)
	}
	orders, err := database.Orders(t.Context(), created.RoomID, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].Order.Type != "SET_MOVE" || orders[0].Order.Coord != destination {
		t.Fatalf("成功回执必须对应真实订单：%+v", orders)
	}
}
