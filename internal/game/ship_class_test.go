package game

import "testing"

// TestShipsDeclareTheirClass 钉子：每艘船都要写舰级。
// 舰名会跨舰级重名——1942 年的阿斯托里亚是新奥尔良级重巡，马里亚纳那条是克利夫兰级轻巡——
// 名册和兵牌上只看舰名分不出是哪条船，舰级才是身份证。
func TestShipsDeclareTheirClass(t *testing.T) {
	_, _, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	missing := []string{}
	classes := map[string]map[string]bool{}
	for _, unit := range catalog.Units {
		if unit.Card.Class == "" {
			missing = append(missing, unit.ID+"（"+unit.Name+"）")
			continue
		}
		if classes[unit.Name] == nil {
			classes[unit.Name] = map[string]bool{}
		}
		classes[unit.Name][unit.Card.Class] = true
	}
	if len(missing) > 0 {
		t.Fatalf("这些船没写舰级：%v", missing)
	}
	// 同名不同级：阿斯托里亚既是新奥尔良级（CA-34）又是克利夫兰级（CL-90）。
	if got := classes["阿斯托里亚"]; len(got) != 2 || !got["新奥尔良级"] || !got["克利夫兰级"] {
		t.Fatalf("阿斯托里亚应该有两个舰级，实际 %v", got)
	}
}
