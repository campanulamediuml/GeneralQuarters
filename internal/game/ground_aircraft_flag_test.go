package game

import "testing"

// TestGroundAircraftKeepsAirfieldOnly 陆基机型投影到地图/任务栏时必须带 airfield_only，
// 否则前端会把它的航程块画成航母舰载机的样式（蓝底），看起来像从航母起飞的。
func TestGroundAircraftKeepsAirfieldOnly(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, unit := range GroundAircraftView(state, "B") {
		source := state.GroundAir[unit.ID]
		if source == nil || !source.AirfieldOnly {
			continue
		}
		checked++
		if !unit.AirfieldOnly {
			t.Fatalf("陆基算子 %s（%s）投影后丢了 airfield_only", unit.ID, unit.Model)
		}
	}
	if checked == 0 {
		t.Fatal("珊瑚海剧本里应有陆基机型可用于校验")
	}
}
