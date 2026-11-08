package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"generalquarters/internal/game"
	"generalquarters/internal/store"
)

type testView struct {
	Turn      int               `json:"turn"`
	Version   int64             `json:"version"`
	Units     []game.Unit       `json:"units"`
	Fleets    []game.FleetGroup `json:"fleets"`
	You       store.Player      `json:"you"`
	Players   []store.Player    `json:"players"`
	Orders    []game.Order      `json:"orders"`
	JoinToken string            `json:"join_token"`
}

type testClient struct {
	client *http.Client
	base   string
}

func newTestClient(t *testing.T, base string) *testClient {
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{client: &http.Client{Jar: jar}, base: base}
}

func (c *testClient) call(t *testing.T, method, path string, input, output any) int {
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

func setupTestServer(t *testing.T) (*testClient, string, *store.Store, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	app := New(database, "../../content", false)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	httpServer := httptest.NewServer(mux)
	cleanup := func() { httpServer.Close(); database.Close() }
	return newTestClient(t, httpServer.URL), path, database, cleanup
}

func TestTwoPlayerCommandAndTurnFlow(t *testing.T) {
	a, dbPath, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A"}, &created); status != http.StatusCreated {
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

	invalid := map[string]any{"command_id": "invalid", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0404"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", invalid, nil); status != http.StatusBadRequest {
		t.Fatalf("非法移动状态码%d", status)
	}
	valid := map[string]any{"command_id": "move-1", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0203"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", valid, nil); status != http.StatusOK {
		t.Fatal("合法移动被拒绝")
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", valid, nil); status != http.StatusOK {
		t.Fatal("幂等重试失败")
	}
	var moved testView
	_ = a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &moved)
	if moved.Version != 2 || len(moved.Orders) != 0 {
		t.Fatalf("立即移动未更新版本或错误生成待结算命令：%+v", moved)
	}
	movedFound := false
	for _, unit := range moved.Units {
		if unit.ID == "a-bb-01" && unit.Coord == "0203" && unit.Moved {
			movedFound = true
		}
	}
	if !movedFound {
		t.Fatal("立即移动结果或移动状态未写入私有快照")
	}

	var readyView testView
	if status := b.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 2, "ready": true}, &readyView); status != http.StatusOK || readyView.Version != 2 {
		t.Fatal("单方ready不应结算")
	}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 2, "ready": true}, &readyView); status != http.StatusOK {
		t.Fatal("双方ready结算失败")
	}
	if readyView.Turn != 2 || readyView.Version != 3 {
		t.Fatalf("回合版本错误：%+v", readyView)
	}
	found := false
	for _, unit := range readyView.Units {
		if unit.ID == "a-bb-01" && unit.Coord == "0203" && !unit.Moved {
			found = true
		}
	}
	if !found {
		t.Fatal("新回合未重置移动状态")
	}

	stale := map[string]any{"command_id": "move-stale", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0204"}
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/commands", stale, nil); status != http.StatusConflict {
		t.Fatal("过期版本未被拒绝")
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
	_ = a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{}, &created)
	var view testView
	if status := a.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"base_version": 1, "ready": true}, &view); status != http.StatusOK || view.Turn != 2 {
		t.Fatalf("单人沙盒未结算：%+v", view)
	}
	other := newTestClient(t, a.base)
	if status := other.call(t, http.MethodPost, "/api/v1/rooms/join", map[string]string{"token": "XXXX-XXXX-XXXX"}, nil); status != http.StatusBadRequest {
		t.Fatal("无效token未被拒绝")
	}
}

func TestRoomActorSerializesAllJobs(t *testing.T) {
	app := &Server{}
	const jobCount = 64
	var active atomic.Int32
	var maximum atomic.Int32
	var completed int
	start := make(chan struct{})
	var workers sync.WaitGroup
	for index := 0; index < jobCount; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			app.submitRoom("same-room", func() {
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
			})
		}()
	}
	close(start)
	workers.Wait()
	if completed != jobCount {
		t.Fatalf("房间任务丢失：%d/%d", completed, jobCount)
	}
	if maximum.Load() != 1 {
		t.Fatalf("房间任务并发执行，最大并发数：%d", maximum.Load())
	}
}

func TestRoomActorContinuesAfterPanic(t *testing.T) {
	app := &Server{}
	app.submitRoom("same-room", func() { panic("test panic") })
	completed := false
	app.submitRoom("same-room", func() { completed = true })
	if !completed {
		t.Fatal("房间actor在任务panic后停止处理后续任务")
	}
}

func TestConcurrentCommandsHaveSingleWinner(t *testing.T) {
	a, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := a.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{}, &created); status != http.StatusCreated {
		t.Fatal("创建房间失败")
	}
	commands := []map[string]any{
		{"command_id": "concurrent-flag", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-bb-01", "coord": "0203"},
		{"command_id": "concurrent-screen", "base_version": 1, "type": "SET_MOVE", "unit_id": "a-dd-01", "coord": "0201"},
	}
	statuses := make(chan int, len(commands))
	errors := make(chan error, len(commands))
	for _, command := range commands {
		go func() {
			body, err := json.Marshal(command)
			if err != nil {
				errors <- err
				return
			}
			request, err := http.NewRequest(http.MethodPost, a.base+"/api/v1/rooms/"+created.RoomID+"/commands", bytes.NewReader(body))
			if err != nil {
				errors <- err
				return
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := a.client.Do(request)
			if err != nil {
				errors <- err
				return
			}
			responseBody, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Logf("并发命令响应：status=%d body=%s", response.StatusCode, responseBody)
			statuses <- response.StatusCode
		}()
	}
	accepted, conflicts := 0, 0
	for range commands {
		select {
		case err := <-errors:
			t.Fatal(err)
		case status := <-statuses:
			switch status {
			case http.StatusOK:
				accepted++
			case http.StatusConflict:
				conflicts++
			default:
				t.Fatalf("并发命令返回意外状态码：%d", status)
			}
		}
	}
	if accepted != 1 || conflicts != 1 {
		t.Fatalf("同一版本并发命令应只有一个成功：accepted=%d conflicts=%d", accepted, conflicts)
	}
	var view testView
	if status := a.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &view); status != http.StatusOK || view.Version != 2 {
		t.Fatalf("并发命令后权威版本错误：%+v", view)
	}
}
