package game

import "testing"

// prepareOnDeckHangar 把机库里的算子摆成"已经整备好、等提升"：挂载手动指定、整备进度写满。
// 这些用例测的是资源争抢，不测整备本身。
func prepareHangar(state *State, ids ...string) {
	for _, id := range ids {
		aircraft := state.Aircraft[id]
		if aircraft == nil {
			continue
		}
		loadout := DefaultLoadoutFor(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, "recon", StrikeTargetShip, aircraft.TorpedoCapable)
		applyLoadoutChoice(aircraft, loadout)
		aircraft.LoadoutManual = true
		aircraft.ServiceTurns = 1
		aircraft.Armed = true
	}
}

// 升降机是硬预算：只剩一部升降机（4 个名额）时，低优先级任务只能拿到剩下的名额，
// 而且优先级的任务先占——这就是"设优先级抢升降机"。
func TestPriorityPreemptsElevatorBudget(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	// 坏两部升降机，本回合只剩 4 个提升名额。
	EnsureDamage(carrier).ElevatorDamage = 2
	EnsureAviation(&state)

	firstIDs := []string{"a2-cv-01-dive_bomber-x1", "a2-cv-01-dive_bomber-x2", "a2-cv-01-dive_bomber-x3"}
	secondIDs := []string{"a2-cv-01-dive_bomber-x4", "a2-cv-01-dive_bomber-x5", "a2-cv-01-dive_bomber-x6"}
	prepareHangar(&state, append(append([]string{}, firstIDs...), secondIDs...)...)
	for _, group := range [][]string{firstIDs, secondIDs} {
		order := Order{Type: "LAUNCH_RECON", AircraftIDs: group, Heading: "W", SectorWidth: 60}
		if err := ValidateAirOrder(state, "A", order); err != nil {
			t.Fatalf("侦察任务被拒：%v", err)
		}
		ApplyAirOrder(&state, order)
	}
	secondTask := state.Aircraft[secondIDs[0]].TaskID
	for _, id := range firstIDs {
		if state.Aircraft[id].TaskID == secondTask {
			t.Fatal("两批飞机不该落到同一个侦察组")
		}
	}
	// 把后建的那组设为最高优先级：它先进升降机。
	ApplyAirOrder(&state, Order{Type: "PRIORITIZE_AIR_TASK", AircraftID: secondIDs[0]})
	if got := state.Aircraft[secondIDs[0]].Priority; got != 1 {
		t.Fatalf("优先级没提上去：%d", got)
	}
	scheduleElevators(&state)

	// 只剩一部升降机、一次作业：这一回合只有高优先级那组能上去。
	if got := state.Aircraft[secondIDs[0]].Status; got != "elevator" || state.Aircraft[secondIDs[2]].Status != "elevator" {
		t.Fatalf("高优先级任务应先占用升降机，实际 %s", got)
	}
	for _, id := range firstIDs {
		if state.Aircraft[id].Status != "service" {
			t.Fatalf("低优先级任务应继续在机库排队，实际 %s", state.Aircraft[id].Status)
		}
	}
	// 升降机这一轮走完之后才轮到低优先级：资源是被抢占的，不是同时进行的。
	completeElevatorCycles(&state)
	scheduleElevators(&state)
	for _, id := range firstIDs {
		if state.Aircraft[id].Status != "elevator" {
			t.Fatalf("高优先级走完后应轮到低优先级，实际 %s", state.Aircraft[id].Status)
		}
	}
}

// 升降机单次容量 4：一次提升放不下的部分留到下一回合，高优先级任务的分批也要排在
// 低优先级任务前面。
func TestElevatorCapacitySpillsToNextTurnBeforeLowerPriority(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	EnsureDamage(carrier).ElevatorDamage = 2
	EnsureAviation(&state)
	// 一台升降机一次 4 个算子。
	manyIDs := []string{"a2-cv-01-fighter-x1", "a2-cv-01-fighter-x2", "a2-cv-01-fighter-x3", "a2-cv-01-fighter-x4", "a2-cv-01-fighter-x5", "a2-cv-01-fighter-x6"}
	otherIDs := []string{"a2-cv-01-fighter-x7", "a2-cv-01-fighter-x8"}
	for _, id := range append(append([]string{}, manyIDs...), otherIDs...) {
		plane := state.Aircraft[id]
		plane.Loadout = LoadoutAirTank
		plane.LoadoutManual = true
		plane.ServiceTurns = 1
	}
	// 高优先级任务 6 架（要分两批），低优先级任务 2 架。
	// 直接下发打击任务：这里的焦点是提升顺序，任务本身的情报校验另有用例。
	ApplyAirOrder(&state, Order{Type: "LAUNCH_STRIKE", AircraftIDs: manyIDs, Coord: "0708"})
	ApplyAirOrder(&state, Order{Type: "LAUNCH_RECON", AircraftIDs: otherIDs, Heading: "W", SectorWidth: 60})
	// 打击任务自己先排到最前面，剩下的 2 架侦察机不能插队。
	scheduleElevators(&state)
	liftedMany, liftedOther := 0, 0
	for _, id := range manyIDs {
		if state.Aircraft[id].Status == "elevator" {
			liftedMany++
		}
	}
	for _, id := range otherIDs {
		if state.Aircraft[id].Status == "elevator" {
			liftedOther++
		}
	}
	if liftedMany != 4 || liftedOther != 0 {
		t.Fatalf("一次提升只能带 4 架：高优先级 %d、低优先级 %d", liftedMany, liftedOther)
	}
	completeElevatorCycles(&state)
	scheduleElevators(&state)
	rest := 0
	for _, id := range manyIDs {
		if state.Aircraft[id].Status == "elevator" {
			rest++
		}
	}
	if rest != 2 {
		t.Fatalf("高优先级剩余 2 架应当继续提升，实际上升降机 %d", rest)
	}
	for _, id := range otherIDs {
		if state.Aircraft[id].Status != "service" {
			t.Fatalf("低优先级还得再等一轮，实际 %s", state.Aircraft[id].Status)
		}
	}
}

// 甲板被占满时提升延后：玩家得先清出甲板（让飞机下降/起飞），否则整备好的算子只能在机库排队。
func TestDeckFullDefersLift(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	prepareHangar(&state, "a2-cv-01-fighter-x1")
	order := Order{Type: "LAUNCH_CAP", AircraftIDs: []string{"a2-cv-01-fighter-x1"}}
	if err := ValidateAirOrder(state, "A", order); err != nil {
		t.Fatalf("CAP 被拒：%v", err)
	}
	ApplyAirOrder(&state, order)

	// 用停场的飞机把甲板塞满：它们没有任务，不会被放飞，只占位置。
	fillers := []*AirUnit{}
	for _, id := range sortedAirUnitIDs(state) {
		other := state.Aircraft[id]
		if other.CarrierID != carrier.ID || other.ID == "a2-cv-01-fighter-x1" {
			continue
		}
		if len(fillers) >= carrier.Deck.Length {
			break
		}
		other.Status = "deck"
		fillers = append(fillers, other)
	}
	if deckUsed(state, carrier.ID) != carrier.Deck.Length {
		t.Fatalf("甲板没占满：%d/%d", deckUsed(state, carrier.ID), carrier.Deck.Length)
	}
	events := scheduleElevators(&state)
	if state.Aircraft["a2-cv-01-fighter-x1"].Status != "service" {
		t.Fatalf("甲板满时不该提升，实际 %s", state.Aircraft["a2-cv-01-fighter-x1"].Status)
	}
	delayed := false
	for _, event := range events {
		if event.Type == "air_preparation_delayed" {
			delayed = true
		}
	}
	if !delayed {
		t.Fatal("甲板满时应给出『整备完成等待提升』的战报")
	}
	// 清出甲板之后下一次调度就能提升。
	for _, other := range fillers {
		other.Status = "hangar"
	}
	scheduleElevators(&state)
	if state.Aircraft["a2-cv-01-fighter-x1"].Status != "elevator" {
		t.Fatalf("甲板空出来后应开始提升，实际 %s", state.Aircraft["a2-cv-01-fighter-x1"].Status)
	}
}

// 甲板只剩一个空位时，优先级决定谁能上去。
func TestPriorityDecidesWhoGetsTheLastDeckSpot(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	prepareHangar(&state, "a2-cv-01-dive_bomber-x1", "a2-cv-01-dive_bomber-x4")
	// 两条各 1 架的侦察组。
	for _, id := range []string{"a2-cv-01-dive_bomber-x1", "a2-cv-01-dive_bomber-x4"} {
		order := Order{Type: "LAUNCH_RECON", AircraftIDs: []string{id}, Heading: "W", SectorWidth: 60}
		if err := ValidateAirOrder(state, "A", order); err != nil {
			t.Fatalf("侦察任务被拒：%v", err)
		}
		ApplyAirOrder(&state, order)
	}
	low := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	high := state.Aircraft["a2-cv-01-dive_bomber-x4"]
	if low.TaskID == high.TaskID {
		t.Fatal("两架应各自成组")
	}
	// 甲板只剩 1 个空位。
	for _, id := range sortedAirUnitIDs(state) {
		other := state.Aircraft[id]
		if other.CarrierID != carrier.ID || other.ID == low.ID || other.ID == high.ID {
			continue
		}
		if deckUsed(state, carrier.ID) < carrier.Deck.Length-1 {
			other.Status = "deck"
		}
	}
	if available := carrier.Deck.Length - deckUsed(state, carrier.ID); available != 1 {
		t.Fatalf("甲板空位应为 1，实际 %d", available)
	}
	// 把后建的那组提到最高优先级。
	ApplyAirOrder(&state, Order{Type: "PRIORITIZE_AIR_TASK", AircraftID: high.ID})
	scheduleElevators(&state)
	if high.Status != "elevator" {
		t.Fatalf("高优先级应该拿到最后一个甲板位，实际 %s", high.Status)
	}
	if low.Status != "service" {
		t.Fatalf("低优先级应该继续排队，实际 %s", low.Status)
	}
	// 下一回合甲板空出来（停场飞机被清走），低优先级的再上。
	for _, id := range sortedAirUnitIDs(state) {
		other := state.Aircraft[id]
		if other.Status == "deck" && other.TaskID == "" {
			other.Status = "hangar"
		}
	}
	completeElevatorCycles(&state)
	scheduleElevators(&state)
	if low.Status != "elevator" && low.Status != "deck" {
		t.Fatalf("甲板空出来之后应该轮到低优先级，实际 %s", low.Status)
	}
}
