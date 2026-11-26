package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// 一次回合结算会连着广播好几条事件（ready_changed、turn_resolved、presence_changed）。
// 观战端应该把这一批合并成一次投影、只下发一帧最终态；
// 以前是每条通知都整份重算，一次结算就是 4 次投影、4 帧几百 KB 的中间态。
func TestSpectatorCoalescesBroadcastBurst(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]any{"name": "A", "scenario_id": "_fixture", "solo": true}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	watcher := newTestClient(t, host.base)
	conn := dialTestWS(t, watcher, "/ws")
	defer conn.Close()
	_ = conn.WriteJSON(map[string]string{"type": "subscribe", "room_id": created.RoomID, "token": created.JoinToken})
	if view := readSnapshot(t, conn); view["turn"] != float64(1) {
		t.Fatalf("首帧回合=%v", view["turn"])
	}

	before := spectateViews.Load()
	if status := host.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("ready status %d", status)
	}
	// 合并窗口内的中间态可能先到一帧，但最终必须落到结算后的状态。
	var last map[string]any
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		last = readSnapshot(t, conn)
		if last["turn"] == float64(2) {
			break
		}
	}
	if last["turn"] != float64(2) {
		t.Fatalf("观战没收到结算后的状态：%v", last["turn"])
	}
	after := spectateViews.Load()
	// 结算的几条广播都在同一个房间任务里，投影只可能发生在任务结束之后，
	// 所以这里必须是 1（老实现是每条通知一次，至少 2 次）。
	if projections := after - before; projections != 1 {
		t.Fatalf("一次结算投影了 %d 次，广播合并没生效", projections)
	}
}

func readSnapshot(t *testing.T, conn interface {
	SetReadDeadline(time.Time) error
	ReadJSON(any) error
}) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var envelope map[string]json.RawMessage
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatal(err)
		}
		var kind string
		_ = json.Unmarshal(envelope["type"], &kind)
		if kind != "snapshot" {
			continue
		}
		var view map[string]any
		if err := json.Unmarshal(envelope["view"], &view); err != nil {
			t.Fatal(err)
		}
		return view
	}
}
