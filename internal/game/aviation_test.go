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

func setAircraftReady(state *State, ids ...string) {
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		aircraft.Status = "deck"
		if aircraft.Seaplane {
			aircraft.Status = "ready"
		}
	}
}

func TestAviationProjectionIncludesIndependentShipDecks(t *testing.T) {
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
	if carrier == nil || carrier.HangarUsed != 4 || carrier.HangarCapacity != 24 || carrier.DeckLength != 9 || len(carrier.Elevators) != 3 {
		t.Fatalf("航母甲板或升降机投影错误：%+v", carrier)
	}
	for _, elevator := range carrier.Elevators {
		if elevator.Status != "idle" || elevator.Capacity != 2 || elevator.Kind != "elevator" {
			t.Fatalf("航母升降机初始状态错误：%+v", elevator)
		}
	}
	cruiser := state.Units["b-ca-01"]
	if cruiser.Deck == nil || cruiser.Deck.Length != 1 || len(cruiser.Deck.Elevators) != 1 || cruiser.Deck.Elevators[0].Kind != "crane" {
		t.Fatalf("水机舰没有独立甲板和起重设备：%+v", cruiser.Deck)
	}
	if seaplanes != 11 {
		t.Fatalf("水上飞机数量错误：%d", seaplanes)
	}
	for _, unitID := range []string{"b-ca-01", "b-ca-02", "b-cl-01", "b2-ca-01", "b2-cl-01"} {
		if state.Units[unitID].Card.HangarCapacity != 2 || state.Units[unitID].Card.Aircraft != 2 {
			t.Fatalf("巡洋舰水机机库或当前搭载错误：%+v", state.Units[unitID])
		}
	}
	for _, unitID := range []string{"b-bb-01", "b-ca-03", "b-ca-04", "b-cl-02", "b-dd-01", "b2-bb-02", "b2-ca-02", "b2-dd-01"} {
		if state.Units[unitID].Deck != nil {
			t.Fatalf("普通舰船被错误赋予航空设施：%+v", state.Units[unitID])
		}
	}
}

func TestCAPLaunchPriorityAndElevatorRecovery(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-01", "air-b-02")
	for _, id := range []string{"air-b-01", "air-b-02"} {
		order := Order{Type: "LAUNCH_CAP", AircraftID: id}
		if err := ValidateAirOrder(state, "B", order); err != nil {
			t.Fatal(err)
		}
		ApplyAirOrder(&state, order)
	}
	if state.Aircraft["air-b-01"].Priority != 1 || state.Aircraft["air-b-02"].Priority != 1 || state.Aircraft["air-b-01"].TaskID != "cap-TG-B" || state.Aircraft["air-b-02"].TaskID != "cap-TG-B" || state.Units["b-cv-01"].Card.Aircraft != 2 {
		t.Fatal("CAP编组、优先级或载机数错误")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "RECOVER_AIR", AircraftID: "air-b-01"}); err == nil {
		t.Fatal("同回合起飞后仍允许降落")
	}
	ApplyAirOrder(&state, Order{Type: "PRIORITIZE_AIR_TASK", AircraftID: "air-b-02"})
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 1 || view.Tasks[0].ID != "cap-TG-B" || len(view.Tasks[0].Aircraft) != 2 || view.Tasks[0].Priority != 1 {
		t.Fatalf("CAP没有合并为单任务单旗帜：%+v", view.Tasks)
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-01"].RemainingRange != 12 || state.Units["b-cv-01"].Deck.Operation != "" {
		t.Fatal("回合燃油或甲板作业状态重置错误")
	}
	recover := Order{Type: "RECOVER_AIR", AircraftID: "air-b-01"}
	if err := ValidateAirOrder(state, "B", recover); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, recover)
	if state.Aircraft["air-b-01"].Status != "recovery" || state.Units["b-cv-01"].Card.Aircraft != 3 {
		t.Fatal("回收飞机没有先降落到甲板")
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-01"].Status != "elevator" || state.Units["b-cv-01"].Deck.Elevators[0].Status != "lowering" {
		t.Fatal("回收飞机没有占用升降机下降")
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-01"].Status != "hangar" || state.Aircraft["air-b-01"].Priority != 0 {
		t.Fatal("升降机下降完成后未回到机库")
	}
}

func TestAviationRejectsNonFighterAndDeckLaunchOverflow(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-01", "air-b-02", "air-b-03")
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-03"}); err == nil {
		t.Fatal("非战斗机被允许执行CAP")
	}
	state.Units["b-cv-01"].Deck.Length = 1
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"}); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"})
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "air-b-02"}); err == nil {
		t.Fatal("甲板起飞容量溢出未被拒绝")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "PREPARE_AIR", AircraftID: "air-b-02"}); err == nil {
		t.Fatal("仍允许不绑定任务的手动整备")
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
	setAircraftReady(&state, "air-b-01")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"})
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 1 || tasks[0].CurrentCoord != "0208" || tasks[0].DistanceReference == nil {
		t.Fatalf("CAP距离参考缺失：%+v", tasks)
	}
}

func TestAssignedTasksUseElevatorsBeforeLaunch(t *testing.T) {
	state := sandboxState(t)
	orders := []Order{
		{Type: "LAUNCH_CAP", AircraftIDs: []string{"air-b-01", "air-b-02"}},
		{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60},
		{Type: "LAUNCH_RECON", AircraftID: "air-b-11", Heading: "W", SectorWidth: 120},
	}
	for _, order := range orders {
		if err := ValidateAirOrder(state, "B", order); err != nil {
			t.Fatalf("机库算子应可先分配任务：%v", err)
		}
		ApplyAirOrder(&state, order)
	}
	for _, id := range []string{"air-b-01", "air-b-02", "air-b-10", "air-b-11"} {
		if state.Aircraft[id].Status != "service" || state.Aircraft[id].TaskKind == "" {
			t.Fatalf("分配任务后未进入整备：%+v", state.Aircraft[id])
		}
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "servicing" || len(task.SectorHexes) != 0 {
			t.Fatalf("预备任务阶段或扇面投影错误：%+v", task)
		}
	}

	AdvanceAviation(&state)
	for _, id := range []string{"air-b-01", "air-b-02", "air-b-10", "air-b-11"} {
		if state.Aircraft[id].Status != "elevator" || state.Aircraft[id].ElevatorID == "" {
			t.Fatalf("第二回合没有占用具体升降机：%+v", state.Aircraft[id])
		}
	}
	if state.Units["b-cv-01"].Deck.Elevators[0].Status != "lifting" || state.Units["b-cv-04"].Deck.Elevators[0].TaskID != "air-b-10" {
		t.Fatal("升降机没有保存正在提升的任务")
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "lifting" {
			t.Fatalf("任务栏未显示提升阶段：%+v", task)
		}
	}

	AdvanceAviation(&state)
	if state.Aircraft["air-b-01"].Status != "cap" || state.Aircraft["air-b-10"].Status != "recon" || state.Aircraft["air-b-11"].Status != "recon" {
		t.Fatal("第三回合任务未在提升完成后放飞")
	}
	if state.Aircraft["air-b-10"].OriginCoord != "0208" || state.Aircraft["air-b-10"].ReconDistance != 0 || state.Aircraft["air-b-11"].SectorWidth != 120 {
		t.Fatal("自动起飞没有使用保存的侦察配置和实际母舰位置")
	}
	if state.Units["b-cv-01"].Card.Aircraft != 2 || state.Units["b-cv-04"].Card.Aircraft != 1 || state.Units["b-cv-01"].Deck.Elevators[0].Status != "idle" {
		t.Fatal("放飞后的载机数或升降机释放状态错误")
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "launching" {
			t.Fatalf("任务栏未显示起飞中：%+v", task)
		}
	}

	AdvanceAviation(&state)
	if state.Aircraft["air-b-01"].RemainingRange != 12 || state.Aircraft["air-b-10"].ReconDistance != 2 || state.Aircraft["air-b-11"].ReconDistance != 1 {
		t.Fatal("自动起飞后的下一回合没有进入正常任务结算")
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "active" {
			t.Fatalf("已起飞任务没有进入执行阶段：%+v", task)
		}
	}
}

func TestSingleElevatorBatchesAndDeckLengthDelay(t *testing.T) {
	state := sandboxState(t)
	for index, id := range []string{"air-test-02", "air-test-03"} {
		clone := *state.Aircraft["air-b-09"]
		clone.ID = id
		state.Aircraft[id] = &clone
		state.Units["b-cv-04"].Card.Aircraft++
		_ = index
	}
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"air-b-09", "air-test-02", "air-test-03"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	state.Units["b-cv-04"].Deck.Length = 1
	AdvanceAviation(&state)
	elevator := state.Units["b-cv-04"].Deck.Elevators[0]
	if len(elevator.AircraftIDs) != 1 || elevator.Status != "lifting" {
		t.Fatalf("甲板长度没有限制本次提升数量：%+v", elevator)
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-09"].Status != "cap" || state.Aircraft["air-test-02"].Status != "elevator" || state.Aircraft["air-test-03"].Status != "service" {
		t.Fatalf("单升降机没有按甲板容量连续排队：%s/%s/%s", state.Aircraft["air-b-09"].Status, state.Aircraft["air-test-02"].Status, state.Aircraft["air-test-03"].Status)
	}
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	for _, id := range []string{"air-b-09", "air-test-02", "air-test-03"} {
		if state.Aircraft[id].Status != "cap" {
			t.Fatalf("排队算子没有最终独立放飞：%s=%s", id, state.Aircraft[id].Status)
		}
	}
}

func TestAttackFormationWaitsForOtherCarrier(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	state.Contacts["B"]["TG-A"] = &FleetContact{FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14}
	first := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"air-b-03", "air-b-04"}, Coord: "0202"}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, first)
	join := Order{Type: "LAUNCH_STRIKE", AircraftID: "air-b-06", TaskID: "air-b-03"}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("同一舰队另一艘航母不能加入攻击编队：%v", err)
	}
	ApplyAirOrder(&state, join)
	state.Aircraft["air-b-05"].Status = "deck"
	state.Units["b-cv-02"].Deck.Length = 1
	AdvanceAviation(&state)
	if state.Aircraft["air-b-03"].Status != "elevator" || state.Aircraft["air-b-06"].Status != "service" {
		t.Fatal("各舰没有独立调度甲板与升降机")
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-03"].Status != "attack_holding" || state.Aircraft["air-b-04"].Status != "attack_holding" || state.Aircraft["air-b-06"].Status != "service" {
		t.Fatal("攻击机不应在编队未到齐时离场")
	}
	initialRange := state.Aircraft["air-b-03"].RemainingRange
	state.Units["b-cv-02"].Deck.Length = 6
	AdvanceAviation(&state)
	if state.Aircraft["air-b-03"].RemainingRange != initialRange-1 || state.Aircraft["air-b-03"].Status != "attack_holding" || state.Aircraft["air-b-06"].Status != "elevator" {
		t.Fatal("等待中的攻击机没有按回合消耗1航程，或另一艘航母没有独立提升")
	}
	AdvanceAviation(&state)
	if state.Aircraft["air-b-03"].RemainingRange != initialRange-2 || state.Aircraft["air-b-03"].Status != "attack" || state.Aircraft["air-b-06"].Status != "attack" {
		t.Fatal("最后一批飞机到齐后整个攻击编队没有离场")
	}
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 1 || tasks[0].Phase != "launching" || len(tasks[0].CarrierIDs) != 2 || tasks[0].AirborneCount != 3 {
		t.Fatalf("跨舰攻击编队投影错误：%+v", tasks)
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_STRIKE", AircraftID: "air-b-08", TaskID: "air-b-03"}); err == nil {
		t.Fatal("已经离场的攻击编队仍允许加入飞机")
	}
}

func TestReturningAssignedAircraftCancelsOrQueuesDescent(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_CAP", AircraftID: "air-b-01"}
	ApplyAirOrder(&state, order)
	returnOrder := Order{Type: "RETURN_TO_HANGAR", AircraftID: "air-b-01"}
	if err := ValidateAirOrder(state, "B", returnOrder); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, returnOrder)
	aircraft := state.Aircraft["air-b-01"]
	if aircraft.Status != "hangar" || aircraft.TaskID != "" || aircraft.TaskKind != "" || aircraft.Priority != 0 {
		t.Fatalf("机库整备任务未取消：%+v", aircraft)
	}
	setAircraftReady(&state, "air-b-02")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "air-b-02"})
	setAircraftReady(&state, "air-b-04")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftID: "air-b-04"})
	setAircraftReady(&state, "air-b-01")
	if err := ValidateAirOrder(state, "B", returnOrder); err == nil {
		t.Fatal("甲板起飞作业中仍允许安排下降")
	}
}

func TestRepeatedJoinOfCurrentTaskIsIdempotent(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	before := *state.Aircraft["air-b-10"]
	repeated := order
	repeated.TaskID = "air-b-10"
	repeated.Heading = "E"
	repeated.SectorWidth = 120
	if err := ValidateAirOrder(state, "B", repeated); err != nil {
		t.Fatalf("重复加入当前任务不应报错：%v", err)
	}
	events := ApplyAirOrder(&state, repeated)
	after := *state.Aircraft["air-b-10"]
	if after != before {
		t.Fatalf("重复加入当前任务改变了状态：之前=%+v 之后=%+v", before, after)
	}
	if len(events) != 1 || events[0].Type != "air_task_unchanged" {
		t.Fatalf("重复加入当前任务没有返回幂等结果：%+v", events)
	}
}

func TestReconGroupsAcceptDifferentAircraftAndRepeatedJoins(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-03", "air-b-04", "air-b-08", "air-b-10")
	first := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-03", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, first)
	join := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-04", TaskID: "air-b-03", Heading: "E", SectorWidth: 120}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("不同可搜索机型不能加入同一侦察组：%v", err)
	}
	ApplyAirOrder(&state, join)
	for _, id := range []string{"air-b-03", "air-b-04"} {
		aircraft := state.Aircraft[id]
		if aircraft.TaskID != "air-b-03" || aircraft.Heading != "W" || aircraft.SectorWidth != 60 {
			t.Fatalf("加入侦察组后没有共享任务ID和原组参数：%+v", aircraft)
		}
	}
	second := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-08", Heading: "W", SectorWidth: 120}
	if err := ValidateAirOrder(state, "B", second); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, second)
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60}); err == nil {
		t.Fatal("已有两个侦察组时仍允许创建第三组")
	}
	AdvanceAviation(&state)
	joinAirborne := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", TaskID: "air-b-03", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", joinAirborne); err != nil {
		t.Fatalf("已升空侦察组不能接受后续飞机：%v", err)
	}
	ApplyAirOrder(&state, joinAirborne)
	if state.Aircraft["air-b-10"].TaskID != "air-b-03" || state.Aircraft["air-b-10"].Status != "recon" {
		t.Fatalf("后续飞机没有直接加入已升空侦察组：%+v", state.Aircraft["air-b-10"])
	}
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 2 {
		t.Fatalf("侦察组数量错误：%+v", tasks)
	}
	for _, task := range tasks {
		if task.ID == "air-b-03" && (len(task.Aircraft) != 3 || task.AirborneCount != 3 || task.Phase != "launching") {
			t.Fatalf("混合侦察组升空投影错误：%+v", task)
		}
	}
}

func TestReconGroupsAreScopedToFleet(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-14", "air-b-15", "air-b-16", "air-b-03")
	for _, id := range []string{"air-b-14", "air-b-15"} {
		order := Order{Type: "LAUNCH_RECON", AircraftID: id, Heading: "W", SectorWidth: 60}
		if err := ValidateAirOrder(state, "B", order); err != nil {
			t.Fatal(err)
		}
		ApplyAirOrder(&state, order)
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: "air-b-16", Heading: "W", SectorWidth: 60}); err == nil {
		t.Fatal("同一舰队已有两组侦察时仍允许创建第三组")
	}
	otherFleet := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-03", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", otherFleet); err != nil {
		t.Fatalf("另一支舰队不能创建自己的侦察组：%v", err)
	}
	ApplyAirOrder(&state, otherFleet)
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: "air-b-16", TaskID: "air-b-03", Heading: "W", SectorWidth: 60}); err == nil {
		t.Fatal("飞机可以跨舰队加入侦察组")
	}
	fleets := map[string]bool{}
	for _, task := range VisibleAviation(state, "B").Tasks {
		fleets[task.FleetID] = true
	}
	if !fleets["TG-B"] || !fleets["TG-B-BB"] {
		t.Fatalf("侦察任务没有保留舰队归属：%+v", fleets)
	}
}

func TestRecoverCAPAcrossCarriers(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-05", "air-b-07")
	for _, id := range []string{"air-b-05", "air-b-07"} {
		ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: id})
	}
	AdvanceAviation(&state)
	order := Order{Type: "RECOVER_AIR", AircraftIDs: []string{"air-b-05", "air-b-07"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("跨母舰CAP回收被拒绝：%v", err)
	}
	ApplyAirOrder(&state, order)
	for _, id := range []string{"air-b-05", "air-b-07"} {
		aircraft := state.Aircraft[id]
		carrier := state.Units[aircraft.CarrierID]
		if aircraft.Status != "recovery" || aircraft.TaskID != "" || carrier.Deck.Operation != "recover" {
			t.Fatalf("跨母舰回收状态错误：%+v %+v", aircraft, carrier)
		}
	}
}
