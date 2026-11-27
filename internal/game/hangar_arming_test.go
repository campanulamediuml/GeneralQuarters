package game

import "testing"

// armOrder 是"机库里选挂载"那一步的命令：不派任务，直接开始整备。
func armOrder(ids []string, loadout Loadout) Order {
	return Order{Type: "SET_LOADOUT", AircraftIDs: ids, Loadout: string(loadout)}
}

func applyArm(t *testing.T, state *State, ids []string, loadout Loadout) {
	t.Helper()
	order := armOrder(ids, loadout)
	if err := ValidateOrder(*state, "A", order); err != nil {
		t.Fatalf("机库选挂载被拒：%v", err)
	}
	ApplyLoadoutOrder(state, order)
}

// 部署时算子默认空载：没有副油箱、没有弹，航程就是机种基础航程。
func TestAircraftDeployWithEmptyLoadout(t *testing.T) {
	state := sandboxState(t)
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft.Loadout != LoadoutNone {
			t.Fatalf("%s部署时应当空载，实际%s", id, aircraft.Loadout)
		}
		if aircraft.MaxRange != aircraft.BaseRange {
			t.Fatalf("%s空载航程应为基础航程%d，实际%d", id, aircraft.BaseRange, aircraft.MaxRange)
		}
		if aircraft.ServiceTurns != 0 || aircraft.Armed {
			t.Fatalf("%s部署时不该是已整备状态", id)
		}
	}
}

// 机库里选好挂载就开始整备，不需要先拖进任务栏；整备完在机库里待发。
func TestArmInHangarStartsService(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	if plane.Status != "hangar" {
		t.Fatalf("前置状态不对：%s", plane.Status)
	}
	applyArm(t, &state, []string{plane.ID}, LoadoutAirTank)
	if plane.Status != "service" || plane.DeckService {
		t.Fatalf("机库选挂载后应在机库整备，实际 status=%s deck_service=%v", plane.Status, plane.DeckService)
	}
	if plane.ServiceTurns != 0 || plane.Armed {
		t.Fatalf("整备刚开始：ServiceTurns=%d Armed=%v", plane.ServiceTurns, plane.Armed)
	}
	if !plane.LoadoutManual {
		t.Fatal("手动选挂载应打上标记，分配任务时不能被默认值覆盖")
	}
	if plane.MaxRange != SortieBaseRange(plane.BaseRange, LoadoutAirTank) {
		t.Fatalf("换挂载后航程没重算：%d", plane.MaxRange)
	}
	advanceAircraftService(&state)
	if plane.ServiceTurns != 1 {
		t.Fatalf("一个回合后整备应完成：%d", plane.ServiceTurns)
	}
	syncArmedAircraft(&state, nil)
	if !plane.Armed {
		t.Fatal("整备完成应当是整装待发")
	}
	// 整备完成的算子仍在机库里等着，玩家自己决定什么时候送它上甲板。
	if plane.Status != "service" || plane.DeckService {
		t.Fatalf("整备完成后应在机库待发，实际 %s", plane.Status)
	}
}

// 再次选择同一套挂载不该把整备进度清零：已经待发的算子点一下下拉框又白等一回合。
func TestRearmSameLoadoutKeepsReadiness(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	applyArm(t, &state, []string{plane.ID}, LoadoutAirTank)
	advanceAircraftService(&state)
	if plane.ServiceTurns != 1 {
		t.Fatalf("前置整备没完成：%d", plane.ServiceTurns)
	}
	applyArm(t, &state, []string{plane.ID}, LoadoutAirTank)
	if plane.ServiceTurns != 1 {
		t.Fatalf("同一套挂载不该重新整备：ServiceTurns=%d", plane.ServiceTurns)
	}
	// 换另一套就要重新整备一轮（规则书 258 行）。
	applyArm(t, &state, []string{plane.ID}, LoadoutAir)
	if plane.ServiceTurns != 0 || plane.Status != "service" {
		t.Fatalf("换挂载应重新整备：ServiceTurns=%d status=%s", plane.ServiceTurns, plane.Status)
	}
}

// 空载不算整备：选"无挂载"就是卸弹回机库，状态直接退回去。
func TestClearLoadoutUnarmsAircraft(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	applyArm(t, &state, []string{plane.ID}, LoadoutBombTank)
	if plane.Status != "service" {
		t.Fatalf("前置状态不对：%s", plane.Status)
	}
	applyArm(t, &state, []string{plane.ID}, LoadoutNone)
	if plane.Status != "hangar" || plane.Loadout != LoadoutNone || plane.Armed {
		t.Fatalf("卸弹后应回机库空载：status=%s loadout=%q armed=%v", plane.Status, plane.Loadout, plane.Armed)
	}
	if plane.LoadoutManual {
		t.Fatal("空载不该保留手动挂载标记")
	}
	if plane.MaxRange != plane.BaseRange {
		t.Fatalf("空载航程应回到基础航程：%d", plane.MaxRange)
	}
}

// 整备好的算子拖进甲板栏：这一回合排队，升降机把它提上去，到甲板就是待发。
func TestLiftRequestedSendsReadyAircraftToDeck(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	applyArm(t, &state, []string{plane.ID}, LoadoutAirTank)
	advanceAircraftService(&state)

	order := Order{Type: "LIFT_TO_DECK", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("拖上甲板被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	if !plane.LiftRequested {
		t.Fatal("拖进甲板栏后应记下提升请求")
	}
	scheduleElevators(&state)
	if plane.Status != "elevator" {
		t.Fatalf("整备好的算子应当回合末上升降机，实际 %s", plane.Status)
	}
	completeElevatorCycles(&state)
	if plane.Status != "deck" {
		t.Fatalf("提升完成应在甲板待发，实际 %s", plane.Status)
	}
	if plane.LiftRequested || plane.DeckService {
		t.Fatal("提升完成后请求标记应清掉")
	}
	syncArmedAircraft(&state, nil)
	if plane.ServiceTurns != 1 || !plane.Armed {
		t.Fatal("上甲板不该丢掉整备状态")
	}
}

// 空载算子没有可整备的东西，拖上甲板可以直接提升，上去再选挂载。
func TestEmptyAircraftLiftsWithoutService(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	order := Order{Type: "LIFT_TO_DECK", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("空载拖上甲板被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	scheduleElevators(&state)
	if plane.Status != "elevator" {
		t.Fatalf("空载算子应立刻排队提升，实际 %s", plane.Status)
	}
	completeElevatorCycles(&state)
	if plane.Status != "deck" || plane.Loadout != LoadoutNone {
		t.Fatalf("空载算子应在甲板上空着，实际 status=%s loadout=%q", plane.Status, plane.Loadout)
	}
	// 甲板位要算上它。
	if used := deckUsed(state, "a2-cv-01"); used != 1 {
		t.Fatalf("甲板占用应为 1，实际 %d", used)
	}
}

// 整备还没走完就拖进甲板栏：请求留着，整备一完成就提升，不会丢。
func TestLiftRequestWaitsForService(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	applyArm(t, &state, []string{plane.ID}, LoadoutBombTank)
	order := Order{Type: "LIFT_TO_DECK", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("整备中拖上甲板被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	scheduleElevators(&state)
	if plane.Status != "service" {
		t.Fatalf("整备没完成不该被提升，实际 %s", plane.Status)
	}
	// 整备在同一个 scheduleElevators 里推进，下一回合才轮到升降机。
	scheduleElevators(&state)
	if plane.Status != "elevator" {
		t.Fatalf("整备完成后应被提升，实际 %s", plane.Status)
	}
}

// 空载算子拖上甲板之后再选挂载：在甲板原地整备，整备完就是甲板待发。
func TestArmOnDeckServicesInPlace(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-torpedo_bomber-x1"]
	plane.Status = "deck"
	applyArm(t, &state, []string{plane.ID}, LoadoutTorpedoTank)
	if plane.Status != "service" || !plane.DeckService {
		t.Fatalf("甲板换挂载应在甲板整备：status=%s deck_service=%v", plane.Status, plane.DeckService)
	}
	if used := deckUsed(state, "a2-cv-01"); used != 1 {
		t.Fatalf("甲板整备应占甲板位，实际 %d", used)
	}
	advanceAircraftService(&state)
	AdvanceAviation(&state)
	if plane.Status != "deck" || plane.DeckService {
		t.Fatalf("整备完成应转为甲板待发：status=%s deck_service=%v", plane.Status, plane.DeckService)
	}
	syncArmedAircraft(&state, nil)
	if !plane.Armed {
		t.Fatal("甲板整备完成也是整装待发")
	}
}

// 甲板整备中的算子拖回机库：走升降机下降，不能直接从甲板消失。
func TestReturnToHangarFromDeckServiceQueuesLowering(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-torpedo_bomber-x1"]
	plane.Status = "deck"
	applyArm(t, &state, []string{plane.ID}, LoadoutTorpedoTank)
	order := Order{Type: "RETURN_TO_HANGAR", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("撤回机库被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	if plane.Status != "recovery" || plane.DeckService {
		t.Fatalf("应排队下降：status=%s deck_service=%v", plane.Status, plane.DeckService)
	}
}

// 直接拖进任务栏仍然是"自动走流程"：配挂载 → 机库整备 → 提升 → 放飞。
func TestDragToTaskStillRunsFullPipeline(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("拖进 CAP 栏被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	if plane.Status != "service" || plane.Loadout == LoadoutNone {
		t.Fatalf("自动流程应先整备并配上挂载：status=%s loadout=%q", plane.Status, plane.Loadout)
	}
	if plane.LoadoutManual {
		t.Fatal("自动配的挂载不该被当成手动挂载")
	}
	for turn := 0; turn < 3; turn++ {
		AdvanceAviation(&state)
	}
	if plane.Status != "cap" {
		t.Fatalf("三个回合后应已升空执行CAP，实际 %s", plane.Status)
	}
}

// 已经整备好的算子贴进任务栏：不再多等一轮整备，直接从提升开始。
func TestPreparedAircraftSkipsExtraService(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	applyArm(t, &state, []string{plane.ID}, LoadoutAirTank)
	advanceAircraftService(&state)
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("拖进 CAP 栏被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	if plane.ServiceTurns != 1 {
		t.Fatalf("已经整备好的算子不该重新整备：ServiceTurns=%d", plane.ServiceTurns)
	}
	AdvanceAviation(&state)
	if plane.Status != "elevator" {
		t.Fatalf("第一回合就应上升降机，实际 %s", plane.Status)
	}
	AdvanceAviation(&state)
	if plane.Status != "cap" {
		t.Fatalf("第二回合就应升空，实际 %s", plane.Status)
	}
}

// 甲板已经塞满时，待发算子留在机库等下一次提升，不会把别的东西挤掉。
func TestDeckFullKeepsLiftRequestQueued(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	plane := state.Aircraft["a2-cv-01-fighter-x1"]
	order := Order{Type: "LIFT_TO_DECK", AircraftIDs: []string{plane.ID}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("拖上甲板被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	// 把甲板位全占掉：模拟"甲板满了，等这一批走完再提升"。
	filler := []*AirUnit{}
	for _, id := range sortedAirUnitIDs(state) {
		other := state.Aircraft[id]
		if other.ID == plane.ID || other.CarrierID != carrier.ID {
			continue
		}
		other.Status = "deck"
		filler = append(filler, other)
	}
	scheduleElevators(&state)
	if plane.Status == "elevator" {
		t.Fatal("甲板没有空位时不该开始提升")
	}
	if !plane.LiftRequested {
		t.Fatal("提升请求要留着，等甲板空出来再提")
	}
	// 甲板清空之后下一次调度就能把它送上去。
	for _, other := range filler {
		other.Status = "hangar"
	}
	scheduleElevators(&state)
	if plane.Status != "elevator" {
		t.Fatalf("甲板空出来后应开始提升，实际 %s", plane.Status)
	}
}
