package server

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// 观战主入口是 /watch/{token}：服务端把 token 解析成房间，页面不必再拼 room ID。
func TestWatchPageResolvesRoomFromTokenOnlyURL(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	watcher := newTestClient(t, host.base)
	body, status := fetchWatchPage(t, watcher, "/watch/"+created.JoinToken)
	if status != http.StatusOK {
		t.Fatalf("观战页状态码 %d", status)
	}
	if !strings.Contains(body, "var room = "+strconv.Quote(created.RoomID)) {
		t.Fatalf("观战页没有把 token 解析成房间：%s", head(body, 400))
	}
	if strings.Contains(body, "setInterval") {
		t.Fatal("观战页仍在轮询")
	}
	if !strings.Contains(body, "?mode=spectator") {
		t.Fatal("观战连接没有带上 spectator 模式")
	}
}

// 已有玩家 Cookie 的浏览器打开观战页时，必须按观战处理而不是复用玩家连接。
func TestWatchPageTokenOnlyWithPlayerCookie(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	var created roomResponse
	if status := host.call(t, http.MethodPost, "/api/v1/rooms", map[string]string{"name": "A", "scenario_id": "_fixture"}, &created); status != http.StatusCreated {
		t.Fatalf("create room status %d", status)
	}
	body, status := fetchWatchPage(t, host, "/watch/"+created.JoinToken)
	if status != http.StatusOK {
		t.Fatalf("观战页状态码 %d", status)
	}
	if !strings.Contains(body, "var room = "+strconv.Quote(created.RoomID)) {
		t.Fatal("带玩家 Cookie 时观战页没有解析房间")
	}
}

func TestWatchPageRejectsUnknownToken(t *testing.T) {
	host, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	if _, status := fetchWatchPage(t, host, "/watch/XXXX-XXXX-XXXX"); status != http.StatusNotFound {
		t.Fatalf("未知 token 应返回 404，实际 %d", status)
	}
}

func fetchWatchPage(t *testing.T, client *testClient, path string) (string, int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, client.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var builder strings.Builder
	buffer := make([]byte, 4096)
	for {
		n, readErr := response.Body.Read(buffer)
		builder.Write(buffer[:n])
		if readErr != nil {
			break
		}
	}
	return builder.String(), response.StatusCode
}

func head(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
