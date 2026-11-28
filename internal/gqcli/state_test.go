package gqcli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newStateTestClient(t *testing.T) *Client {
	t.Helper()
	dir := filepath.Join("..", "..", ".gq", "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stateDir, err := os.MkdirTemp(dir, "gqcli-state-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stateDir) })
	client, err := NewClient(Config{StateDir: stateDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func TestFullStateIsSanitizedAndWaitReturnsOnlyChanges(t *testing.T) {
	client := newStateTestClient(t)
	initial := json.RawMessage(`{
		"room_id":"room-a","status":"running","turn":1,"phase":"battle","join_token":"SECRET",
		"you":{"id":"player-a","faction":"A","name":"A"},
		"players":[{"id":"player-a","faction":"A","ready":true,"connected":true}],
		"fleets":[
			{"id":"a-1","fleet_id":"TF17","fleet_name":"TF17","faction":"A","coord":"0203","members":[{"id":"a-1","card":{"gunnery":8}}]},
			{"id":"b-1","fleet_id":"TF8","fleet_name":"contact","faction":"B","coord":"0804","contact_level":2}
		],
		"aviation":{"carriers":[],"tasks":[]},"ground_tasks":[],
		"events":[{"id":10,"turn":1,"sequence":0,"type":"battle_start","text":"开战","created_at":"2026-01-01T00:00:00Z"}]
	}`)
	if err := client.replaceSnapshot(initial, true); err != nil {
		t.Fatal(err)
	}
	client.markDelivered()
	full, err := client.state(context.Background(), stateParams{Section: "full"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(full)
	if string(encoded) == "" || containsString(string(encoded), "SECRET") || containsString(string(encoded), "join_token") {
		t.Fatalf("完整输出泄露了凭证：%s", encoded)
	}
	summary, err := client.state(context.Background(), stateParams{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(summary)
	if containsString(string(encoded), "gunnery") {
		t.Fatalf("摘要不应携带完整舰船卡：%s", encoded)
	}
	fleet, err := client.state(context.Background(), stateParams{Section: "fleet", ID: "TF17"})
	if err != nil {
		t.Fatal(err)
	}
	if fleet == nil {
		t.Fatal("按 fleet_id 查询失败")
	}

	next := json.RawMessage(`{
		"room_id":"room-a","status":"running","turn":2,"phase":"battle","join_token":"SECRET",
		"you":{"id":"player-a","faction":"A","name":"A"},
		"players":[{"id":"player-a","faction":"A","ready":true,"connected":true}],
		"fleets":[
			{"id":"a-1","fleet_id":"TF17","fleet_name":"TF17","faction":"A","coord":"0204","members":[{"id":"a-1","card":{"gunnery":8}}]},
			{"id":"b-1","fleet_id":"TF8","fleet_name":"contact","faction":"B","coord":"0804","contact_level":2}
		],
		"aviation":{"carriers":[],"tasks":[]},"ground_tasks":[],
		"events":[
			{"id":10,"turn":1,"sequence":0,"type":"battle_start","text":"开战","created_at":"2026-01-01T00:00:00Z"},
			{"id":11,"turn":2,"sequence":1,"type":"unit_moved","text":"舰队移动","created_at":"2026-01-01T00:01:00Z"}
		]
	}`)
	if err := client.replaceSnapshot(next, false); err != nil {
		t.Fatal(err)
	}
	result, err := client.wait(context.Background(), waitParams{TimeoutMS: 100})
	if err != nil {
		t.Fatal(err)
	}
	waitResult := result.(map[string]any)
	if waitResult["changed"] != true || waitResult["revision"] != uint64(2) {
		t.Fatalf("等待结果不正确：%#v", waitResult)
	}
	sections := waitResult["changed_sections"].([]string)
	if !containsStringSlice(sections, "fleets") || !containsStringSlice(sections, "events") {
		t.Fatalf("缺少变化 section：%v", sections)
	}
	events := waitResult["events"].([]Event)
	if len(events) != 1 || events[0].ID != 11 {
		t.Fatalf("增量事件不正确：%v", events)
	}
	second, err := client.wait(context.Background(), waitParams{TimeoutMS: 10})
	if err != nil {
		t.Fatal(err)
	}
	if second.(map[string]any)["timed_out"] != true {
		t.Fatalf("没有新变化时应超时：%#v", second)
	}
}

func TestSnapshotIsPersistedAtomically(t *testing.T) {
	client := newStateTestClient(t)
	if err := client.replaceSnapshot(json.RawMessage(`{"turn":3,"events":[]}`), true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(client.cfg.StateDir, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil || snapshot["turn"] != float64(3) {
		t.Fatalf("持久化快照不正确：%s (%v)", data, err)
	}
}

func containsString(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}

func containsStringSlice(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestWaitHonorsCancellation(t *testing.T) {
	client := newStateTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.wait(ctx, waitParams{TimeoutMS: 1000}); err == nil {
		t.Fatal("取消的 wait 不应成功")
	}
}

func TestCommandParamsRejectUnknownFields(t *testing.T) {
	var params commandParams
	if err := decodeParams(json.RawMessage(`{"type":"LAUNCH_RECON","recon_radius":4}`), &params); err == nil {
		t.Fatal("未知命令字段不应被静默忽略")
	}
}
