package server

import (
	"os"
	"path/filepath"
	"testing"
)

// 测试服务器使用独立内容根：小型规则夹具只给基础通信/航空用例，
// 正式珊瑚海剧本通过软链接提供。这样公开 content 目录里不再保留测试图。
func testContentRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	copyServerFile(t, filepath.Join("..", "game", "testdata", "fixture", "scenario.json"), filepath.Join(root, "scenarios", "_fixture", "scenario.json"))
	copyServerFile(t, filepath.Join("..", "game", "testdata", "fixture", "map.json"), filepath.Join(root, "maps", "_fixture", "map.json"))
	units, err := filepath.Abs(filepath.Join("..", "..", "content", "units"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(units, filepath.Join(root, "units")); err != nil {
		t.Fatal(err)
	}
	realScenarios, err := filepath.Abs(filepath.Join("..", "..", "content", "scenarios", "_coral_real"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scenarios"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realScenarios, filepath.Join(root, "scenarios", "_coral_real")); err != nil {
		t.Fatal(err)
	}
	realMaps, err := filepath.Abs(filepath.Join("..", "..", "content", "maps", "_coral_real"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "maps"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realMaps, filepath.Join(root, "maps", "_coral_real")); err != nil {
		t.Fatal(err)
	}
	return root
}

func copyServerFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
