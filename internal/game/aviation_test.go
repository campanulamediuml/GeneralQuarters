package game

import "testing"

func sandboxState(t *testing.T) State {
	t.Helper()
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_sandbox")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestAviationProjectionIncludesCarriersAndSeaplaneShips(t *testing.T) {
	state := sandboxState(t)
	view := VisibleAviation(state, "B")
	if len(view.Carriers) != 10 || len(view.Tasks) != 0 {
		t.Fatalf("航空作业舰投影错误：%+v", view)
	}
	var carrier *CarrierAirStatus
	seaplanes := 0
	for index := range view.Carriers {
		item := &view.Carriers[index]
		if item.UnitID == "b-cv-01" {
			carrier = item
		}
		for _, aircraft := range item.Aircraft {
			if aircraft.Seaplane {
				seaplanes++
			}
		}
	}
	if carrier == nil || carrier.HangarUsed != 4 || carrier.HangarCapacity != 24 || carrier.DeckCapacity != 9 {
		t.Fatalf("航母机库或甲板容量错误：%+v", carrier)
	}
	if seaplanes != 6 {
		t.Fatalf("水上飞机数量错误：%d", seaplanes)
	}
	for _, unitID := range []string{"b-ca-01", "b-ca-02", "b-cl-01", "b2-bb-01", "b2-ca-01", "b2-cl-01"} {
		if state.Units[unitID].Card.HangarCapacity != 1 || state.Units[unitID].Card.Aircraft != 1 {
			t.Fatalf("指定水机舰能力或当前搭载错误：%+v", state.Units[unitID])
		}
	}
	for _, unitID := range []string{"b-bb-01", "b-ca-03", "b-ca-04", "b-cl-02", "b-dd-01", "b2-bb-02", "b2-ca-02", "b2-dd-01"} {
		if state.Units[unitID].Card.HangarCapacity != 0 || state.Units[unitID].Card.Aircraft != 0 {
			t.Fatalf("普通舰船被错误赋予水机作业能力：%+v", state.Units[unitID])
		}
	}
	viewA := VisibleAviation(state, "A")
	if len(viewA.Carriers) != 10 {
		t.Fatalf("A方两支编队的航空作业舰投影错误：%+v", viewA.Carriers)
	}
}

func TestCAPLaunchPriorityRecoveryAndFuel(t *testing.T) {
	state := sandboxState(t)
	state.CarrierOperations = nil
	first := Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, first)
	second := Order{Type: "LAUNCH_CAP", AircraftID: "air-b-02"}
	if err := ValidateAirOrder(state, "B", second); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, second)
	if state.Aircraft["air-b-01"].Priority != 1 || state.Aircraft["air-b-02"].Priority != 2 {
		t.Fatal("CAP初始任务优先级错误")
	}
	if state.Units["b-cv-01"].Card.Aircraft != 2 {
		t.Fatal("升空后机库数量未扣减")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "RECOVER_AIR", AircraftID: "air-b-01"}); err == nil {
		t.Fatal("同回合起飞后仍允许降落")
	}
	priority := Order{Type: "PRIORITIZE_AIR_TASK", AircraftID: "air-b-02"}
	ApplyAirOrder(&state, priority)
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 2 || view.Tasks[0].ID != "air-b-02" || view.Tasks[0].Priority != 1 || view.Tasks[1].ID != "air-b-01" {
		t.Fatalf("任务旗子未由服务端调整顺序：%+v", view.Tasks)
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-01"].RemainingRange != 12 || state.CarrierOperations["b-cv-01"] != "" {
		t.Fatal("回合燃油或航空作业状态重置错误")
	}
	recover := Order{Type: "RECOVER_AIR", AircraftID: "air-b-01"}
	if err := ValidateAirOrder(state, "B", recover); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, recover)
	if state.Aircraft["air-b-01"].Status != "hangar" || state.Aircraft["air-b-01"].Priority != 0 || state.Units["b-cv-01"].Card.Aircraft != 3 {
		t.Fatal("回收后的机库和任务状态错误")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"}); err == nil {
		t.Fatal("同回合降落后仍允许起飞")
	}
}

func TestAviationRejectsNonFighterAndDeckOverflow(t *testing.T) {
	state := sandboxState(t)
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-03"}); err == nil {
		t.Fatal("非战斗机被允许执行CAP")
	}
	state.Units["b-cv-01"].Card.DeckCapacity = 1
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"}); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"})
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-02"}); err == nil {
		t.Fatal("甲板容量溢出未被拒绝")
	}
}

func TestDistanceReferenceProjection(t *testing.T) {
	state := sandboxState(t)
	var reference *DistanceReference
	for _, fleet := range VisibleFleets(state, "A") {
		if fleet.FleetID == "TG-A" {
			reference = fleet.DistanceReference
		}
	}
	if reference == nil || reference.Origin != "0202" || reference.MaxDistance != 8 || len(reference.Cells) != 59 {
		t.Fatalf("舰队距离参考投影错误：%+v", reference)
	}
	for _, cell := range reference.Cells {
		if cell.Distance != Distance(reference.Origin, cell.Coord) {
			t.Fatalf("距离参考与服务端距离不一致：%+v", cell)
		}
	}
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"})
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 1 || tasks[0].CurrentCoord != "0208" || tasks[0].DistanceReference == nil {
		t.Fatalf("CAP距离参考缺失：%+v", tasks)
	}
	for _, cell := range tasks[0].DistanceReference.Cells {
		if cell.Coord == "0202" && cell.Distance != 6 {
			t.Fatalf("CAP中心到0202的距离应为6：%+v", cell)
		}
	}
}

func TestDeckPreparationReturnAndBatchCAP(t *testing.T) {
	state := sandboxState(t)
	prepare := Order{Type: "PREPARE_AIR", AircraftIDs: []string{"air-b-01", "air-b-02"}}
	if err := ValidateAirOrder(state, "B", prepare); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, prepare)
	if state.Aircraft["air-b-01"].Status != "deck" || state.Aircraft["air-b-02"].Status != "deck" || state.Units["b-cv-01"].Card.Aircraft != 4 {
		t.Fatal("批量提升甲板状态或母舰载机数错误")
	}
	carrier := VisibleAviation(state, "B").Carriers[3]
	if carrier.UnitID != "b-cv-01" || carrier.HangarUsed != 2 || carrier.DeckUsed != 2 || carrier.LaunchUsed != 0 {
		t.Fatalf("甲板与机库投影错误：%+v", carrier)
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"}); err == nil {
		t.Fatal("提前整备同回合仍允许起飞")
	}
	returnOrder := Order{Type: "RETURN_TO_HANGAR", AircraftIDs: []string{"air-b-01", "air-b-02"}}
	if err := ValidateAirOrder(state, "B", returnOrder); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, returnOrder)
	if state.Aircraft["air-b-01"].Status != "hangar" || state.CarrierOperations["b-cv-01"] != "" {
		t.Fatal("拉回机库未清除整备状态")
	}
	ApplyAirOrder(&state, prepare)
	AdvanceAviation(&state)
	launch := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"air-b-01", "air-b-02"}}
	if err := ValidateAirOrder(state, "B", launch); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, launch)
	if state.Aircraft["air-b-01"].TaskID != "air-b-01" || state.Aircraft["air-b-02"].TaskID != "air-b-01" || state.Units["b-cv-01"].Card.Aircraft != 2 {
		t.Fatal("批量CAP未形成同一任务或载机数错误")
	}
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 1 || len(tasks[0].AircraftIDs) != 2 || len(tasks[0].Aircraft) != 2 || tasks[0].ID != "air-b-01" {
		t.Fatalf("批量CAP任务投影错误：%+v", tasks)
	}
	AdvanceAviation(&state)
	recover := Order{Type: "RECOVER_AIR", AircraftIDs: []string{"air-b-01", "air-b-02"}}
	if err := ValidateAirOrder(state, "B", recover); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, recover)
	if state.Aircraft["air-b-01"].Status != "hangar" || state.Aircraft["air-b-02"].Status != "hangar" || state.Units["b-cv-01"].Card.Aircraft != 4 {
		t.Fatal("批量回收未恢复机库状态")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "PREPARE_AIR", AircraftID: "air-b-11"}); err == nil {
		t.Fatal("水机舰被错误提供飞行甲板")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"air-b-01", "air-b-05"}}); err == nil {
		t.Fatal("跨母舰批量命令未被拒绝")
	}
}
