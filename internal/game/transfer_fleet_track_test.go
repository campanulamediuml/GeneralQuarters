package game

import "testing"

// transferFleetFixture 造一个"凯恩斯机场 + 一艘在 0103 的航母"的转场场景，
// 机场上有两架 F4F 正在转场到 TF1。舰队 ID 记在任务上，模拟玩家点选航母那一刻。
func transferFleetFixture() State {
	state := State{
		Map:       MapDefinition{Rows: 12, Cols: 12},
		Units:     map[string]*Unit{},
		Aircraft:  map[string]*AirUnit{},
		Airfields: map[string]*Airfield{},
		GroundAir: map[string]*AirfieldAirUnit{},
	}
	state.Airfields["home"] = &Airfield{
		ID: "home", Faction: "A", Name: "凯恩斯机场", Coord: "0101",
		Type: AirfieldLarge, Runways: 3, Strength: 15, MaxStrength: 15, HangarCapacity: AirfieldHangarUnlimited,
	}
	state.Units["cv1"] = &Unit{
		ID: "cv1", Name: "列克星敦", Faction: "A", Type: "CV", Coord: "0103", FleetID: "TF1",
		Card: ShipCard{HangarCapacity: 10, DeckCapacity: 4}, Deck: &FlightDeck{},
	}
	for _, id := range []string{"p1", "p2"} {
		state.GroundAir[id] = &AirfieldAirUnit{
			ID: id, DefinitionID: "fighter", AirfieldID: "home", Faction: "A",
			Model: "F4F 野猫", Kind: "fighter", Status: "transfer", TaskID: "t1", TaskKind: "transfer",
			Coord: "0101", OriginCoord: "0101", TargetCoord: "0103", TargetFleetID: "TF1",
			BaseRange: 20, MaxRange: 20, RemainingRange: 20,
		}
	}
	return state
}

func eventType(events []EngineEvent, kind string) bool {
	for _, event := range events {
		if event.Type == kind {
			return true
		}
	}
	return false
}

// TestGroundTransferTracksMovingFleet 校验转场到舰队时目标会跟着舰队走，
// 而不是把舰队离开原始 hex 当成"目的地不再是我方机场或航母"。
func TestGroundTransferTracksMovingFleet(t *testing.T) {
	state := transferFleetFixture()
	leader := state.GroundAir["p1"]

	// 第一回合：目标舰队还在 0103，编队飞一格。
	events := advanceGroundTransfer(&state, leader)
	if eventType(events, "transfer_aborted") {
		t.Fatalf("舰队还在目的格，不该中止：%+v", events)
	}
	if leader.Coord != "0102" {
		t.Fatalf("转场编队没有向前飞：%s", leader.Coord)
	}

	// 舰队开走：只认 hex 的旧逻辑下一回合就会判目的地失效。
	state.Units["cv1"].Coord = "0106"
	events = advanceGroundTransfer(&state, leader)
	if eventType(events, "transfer_aborted") {
		t.Fatalf("舰队只是移动，不该中止转场：%+v", events)
	}
	if !eventType(events, "transfer_tracking") {
		t.Fatalf("没有跟随舰队改目标：%+v", events)
	}
	if leader.TargetCoord != "0106" {
		t.Fatalf("目标 hex 没有跟到舰队当前位置：%s", leader.TargetCoord)
	}

	// 一直追到 0106，最后应当上舰。
	for i := 0; i < 10 && state.GroundAir["p1"] != nil; i++ {
		advanceGroundTransfer(&state, leader)
	}
	if state.GroundAir["p1"] != nil {
		t.Fatalf("追了 10 回合还没上舰，停在 %s", leader.Coord)
	}
	if boarded := state.Aircraft["p1"]; boarded == nil || boarded.CarrierID != "cv1" {
		t.Fatalf("没有转成舰载机算子：%+v", boarded)
	}
}

// TestGroundTransferAbortsWhenFleetDisappears 校验舰队沉光之后还是要中止返航，不能一直追空 hex。
func TestGroundTransferAbortsWhenFleetDisappears(t *testing.T) {
	state := transferFleetFixture()
	delete(state.Units, "cv1")
	leader := state.GroundAir["p1"]
	events := advanceGroundTransfer(&state, leader)
	if !eventType(events, "transfer_aborted") {
		t.Fatalf("舰队没了却没有中止：%+v", events)
	}
	if !leader.Returning || leader.TargetCoord != leader.OriginCoord {
		t.Fatalf("中止后没有掉头返航：%+v", leader)
	}
}

// TestGroundTransferGivesUpWhenFuelShort 校验追舰追到油不够时放弃转场返航。
func TestGroundTransferGivesUpWhenFuelShort(t *testing.T) {
	state := transferFleetFixture()
	state.Units["cv1"].Coord = "0109"
	for _, id := range []string{"p1", "p2"} {
		state.GroundAir[id].SetRange(3)
	}
	leader := state.GroundAir["p1"]
	events := advanceGroundTransfer(&state, leader)
	if !eventType(events, "transfer_rtb") {
		t.Fatalf("油不够够不到舰队却没有返航：%+v", events)
	}
	if !leader.Returning {
		t.Fatalf("应当转入返航状态：%+v", leader)
	}
}

// TestFriendlyFleetLookupHelpers 校验按舰队 ID 查 hex 的辅助函数。
func TestFriendlyFleetLookupHelpers(t *testing.T) {
	state := transferFleetFixture()
	if got := friendlyFleetIDAt(state, "A", "0103"); got != "TF1" {
		t.Fatalf("0103 的己方舰队=%q，期望 TF1", got)
	}
	if got := friendlyFleetIDAt(state, "B", "0103"); got != "" {
		t.Fatalf("敌方阵营不该查到己方舰队：%q", got)
	}
	if coord, ok := friendlyFleetCoord(state, "A", "TF1"); !ok || coord != "0103" {
		t.Fatalf("TF1 当前位置=%q,%v", coord, ok)
	}
	if _, ok := friendlyFleetCoord(state, "A", "TF404"); ok {
		t.Fatal("不存在的舰队不该有坐标")
	}
}
