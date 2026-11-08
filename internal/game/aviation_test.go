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
