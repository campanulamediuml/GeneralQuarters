package game

import "testing"

// TestMarianasShipSilhouettesMatchClass 钉住"舰级 → 剪影"的对应：
// 直接引用素材库原图（content/misc/image/ship/library/ 下保留素材原始文件名），
// 扶桑级、伊势级、信浓、苍龙/飞龙、最上级都有独立剪影，不能拿金刚/长门/大和/云龙/利根的图凑数。
func TestMarianasShipSilhouettesMatchClass(t *testing.T) {
	_, _, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	lib := func(name string) string { return "misc/image/ship/library/" + name }
	want := map[string]string{
		"大和": lib("JAP_BB5_ship.png"), "武藏": lib("JAP_BB5_ship.png"),
		"长门": lib("JAP_BB4_ship.png"), "陆奥": lib("JAP_BB4_ship.png"),
		"金刚": lib("JAP_BC1_ship.png"), "榛名": lib("JAP_BC1_ship.png"),
		"比叡": lib("JAP_BC1_ship.png"), "雾岛": lib("JAP_BC1_ship.png"),
		"扶桑": lib("JAP_BB1_ship.png"), "山城": lib("JAP_BB1_ship.png"),
		"伊势": lib("JAP_BB2_ship.png"), "日向": lib("JAP_BB2_ship.png"),
		"赤城": lib("JAP_CV1 Akagi_ship.png"), "加贺": lib("JAP_CV1 Kaga_ship.png"),
		"翔鹤": lib("JAP_CV8_ship.png"), "瑞鹤": lib("JAP_CV8_ship.png"),
		"云龙": lib("JAP_CV3_ship.png"), "天城": lib("JAP_CV3_ship.png"), "葛城": lib("JAP_CV3_ship.png"),
		"苍龙": lib("JAP_CV6_ship.png"), "飞龙": lib("JAP_CV7_ship.png"), "信浓": lib("JAP_CV5_ship.png"),
		"大凤": lib("JAP_CV4_ship.png"),
		"隼鹰": lib("JAP_CV2_ship.png"), "飞鹰": lib("JAP_CV2_ship.png"),
		"龙凤": lib("JAP_CVL4_ship.png"),
		"千岁": lib("JAP_CVL1_ship.png"), "千代田": lib("JAP_CVL1_ship.png"), "瑞凤": lib("JAP_CVL1_ship.png"),
		"妙高": lib("JAP_CA1_ship.png"), "羽黑": lib("JAP_CA1_ship.png"),
		"那智": lib("JAP_CA1_ship.png"), "足柄": lib("JAP_CA1_ship.png"),
		"高雄": lib("JAP_CA2_ship.png"), "爱宕": lib("JAP_CA2_ship.png"),
		"摩耶": lib("JAP_CA2_ship.png"), "鸟海": lib("JAP_CA2_ship.png"),
		"利根": lib("JAP_CA10_ship.png"), "筑摩": lib("JAP_CA10_ship.png"),
		"铃谷": lib("JAP_CA3_ship.png"), "熊野": lib("JAP_CA3_ship.png"), "最上": lib("JAP_CA3_ship.png"),
		"矢矧": lib("JAP_CL2_ship.png"), "大淀": lib("JAP_CL2_ship.png"),
		"能代": lib("JAP_CL2_ship.png"), "阿贺野": lib("JAP_CL2_ship.png"),
		"神通": lib("JAP_CL1_ship.png"), "川内": lib("JAP_CL1_ship.png"),
	}
	found := map[string]bool{}
	for _, unit := range catalog.Units {
		expect, ok := want[unit.Name]
		if !ok {
			continue
		}
		found[unit.Name] = true
		if unit.Picture != expect {
			t.Fatalf("%s 的剪影是 %q，期望 %q", unit.Name, unit.Picture, expect)
		}
	}
	for name := range want {
		if !found[name] {
			t.Fatalf("日本舰队里没找到 %s", name)
		}
	}
	// 通用驱逐舰保留一张剪影，不按舰级拆图（只查本剧本编成里的 jp-dd-*，
	// 具名舰库里的白露级等仍然用白露级剪影）。
	dd := 0
	for _, unit := range catalog.Units {
		if len(unit.ID) < 6 || unit.ID[:6] != "jp-dd-" {
			continue
		}
		dd++
		if unit.Picture != lib("JAP_DD1_ship.png") {
			t.Fatalf("%s 驱逐舰剪影是 %q，期望通用 JAP_DD1", unit.Name, unit.Picture)
		}
	}
	if dd != 22 {
		t.Fatalf("日军驱逐舰 %d 艘，期望 22", dd)
	}
}
