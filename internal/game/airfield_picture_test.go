package game

import "testing"

// 机场标识图片按规模分三档：小型一条跑道、中型两条、大型三条（规则书 92 行"机场标识图片"）。
func TestAirfieldPictureBySize(t *testing.T) {
	cases := []struct {
		kind AirfieldType
		want string
	}{
		{AirfieldSmall, "misc/image/airfield/small.png"},
		{AirfieldMedium, "misc/image/airfield/medium.png"},
		{AirfieldLarge, "misc/image/airfield/large.png"},
		{AirfieldSeaplane, "misc/image/airfield/seaplane.png"},
	}
	for _, item := range cases {
		if got := AirfieldPicture(item.kind, ""); got != item.want {
			t.Fatalf("%s 的标识图片应为 %q，实际 %q", item.kind, item.want, got)
		}
	}
	// 剧本可以单点覆盖某一座机场的图标。
	if got := AirfieldPicture(AirfieldLarge, "misc/image/airfield/custom.png"); got != "misc/image/airfield/custom.png" {
		t.Fatalf("剧本覆盖的 picture 没有被采用：%q", got)
	}
}

// 珊瑚海：莫尔兹比港与拉包尔都是中型（2 条跑道），凯恩斯是大型。
func TestCoralAirfieldSizes(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatalf("载入剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 12345)
	if err != nil {
		t.Fatalf("建立状态失败：%v", err)
	}
	want := map[string]AirfieldType{
		"af-a-moresby": AirfieldMedium,
		"af-a-cairns":  AirfieldLarge,
		"af-b-rabaul":  AirfieldMedium,
	}
	for id, kind := range want {
		airfield := state.Airfields[id]
		if airfield == nil {
			t.Fatalf("缺少机场 %s", id)
		}
		if airfield.Type != kind {
			t.Fatalf("%s 的规模应为 %s，实际 %s", id, kind, airfield.Type)
		}
	}
	if picture := state.Airfields["af-a-moresby"].PictureOrDefault(); picture != "misc/image/airfield/medium.png" {
		t.Fatalf("莫尔兹比港应挂中型机场图标，实际 %q", picture)
	}
	if picture := state.Airfields["af-b-rabaul"].PictureOrDefault(); picture != "misc/image/airfield/medium.png" {
		t.Fatalf("拉包尔应挂中型机场图标，实际 %q", picture)
	}
	if cairns := state.Airfields["af-a-cairns"]; cairns.Runways != 3 || cairns.MaxStrength != 15 {
		t.Fatalf("凯恩斯应为 3 条跑道 / 15 点生命值，实际 %d / %d", cairns.Runways, cairns.MaxStrength)
	}
	for _, id := range []string{"af-b-rabaul", "af-a-cairns"} {
		if airfield := state.Airfields[id]; airfield == nil || airfield.Coastal != 6 {
			t.Fatalf("%s 的岸炮应为 6，实际 %+v", id, airfield)
		}
	}
	// 老存档没有 picture 字段：按 type 现算，也能拿到图标。
	legacy := Airfield{Type: AirfieldLarge}
	if legacy.PictureOrDefault() != "misc/image/airfield/large.png" {
		t.Fatalf("老存档应回退到按规模计算的图标：%q", legacy.PictureOrDefault())
	}
}
