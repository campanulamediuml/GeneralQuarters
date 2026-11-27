package game

import "testing"

// TestMarianasShipSilhouettesMatchClass 钉住"舰级 → 剪影"的对应：
// 素材包（素材/船只剪影/正常）里扶桑级、伊势级、信浓、苍龙级、最上级都有独立剪影，
// 不能用金刚级/长门级/大和级/云龙级/利根级的图凑数，否则兵牌外形会认错船。
func TestMarianasShipSilhouettesMatchClass(t *testing.T) {
	_, _, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	pic := func(name string) string { return "misc/image/ship/" + name + ".png" }
	want := map[string]string{
		"大和": pic("yamato"), "武藏": pic("yamato"),
		"长门": pic("nagato"), "陆奥": pic("nagato"),
		"金刚": pic("kongo"), "榛名": pic("haruna"), "比叡": pic("hiei"), "雾岛": pic("kirishima"),
		"扶桑": pic("fuso"), "山城": pic("fuso"),
		"伊势": pic("ise"), "日向": pic("ise"),
		"赤城": pic("akagi"), "加贺": pic("kaga"),
		"翔鹤": pic("shokaku"), "瑞鹤": pic("shokaku"),
		"云龙": pic("unryu"), "天城": pic("unryu"), "葛城": pic("unryu"),
		"苍龙": pic("soryu"), "飞龙": pic("hiryu"), "信浓": pic("shinano"),
		"大凤": pic("taiho"),
		"隼鹰": pic("hiyo"), "飞鹰": pic("hiyo"), "龙凤": pic("ryuho"),
		"千岁": pic("jap_cvl1"), "千代田": pic("jap_cvl1"), "瑞凤": pic("jap_cvl1"),
		"妙高": pic("myoko"), "羽黑": pic("myoko"), "那智": pic("myoko"), "足柄": pic("myoko"),
		"高雄": pic("takao"), "爱宕": pic("takao"), "摩耶": pic("takao"), "鸟海": pic("takao"),
		"利根": pic("tone"), "筑摩": pic("tone"),
		"铃谷": pic("mogami"), "熊野": pic("mogami"), "最上": pic("mogami"),
		"矢矧": pic("agano"), "大淀": pic("agano"), "能代": pic("agano"), "阿贺野": pic("agano"),
		"神通": pic("yubari"), "川内": pic("yubari"),
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
	// 具名舰库里的白露级等用 shiratsuyu.png）。
	dd := 0
	for _, unit := range catalog.Units {
		if len(unit.ID) < 6 || unit.ID[:6] != "jp-dd-" {
			continue
		}
		dd++
		if unit.Picture != pic("jap_dd1") {
			t.Fatalf("%s 驱逐舰剪影是 %q，期望通用 jap_dd1", unit.Name, unit.Picture)
		}
	}
	if dd != 22 {
		t.Fatalf("日军驱逐舰 %d 艘，期望 22", dd)
	}
}
