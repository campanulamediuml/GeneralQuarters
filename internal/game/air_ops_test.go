package game

import "testing"

// TestCoverFliesToStationAndReturnsOnFuel 校验空中掩护：飞到目标 hex 上空值班，燃料只够回家就自动返航。
func TestCoverFliesToStationAndReturnsOnFuel(t *testing.T) {
	state := sandboxState(t)
	cover := state.Aircraft["a2-cv-01-fighter-x3"]
	cover.Status = "cover"
	cover.TaskID = "COVER-1"
	cover.TaskKind = "cover"
	cover.Coord = "0602"
	cover.TargetCoord = "0604"
	cover.Launched = true
	applyLoadoutChoice(cover, LoadoutAirTank)

	// 每回合推进 2 hex：0602 → 0604 一回合到位。
	AdvanceAviation(&state)
	if cover.Status != "cover" || cover.Coord != "0604" {
		t.Fatalf("空中掩护没有抵达目标上空：coord=%s status=%s", cover.Coord, cover.Status)
	}

	// 把油量压到"刚好够回家"，下一回合就应该自动返航。
	cover.SetRange(Distance(cover.Coord, "0602") + 1)
	events := AdvanceAviation(&state)
	found := false
	for _, event := range events {
		if event.Type == "cover_rtb" && event.Audience == "A" {
			found = true
		}
	}
	if !found || !cover.Returning {
		t.Fatalf("燃料见底没有自动返航：returning=%v events=%+v", cover.Returning, events)
	}
}

// TestTransferLandsWhenHangarHasRoom 校验转场：目标舰队有机库空位就降落。
func TestTransferLandsWhenHangarHasRoom(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	plane.Status = "transfer"
	plane.TaskID = "TRANSFER-1"
	plane.TaskKind = "transfer"
	plane.Coord = "0602"
	plane.TargetCoord = "0602"
	plane.Launched = true
	applyLoadoutChoice(plane, LoadoutTransfer)
	// 其他航母都满员，只给 a2-cv-02 腾一个位置。
	state.Units["a2-cv-02"].Card.Aircraft--

	AdvanceAviation(&state)
	// 落地当回合就可能被升降机接走，所以只断言"已经不在空中"。
	if airborne(plane) || plane.CannotLand {
		t.Fatalf("转场没有降落：status=%s cannot_land=%v", plane.Status, plane.CannotLand)
	}
	if plane.CarrierID != "a2-cv-02" {
		t.Fatalf("转场没有落到有机库空位的航母：%s", plane.CarrierID)
	}
}

// TestTransferWaitsInAirWhenFleetFull 校验转场备降：目标舰队机库全满时挂在天上并标记无法降落。
func TestTransferWaitsInAirWhenFleetFull(t *testing.T) {
	state := sandboxState(t)
	plane := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	plane.Status = "transfer"
	plane.TaskID = "TRANSFER-2"
	plane.TaskKind = "transfer"
	plane.Coord = "0602"
	plane.TargetCoord = "0602"
	plane.Launched = true
	applyLoadoutChoice(plane, LoadoutTransfer)
	for _, id := range []string{"a2-cv-01", "a2-cv-02", "a2-cv-03", "a2-cv-04"} {
		state.Units[id].Card.Aircraft = state.Units[id].Card.HangarCapacity
	}

	events := AdvanceAviation(&state)
	if plane.Status != "transfer" || !plane.CannotLand {
		t.Fatalf("机库全满时不应该降落：status=%s cannot_land=%v", plane.Status, plane.CannotLand)
	}
	found := false
	for _, event := range events {
		if event.Type == "transfer_cannot_land" {
			found = true
		}
	}
	if !found {
		t.Fatalf("没有给出无法降落的事件：%+v", events)
	}

	// 腾出一个位置之后，下一回合自动落下去。
	state.Units["a2-cv-03"].Card.Aircraft--
	AdvanceAviation(&state)
	if airborne(plane) || plane.CannotLand {
		t.Fatalf("腾出机位后仍没有降落：status=%s cannot_land=%v", plane.Status, plane.CannotLand)
	}
}

// TestCoverCountsAsCAPOverStation 校验空中掩护到位后会被当作目标 hex 的 CAP 参与防守。
func TestCoverCountsAsCAPOverStation(t *testing.T) {
	state := sandboxState(t)
	cover := state.Aircraft["a2-cv-01-fighter-x3"]
	cover.Status = "cover"
	cover.TaskID = "COVER-2"
	cover.TaskKind = "cover"
	cover.Coord = "0602"
	cover.TargetCoord = "0602"
	cover.Launched = true
	groups := strikeCAPGroups(state, "A", "0602")
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total != 1 {
		t.Fatalf("空中掩护没有被计入防守 CAP：%d", total)
	}
}
