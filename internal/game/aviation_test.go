package game

import "testing"

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
	// 机库容量按角色卡（日军航母 18、美军 24），这里只校验投影与角色卡一致。
	// 甲板长度是"基础容量 + 风向修正"，开局就该是修正后的值（规则书 817–824 行）。
	wantHangar := state.Units["b-cv-01"].Card.HangarCapacity
	wantDeck := deckLengthFor(state, state.Units["b-cv-01"])
	if carrier == nil || carrier.HangarCapacity != wantHangar || carrier.DeckLength != wantDeck || len(carrier.Elevators) != 3 || carrier.HangarUsed == 0 || carrier.HangarUsed > carrier.HangarCapacity {
		t.Fatalf("航母甲板或升降机投影错误：%+v", carrier)
	}
	for _, elevator := range carrier.Elevators {
		if elevator.Status != "idle" || elevator.Capacity != 4 || elevator.Kind != "elevator" {
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
	initialCarrierAircraft := state.Units["b-cv-01"].Card.Aircraft
	setAircraftReady(&state, "b-cv-01-fighter-x1", "b-cv-01-fighter-x2")
	for _, id := range []string{"b-cv-01-fighter-x1", "b-cv-01-fighter-x2"} {
		order := Order{Type: "LAUNCH_CAP", AircraftID: id}
		if err := ValidateAirOrder(state, "B", order); err != nil {
			t.Fatal(err)
		}
		ApplyAirOrder(&state, order)
	}
	if state.Aircraft["b-cv-01-fighter-x1"].Priority != 1 || state.Aircraft["b-cv-01-fighter-x2"].Priority != 1 || state.Aircraft["b-cv-01-fighter-x1"].TaskID != "cap-TG-B" || state.Aircraft["b-cv-01-fighter-x2"].TaskID != "cap-TG-B" || state.Units["b-cv-01"].Card.Aircraft != initialCarrierAircraft-2 {
		t.Fatal("CAP编组、优先级或载机数错误")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "RECOVER_AIR", AircraftID: "b-cv-01-fighter-x1"}); err == nil {
		t.Fatal("同回合起飞后仍允许降落")
	}
	ApplyAirOrder(&state, Order{Type: "PRIORITIZE_AIR_TASK", AircraftID: "b-cv-01-fighter-x2"})
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 1 || view.Tasks[0].ID != "cap-TG-B" || len(view.Tasks[0].Aircraft) != 2 || view.Tasks[0].Priority != 1 {
		t.Fatalf("CAP没有合并为单任务单旗帜：%+v", view.Tasks)
	}
	AdvanceAviation(&state)
	// 主燃油 = 基础航程 × 副油箱系数。上一回合已消耗 1 点，
	// 这里校验剩余航程与甲板作业状态都被正确重置。
	f6f := state.Aircraft["b-cv-01-fighter-x1"]
	wantRange := SortieBaseRange(f6f.BaseRange, f6f.Loadout) - 1
	if f6f.Range() != wantRange || state.Units["b-cv-01"].Deck.Operation != "" {
		t.Fatalf("回合燃油或甲板作业状态重置错误：range=%d（期望%d）", f6f.Range(), wantRange)
	}
	recover := Order{Type: "RECOVER_AIR", AircraftID: "b-cv-01-fighter-x1"}
	if err := ValidateAirOrder(state, "B", recover); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, recover)
	if state.Aircraft["b-cv-01-fighter-x1"].Status != "recovery" || state.Units["b-cv-01"].Card.Aircraft != initialCarrierAircraft-1 {
		t.Fatal("回收飞机没有先降落到甲板")
	}
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-fighter-x1"].Status != "elevator" || state.Units["b-cv-01"].Deck.Elevators[0].Status != "lowering" {
		t.Fatal("回收飞机没有占用升降机下降")
	}
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-fighter-x1"].Status != "hangar" || state.Aircraft["b-cv-01-fighter-x1"].Priority != 0 {
		t.Fatal("升降机下降完成后未回到机库")
	}
}

func TestAviationRejectsNonFighterAndDeckLaunchOverflow(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "b-cv-01-fighter-x1", "b-cv-01-fighter-x2", "b-cv-01-dive_bomber-x1")
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-dive_bomber-x1"}); err == nil {
		t.Fatal("非战斗机被允许执行CAP")
	}
	state.Units["b-cv-01"].Deck.Length = 1
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x1"}); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x1"})
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x2"}); err == nil {
		t.Fatal("甲板起飞容量溢出未被拒绝")
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "PREPARE_AIR", AircraftID: "b-cv-01-fighter-x2"}); err == nil {
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
	setAircraftReady(&state, "b-cv-01-fighter-x1")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x1"})
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 1 || tasks[0].CurrentCoord != "0208" || tasks[0].DistanceReference == nil {
		t.Fatalf("CAP距离参考缺失：%+v", tasks)
	}
}

func TestAssignedTasksUseElevatorsBeforeLaunch(t *testing.T) {
	state := sandboxState(t)
	initialAircraftGo := state.Units["b-cv-01"].Card.Aircraft
	initialAircraftGo4 := state.Units["b-cv-04"].Card.Aircraft
	orders := []Order{
		{Type: "LAUNCH_CAP", AircraftIDs: []string{"b-cv-01-fighter-x1", "b-cv-01-fighter-x2"}},
		{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 60},
		{Type: "LAUNCH_RECON", AircraftID: "air-b-11", Heading: "W", SectorWidth: 120},
	}
	for _, order := range orders {
		if err := ValidateAirOrder(state, "B", order); err != nil {
			t.Fatalf("机库算子应可先分配任务：%v", err)
		}
		ApplyAirOrder(&state, order)
	}
	for _, id := range []string{"b-cv-01-fighter-x1", "b-cv-01-fighter-x2", "b-cv-04-dive_bomber-x1", "air-b-11"} {
		if state.Aircraft[id].Status != "service" || state.Aircraft[id].TaskKind == "" {
			t.Fatalf("分配任务后未进入整备：%+v", state.Aircraft[id])
		}
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "servicing" || len(task.SectorHexes) != 0 {
			t.Fatalf("预备任务阶段或扇面投影错误：%+v", task)
		}
	}

	primeService(&state)
	AdvanceAviation(&state)
	for _, id := range []string{"b-cv-01-fighter-x1", "b-cv-01-fighter-x2", "b-cv-04-dive_bomber-x1", "air-b-11"} {
		if state.Aircraft[id].Status != "elevator" || state.Aircraft[id].ElevatorID == "" {
			t.Fatalf("第二回合没有占用具体升降机：%+v", state.Aircraft[id])
		}
	}
	if state.Units["b-cv-01"].Deck.Elevators[0].Status != "lifting" || state.Units["b-cv-04"].Deck.Elevators[0].TaskID != "b-cv-04-dive_bomber-x1" {
		t.Fatal("升降机没有保存正在提升的任务")
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "lifting" {
			t.Fatalf("任务栏未显示提升阶段：%+v", task)
		}
	}

	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-fighter-x1"].Status != "cap" || state.Aircraft["b-cv-04-dive_bomber-x1"].Status != "recon" || state.Aircraft["air-b-11"].Status != "recon" {
		t.Fatal("第三回合任务未在提升完成后放飞")
	}
	if state.Aircraft["b-cv-04-dive_bomber-x1"].OriginCoord != "0208" || state.Aircraft["b-cv-04-dive_bomber-x1"].ReconDistance != 0 || state.Aircraft["air-b-11"].SectorWidth != 120 {
		t.Fatal("自动起飞没有使用保存的侦察配置和实际母舰位置")
	}
	if state.Units["b-cv-01"].Card.Aircraft != initialAircraftGo-2 || state.Units["b-cv-04"].Card.Aircraft != initialAircraftGo4-1 || state.Units["b-cv-01"].Deck.Elevators[0].Status != "idle" {
		t.Fatal("放飞后的载机数或升降机释放状态错误")
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "launching" {
			t.Fatalf("任务栏未显示起飞中：%+v", task)
		}
	}

	AdvanceAviation(&state)
	capRange := SortieBaseRange(state.Aircraft["b-cv-01-fighter-x1"].BaseRange, state.Aircraft["b-cv-01-fighter-x1"].Loadout) - 1
	if state.Aircraft["b-cv-01-fighter-x1"].RemainingRange != capRange || state.Aircraft["b-cv-04-dive_bomber-x1"].ReconDistance != 2 || state.Aircraft["air-b-11"].ReconDistance != 1 {
		t.Fatal("自动起飞后的下一回合没有进入正常任务结算")
	}
	for _, task := range VisibleAviation(state, "B").Tasks {
		if task.Phase != "active" {
			t.Fatalf("已起飞任务没有进入执行阶段：%+v", task)
		}
	}
}

func TestCapAndReconJoiningLaterKeepIndependentStages(t *testing.T) {
	tests := []struct {
		name        string
		firstID     string
		secondID    string
		firstOrder  Order
		secondOrder Order
		airborne    string
	}{
		{
			name: "CAP", firstID: "b-cv-01-fighter-x1", secondID: "b-cv-01-fighter-x2", airborne: "cap",
			firstOrder:  Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x1"},
			secondOrder: Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x2"},
		},
		{
			name: "侦察", firstID: "b-cv-01-dive_bomber-x1", secondID: "b-cv-01-torpedo_bomber-x1", airborne: "recon",
			firstOrder:  Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60},
			secondOrder: Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-torpedo_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := sandboxState(t)
			if err := ValidateAirOrder(state, "B", test.firstOrder); err != nil {
				t.Fatal(err)
			}
			ApplyAirOrder(&state, test.firstOrder)
			primeService(&state)
			AdvanceAviation(&state)
			if state.Aircraft[test.firstID].Status != "elevator" {
				t.Fatalf("第一架未进入提升：%+v", state.Aircraft[test.firstID])
			}
			if err := ValidateAirOrder(state, "B", test.secondOrder); err != nil {
				t.Fatal(err)
			}
			ApplyAirOrder(&state, test.secondOrder)
			if state.Aircraft[test.firstID].Status != "elevator" || state.Aircraft[test.secondID].Status != "service" {
				t.Fatalf("第二回合加入时阶段被合并：第一架=%s 第二架=%s", state.Aircraft[test.firstID].Status, state.Aircraft[test.secondID].Status)
			}
			// 后加入的第二架在机库里整备，第一架已经在上电梯。
			if state.Aircraft[test.secondID].ServiceTurns != 0 {
				t.Fatalf("新加入的算子应重新开始机库整备：%d", state.Aircraft[test.secondID].ServiceTurns)
			}
			AdvanceAviation(&state)
			// 第一架放飞，第二架此时才整备完成（ServiceTurns=1）：引入整备计数后，
			// "后加入的算子"要多花一回合在机库里，这符合规则书 231–236 行。
			if state.Aircraft[test.firstID].Status != test.airborne || state.Aircraft[test.secondID].Status != "service" {
				t.Fatalf("第一架放飞时带走了第二架：第一架=%s 第二架=%s", state.Aircraft[test.firstID].Status, state.Aircraft[test.secondID].Status)
			}
			AdvanceAviation(&state)
			if state.Aircraft[test.secondID].Status != "elevator" {
				t.Fatalf("第二架没有轮到自己提升：%s", state.Aircraft[test.secondID].Status)
			}
			tasks := VisibleAviation(state, "B").Tasks
			if len(tasks) != 1 || tasks[0].AirborneCount != 1 || tasks[0].Phase != "lifting" {
				t.Fatalf("分批任务投影没有保留部分升空和提升中：%+v", tasks)
			}
			AdvanceAviation(&state)
			if state.Aircraft[test.secondID].Status != test.airborne {
				t.Fatalf("第二架没有在自己的提升完成后放飞：%+v", state.Aircraft[test.secondID])
			}
		})
	}
}

func TestSingleElevatorBatchesAndDeckLengthDelay(t *testing.T) {
	state := sandboxState(t)
	for index, id := range []string{"zz-test-02", "zz-test-03"} {
		clone := *state.Aircraft["b-cv-04-fighter-x1"]
		clone.ID = id
		state.Aircraft[id] = &clone
		state.Units["b-cv-04"].Card.Aircraft++
		_ = index
	}
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"b-cv-04-fighter-x1", "zz-test-02", "zz-test-03"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	state.Units["b-cv-04"].Deck.Length = 1
	primeService(&state)
	AdvanceAviation(&state)
	elevator := state.Units["b-cv-04"].Deck.Elevators[0]
	if len(elevator.AircraftIDs) != 1 || elevator.Status != "lifting" {
		t.Fatalf("甲板长度没有限制本次提升数量：%+v", elevator)
	}
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-04-fighter-x1"].Status != "cap" || state.Aircraft["zz-test-02"].Status != "elevator" || state.Aircraft["zz-test-03"].Status != "service" {
		t.Fatalf("单升降机没有按甲板容量连续排队：%s/%s/%s", state.Aircraft["b-cv-04-fighter-x1"].Status, state.Aircraft["zz-test-02"].Status, state.Aircraft["zz-test-03"].Status)
	}
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	for _, id := range []string{"b-cv-04-fighter-x1", "zz-test-02", "zz-test-03"} {
		if state.Aircraft[id].Status != "cap" {
			t.Fatalf("排队算子没有最终独立放飞：%s=%s", id, state.Aircraft[id].Status)
		}
	}
}

func TestAttackFormationWaitsForOtherCarrier(t *testing.T) {
	state := sandboxState(t)
	state.Contacts["B"] = map[string]*FleetContact{}
	// 打击目标要用"显示位置"：情报等级 1 时真实坐标不可见（规则书 144 行）。
	contact := &FleetContact{FleetID: "TG-A", Faction: "A", Level: 1, Coord: "0202", Count: 14}
	state.Contacts["B"]["TG-A"] = contact
	first := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}, Coord: intelDisplayCoord(*contact)}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, first)
	join := Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-02-dive_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1"}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("同一舰队另一艘航母不能加入攻击编队：%v", err)
	}
	ApplyAirOrder(&state, join)
	state.Aircraft["b-cv-02-fighter-x1"].Status = "deck"
	state.Units["b-cv-02"].Deck.Length = 1
	primeService(&state)
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "elevator" || state.Aircraft["b-cv-02-dive_bomber-x1"].Status != "service" {
		t.Fatal("各舰没有独立调度甲板与升降机")
	}
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "attack_holding" || state.Aircraft["b-cv-01-torpedo_bomber-x1"].Status != "attack_holding" || state.Aircraft["b-cv-02-dive_bomber-x1"].Status != "service" {
		t.Fatal("攻击机不应在编队未到齐时离场")
	}
	initialRange := state.Aircraft["b-cv-01-dive_bomber-x1"].RemainingRange
	state.Units["b-cv-02"].Deck.Length = 6
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-dive_bomber-x1"].RemainingRange != initialRange-1 || state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "attack_holding" || state.Aircraft["b-cv-02-dive_bomber-x1"].Status != "elevator" {
		t.Fatal("等待中的攻击机没有按回合消耗1航程，或另一艘航母没有独立提升")
	}
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-01-dive_bomber-x1"].RemainingRange != initialRange-2 || state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "attack" || state.Aircraft["b-cv-02-dive_bomber-x1"].Status != "attack" {
		t.Fatal("最后一批飞机到齐后整个攻击编队没有离场")
	}
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 1 || tasks[0].Phase != "launching" || len(tasks[0].CarrierIDs) != 2 || tasks[0].AirborneCount != 3 {
		t.Fatalf("跨舰攻击编队投影错误：%+v", tasks)
	}
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-03-torpedo_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1"}); err == nil {
		t.Fatal("已经离场的攻击编队仍允许加入飞机")
	}
}

func TestReturningAssignedAircraftCancelsOrQueuesDescent(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x1"}
	ApplyAirOrder(&state, order)
	returnOrder := Order{Type: "RETURN_TO_HANGAR", AircraftID: "b-cv-01-fighter-x1"}
	if err := ValidateAirOrder(state, "B", returnOrder); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, returnOrder)
	aircraft := state.Aircraft["b-cv-01-fighter-x1"]
	if aircraft.Status != "hangar" || aircraft.TaskID != "" || aircraft.TaskKind != "" || aircraft.Priority != 0 {
		t.Fatalf("机库整备任务未取消：%+v", aircraft)
	}
	setAircraftReady(&state, "b-cv-01-fighter-x2")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: "b-cv-01-fighter-x2"})
	setAircraftReady(&state, "b-cv-01-torpedo_bomber-x1")
	ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-01-torpedo_bomber-x1"})
	// 甲板本回合在做起飞作业，也要允许把甲板上的算子收回机库：
	// 起飞和下降走的是不同机构（甲板 vs 升降机），以前一刀切拒绝，
	// 表现就是"甲板上有飞机却拖不回机库"。
	setAircraftReady(&state, "b-cv-01-fighter-x1")
	if err := ValidateAirOrder(state, "B", returnOrder); err != nil {
		t.Fatalf("甲板起飞作业中应仍允许安排下降：%v", err)
	}
}

func TestRepeatedJoinOfCurrentTaskIsIdempotent(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	before := *state.Aircraft["b-cv-04-dive_bomber-x1"]
	repeated := order
	repeated.TaskID = "b-cv-04-dive_bomber-x1"
	repeated.Heading = "E"
	repeated.SectorWidth = 120
	if err := ValidateAirOrder(state, "B", repeated); err != nil {
		t.Fatalf("重复加入当前任务不应报错：%v", err)
	}
	events := ApplyAirOrder(&state, repeated)
	after := *state.Aircraft["b-cv-04-dive_bomber-x1"]
	if after != before {
		t.Fatalf("重复加入当前任务改变了状态：之前=%+v 之后=%+v", before, after)
	}
	if len(events) != 1 || events[0].Type != "air_task_unchanged" {
		t.Fatalf("重复加入当前任务没有返回幂等结果：%+v", events)
	}
}

func TestReconGroupsAcceptDifferentAircraftAndRepeatedJoins(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1", "b-cv-03-torpedo_bomber-x1", "b-cv-04-dive_bomber-x1")
	first := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, first)
	join := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-torpedo_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1", Heading: "E", SectorWidth: 120}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("不同可搜索机型不能加入同一侦察组：%v", err)
	}
	ApplyAirOrder(&state, join)
	// 共享任务 ID；但这次显式写了 E/120，就按玩家写的来（不再被原组参数覆盖）。
	joined := state.Aircraft["b-cv-01-torpedo_bomber-x1"]
	if joined.TaskID != "b-cv-01-dive_bomber-x1" || joined.Heading != "E" || joined.SectorWidth != 120 {
		t.Fatalf("加入侦察组时应共享任务ID并采用本次指定参数：%+v", joined)
	}
	// 只给 task_id、不写参数时继承原组参数。
	// 注意原组领队已经升空（航向锁定在起飞那一刻的 W/60），
	// 所以这里继承到的是领队的 W/60，而不是刚才给新加入那架改的 E/120。
	bare := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1"}
	if err := ValidateAirOrder(state, "B", bare); err != nil {
		t.Fatalf("只带 task_id 加入应被接受：%v", err)
	}
	ApplyAirOrder(&state, bare)
	if inherited := state.Aircraft["b-cv-04-dive_bomber-x1"]; inherited.Heading != "W" || inherited.SectorWidth != 60 {
		t.Fatalf("不带参数加入时应继承原组参数：%+v", inherited)
	}
	second := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-03-torpedo_bomber-x1", Heading: "W", SectorWidth: 120}
	if err := ValidateAirOrder(state, "B", second); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, second)
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 60}); err == nil {
		t.Fatal("已有两个侦察组时仍允许创建第三组")
	}
	AdvanceAviation(&state)
	joinAirborne := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", joinAirborne); err != nil {
		t.Fatalf("已升空侦察组不能接受后续飞机：%v", err)
	}
	ApplyAirOrder(&state, joinAirborne)
	if state.Aircraft["b-cv-04-dive_bomber-x1"].TaskID != "b-cv-01-dive_bomber-x1" || state.Aircraft["b-cv-04-dive_bomber-x1"].Status != "recon" {
		t.Fatalf("后续飞机没有直接加入已升空侦察组：%+v", state.Aircraft["b-cv-04-dive_bomber-x1"])
	}
	tasks := VisibleAviation(state, "B").Tasks
	if len(tasks) != 2 {
		t.Fatalf("侦察组数量错误：%+v", tasks)
	}
	for _, task := range tasks {
		// launching / active 都表示"已经在天上"：前者是刚放飞的当回合，后者是之后。
		airbornePhase := task.Phase == "launching" || task.Phase == "active"
		if task.ID == "b-cv-01-dive_bomber-x1" && (len(task.Aircraft) != 3 || task.AirborneCount != 3 || !airbornePhase) {
			t.Fatalf("混合侦察组升空投影错误：%+v", task)
		}
	}
}

func TestReconGroupsAreScopedToFleet(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-14", "air-b-15", "air-b-16", "b-cv-01-dive_bomber-x1")
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
	otherFleet := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", otherFleet); err != nil {
		t.Fatalf("另一支舰队不能创建自己的侦察组：%v", err)
	}
	ApplyAirOrder(&state, otherFleet)
	if err := ValidateAirOrder(state, "B", Order{Type: "LAUNCH_RECON", AircraftID: "air-b-16", TaskID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}); err == nil {
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
	setAircraftReady(&state, "b-cv-02-fighter-x1", "b-cv-03-fighter-x1")
	for _, id := range []string{"b-cv-02-fighter-x1", "b-cv-03-fighter-x1"} {
		ApplyAirOrder(&state, Order{Type: "LAUNCH_CAP", AircraftID: id})
	}
	AdvanceAviation(&state)
	order := Order{Type: "RECOVER_AIR", AircraftIDs: []string{"b-cv-02-fighter-x1", "b-cv-03-fighter-x1"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("跨母舰CAP回收被拒绝：%v", err)
	}
	ApplyAirOrder(&state, order)
	for _, id := range []string{"b-cv-02-fighter-x1", "b-cv-03-fighter-x1"} {
		aircraft := state.Aircraft[id]
		carrier := state.Units[aircraft.CarrierID]
		if aircraft.Status != "recovery" || aircraft.TaskID != "" || carrier.Deck.Operation != "recover" {
			t.Fatalf("跨母舰回收状态错误：%+v %+v", aircraft, carrier)
		}
	}
}
