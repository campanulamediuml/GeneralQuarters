package gqcli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPatchOnlyTriggersFullSnapshotRefresh(t *testing.T) {
	sendPatch := make(chan struct{})
	finished := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	host := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/rooms/join":
			http.SetCookie(writer, &http.Cookie{Name: "gq_session", Value: "session-a"})
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"room_id":"room-a","player_id":"player-a","faction":"A"}`))
		case "/ws":
			conn, err := upgrader.Upgrade(writer, request, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			var initial rpcRequest
			if err := conn.ReadJSON(&initial); err != nil || !strings.HasSuffix(initial.Path, "/state") {
				t.Errorf("首个 RPC 不是 GET state：%v, %#v", err, initial)
				return
			}
			writeRPC(t, conn, initial.ID, `{"turn":1,"phase":"battle","you":{"id":"player-a","faction":"A"},"fleets":[],"events":[{"id":1,"turn":1,"type":"battle_start","text":"开战"}]}`)
			<-sendPatch
			if err := conn.WriteJSON(map[string]any{"type": "state", "patch": map[string]any{"turn": 2}}); err != nil {
				return
			}
			var refreshed rpcRequest
			if err := conn.ReadJSON(&refreshed); err != nil || !strings.HasSuffix(refreshed.Path, "/state") {
				t.Errorf("补丁后没有重新 GET state：%v, %#v", err, refreshed)
				return
			}
			writeRPC(t, conn, refreshed.ID, `{"turn":2,"phase":"battle","you":{"id":"player-a","faction":"A"},"fleets":[],"events":[{"id":1,"turn":1,"type":"battle_start","text":"开战"},{"id":2,"turn":2,"type":"unit_moved","text":"移动"}]}`)
			<-finished
		default:
			http.NotFound(writer, request)
		}
	}))
	defer host.Close()
	defer close(finished)
	client := newStateTestClient(t)
	client.cfg.ServerURL = host.URL
	if _, err := client.join(context.Background(), joinParams{Token: "token", Faction: "A"}); err != nil {
		t.Fatal(err)
	}
	waitResult := make(chan any, 1)
	waitError := make(chan error, 1)
	go func() {
		result, err := client.wait(context.Background(), waitParams{TimeoutMS: 2000})
		if err != nil {
			waitError <- err
			return
		}
		waitResult <- result
	}()
	close(sendPatch)
	select {
	case err := <-waitError:
		t.Fatal(err)
	case result := <-waitResult:
		object := result.(map[string]any)
		var turn int
		raw, _ := json.Marshal(object["turn"])
		_ = json.Unmarshal(raw, &turn)
		if turn != 2 || object["resynced"] != false {
			t.Fatalf("全量刷新结果不正确：%#v", object)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("等待全量刷新超时")
	}
}

func writeRPC(t *testing.T, conn *websocket.Conn, id string, body string) {
	t.Helper()
	if err := conn.WriteJSON(rpcResponse{Type: "rpc_response", ID: id, Status: http.StatusOK, Body: json.RawMessage(body)}); err != nil {
		t.Errorf("写 RPC 响应失败：%v", err)
	}
}
