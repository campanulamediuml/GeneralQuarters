package server

import (
	"fmt"
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
		placeSubmarines(t, client, roomID)
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

// placeSubmarines 把本方还没摆的潜艇摆进部署区：
// 潜艇没摆完时服务端会拒绝"部署完毕"，所以每个走部署流程的用例都要先过这一步。
// 落点直接借用己方单位的 legal_moves（部署阶段就是整片部署区）。
func placeSubmarines(t *testing.T, client *testClient, roomID string) {
	t.Helper()
	var view map[string]any
	if status := client.call(t, http.MethodGet, "/api/v1/rooms/"+roomID+"/state", nil, &view); status != http.StatusOK {
		t.Fatalf("读取状态失败：%d", status)
	}
	you, _ := view["you"].(map[string]any)
	faction, _ := you["faction"].(string)
	submarines, _ := view["submarines"].([]any)
	if len(submarines) == 0 {
		return
	}
	coords := []string{}
	for _, raw := range view["units"].([]any) {
		unit, _ := raw.(map[string]any)
		if unit["faction"] != faction {
			continue
		}
		moves, _ := unit["legal_moves"].([]any)
		for _, move := range moves {
			coord, _ := move.(string)
			if coord != "" {
				coords = append(coords, coord)
			}
		}
		if len(coords) > 0 {
			break
		}
	}
	if len(coords) == 0 {
		t.Fatal("部署阶段找不到潜艇落点：己方单位没有 legal_moves")
	}
	index := 0
	for _, raw := range submarines {
		sub, _ := raw.(map[string]any)
		if deployed, _ := sub["deployed"].(bool); deployed {
			continue
		}
		id, _ := sub["id"].(string)
		payload := map[string]any{
			"command_id": fmt.Sprintf("deploy-sub-%s-%d", id, index),
			"type":       "DEPLOY_SUB",
			"unit_id":    id,
			"coord":      coords[index%len(coords)],
		}
		if status := client.call(t, http.MethodPost, "/api/v1/rooms/"+roomID+"/commands", payload, nil); status != http.StatusOK {
			t.Fatalf("部署潜艇 %s 失败：%d", id, status)
		}
		index++
	}
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
