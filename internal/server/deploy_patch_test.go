package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

// 部署阶段切到开战时，服务端会把一大批字段一次改完（phase、deploy_zone、units/fleets 的
// legal_moves）。这条用例直接把那一帧补丁按客户端的合并语义打回底座，要求结果和全量快照一致：
// 之前这里发出的是实体列表补丁，但客户端的底座被合成了对象，于是 players.filter 直接抛错，
// 整个页面渲染崩成空白。
func TestDeployToBattlePatchMerges(t *testing.T) {
	database, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	app := New(database, testContentRoot(t), false)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := newTestClient(t, server.URL)

	created := createTestPlayerRoom(t, client, "_coral_real", "A", true)
	var base map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &base); status != http.StatusOK {
		t.Fatalf("读取状态失败：%d", status)
	}
	if base["phase"] != "deploy" {
		t.Fatalf("新房间应在部署阶段，实际 %v", base["phase"])
	}

	actor, err := app.actorRoom(created.RoomID)
	if err != nil {
		t.Fatal(err)
	}
	var player store.Player
	for _, item := range actor.players {
		player = item
	}
	ctx := context.Background()
	prev, err := app.privateView(ctx, actor.room, actor.players, player)
	if err != nil {
		t.Fatal(err)
	}
	game.StartBattle(&actor.room.State)
	next, err := app.privateView(ctx, actor.room, actor.players, player)
	if err != nil {
		t.Fatal(err)
	}
	patch := diffViewWith(actor.meta, prev, next)
	rawPatch, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}
	var patchAny any
	if err := json.Unmarshal(rawPatch, &patchAny); err != nil {
		t.Fatal(err)
	}
	merged := applyTestPatch(base, patchAny)
	rawNext, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	var nextAny any
	if err := json.Unmarshal(rawNext, &nextAny); err != nil {
		t.Fatal(err)
	}
	compareViewJSON(t, "deploy->battle", merged, nextAny)
}
