package server

import (
	"net/http"
	"testing"
	"time"
)

// settleDeployment 把房间从开局部署阶段推进到第 1 回合：每个玩家各点一次"部署完毕"。
// 部署不算回合，所以回合号不变——这是它和普通 ready 结算的关键区别。
// 房间不在部署阶段（老剧本/已开战）时什么都不做。
func settleDeployment(t *testing.T, roomID string, clients ...*testClient) {
	t.Helper()
	if len(clients) == 0 {
		return
	}
	phase := func(client *testClient) string {
		var view map[string]any
		if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
			t.Fatalf("读取状态失败：%d", status)
		}
		current, _ := view["phase"].(string)
		return current
	}
	if phase(clients[0]) != "deploy" {
		return
	}
	for _, client := range clients {
		if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+roomID+"/ready", map[string]any{"ready": true}, nil); status != http.StatusOK {
			t.Fatalf("部署完毕失败：%d", status)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if phase(clients[0]) == "battle" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("双方都点部署完毕后仍在部署阶段")
}

// deployStarted 判断房间是否还在部署阶段（用例据此决定要不要先摆船）。
func deployStarted(t *testing.T, client *testClient, roomID string) bool {
	t.Helper()
	var view map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatalf("读取状态失败：%d", status)
	}
	phase, _ := view["phase"].(string)
	return phase == "deploy"
}
