package game

import (
	"os"
	"path/filepath"
	"testing"
)

// 基础规则测试使用独立的小型规则夹具，不再保留公开的沙盒测试剧本。
// 测试时把 fixture 场景/地图复制到临时目录，再把 content/units 软链过去，
// 这样公开内容目录里只剩正式珊瑚海剧本，测试也不复制 8MB 素材。
func loadSandboxScenario(t *testing.T) (ScenarioDefinition, MapDefinition, UnitCatalog) {
	t.Helper()
	root := t.TempDir()
	copyTestFile(t, filepath.Join("testdata", "fixture", "scenario.json"), filepath.Join(root, "scenarios", "_fixture", "scenario.json"))
	copyTestFile(t, filepath.Join("testdata", "fixture", "map.json"), filepath.Join(root, "maps", "_fixture", "map.json"))
	units, err := filepath.Abs(filepath.Join("..", "..", "content", "units"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(units, filepath.Join(root, "units")); err != nil {
		t.Fatal(err)
	}
	scenario, gameMap, catalog, err := LoadScenario(root, "_fixture")
	if err != nil {
		t.Fatal(err)
	}
	return scenario, gameMap, catalog
}

func sandboxState(t *testing.T) State {
	t.Helper()
	scenario, gameMap, catalog := loadSandboxScenario(t)
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func copyTestFile(t *testing.T, src, dst string) {
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
