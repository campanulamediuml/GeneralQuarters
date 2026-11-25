package game

import "testing"

// TestSeaplaneCannotLandOnCarrier 水上飞机没有起落架，不能降落在航母甲板上，
// 只能回自己的巡洋舰母舰或水上飞机基地。以前返航时航母也被算作候选，
// 结果巡洋舰的水侦落到了航母上。
func TestSeaplaneCannotLandOnCarrier(t *testing.T) {
	state := sandboxState(t)
	var plane *AirUnit
	for _, aircraft := range state.Aircraft {
		if aircraft.Seaplane && aircraft.Faction == "B" {
			plane = aircraft
			break
		}
	}
	if plane == nil {
		t.Fatal("沙盒里没有水上飞机算子")
	}
	// 把一艘航母挪到同一格：如果不分机型，它会被当成备降场。
	if carrier := state.Units["b-cv-01"]; carrier != nil {
		carrier.Coord = plane.Coord
	}
	if host := altLanding(&state, plane); host != nil {
		t.Fatalf("水上飞机不该把航母当备降场：%s", host.Name)
	}
	if landIfCapacity(&state, plane, nil) {
		t.Fatal("母舰不在时水上飞机不该被航母接走")
	}
}
