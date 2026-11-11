package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"generalquarters/internal/game"
)

func TestAviationQueueThroughREST(t *testing.T) {
	client, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	commands := []map[string]any{
		{"command_id": "cap-1", "base_version": 1, "type": "LAUNCH_CAP", "aircraft_ids": []string{"air-a-01"}},
		{"command_id": "recon-1", "base_version": 2, "type": "LAUNCH_RECON", "aircraft_ids": []string{"air-a-10"}, "heading": "W", "sector_width": 60},
		{"command_id": "priority-1", "base_version": 3, "type": "PRIORITIZE_AIR_TASK", "aircraft_id": "air-a-10"},
	}
	for _, command := range commands {
		var response map[string]any
		if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, &response); status != http.StatusOK {
			t.Fatalf("航空命令%s状态码%d：%+v", command["type"], status, response)
		}
	}
	var view testView
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatal("读取航空队列失败")
	}
	if len(view.Aviation.Tasks) != 2 || view.Aviation.Tasks[0].Kind != "recon" || view.Aviation.Tasks[0].QueuePosition != 1 || view.Aviation.Tasks[1].QueuePosition != 2 {
		t.Fatalf("REST队列投影错误：%+v", view.Aviation.Tasks)
	}
}

func TestAirCombatResolutionThroughREST(t *testing.T) {
	client, _, database, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	room, err := database.Room(context.Background(), created.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	room.State.Aircraft = map[string]*game.AirUnit{
		"strike-a": {ID: "strike-a", CarrierID: "a2-cv-01", Faction: "A", Model: "SB2C", Kind: "dive_bomber", Status: "attack", Strength: 2, AirCombat: 2, AntiShip: 4, TaskID: "strike-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 12, MaxRange: 12},
		"escort-a": {ID: "escort-a", CarrierID: "a2-cv-01", Faction: "A", Model: "F6F", Kind: "fighter", Status: "attack", Strength: 2, AirCombat: 5, TaskID: "strike-a", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 13, MaxRange: 13},
		"cap-b":    {ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Model: "A6M", Kind: "fighter", Status: "cap", Strength: 2, AirCombat: 5, TaskID: "cap-b", TaskKind: "cap", Coord: "0608", RemainingRange: 13, MaxRange: 13},
	}
	room.State.Units["b-cv-01"].Coord = "0608"
	room.State.RNGState = 42
	stateJSON, err := json.Marshal(room.State)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(context.Background(), "update rooms set state_json=? where id=?", stateJSON, created.RoomID); err != nil {
		t.Fatal(err)
	}
	var resolved testView
	if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 1, "ready": true}, &resolved); status != http.StatusOK {
		t.Fatalf("Ready结算状态码%d", status)
	}
	if resolved.Turn != 2 || len(resolved.AirCombats) != 1 {
		t.Fatalf("REST没有返回空战报告：回合%d 报告%+v", resolved.Turn, resolved.AirCombats)
	}
	report := resolved.AirCombats[0]
	if report.AttackerCount != 2 || report.DefenderCount != 1 || report.AttackerFaction != "A" || report.DefenderFaction != "B" {
		t.Fatalf("REST空战报告内容错误：%+v", report)
	}
	persisted, err := database.Room(context.Background(), created.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.State.AirCombats) != 1 || persisted.State.AirCombats[0].ID != report.ID {
		t.Fatalf("空战报告没有持久化：%+v", persisted.State.AirCombats)
	}
}

func TestCrossCarrierStrikeUsesSingleTaskThroughREST(t *testing.T) {
	client, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := client.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A"}, &created); status != http.StatusCreated {
		t.Fatalf("创建房间状态码%d", status)
	}
	commands := []map[string]any{
		{"command_id": "strike-1", "base_version": 1, "type": "LAUNCH_STRIKE", "aircraft_ids": []string{"air-a-03"}},
		{"command_id": "strike-2", "base_version": 2, "type": "LAUNCH_STRIKE", "aircraft_ids": []string{"air-a-06"}, "task_id": "air-a-03"},
		{"command_id": "strike-3", "base_version": 3, "type": "LAUNCH_STRIKE", "aircraft_ids": []string{"air-a-08"}, "task_id": "air-a-03"},
	}
	for _, command := range commands {
		var response map[string]any
		if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", command, &response); status != http.StatusOK {
			t.Fatalf("跨母舰攻击命令状态码%d：%+v", status, response)
		}
	}
	var view testView
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatal("读取攻击编队失败")
	}
	if len(view.Aviation.Tasks) != 1 {
		t.Fatalf("同一攻击任务被拆成多个编队：%+v", view.Aviation.Tasks)
	}
	task := view.Aviation.Tasks[0]
	if task.ID != "air-a-03" || len(task.AircraftIDs) != 3 || len(task.CarrierIDs) != 3 {
		t.Fatalf("跨母舰没有加入同一task_id：%+v", task)
	}
}
