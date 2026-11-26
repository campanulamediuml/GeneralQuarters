package store

import (
	"context"
	"testing"
	"time"

	"generalquarters/internal/game"
)

// 闲置房间的判定：活动时间早于门槛、而且没人在线，两条都成立才回收。
// 玩家上线会刷新活动时间，所以"刚有人看过"的房间不会被误删。
func TestIdleRoomsOnlyDropsDisconnectedOnes(t *testing.T) {
	ctx := context.Background()
	database := newTestStore(t)
	state := game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 1}
	makeRoom := func(id string) {
		room := Room{ID: id, JoinToken: "tok-" + id, Status: "running", Turn: 1, Version: 1, State: state}
		if err := database.CreateRoom(ctx, room, Player{ID: "p-" + id, Faction: "A", Name: "甲", IsHost: true}, "hash-"+id); err != nil {
			t.Fatalf("建房失败：%v", err)
		}
	}
	makeRoom("idle")
	makeRoom("live")

	// 两间都闲置 13 小时，然后 live 里有人上线：上线本身就算一次活动。
	old := time.Now().UTC().Add(-13 * time.Hour).Format(time.RFC3339Nano)
	if _, err := database.DB.Exec("UPDATE rooms SET updated_at=?", old); err != nil {
		t.Fatalf("改写活动时间失败：%v", err)
	}
	if err := database.SetConnected(ctx, "p-live", true); err != nil {
		t.Fatalf("置在线失败：%v", err)
	}

	ids, err := database.IdleRoomIDs(ctx, 12*time.Hour)
	if err != nil {
		t.Fatalf("查闲置房间失败：%v", err)
	}
	if len(ids) != 1 || ids[0] != "idle" {
		t.Fatalf("只该回收 idle，实际 %v", ids)
	}

	if err := database.DeleteRooms(ctx, ids); err != nil {
		t.Fatalf("删除闲置房间失败：%v", err)
	}
	if _, err := database.Room(ctx, "idle"); err == nil {
		t.Fatal("闲置房间还留在库里")
	}
	if _, err := database.Room(ctx, "live"); err != nil {
		t.Fatalf("在线房间被误删：%v", err)
	}
	// 战报、快照、玩家会话都要跟着走，不然库里只剩一堆孤儿行。
	for _, query := range []string{
		"SELECT COUNT(*) FROM events WHERE room_id='idle'",
		"SELECT COUNT(*) FROM turns WHERE room_id='idle'",
		"SELECT COUNT(*) FROM orders WHERE room_id='idle'",
		"SELECT COUNT(*) FROM players WHERE room_id='idle'",
		"SELECT COUNT(*) FROM sessions WHERE room_id='idle'",
	} {
		var count int
		if err := database.DB.QueryRow(query).Scan(&count); err != nil {
			t.Fatalf("统计失败：%v", err)
		}
		if count != 0 {
			t.Fatalf("%s 还有 %d 行没删", query, count)
		}
	}
}

// 服务重启后谁都不在线：ResetConnections 必须把上次留下的在线标记清掉，
// 否则闲置房间永远满足不了"没人在线"这个条件。
func TestResetConnectionsClearsStaleFlags(t *testing.T) {
	ctx := context.Background()
	database := newTestStore(t)
	state := game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 1}
	room := Room{ID: "r1", JoinToken: "tok", Status: "running", Turn: 1, Version: 1, State: state}
	if err := database.CreateRoom(ctx, room, Player{ID: "p1", Faction: "A", Name: "甲", IsHost: true}, "hash"); err != nil {
		t.Fatalf("建房失败：%v", err)
	}
	if err := database.SetConnected(ctx, "p1", true); err != nil {
		t.Fatalf("置在线失败：%v", err)
	}
	if err := database.ResetConnections(ctx); err != nil {
		t.Fatalf("清零在线标记失败：%v", err)
	}
	players, err := database.Players(ctx, "r1")
	if err != nil {
		t.Fatalf("读玩家失败：%v", err)
	}
	for _, player := range players {
		if player.Connected {
			t.Fatalf("%s 重启后仍被标成在线", player.ID)
		}
	}
}
