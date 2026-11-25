package game

import "testing"

// TestShoreBombardmentRequiresStayAndNoEnemy 岸轰条件：停留满 1 回合、有主炮、同 hex 无敌舰队、无混乱。
func TestShoreBombardmentRequiresStayAndNoEnemy(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = airfield.Coord
		}
	}
	leader := state.Units["a-bb-01"]
	if canBombard(state, leader) != nil {
		t.Fatal("刚到该 hex 的舰队不应立即岸轰")
	}
	state.FleetStays = map[string]string{"TG-A": airfield.Coord}
	if canBombard(state, leader) != airfield {
		t.Fatal("停留满 1 回合且有主炮的舰队应能岸轰")
	}
	leader.Confusion = 4
	if canBombard(state, leader) != nil {
		t.Fatal("有混乱值的舰队不能岸轰")
	}
	leader.Confusion = 0
	state.Units["b-cv-01"].Coord = airfield.Coord
	if canBombard(state, leader) != nil {
		t.Fatal("同 hex 有敌舰队时不能岸轰")
	}
}

// TestShoreBombardmentDamagesAirfieldAndChargesConfusion 岸轰命中扣机场生命值、摧毁地面飞机，并给舰队加 4 点混乱。
func TestShoreBombardmentDamagesAirfieldAndChargesConfusion(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = airfield.Coord
		}
	}
	state.FleetStays = map[string]string{"TG-A": airfield.Coord}
	before := airfield.Strength
	beforeCoastal := airfield.Coastal
	beforeGround := len(sortedGroundAirIDs(state, airfield.ID))
	// 用固定种子（52）保证本场齐射至少命中 2 次，否则断言"没掉血"会是测试本身失效。
	state.RNGState = 52
	events := resolveShoreBombardment(&state)
	if len(events) == 0 {
		t.Fatal("岸轰应产生战报")
	}
	if airfield.Strength >= before {
		t.Fatalf("岸轰应减少机场生命值：%d -> %d", before, airfield.Strength)
	}
	if airfield.Strength < 0 {
		t.Fatalf("机场生命值不应跌破 0：%d", airfield.Strength)
	}
	if airfield.Coastal >= beforeCoastal {
		t.Fatalf("岸轰命中应削减岸炮：%d -> %d", beforeCoastal, airfield.Coastal)
	}
	if airfield.Coastal < 0 {
		t.Fatalf("岸炮不应跌破 0：%d", airfield.Coastal)
	}
	if len(sortedGroundAirIDs(state, airfield.ID)) > beforeGround {
		t.Fatal("岸轰不应增加地面飞机")
	}
	if state.Units["a-bb-01"].Confusion < 4 {
		t.Fatalf("岸轰后舰队应获得 4 点混乱，实际 %d", state.Units["a-bb-01"].Confusion)
	}
}

// 岸轰不是每回合都开火：停满一回合后开始，每次齐射给舰队 4 点混乱，
// 混乱每回合 -1，清零后才会再次开火。
func TestShoreBombardmentStartsAfterOneTurnAndRepeats(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	for _, unit := range state.Units {
		if FleetKey(unit) != "TG-A" {
			continue
		}
		unit.Coord = airfield.Coord
		unit.Destination = ""
		unit.MoveCooldown = 0
		unit.Confusion = 0
	}
	state.FleetStays = map[string]string{}
	fire := func(events []EngineEvent) bool {
		for _, event := range events {
			if event.Type == "shore_bombardment" {
				return true
			}
		}
		return false
	}
	// 第一回合刚到位：只记录停留位置，不开火。
	if events := ResolveOrders(&state, nil); fire(events) {
		t.Fatal("舰队刚进入机场 hex 的当回合不应岸轰")
	}
	// 第二回合仍在原地：开始岸轰，并获得 4 点混乱。
	if events := ResolveOrders(&state, nil); !fire(events) {
		t.Fatal("停满一回合后应开始岸轰")
	}
	if got := FleetConfusion(state, "TG-A"); got < 4 {
		t.Fatalf("岸轰后舰队应有 4 点混乱，实际 %d", got)
	}
	// 接下来三回合混乱还没清零，不能连续开火。
	for i := 0; i < 3; i++ {
		if events := ResolveOrders(&state, nil); fire(events) {
			t.Fatalf("混乱未清零时不应再次岸轰（第 %d 回合）", i+1)
		}
	}
	// 第四回合混乱归零，重新开火。
	if events := ResolveOrders(&state, nil); !fire(events) {
		t.Fatal("混乱清零后应再次岸轰")
	}
}

// TestAirfieldSortieBudget 跑道作业上限：每条可用跑道每回合 12 次起降，超出的顺延，
// 机场被空袭打断的整备会占用一次通行名额（规则书 250、218 行）。
func TestAirfieldSortieBudget(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-a-lighthouse"]
	// 造一个整备耗时更长的场景：塞满跑道作业预算后，新整备的算子应当顺延
	state.GroundAir["fuel-1"] = &AirfieldAirUnit{ID: "fuel-1", AirfieldID: airfield.ID, Faction: "A", Kind: "recon", Model: "F6F-5P", Status: "service", MaxRange: 16, RemainingRange: 16}
	// AdvanceAirfields 每回合会按跑道数重置预算（跑道×12），常规编制下永远用不完；
	// 这里用负数模拟"本回合预算已被人为耗尽"（机场刚被打断作业），算子必须原地顺延。
	state.GroundAir = map[string]*AirfieldAirUnit{"fuel-1": state.GroundAir["fuel-1"]}
	airfield.OpsRemaining = -1
	AdvanceAirfields(&state)
	if got := state.GroundAir["fuel-1"].Status; got != "service" {
		t.Fatalf("跑道作业已满时应顺延，实际状态 %s", got)
	}
	// 打断整备会消耗一次通行名额
	airfield.OpsRemaining = 1
	state.GroundAir["fuel-1"].Status = "deck"
	interruptAirfieldOperations(&state, airfield)
	if airfield.OpsRemaining != 0 || state.GroundAir["fuel-1"].Status != "hangar" {
		t.Fatalf("受袭打断应退回机库并占用一次通行名额：ops=%d status=%s", airfield.OpsRemaining, state.GroundAir["fuel-1"].Status)
	}
}

// TestNightLandingOnCarrierIsRisky 夜航：夜间降落航母每个算子额外 70% 摔毁判定。
func TestNightLandingOnCarrierIsRisky(t *testing.T) {
	destroyed := 0
	for seed := uint64(1); seed <= 40; seed++ {
		state := sandboxState(t)
		state.Night = true
		aircraft := &AirUnit{ID: "night-land", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "cap",
			Strength: 2, RemainingRange: 6, MaxRange: 12, Returning: true, Coord: "0602"}
		state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
		state.Units["a2-cv-01"].Deck.Operation = ""
		state.RNGState = seed
		advanceAircraftReturn(&state, aircraft)
		if _, still := state.Aircraft[aircraft.ID]; !still {
			destroyed++
		}
	}
	if destroyed < 15 || destroyed > 35 {
		t.Fatalf("夜间降落航母的摔毁比例应接近 70%%：40 次中 %d 次被摧毁", destroyed)
	}
}

// TestDayLandingOnCarrierIsSafe 白天同条件不应摔毁。
func TestDayLandingOnCarrierIsSafe(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		state := sandboxState(t)
		aircraft := &AirUnit{ID: "day-land", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "cap",
			Strength: 2, RemainingRange: 6, MaxRange: 12, Returning: true, Coord: "0602"}
		state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
		state.Units["a2-cv-01"].Deck.Operation = ""
		state.RNGState = seed
		advanceAircraftReturn(&state, aircraft)
		if _, still := state.Aircraft[aircraft.ID]; !still {
			t.Fatalf("白天降落不应摔毁（seed=%d）", seed)
		}
	}
}
