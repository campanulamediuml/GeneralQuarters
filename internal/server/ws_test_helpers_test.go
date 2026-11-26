package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// applyTestPatch 按补丁协议把补丁合并回底座，用来验证"客户端照这个协议合并能还原新视图"。
// 和 web/src/patch.ts、watch.html 里的实现是同一套语义。
func applyTestPatch(base, patch any) any {
	switch typed := patch.(type) {
	case map[string]any:
		if list, ok := base.([]any); ok {
			if _, isPatch := typed["upsert"]; isPatch || typed["remove"] != nil {
				merged := append([]any{}, list...)
				for _, item := range typed["upsert"].([]any) {
					entry := item.(map[string]any)
					key := testPatchKey(entry)
					replaced := false
					for index, existing := range merged {
						// upsert 里的元素是"变了的属性"（服务端只发这些），要合并进原记录。
						if old, ok := existing.(map[string]any); ok && testPatchKey(old) == key {
							merged[index] = applyTestPatch(old, entry)
							replaced = true
							break
						}
					}
					if !replaced {
						merged = append(merged, entry)
					}
				}
				if remove := testPatchStringList(typed["remove"]); len(remove) > 0 {
					kept := merged[:0]
					for _, existing := range merged {
						entry, _ := existing.(map[string]any)
						drop := false
						for _, text := range remove {
							if entry != nil && testPatchKey(entry) == text {
								drop = true
							}
						}
						if !drop {
							kept = append(kept, existing)
						}
					}
					merged = kept
				}
				return merged
			}
		}
		baseMap, _ := base.(map[string]any)
		merged := make(map[string]any, len(baseMap)+len(typed))
		for key, value := range baseMap {
			merged[key] = value
		}
		// 字符串键 map 删键：{"set":{变化的键},"remove":[消失的键]}。
		remove := testPatchStringList(typed["remove"])
		set, _ := typed["set"].(map[string]any)
		if set != nil || len(remove) > 0 {
			for key, value := range set {
				merged[key] = applyTestPatch(baseMap[key], value)
			}
			for _, key := range remove {
				delete(merged, key)
			}
			return merged
		}
		for key, value := range typed {
			merged[key] = applyTestPatch(baseMap[key], value)
		}
		return merged
	default:
		return patch
	}
}

// testPatchKey 必须和 patch.go 的 patchKey 认出同一个键。
func testPatchKey(item map[string]any) string {
	for _, field := range []string{"id", "unit_id", "task_id", "carrier_id", "fleet_id", "coord"} {
		switch value := item[field].(type) {
		case string:
			if value != "" {
				return value
			}
		case float64:
			return fmt.Sprintf("%d", int64(value))
		}
	}
	return ""
}

func viewFromJSON(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

// wsViewReader 是测试里的"客户端"：把推来的全量和补丁合并成一份完整视图。
// 推送协议一旦改成补丁，测试就必须像客户端那样合并，否则读到的永远是残缺帧。
type wsViewReader struct {
	view map[string]any
	kind string
}

func (r *wsViewReader) apply(t *testing.T, envelope map[string]json.RawMessage) bool {
	t.Helper()
	var kind string
	_ = json.Unmarshal(envelope["type"], &kind)
	if kind != "state" && kind != "snapshot" {
		return false
	}
	r.kind = kind
	if raw, ok := envelope["view"]; ok {
		var full map[string]any
		if err := json.Unmarshal(raw, &full); err != nil {
			t.Fatal(err)
		}
		r.view = full
		return true
	}
	raw, ok := envelope["patch"]
	if !ok {
		return false
	}
	var patch any
	if err := json.Unmarshal(raw, &patch); err != nil {
		t.Fatal(err)
	}
	if r.view == nil {
		t.Fatal("收到补丁但没有底座视图")
	}
	merged, ok := applyTestPatch(r.view, patch).(map[string]any)
	if !ok {
		t.Fatal("补丁合并后不是对象")
	}
	r.view = merged
	return true
}

// read 读帧直到拿到一帧推送并合并，返回合并后的完整视图。
func (r *wsViewReader) read(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var envelope map[string]json.RawMessage
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatal(err)
		}
		if r.apply(t, envelope) {
			return r.view
		}
	}
}

// readWhere 一直读到合并后的视图满足条件；超时由读超时兜底。
func (r *wsViewReader) readWhere(t *testing.T, conn *websocket.Conn, match func(map[string]any) bool) map[string]any {
	t.Helper()
	for {
		if view := r.read(t, conn); match(view) {
			return view
		}
	}
}

// 每条连接一个合并器：补丁要叠在同一条连接的底座上，不能跨连接混用。
var viewReaders sync.Map

func viewReader(conn *websocket.Conn) *wsViewReader {
	value, _ := viewReaders.LoadOrStore(conn, &wsViewReader{})
	return value.(*wsViewReader)
}

// readStateView 读到第 turn 回合的视图为止。
func readStateView(t *testing.T, conn *websocket.Conn, turn float64) map[string]any {
	t.Helper()
	return readStateViewWhere(t, conn, func(view map[string]any) bool { return view["turn"] == turn })
}

// readStateViewWhere 读到合并后的视图满足条件为止。
func readStateViewWhere(t *testing.T, conn *websocket.Conn, match func(map[string]any) bool) map[string]any {
	t.Helper()
	return viewReader(conn).readWhere(t, conn, match)
}

// testPatchStringList 兼容 []string（Go 侧直接产出的补丁）和 []any（JSON 解出来的补丁）。
func testPatchStringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	}
	return nil
}

// seedPlayerView 像客户端那样先发一条 GET state 把全量拿走：玩家连接建立后
// 服务端不会主动推全量，底座就是这条响应，之后的推送都是相对它的补丁。
func seedPlayerView(t *testing.T, conn *websocket.Conn, roomID string) map[string]any {
	t.Helper()
	const id = "seed-state"
	if err := conn.WriteJSON(wsRequest{Type: "rpc_request", ID: id, Method: http.MethodGet, Path: "/api/v1/rooms/" + roomID + "/state"}); err != nil {
		t.Fatal(err)
	}
	response := readWSResponse(t, conn, id)
	if response.Status != http.StatusOK {
		t.Fatalf("seed state status=%d body=%s", response.Status, response.Body)
	}
	var view map[string]any
	if err := json.Unmarshal(response.Body, &view); err != nil {
		t.Fatal(err)
	}
	viewReader(conn).view = view
	return view
}

// readRawViewFrame 读下一帧推送的原始字节。和 wsViewReader.read 的区别是它不合并，
// 用来断言"这帧到底是补丁还是整份视图"。
func readRawViewFrame(t *testing.T, conn *websocket.Conn) (map[string]json.RawMessage, int) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var raw json.RawMessage
		if err := conn.ReadJSON(&raw); err != nil {
			t.Fatal(err)
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatal(err)
		}
		var kind string
		_ = json.Unmarshal(envelope["type"], &kind)
		if kind == "state" || kind == "snapshot" {
			return envelope, len(raw)
		}
	}
}
