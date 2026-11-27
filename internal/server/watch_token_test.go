package server

import (
	"net/http"
	"testing"
)

// 观战入口现在拆成两半：/watch/{token} 是前端 SPA 路由，服务端只提供
// /api/v1/watch/{token} 把 token 解析成 room_id，页面据此再连 /ws?mode=spectator。
func TestWatchTokenResolvesRoom(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	var resolved struct {
		RoomID string `json:"room_id"`
	}
	if status := host.call(t, http.MethodGet, "/api/v1/watch/"+created.JoinToken, nil, &resolved); status != http.StatusOK {
		t.Fatalf("解析观战 token 状态码 %d", status)
	}
	if resolved.RoomID != created.RoomID {
		t.Fatalf("token 解析出 %q，期望 %q", resolved.RoomID, created.RoomID)
	}
}

// 观战不带玩家 Cookie 也必须能解析：页面本身就是公开只读的，认证靠 token。
func TestWatchTokenIgnoresPlayerCookie(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	anonymous := newTestClient(t, host.base)
	var resolved struct {
		RoomID string `json:"room_id"`
	}
	if status := anonymous.call(t, http.MethodGet, "/api/v1/watch/"+created.JoinToken, nil, &resolved); status != http.StatusOK || resolved.RoomID != created.RoomID {
		t.Fatalf("匿名解析失败：status=%d room=%q", status, resolved.RoomID)
	}
}

func TestWatchTokenRejectsUnknown(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	status := host.call(t, http.MethodGet, "/api/v1/watch/XXXX-XXXX-XXXX", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("未知 token 应返回 404，实际 %d", status)
	}
	status = host.call(t, http.MethodGet, "/api/v1/watch/short", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("长度不对的 token 应返回 404，实际 %d", status)
	}
}
