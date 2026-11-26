package game

import "testing"

// 备降机场必须和起降方式对得上：水上飞机只能落水上基地，陆基机不能落水上基地。
// 以前这里反着写，莫尔兹比港这种陆地机场会把整队 SOC 海鸥收进机库。
func TestDivertAirfieldMatchesTakeoffMode(t *testing.T) {
	state := State{Airfields: map[string]*Airfield{
		"land":     {ID: "land", Faction: "A", Type: AirfieldLarge, Coord: "1611"},
		"seaplane": {ID: "seaplane", Faction: "A", Type: AirfieldSeaplane, Coord: "4613"},
	}}
	floatplane := &AirUnit{ID: "soc", Faction: "A", Kind: "recon", Seaplane: true}
	landplane := &AirUnit{ID: "b17", Faction: "A", Kind: "heavy_bomber"}
	fighter := &AirUnit{ID: "p40", Faction: "A", Kind: "fighter"}

	if airfieldCanAccept(&state, state.Airfields["land"], floatplane) {
		t.Fatal("水上飞机不该在陆地机场备降")
	}
	if !airfieldCanAccept(&state, state.Airfields["seaplane"], floatplane) {
		t.Fatal("水上飞机应当可以备降水上基地")
	}
	for _, plane := range []*AirUnit{landplane, fighter} {
		if !airfieldCanAccept(&state, state.Airfields["land"], plane) {
			t.Fatalf("%s 应当可以备降陆地机场", plane.ID)
		}
		if airfieldCanAccept(&state, state.Airfields["seaplane"], plane) {
			t.Fatalf("%s 不该落到水上基地", plane.ID)
		}
	}
	// 敌我不同阵营的机场一律不收。
	state.Airfields["enemy"] = &Airfield{ID: "enemy", Faction: "B", Type: AirfieldLarge, Coord: "0101"}
	if airfieldCanAccept(&state, state.Airfields["enemy"], landplane) {
		t.Fatal("敌方机场不能作为备降点")
	}
}
