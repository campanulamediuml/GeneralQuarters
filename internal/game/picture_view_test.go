package game

import "testing"

// 敌机图标要用服务端下发的素材：接触里必须带 pictures，
// 前端不该再按机型名去猜内置矢量剪影。
func TestEnemyAirContactCarriesPictures(t *testing.T) {
	state := State{
		Turn: 3,
		Aircraft: map[string]*AirUnit{
			"b-strike": {ID: "b-strike", Faction: "B", TaskID: "task-1", TaskKind: "attack", Status: "attack", Model: "B5N2 九七舰攻", Kind: "torpedo_bomber", Picture: "misc/image/aircraft/b5n2.png", Coord: "0606"},
		},
	}
	revealAirTask(&state, "A", state.Aircraft["b-strike"], "radar")
	contacts := VisibleEnemyAir(state, "A")
	if len(contacts) != 1 {
		t.Fatalf("应当发现 1 支敌机编队，实际 %d", len(contacts))
	}
	if len(contacts[0].Pictures) != 1 || contacts[0].Pictures[0] != "misc/image/aircraft/b5n2.png" {
		t.Fatalf("敌机接触应带素材路径，实际 %v", contacts[0].Pictures)
	}
}

// 老存档的舰船卡没有 picture：载入房间时由服务端按单位库补上，
// 前端不再靠"舰名 → 图片文件"的映射兜底。
func TestBackfillCardPicturesFillsOldSaves(t *testing.T) {
	state := State{
		Units: map[string]*Unit{
			"a-cv-01": {ID: "a-cv-01", DefinitionID: "yorktown", Card: ShipCard{Picture: ""}},
			"a-dd-01": {ID: "a-dd-01", DefinitionID: "fletcher", Card: ShipCard{Picture: "misc/image/ship/own.png"}},
		},
	}
	catalog := UnitCatalog{Units: []UnitDefinition{
		{ID: "yorktown", Picture: "misc/image/ship/yorktown.png"},
		{ID: "fletcher", Picture: "misc/image/ship/fletcher.png"},
	}}
	if !BackfillCardPictures(&state, catalog) {
		t.Fatal("应当补上缺失的素材路径")
	}
	if got := state.Units["a-cv-01"].Card.Picture; got != "misc/image/ship/yorktown.png" {
		t.Fatalf("补图结果不对：%s", got)
	}
	if got := state.Units["a-dd-01"].Card.Picture; got != "misc/image/ship/own.png" {
		t.Fatalf("已有素材不该被覆盖：%s", got)
	}
	if BackfillCardPictures(&state, catalog) {
		t.Fatal("没有缺图时不应再改动")
	}
}
