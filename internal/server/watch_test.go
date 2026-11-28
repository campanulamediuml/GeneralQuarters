package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

type spectateView struct {
	Turn       int                        `json:"turn"`
	Units      []game.Unit                `json:"units"`
	Fleets     []game.FleetGroup          `json:"fleets"`
	Airfields  []game.AirfieldContact     `json:"airfields"`
	GroundAir  []game.AirfieldAirStatus   `json:"ground_air"`
	Aviation   game.AviationView          `json:"aviation"`
	Events     []store.Event              `json:"events"`
	Losses     map[string]game.LossLedger `json:"losses"`
	Submarines []game.Submarine           `json:"submarines"`
	Spectator  bool                       `json:"spectator"`
	// 观战页要摊开双方的每一笔战果分：这两个字段必须一起下发，明细之和还要等于总分。
	Scores         map[string]float64          `json:"scores"`
	ScoreBreakdown map[string][]game.ScoreLine `json:"score_breakdown"`
}

// TestSpectateMergesBothFactions 校验观战快照：两边各自的合法视图按单位归属合并，
// 且必须用 join_token 认证——玩家 Cookie 那套权限不能拿来偷看对方。
func TestSpectateMergesBothFactions(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestPlayerRoom(t, host, "_coral_real", "A", false)
	guest := newTestClient(t, host.base)
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B", "faction": "B"}, nil); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	watcher := newTestClient(t, host.base)
	conn := dialTestWS(t, watcher, "/ws?mode=spectator")
	if err := conn.WriteJSON(map[string]string{"type": "subscribe", "room_id": created.RoomID, "token": created.JoinToken}); err != nil {
		t.Fatal(err)
	}
	snapshot := readStateView(t, conn, 1)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var view spectateView
	if err := json.Unmarshal(encoded, &view); err != nil {
		t.Fatal(err)
	}
	if !view.Spectator {
		t.Fatal("观战快照没有标记 spectator")
	}
	// 得分明细：开局还没战果，但两个阵营的账本必须都在，观战页才能画出"双方得分"面板。
	for _, side := range []string{"A", "B"} {
		if _, ok := view.Scores[side]; !ok {
			t.Fatalf("观战快照缺少 %s 的战果分：%v", side, view.Scores)
		}
		if _, ok := view.ScoreBreakdown[side]; !ok {
			t.Fatalf("观战快照缺少 %s 的得分明细：%v", side, view.ScoreBreakdown)
		}
	}
	sides := map[string]bool{}
	for _, unit := range view.Units {
		sides[unit.Faction] = true
	}
	if !sides["A"] || !sides["B"] {
		t.Fatalf("观战快照缺少某一方的舰船：%v", sides)
	}
	fleetSides := map[string]bool{}
	for _, fleet := range view.Fleets {
		fleetSides[fleet.Faction] = true
	}
	if !fleetSides["A"] || !fleetSides["B"] {
		t.Fatalf("观战快照缺少某一方的舰队：%v", fleetSides)
	}
	// 机场也要合并：珊瑚海剧本里 A 有莫尔兹比港、凯恩斯和鲍恩，B 有拉包尔、莱城、肖特兰和莱卡塔。
	airfieldSides := map[string]int{}
	for _, airfield := range view.Airfields {
		airfieldSides[airfield.Faction]++
	}
	if airfieldSides["A"] == 0 || airfieldSides["B"] == 0 {
		t.Fatalf("观战快照缺少某一方的机场：%v", airfieldSides)
	}
	// 基地卡片要能点出属性与驻机：观战快照的 ground_air 必须带完整状态和机上名单，
	// 不能只剩一个坐标——否则点开基地只有名字，没有跑道、防空和机库数。
	if len(view.GroundAir) == 0 {
		t.Fatal("观战快照缺少 ground_air")
	}
	listed := 0
	for _, airfield := range view.GroundAir {
		if airfield.Coord == "" || airfield.MaxRunways == 0 {
			t.Fatalf("基地%s缺少坐标或跑道数据：%+v", airfield.Name, airfield)
		}
		if len(airfield.Aircraft) > 0 {
			listed++
			if airfield.Aircraft[0].Model == "" {
				t.Fatalf("基地%s驻机缺少机型", airfield.Name)
			}
		}
	}
	if listed == 0 {
		t.Fatal("观战快照里没有任何驻机数据，基地详情面板会是空的")
	}
	badConn := dialTestWS(t, watcher, "/ws?mode=spectator")
	if err := badConn.WriteJSON(map[string]string{"type": "subscribe", "room_id": created.RoomID, "token": "XXXX-XXXX-XXXX"}); err != nil {
		t.Fatal(err)
	}
	if frame := readWSEvent(t, badConn, "error"); len(frame["error"]) == 0 {
		t.Fatal("错误 token 应返回观战错误帧")
	}
}

func TestSpectatorSeesBothSubmarineDeployments(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestPlayerRoom(t, host, "_coral_real", "A", false)
	guest := newTestClient(t, host.base)
	if status := guest.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": created.JoinToken, "name": "B", "faction": "B"}, nil); status != http.StatusOK {
		t.Fatalf("加入房间状态码%d", status)
	}
	placeSubmarines(t, host, created.RoomID)
	placeSubmarines(t, guest, created.RoomID)

	watcher := newTestClient(t, host.base)
	conn := dialTestWS(t, watcher, "/ws?mode=spectator")
	if err := conn.WriteJSON(map[string]string{"type": "subscribe", "room_id": created.RoomID, "token": created.JoinToken}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(readStateView(t, conn, 1))
	if err != nil {
		t.Fatal(err)
	}
	var view spectateView
	if err := json.Unmarshal(encoded, &view); err != nil {
		t.Fatal(err)
	}
	deployable := map[string]int{}
	deployed := map[string]int{}
	for _, submarine := range view.Submarines {
		if !submarine.Reserve {
			deployable[submarine.Faction]++
		}
		if submarine.Deployed && submarine.Coord != "" {
			deployed[submarine.Faction]++
		}
	}
	for _, faction := range []string{"A", "B"} {
		if deployable[faction] != 6 || deployed[faction] != 9 {
			t.Fatalf("观战应看到 %s 的 6 艘部署艇和 3 艘预备艇：deployable=%d deployed=%d", faction, deployable[faction], deployed[faction])
		}
	}

	playerView := fetchViewMap(t, host, created.RoomID)
	for _, raw := range playerView["submarines"].([]any) {
		submarine := raw.(map[string]any)
		if submarine["faction"] != "A" {
			t.Fatalf("玩家视图不应因观战功能泄露对方潜艇：%+v", submarine)
		}
	}
}

// 观战页的基地侦察扇面来自 ground_tasks。以前只合并了舰队航空，
// 机场侦察机飞出去后整条搜索带都不显示，玩家只能看见一个孤零零的机场牌。
func TestSpectatorGroundTasksCarryReconSectors(t *testing.T) {
	scenario, gameMap, catalog, err := game.LoadScenario(testContentRoot(t), "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := game.NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	pby := state.GroundAir["af-a-vila-pby-1"]
	if pby == nil {
		t.Fatal("缺少维拉港 PBY")
	}
	airfield := state.Airfields[pby.AirfieldID]
	if airfield == nil {
		t.Fatal("缺少维拉港机场")
	}
	pby.Status = "recon"
	pby.TaskID = "recon-vila"
	pby.TaskKind = "recon"
	pby.Heading = "W"
	pby.SectorWidth = 60
	pby.Coord = airfield.Coord
	pby.OriginCoord = airfield.Coord
	pby.ReconRadius = game.GroundReconRadius(pby)
	pby.ReconDistance = 0

	tasks := spectatorGroundTasks(state)
	if len(tasks) != 1 {
		t.Fatalf("观战机场任务数=%d，期望1", len(tasks))
	}
	task := tasks[0]
	if task.Faction != "A" {
		t.Fatalf("机场任务 faction=%q，期望 A", task.Faction)
	}
	if len(task.ReconSectors) != 1 {
		t.Fatalf("机场侦察任务缺少 recon_sectors：%+v", task)
	}
	if len(task.ReconSectors[0].SectorHexes) == 0 {
		t.Fatal("机场侦察扇面没有 hex，观战页画不出来")
	}
}

// 观战视图也必须是值快照：岸头物资、舰船损伤这些都是房间内存里原地改的，
// 视图要是共享那张表，"上一帧"会跟着变，补丁就算不出差异，观战页会停在旧画面。
func TestSpectateViewIsSnapshot(t *testing.T) {
	scenario, gameMap, catalog, err := game.LoadScenario(testContentRoot(t), "_fixture")
	if err != nil {
		t.Fatal(err)
	}
	state, err := game.NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	room := store.Room{ID: "room-1", Status: "running", Turn: 1, State: state}
	view := buildSpectateView(room, nil, nil)
	before, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if state.ShoreSupplies == nil {
		state.ShoreSupplies = map[string]int{}
	}
	state.ShoreSupplies["A|0101"] = 99
	for _, unit := range state.Units {
		if unit.Damage == nil {
			unit.Damage = &game.ShipDamage{}
		}
		unit.Damage.Fire = "fire"
		if unit.Deck != nil {
			unit.Deck.Length += 3
		}
	}
	after, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("观战视图跟着房间状态一起变了：视图必须是值快照")
	}
}
