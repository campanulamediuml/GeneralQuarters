package game

import "testing"

func TestSunkCarrierAircraftFlyToSameFleetCarrierFirst(t *testing.T) {
	state := sandboxState(t)
	origin := state.Units["b-cv-01"]
	sameFleet := state.Units["b-cv-02"]
	otherFleet := state.Units["b-cv-03"]
	if origin == nil || sameFleet == nil || otherFleet == nil {
		t.Fatal("沙盒缺少测试用航母")
	}
	origin.Coord = "0101"
	sameFleet.Coord = "0105"
	otherFleet.Coord = "0102"
	origin.FleetID = "TG-ORIGINAL"
	sameFleet.FleetID = "TG-ORIGINAL"
	otherFleet.FleetID = "TG-OTHER"
	for id, unit := range state.Units {
		if unit.Faction == "B" && IsCarrier(unit) && id != origin.ID && id != sameFleet.ID && id != otherFleet.ID {
			delete(state.Units, id)
		}
	}

	plane := &AirUnit{ID: "cap-orphan", CarrierID: origin.ID, Faction: "B", Kind: "fighter", Model: "A6M2",
		Status: "cap", Strength: 2, AirCombat: 5, TaskID: "cap-orphan", TaskKind: "cap",
		Coord: origin.Coord, OriginCoord: origin.Coord, RemainingRange: 20, MaxRange: 20}
	state.Aircraft = map[string]*AirUnit{plane.ID: plane}
	sinkUnit(&state, origin)
	AdvanceAviation(&state)

	if plane.CarrierID != sameFleet.ID {
		t.Fatalf("应优先选择原舰队航母，而不是更近的其他舰队航母：%s", plane.CarrierID)
	}
	if !plane.Returning || plane.Status != "recovery" || !airborne(plane) {
		t.Fatalf("飞机应仍处于返航途中：status=%s returning=%v", plane.Status, plane.Returning)
	}
	if plane.Coord == sameFleet.Coord || plane.Range() >= 20 {
		t.Fatalf("飞机应逐格飞行并消耗航程：coord=%s range=%d", plane.Coord, plane.Range())
	}
	if used := deckUsed(state, sameFleet.ID); used != 0 {
		t.Fatalf("返航途中不应占用新母舰甲板：%d", used)
	}
	if used := aboardAircraft(state, sameFleet.ID); used != 0 {
		t.Fatalf("返航途中不应占用新母舰机库：%d", used)
	}

	for range 4 {
		if !plane.Returning {
			break
		}
		AdvanceAviation(&state)
	}
	if plane.Returning || plane.CarrierID != sameFleet.ID {
		t.Fatalf("飞抵后应在新母舰完成回收：status=%s returning=%v carrier=%s", plane.Status, plane.Returning, plane.CarrierID)
	}
	if plane.Status != "recovery" && plane.Status != "elevator" {
		t.Fatalf("实际降落后应等待或开始下降：%s", plane.Status)
	}
	if used := aboardAircraft(state, sameFleet.ID); used != 1 {
		t.Fatalf("实际降落后才应计入在舰飞机：%d", used)
	}
}

func TestSunkCarrierAircraftFlyToAirfieldBeforeLanding(t *testing.T) {
	state := sandboxState(t)
	origin := state.Units["b-cv-01"]
	airfield := state.Airfields["af-b-sentry"]
	if origin == nil || airfield == nil {
		t.Fatal("沙盒缺少测试用航母或机场")
	}
	for id, unit := range state.Units {
		if unit.Faction == "B" && IsCarrier(unit) && id != origin.ID {
			delete(state.Units, id)
		}
	}
	for id, candidate := range state.Airfields {
		if candidate.Faction == "B" && id != airfield.ID {
			delete(state.Airfields, id)
		}
	}
	origin.Coord = "0101"
	plane := &AirUnit{ID: "cap-airfield", CarrierID: origin.ID, Faction: "B", Kind: "fighter", Model: "A6M2",
		Status: "cap", Strength: 2, AirCombat: 5, TaskID: "cap-airfield", TaskKind: "cap",
		Coord: origin.Coord, OriginCoord: origin.Coord, RemainingRange: 30, MaxRange: 30}
	state.Aircraft = map[string]*AirUnit{plane.ID: plane}
	sinkUnit(&state, origin)
	AdvanceAviation(&state)

	if state.Aircraft[plane.ID] == nil || state.GroundAir[plane.ID] != nil {
		t.Fatal("尚未飞抵机场时不应直接进入机场机库")
	}
	if plane.DivertAirfield != airfield.ID || plane.Coord == airfield.Coord {
		t.Fatalf("应选定机场并逐格飞行：divert=%s coord=%s target=%s", plane.DivertAirfield, plane.Coord, airfield.Coord)
	}
	for range 12 {
		if state.Aircraft[plane.ID] == nil {
			break
		}
		AdvanceAviation(&state)
	}
	landed := state.GroundAir[plane.ID]
	if landed == nil || landed.AirfieldID != airfield.ID {
		t.Fatalf("飞抵机场后应进入对应机场：%+v", landed)
	}
}
