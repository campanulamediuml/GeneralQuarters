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
//
// 这里不数服务端内部的投影次数，而是直接数观战连接收到了几帧：
// 合并没生效就会多出中间态。房间内部的状态不该为了"被测试看一眼"而引入原子量，
// 而"客户端看到几帧"本来就是玩家能感知的那一面。
func TestSpectatorCoalescesBroadcastBurst(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	created := createTestPlayerRoom(t, host, "_fixture", "A", true)
	// 先完成玩家连接的上线通知，避免把它误算进后面的回合结算广播。
	var initial testView
	if status := host.call(t, http.MethodGet, "/api/v1/rooms/"+created.RoomID+"/state", nil, &initial); status != http.StatusOK {
		t.Fatalf("initial state status %d", status)
	}
	watcher := newTestClient(t, host.base)
	conn := dialTestWS(t, watcher, "/ws")
	defer conn.Close()
	_ = conn.WriteJSON(map[string]string{"type": "subscribe", "room_id": created.RoomID, "token": created.JoinToken})
	if view := readStateView(t, conn, 1); view["turn"] != float64(1) {
		t.Fatalf("首帧回合=%v", view["turn"])
	}

	if status := host.call(t, http.MethodPost, "/api/v1/rooms/"+created.RoomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
		t.Fatalf("ready status %d", status)
	}
	// 数到结算后那一帧为止：整批广播只该产生一帧。
	reader := viewReader(conn)
	frames := 0
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var envelope map[string]json.RawMessage
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatal(err)
		}
		if !reader.apply(t, envelope) {
			continue
		}
		frames++
		if reader.view["turn"] == float64(2) {
			break
		}
	}
	if frames != 1 {
		t.Fatalf("一次结算给观战推了 %d 帧，广播合并没生效", frames)
	}
}
