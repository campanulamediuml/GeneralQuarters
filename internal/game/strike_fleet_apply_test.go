package game

import "testing"

// 无线电改目标同样要点名"打哪支舰队"：同格停着两支敌舰队时，
// 按情报顺序认领第一支等于玩家白点了。这里直接量 applyStrikeFleet 的落点。
func TestApplyStrikeFleetUsesDisplayCoord(t *testing.T) {
	state := sandboxState(t)
	contact := &FleetContact{FleetID: "TG-B", Faction: "B", Level: 3, Coord: "0506", DisplayCoord: "0505", Count: 6}
	state.Contacts["A"] = map[string]*FleetContact{"TG-B": contact}
	aircraft := &AirUnit{ID: "strike-lead", Faction: "A", TaskKind: "attack", TargetCoord: "0606", TargetFleetID: ""}
	applyStrikeFleet(state, aircraft, "A", "TG-B")
	if aircraft.TargetFleetID != "TG-B" {
		t.Fatalf("没有认领指定舰队：%q", aircraft.TargetFleetID)
	}
	if want := intelDisplayCoord(*contact); aircraft.TargetCoord != want {
		t.Fatalf("目标坐标应落在情报显示位置 %s，实际 %s", want, aircraft.TargetCoord)
	}

	// 空 ID（点的是空格/机场）不能把原目标清掉，也不能凭空认领一支舰队。
	before := *aircraft
	applyStrikeFleet(state, aircraft, "A", "")
	if *aircraft != before {
		t.Fatalf("空舰队 ID 不应改动目标：%+v", *aircraft)
	}
}
