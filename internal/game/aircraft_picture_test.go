package game

import "testing"

// TestAircraftPicturesComeFromCatalog 校验航空算子的 picture 和舰船一样由 JSON 配置下发：
// units/*.json 的 aircraft[].picture 会注入 AirUnit 快照，前端据此换成 /content/ 下的图片。
func TestAircraftPicturesComeFromCatalog(t *testing.T) {
	state := sandboxState(t)
	cases := map[string]string{
		"a2-cv-01-fighter-x1":        "misc/image/aircraft/f4f.png",
		"a2-cv-01-dive_bomber-x1":    "misc/image/aircraft/sbd.png",
		"a2-cv-01-torpedo_bomber-x1": "misc/image/aircraft/tbd.png",
		"air-a-11":                   "misc/image/aircraft/soc.png",
		"b-cv-01-fighter-x1":         "misc/image/aircraft/a6m2.png",
		"b-cv-01-dive_bomber-x1":     "misc/image/aircraft/d3a1.png",
		"b-cv-01-torpedo_bomber-x1":  "misc/image/aircraft/b5n2.png",
		"air-b-11":                   "misc/image/aircraft/e13a.png",
	}
	for id, want := range cases {
		aircraft := state.Aircraft[id]
		if aircraft == nil {
			t.Fatalf("缺少飞机%s", id)
		}
		if aircraft.Picture != want {
			t.Errorf("%s picture=%q，期望%q", id, aircraft.Picture, want)
		}
		if aircraft.PictureMode != "image" {
			t.Errorf("%s picture_mode=%q，期望 image（飞机默认按彩图渲染）", id, aircraft.PictureMode)
		}
	}
	for id, aircraft := range state.Aircraft {
		if aircraft.Picture == "" {
			t.Errorf("%s没有配置 picture：新增机型要在 units/*.json 里补图", id)
		}
	}
}

// TestAirfieldAircraftPicturesSurviveProjection 校验机场算子同样带图，
// 而且转成前端通用 AirUnit 快照时不会把 picture 丢掉。
func TestAirfieldAircraftPicturesSurviveProjection(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	p39 := state.GroundAir["af-a-moresby-p39-1"]
	if p39 == nil || p39.Picture != "misc/image/aircraft/p39.png" || p39.PictureMode != "image" {
		t.Fatalf("P-39 图片配置错误：%+v", p39)
	}
	ground := state.GroundAir["af-b-rabaul-fighter-1"]
	if ground == nil {
		t.Fatal("缺少机场算子 af-b-rabaul-fighter-1")
	}
	if ground.Picture != "misc/image/aircraft/a6m2.png" {
		t.Fatalf("机场算子 picture=%q", ground.Picture)
	}
	if ground.PictureMode != "image" {
		t.Fatalf("机场算子 picture_mode=%q", ground.PictureMode)
	}
	projected := groundAirUnit(state, ground)
	if projected.Picture != ground.Picture || projected.PictureMode != ground.PictureMode {
		t.Fatalf("投影后 picture/picture_mode=%q/%q，期望%q/%q", projected.Picture, projected.PictureMode, ground.Picture, ground.PictureMode)
	}
	for _, carrier := range GroundAviationView(state, "B").Carriers {
		if carrier.UnitID != "af-b-rabaul" {
			continue
		}
		for _, aircraft := range carrier.Aircraft {
			if aircraft.ID == ground.ID {
				if aircraft.Picture == "" {
					t.Fatal("机场航空视图丢失 picture")
				}
				return
			}
		}
	}
	t.Fatal("机场 af-b-rabaul 没有出现在航空视图中")
}
