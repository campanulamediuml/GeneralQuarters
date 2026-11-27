package game

import "testing"

// 展示文案表放在 content/misc/display.json：服务端只负责原样下发，
// 前端按枚举查表渲染。这里量两件事：文件能读、分组结构是预期的。
func TestLoadDisplayReadsContentDictionary(t *testing.T) {
	display, err := LoadDisplay("../../content")
	if err != nil {
		t.Fatalf("读取展示文案失败：%v", err)
	}
	shipType, ok := display.Labels["ship_type"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 ship_type 分组：%+v", display.Labels)
	}
	if shipType["CV"] != "航空母舰" {
		t.Fatalf("CV 的舰种文案不对：%v", shipType["CV"])
	}
	airfieldType, ok := display.Labels["airfield_type"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 airfield_type 分组")
	}
	small, ok := airfieldType["small"].(map[string]any)
	if !ok || small["label"] == "" || small["hint"] == "" {
		t.Fatalf("机场规模条目要同时有 label 和 hint：%+v", airfieldType)
	}
}

// 内容目录没有文案表时不算错误：界面退回显示枚举 id。
func TestLoadDisplayMissingFileIsEmpty(t *testing.T) {
	display, err := LoadDisplay(t.TempDir())
	if err != nil {
		t.Fatalf("缺文件不应报错：%v", err)
	}
	if len(display.Labels) != 0 {
		t.Fatalf("缺文件时应是空表：%+v", display.Labels)
	}
}
