package game

import "testing"

// TestAdvanceMovementCooldownLocksFleetUntilZero 回归：冷却未归零前 LegalMoves 必须为空、
// SET_MOVE 必须被拒绝，减到 0 之后必须恢复可移动。旧实现在冷却期间直接返回空列表并让校验依赖该列表，
// 导致舰队移动一次后永远无法再下达移动命令。
// 3 回合周期：移动后挂 2 点冷却，两次结算之后才归零。
func TestAdvanceMovementCooldownLocksFleetUntilZero(t *testing.T) {
	state := sandboxState(t)
	state.Units["probe"] = &Unit{ID: "probe", Type: "DD", Faction: "A", Coord: "0202", FleetID: "probe", Name: "探路舰", Strength: 3, MaxStrength: 3, Card: ShipCard{SpeedClass: "fast", DamageControl: 5}}
	unit := state.Units["probe"]
	start := unit.Coord
	var move string
	for _, coord := range LegalMoves(state, unit) {
		if Distance(coord, start) == 1 {
			move = coord
			break
		}
	}
	if move == "" {
		t.Fatal("初始状态应当有合法移动")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: "probe", Coord: move}); err != nil {
		t.Fatalf("初始移动被拒绝：%v", err)
	}
	ApplyImmediateMove(&state, Order{Type: "SET_MOVE", UnitID: "probe", Coord: move})
	if unit.MoveCooldown != 2 {
		t.Fatalf("3 回合周期，移动后应挂 2 点冷却，实际 %d", unit.MoveCooldown)
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: "probe", Coord: unit.Coord}); err == nil {
		t.Fatal("冷却期间不应接受移动")
	}
	AdvanceMovementCooldown(&state)
	if unit.Moved {
		t.Fatal("结算后 Moved 应复位")
	}
	if unit.MoveCooldown != 1 {
		t.Fatalf("第一次结算后冷却应剩 1，实际 %d", unit.MoveCooldown)
	}
	if moves := LegalMoves(state, unit); len(moves) != 0 {
		t.Fatalf("冷却没结束就不该有合法移动：%v", moves)
	}
	AdvanceMovementCooldown(&state)
	if unit.MoveCooldown != 0 {
		t.Fatalf("第二次结算后冷却应归零，实际 %d", unit.MoveCooldown)
	}
	if moves := LegalMoves(state, unit); len(moves) == 0 {
		t.Fatal("冷却结束后必须恢复合法移动，否则舰队会被永久锁死")
	}
	if err := ValidateOrder(state, "A", Order{Type: "SET_MOVE", UnitID: "probe", Coord: LegalMoves(state, unit)[0]}); err != nil {
		t.Fatalf("冷却结束后的移动应被接受：%v", err)
	}
}

// TestLandedReconStaysVisibleInTaskBar 回归：侦察机着舰时若清空 TaskID，整组侦察任务会从任务栏消失。
func TestLandedReconStaysVisibleInTaskBar(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "a2-cv-04-dive_bomber-x1", CarrierID: "a2-cv-01", Faction: "A", Kind: "recon", Model: "F6F-5P", Status: "recon", Strength: 1, TaskID: "a2-cv-04-dive_bomber-x1", TaskKind: "recon", Heading: "E", SectorWidth: 60, ReconRadius: 3, OriginCoord: "0602", Coord: "0602", Returning: true, RemainingRange: 10, MaxRange: 16}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.Units[aircraft.CarrierID].Deck.Operation = ""
	landIfCapacity(&state, aircraft, state.Units[aircraft.CarrierID])
	if aircraft.Status != "recovery" {
		t.Fatalf("侦察机没有进入回收状态：%s", aircraft.Status)
	}
	view := VisibleAviation(state, "A")
	found := false
	for _, task := range view.Tasks {
		if task.ID == "a2-cv-04-dive_bomber-x1" {
			found = true
		}
	}
	if !found {
		t.Fatal("着舰后的侦察任务不应从任务栏消失")
	}
}

// TestLandingRespectsDeckCapacityWithinOneTurn 回归：同一回合多架飞机返航时必须累计占用甲板，不能超容。
func TestLandingRespectsDeckCapacityWithinOneTurn(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	carrier.Deck.Operation = ""
	carrier.Deck.Length = 2
	state.Aircraft = map[string]*AirUnit{}
	fleet := []*AirUnit{
		{ID: "r1", CarrierID: "a2-cv-01", Faction: "A", Kind: "recon", Status: "recon", TaskID: "r", Coord: carrier.Coord, Returning: true, MaxRange: 10, RemainingRange: 5},
		{ID: "r2", CarrierID: "a2-cv-01", Faction: "A", Kind: "recon", Status: "recon", TaskID: "r", Coord: carrier.Coord, Returning: true, MaxRange: 10, RemainingRange: 5},
		{ID: "r3", CarrierID: "a2-cv-01", Faction: "A", Kind: "recon", Status: "recon", TaskID: "r", Coord: carrier.Coord, Returning: true, MaxRange: 10, RemainingRange: 5},
	}
	for _, aircraft := range fleet {
		state.Aircraft[aircraft.ID] = aircraft
	}
	landed := 0
	for _, aircraft := range fleet {
		carrier.Deck.Operation = ""
		if landIfCapacity(&state, aircraft, carrier) {
			landed++
		}
	}
	// 第 3 架换一艘航母落地（同编队同 hex），而不是硬塞或瞬移去机场。
	if landed != 3 || deckUsed(state, carrier.ID) != 2 {
		t.Fatalf("甲板长度 2 时这条甲板只接 2 架，实际降落 %d 架、甲板占用 %d", landed, deckUsed(state, carrier.ID))
	}
	if diverted := state.GroundAir["r3"]; diverted != nil {
		t.Fatalf("甲板满不该把飞机送去机场：%+v", diverted)
	}
	r3 := state.Aircraft["r3"]
	if r3 == nil {
		t.Fatalf("第 3 架应换一艘航母降落，而不是消失")
	}
	if r3.CarrierID == carrier.ID {
		t.Fatalf("第 3 架不该塞进已满的 %s", carrier.ID)
	}
}

// TestStrikeHitChanceSpeedDirection 回归：快速目标应比慢速目标更难命中，不能被错误分支反转。
func TestStrikeHitChanceSpeedDirection(t *testing.T) {
	state := sandboxState(t)
	slow := &Unit{ID: "slow", Type: "CL", Faction: "B", Coord: "0608", Card: ShipCard{SpeedClass: "slow", DamageControl: 5}}
	fast := &Unit{ID: "fast", Type: "CL", Faction: "B", Coord: "0608", Card: ShipCard{SpeedClass: "fast", DamageControl: 5}}
	attacker := &AirUnit{ID: "sb", Kind: "dive_bomber", Faction: "A", AntiShip: 4}
	slowChance := strikeHitChance(state, attacker, slow, false, true)
	fastChance := strikeHitChance(state, attacker, fast, false, true)
	if fastChance != slowChance-10 {
		t.Fatalf("快速目标命中率应比慢速低 10 点：fast=%.1f slow=%.1f", fastChance, slowChance)
	}
}

// TestAirCombatWinnerNotConfusedByReturn 回归：攻击机打完必然返航，幸存不等于被击落。
func TestAirCombatWinnerNotConfusedByReturn(t *testing.T) {
	// 一个幸存攻击机对一架 CAP：若把"返航"当成"全灭"，胜负会被错误判给防守方。
	state := sandboxState(t)
	attacker := &AirUnit{ID: "atk", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack", Strength: 2, AirCombat: 1, AntiShip: 0, TaskID: "atk", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 8, MaxRange: 12}
	capAircraft := &AirUnit{ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Model: "F6F", Status: "cap", Strength: 3, AirCombat: 5, TaskID: "cap-TG-B", TaskKind: "cap"}
	state.Aircraft = map[string]*AirUnit{attacker.ID: attacker, capAircraft.ID: capAircraft}
	// CAP 按母舰坐标判定拦截范围，必须把母舰放到目标上空才会进入空战。
	state.Units["b-cv-01"].Coord = "0608"
	state.RNGState = 991
	resolveAirStrike(&state, "atk", "0608")
	if len(state.AirCombats) != 1 {
		t.Fatalf("应生成一份空战报告：%+v", state.AirCombats)
	}
	report := state.AirCombats[0]
	if report.AttackerCount != 1 || report.DefenderCount != 1 {
		t.Fatalf("空战双方数量错误：%+v", report)
	}
	if report.Winner == "" {
		t.Fatal("空战胜负不应为空")
	}
	if state.Aircraft[attacker.ID] != nil && !attacker.Returning {
		t.Fatal("空袭完成后攻击机应返航")
	}
}

// —— 对局测试中发现的回归 ——

// 飞机可以飞越岛屿：早期版本复用了舰队水面通行规则，飞往岛屿机场的编队永远到不了。
func TestAircraftCanFlyOverIsland(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "probe", Faction: "B", CarrierID: "b-cv-01", Coord: "0208"}
	if !flightCanEnter(state, aircraft, "0304") {
		t.Fatal("飞机应当能飞越岛屿（0304 是机场所在岛）")
	}
	if next := nextFlightCoord(state, aircraft, "0304"); next == "" {
		t.Fatal("飞往岛屿机场的航线不应为空")
	}
}

// 机场整备完成（ready）的编队会自动起飞，不需要玩家再下一条命令。
func TestAirfieldLaunchReadyTaskAutomatically(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.GroundAir["af-b-fighter-1"]
	if aircraft == nil {
		t.Fatal("B 机场应有战斗机")
	}
	order := Order{Type: "LAUNCH_CAP", AircraftID: aircraft.ID}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("机场 CAP 命令应合法：%v", err)
	}
	ApplyGroundAirOrder(&state, order)
	for step := 0; step < 4 && !airfieldAirborne(aircraft); step++ {
		AdvanceAirfields(&state)
	}
	if !airfieldAirborne(aircraft) {
		t.Fatalf("整备完成的机场编队应当自动起飞：%s", aircraft.Status)
	}
}

// 机场编队同样会按任务换挂载：对地挂炸弹、对舰优先鱼雷。
func TestGroundAircraftGetsLoadoutByTask(t *testing.T) {
	state := sandboxState(t)
	bomber := state.GroundAir["af-b-dive_bomber-1"]
	if bomber == nil {
		t.Fatal("B 机场应有俯冲轰炸机")
	}
	setAirfieldContact(&state, "B", state.Airfields["af-a-lighthouse"])
	ÄpplyOrder := Order{Type: "LAUNCH_STRIKE", AircraftID: bomber.ID, Coord: "0304"}
	if err := ValidateOrder(state, "B", ÄpplyOrder); err != nil {
		t.Fatalf("机场对地打击命令应合法：%v", err)
	}
	ApplyGroundAirOrder(&state, ÄpplyOrder)
	if bomber.Loadout != LoadoutBombTank {
		t.Fatalf("对地攻击应挂炸弹+副油箱：%s", bomber.Loadout)
	}
	if want := SortieBaseRange(bomber.BaseRange, LoadoutBombTank); bomber.MaxRange != want {
		t.Fatalf("挂载后航程错误：%d（期望%d）", bomber.MaxRange, want)
	}
}

// 机场是固定设施，位置始终可见；但细节（类型/跑道）需要通过侦察确认。
func TestAirfieldsAreAlwaysVisible(t *testing.T) {
	state := sandboxState(t)
	// 没有任何侦察情报时，A 方也应当看到 B 的机场
	state.AirfieldContacts = nil
	visible := VisibleAirfields(state, "A")
	enemy := 0
	for _, airfield := range visible {
		if airfield.Faction == "B" {
			enemy++
			if airfield.Coord == "" {
				t.Fatal("敌方机场应当带有坐标")
			}
			if airfield.ContactLevel != 1 {
				t.Fatalf("未侦察的敌方机场情报等级应当是1：%d", airfield.ContactLevel)
			}
		}
	}
	if enemy == 0 {
		t.Fatal("机场固定不动，应当始终可见")
	}
	// 侦察之后情报等级提升：首次发现从 0 → 1，再次侦察 → 2
	state.AirfieldContacts = map[string]map[string]*AirfieldContact{"A": {}}
	scouted := state.Airfields["af-b-sentry"]
	setAirfieldContact(&state, "A", scouted)
	setAirfieldContact(&state, "A", scouted)
	for _, airfield := range VisibleAirfields(state, "A") {
		if airfield.ID == scouted.ID && airfield.ContactLevel != 2 {
			t.Fatalf("两次侦察后情报等级应为2：%d", airfield.ContactLevel)
		}
	}
}

// 机场固定不动、永远显示在地图上，所以未侦察也可以直接打（盲打机场允许）。
func TestBlindStrikeAgainstAirfield(t *testing.T) {
	state := sandboxState(t)
	state.AirfieldContacts = nil
	state.Contacts = map[string]map[string]*FleetContact{}
	if err := ValidateStrikeTarget(state, "A", "0207"); err != nil {
		t.Fatalf("未侦察的敌方机场也应允许打击：%v", err)
	}
}

// 普通/快速舰队是 3 回合移动一次：移动后挂 2 点冷却，
// 经过两个回合结算的递减后归零，第三个回合重新可动。
func TestNormalFleetMovesEveryThreeTurns(t *testing.T) {
	state := sandboxState(t)
	state.Units["probe"] = &Unit{ID: "probe", Type: "CA", Faction: "A", Coord: "0202", FleetID: "probe", Name: "探路舰", Strength: 3, MaxStrength: 3, Card: ShipCard{SpeedClass: "normal", DamageControl: 5}}
	unit := state.Units["probe"]
	start := unit.Coord
	move := ""
	for _, coord := range LegalMoves(state, unit) {
		if Distance(coord, start) == 1 {
			move = coord
			break
		}
	}
	if move == "" {
		t.Fatal("初始状态应当有合法移动")
	}
	ApplyImmediateMove(&state, Order{Type: "SET_MOVE", UnitID: "probe", Coord: move})
	if unit.MoveCooldown != 2 {
		t.Fatalf("3 回合周期移动后应挂 2 点冷却，实际 %d", unit.MoveCooldown)
	}
	AdvanceMovementCooldown(&state)
	if unit.MoveCooldown != 1 {
		t.Fatalf("第一次递减后应为 1，实际 %d", unit.MoveCooldown)
	}
	if moves := LegalMoves(state, unit); len(moves) != 0 {
		t.Fatalf("第一个空档回合不应可动：%v", moves)
	}
	AdvanceMovementCooldown(&state)
	if unit.MoveCooldown != 0 {
		t.Fatalf("第二次递减后应归零，实际 %d", unit.MoveCooldown)
	}
	if moves := LegalMoves(state, unit); len(moves) == 0 {
		t.Fatal("第三个回合应恢复可动")
	}
}
