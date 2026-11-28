package store

import (
	"context"
	"testing"
	"time"

	"generalquarters/internal/game"
)

// 闲置判定：活动时间早于门槛、而且没人在线，两条都成立才算"可以释放内存"。
// 这里只判断能不能放内存，房间和它的对战日志一律保留。
func TestIdleRoomsOnlyListsDisconnectedOnes(t *testing.T) {
	ctx := context.Background()
	database := newTestStore(t)
	state := game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 1}
	makeRoom := func(id string) {
		room := Room{ID: id, JoinToken: "tok-" + id, Status: "running", Turn: 1, Version: 1, State: state}
		if err := database.CreateRoom(ctx, room); err != nil {
			t.Fatalf("建房失败：%v", err)
		}
		if _, err := database.JoinRoom(ctx, room.JoinToken, "A", "甲", "p-"+id, "hash-"+id); err != nil {
			t.Fatalf("加入房间失败：%v", err)
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
		t.Fatalf("只该列出 idle，实际 %v", ids)
	}
	// 关键：选了它也不代表要删——库里的房间、对战日志、快照都得留着复盘。
	var events int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM events WHERE room_id='idle'").Scan(&events); err != nil {
		t.Fatalf("统计事件失败：%v", err)
	}
	if events == 0 {
		t.Fatal("房间的事件日志不该被清掉")
	}
	if _, err := database.Room(ctx, "idle"); err != nil {
		t.Fatalf("闲置房间仍然要能读出来：%v", err)
	}
}

// 服务重启后谁都不在线：ResetConnections 必须把上次留下的在线标记清掉，
// 否则闲置房间永远满足不了"没人在线"这个条件。
func TestResetConnectionsClearsStaleFlags(t *testing.T) {
	ctx := context.Background()
	database := newTestStore(t)
	state := game.State{ScenarioID: "_test", ScenarioName: "测试", Turn: 1}
	room := Room{ID: "r1", JoinToken: "tok", Status: "running", Turn: 1, Version: 1, State: state}
	if err := database.CreateRoom(ctx, room); err != nil {
		t.Fatalf("建房失败：%v", err)
	}
	if _, err := database.JoinRoom(ctx, room.JoinToken, "A", "甲", "p1", "hash"); err != nil {
		t.Fatalf("加入房间失败：%v", err)
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
