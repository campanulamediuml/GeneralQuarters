package server

import (
	"encoding/json"
	"reflect"
	"testing"
)

// 补丁必须能从旧视图精确还原新视图：漏掉一个字段，客户端就会停在上一个状态。
func TestViewPatchRoundTrip(t *testing.T) {
	cases := []struct {
		name       string
		before     string
		after      string
		expectNone bool
	}{
		{
			name:       "没变化",
			before:     `{"version":1,"units":[{"id":"a","coord":"0101"}],"wind":{"wind":"SE"}}`,
			after:      `{"version":1,"units":[{"id":"a","coord":"0101"}],"wind":{"wind":"SE"}}`,
			expectNone: true,
		},
		{
			name:   "标量变化",
			before: `{"version":1,"turn":1}`,
			after:  `{"version":2,"turn":1}`,
		},
		{
			name:   "单位改名只带那一个",
			before: `{"units":[{"id":"a","coord":"0101"},{"id":"b","coord":"0202"}]}`,
			after:  `{"units":[{"id":"a","coord":"0101"},{"id":"b","coord":"0303"}]}`,
		},
		{
			name:   "新增和移除",
			before: `{"orders":[{"unit_id":"a","coord":"0101"}]}`,
			after:  `{"orders":[{"task_id":"t1","coord":"0202"}]}`,
		},
		{
			name:   "嵌套集合",
			before: `{"aviation":{"carriers":[{"unit_id":"cv1","deck_length":5}],"tasks":[{"id":"t1","status":"deck"}]}}`,
			after:  `{"aviation":{"carriers":[{"unit_id":"cv1","deck_length":8}],"tasks":[{"id":"t1","status":"airborne"},{"id":"t2","status":"deck"}]}}`,
		},
		{
			name:   "普通数组整块替换",
			before: `{"units":[{"id":"a","legal_moves":["0101","0102"]}]}`,
			after:  `{"units":[{"id":"a","legal_moves":["0202","0203"]}]}`,
		},
		{
			name:   "空对象和 null",
			before: `{"losses":{"A":{},"B":{}},"outcome":null}`,
			after:  `{"losses":{"A":{"ship":1},"B":{}},"outcome":{"reason":"x"}}`,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			before := viewFromJSON(t, item.before)
			after := viewFromJSON(t, item.after)
			patch := diffView(before, after)
			if item.expectNone {
				if patch != any(patchNone) {
					t.Fatalf("期望空补丁，实际 %#v", patch)
				}
				return
			}
			if patch == any(patchNone) {
				t.Fatal("补丁为空，但视图变了")
			}
			merged := applyTestPatch(before, patch)
			if !reflect.DeepEqual(merged, after) {
				t.Fatalf("合并结果和新视图不一致\n得到 %#v\n期望 %#v\n补丁 %#v", merged, after, patch)
			}
		})
	}
}

// 换航向这条命令在投影里同时改了 wind 和三艘航母的甲板长度；
// 补丁必须把它们全都带上，否则界面上的甲板数会和服务端校验对不上。
func TestViewPatchCarriesDerivedDeckLengths(t *testing.T) {
	before := viewFromJSON(t, `{"version":10,"wind":{"wind":"SE","fleets":[{"fleet_id":"6S","heading":"NE","relation":"meta","deck_modifier":-1}]},"units":[{"id":"cv1","deck":{"length":5,"base_length":6}}],"aviation":{"carriers":[{"unit_id":"cv1","deck_length":5}],"tasks":[]}}`)
	after := viewFromJSON(t, `{"version":11,"wind":{"wind":"SE","fleets":[{"fleet_id":"6S","heading":"SE","relation":"para","deck_modifier":2}]},"units":[{"id":"cv1","deck":{"length":8,"base_length":6}}],"aviation":{"carriers":[{"unit_id":"cv1","deck_length":8}],"tasks":[]}}`)
	patch := diffView(before, after)
	if !reflect.DeepEqual(applyTestPatch(before, patch), after) {
		t.Fatalf("补丁没能还原：%#v", patch)
	}
	encoded, _ := json.Marshal(patch)
	for _, needle := range []string{`"deck_length":8`, `"length":8`, `"deck_modifier":2`} {
		if !containsString(string(encoded), needle) {
			t.Fatalf("补丁漏了 %s：%s", needle, encoded)
		}
	}
	// 只发变了的属性：没变的 base_length 不该出现在补丁里，体积也该是几百字节。
	if containsString(string(encoded), "base_length") {
		t.Fatalf("补丁带了没变的字段：%s", encoded)
	}
	if len(encoded) > 600 {
		t.Fatalf("补丁有 %d 字节，应该只发变了的属性：%s", len(encoded), encoded)
	}
}

func containsString(haystack, needle string) bool {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return true
		}
	}
	return false
}

// 主键识别被缓存成了类型元数据，这里把它认得住的几种形状钉住：
// 字段名和 JSON 名不一致、多个候选时按 patchKeyFields 的优先级、嵌入字段。
func TestPatchKeyFromCachedMeta(t *testing.T) {
	type plain struct {
		ID    string `json:"id"`
		Coord string `json:"coord"`
	}
	if field, key := patchKey(newDiffCache(), reflect.ValueOf(plain{ID: "u1", Coord: "0101"})); field != "id" || key != "u1" {
		t.Fatalf("多个候选时应取 id：%q %q", field, key)
	}
	type renamed struct {
		UnitID string `json:"unit_id"`
		Value  int
	}
	if field, key := patchKey(newDiffCache(), reflect.ValueOf(renamed{UnitID: "u2"})); field != "unit_id" || key != "u2" {
		t.Fatalf("JSON 名和字段名不一致时应认 JSON 名：%q %q", field, key)
	}
	type embedded struct {
		Key string `json:"id"`
	}
	type outer struct {
		embedded
		Value int
	}
	if field, key := patchKey(newDiffCache(), reflect.ValueOf(outer{embedded: embedded{Key: "u3"}})); field != "id" || key != "u3" {
		t.Fatalf("嵌入字段也要能当主键：%q %q", field, key)
	}
	if field, key := patchKey(newDiffCache(), reflect.ValueOf(plain{})); field != "" || key != "" {
		t.Fatalf("空主键应返回空：%q %q", field, key)
	}
	// 数字主键（战报事件）不能丢。
	type event struct {
		ID   int64  `json:"id"`
		Text string `json:"text"`
	}
	if field, key := patchKey(newDiffCache(), reflect.ValueOf(event{ID: 42})); field != "id" || key != "42" {
		t.Fatalf("数字主键应转成字符串：%q %q", field, key)
	}
}
