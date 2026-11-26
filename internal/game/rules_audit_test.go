package game

import "testing"

// 规则书 119 行：CVL/CVE 在空袭结算中视为 CV，应拿到 CV 的目标权重 100。
func TestLightCarriersCountAsCarriersInStrikeWeight(t *testing.T) {
	for _, shipType := range []string{"CV", "CVL", "CVE"} {
		if got := strikeShipWeight(shipType); got != 100 {
			t.Fatalf("%s 的空袭目标权重应为 100（按 CV 结算），实际 %d", shipType, got)
		}
	}
	if got := strikeShipWeight("DD"); got != 1 {
		t.Fatalf("驱逐舰权重应为 1，实际 %d", got)
	}
}

// 规则书 119 行：美军轻航母同样享受"并入同格最高轻防空"。
func TestLightCarrierMergesFleetLightAA(t *testing.T) {
	state := State{
		Doctrines: map[string]string{"A": "us"},
		Units: map[string]*Unit{
			"cve": {ID: "cve", Faction: "A", Type: "CVE", Coord: "0101", Card: ShipCard{LightAA: 3}},
			"ca":  {ID: "ca", Faction: "A", Type: "CA", Coord: "0101", Card: ShipCard{LightAA: 8}},
		},
	}
	cve := state.Units["cve"]
	value := lightAAValue(state, &AirUnit{Kind: "dive_bomber"}, cve)
	if value != 11 {
		t.Fatalf("轻航母轻防空应为 3+8=11，实际 %.1f", value)
	}
}

// 规则书 269–278 行：极慢本身就是档位，不该被进水一档就直接判成极慢。
func TestVerySlowSpeedClassIsItsOwnTier(t *testing.T) {
	unit := &Unit{Card: ShipCard{SpeedClass: "very_slow"}}
	if got := EffectiveSpeedClass(unit); got != "very_slow" {
		t.Fatalf("极慢舰船应为 very_slow，实际 %s", got)
	}
	fleet := State{Units: map[string]*Unit{"x": {ID: "x", FleetID: "x", Type: "BB", Card: ShipCard{SpeedClass: "very_slow"}}}}
	if got := FleetMoveInterval(fleet, fleet.Units["x"]); got != 10 {
		t.Fatalf("极慢舰队移动间隔应为 10，实际 %d", got)
	}
	slow := &Unit{ID: "s", FleetID: "s", Card: ShipCard{SpeedClass: "slow", DamageControl: 5}, Damage: &ShipDamage{Flooding: []int{1}}}
	if got := EffectiveSpeedClass(slow); got != "very_slow" {
		t.Fatalf("慢速进水一档应降为 very_slow，实际 %s", got)
	}
}

// 规则书 387 行：损伤算子每回合 10% 修复，日军 3%。阵营必须看 doctrine 而不是内部代号。
func TestAircraftRepairChanceUsesDoctrine(t *testing.T) {
	if got := aircraftRepairChance(State{Doctrines: map[string]string{"B": "japan"}}, "B"); got != 3 {
		t.Fatalf("日军损伤算子修复率应为 3%%，实际 %v", got)
	}
	if got := aircraftRepairChance(State{Doctrines: map[string]string{"A": "us"}}, "A"); got != 10 {
		t.Fatalf("美军损伤算子修复率应为 10%%，实际 %v", got)
	}
	if got := aircraftRepairChance(State{Doctrines: map[string]string{"J": "japan"}}, "J"); got != 3 {
		t.Fatalf("按 doctrine 判断日军修复率应为 3%%，实际 %v", got)
	}
}

// 规则书 647 行：战略轰炸机对舰命中概率减半，最低 5%。
func TestStrategicBomberHalvesStrikeChance(t *testing.T) {
	state := State{Map: MapDefinition{}}
	target := &Unit{ID: "t", Type: "BB", FleetID: "f", Coord: "0101", Card: ShipCard{SpeedClass: "normal"}}
	bomber := &AirUnit{ID: "b", Kind: "strategic_bomber", Model: "B-17"}
	dive := &AirUnit{ID: "d", Kind: "dive_bomber", Model: "SBD"}
	normal := strikeHitChance(state, dive, target, false, true)
	halved := strikeHitChance(state, bomber, target, false, true)
	if halved != normal/2 {
		t.Fatalf("战略轰炸机命中率应为普通轰炸机的一半：普通 %.1f → 战略 %.1f", normal, halved)
	}
	state.Map = MapDefinition{Cells: []MapCell{{Coord: "0101", Weather: "rain"}}}
	heavy := &Unit{ID: "h", Type: "DD", FleetID: "f", Coord: "0101", Card: ShipCard{SpeedClass: "fast", LightAA: 70}}
	if got := strikeHitChance(state, bomber, heavy, false, true); got < 5 {
		t.Fatalf("战略轰炸机对舰命中率不应低于 5%%，实际 %.1f", got)
	}
}

// 规则书 657 行：机场每有一个正在起降的算子，对地攻击 +1%。
func TestGroundAttackBonusPerAircraftInOps(t *testing.T) {
	state := State{Map: MapDefinition{}}
	airfield := &Airfield{ID: "f1", Coord: "0101", LightAA: 0}
	base := groundHitChance(state, airfield, &AirfieldAirUnit{AirfieldID: "f1"}, false)
	state.GroundAir = map[string]*AirfieldAirUnit{
		"g1": {ID: "g1", AirfieldID: "f1", Status: "deck"},
		"g2": {ID: "g2", AirfieldID: "f1", Status: "ready"},
		"g3": {ID: "g3", AirfieldID: "f1", Status: "hangar"},
	}
	withOps := groundHitChance(state, airfield, &AirfieldAirUnit{AirfieldID: "f1"}, false)
	if withOps-base != 2 {
		t.Fatalf("两个在起降中的算子应 +2%%：%.1f → %.1f", base, withOps)
	}
}

// 规则书 624–631 行：分散攻击均匀随机选船，不会像集中攻击那样永远扑向航母。
func TestDisperseStrikeSpreadAcrossShips(t *testing.T) {
	state := State{Units: map[string]*Unit{
		"cv": {ID: "cv", Type: "CV", Coord: "0101"},
		"bb": {ID: "bb", Type: "BB", Coord: "0101"},
		"dd": {ID: "dd", Type: "DD", Coord: "0101"},
	}, RNGState: 12345}
	ships := []Unit{*state.Units["cv"], *state.Units["bb"], *state.Units["dd"]}
	concentrated := &AirUnit{ID: "a1", Kind: "dive_bomber"}
	hits := map[string]int{}
	for i := 0; i < 600; i++ {
		if target := selectStrikeShip(&state, concentrated, ships); target != nil {
			hits[target.ID]++
		}
	}
	if hits["cv"] < 450 {
		t.Fatalf("集中攻击应几乎全部扑向航母，实际 %v", hits)
	}
	dispersed := &AirUnit{ID: "a2", Kind: "dive_bomber", Disperse: true}
	spread := map[string]int{}
	for i := 0; i < 600; i++ {
		if target := selectStrikeShip(&state, dispersed, ships); target != nil {
			spread[target.ID]++
		}
	}
	for _, id := range []string{"cv", "bb", "dd"} {
		if spread[id] < 120 || spread[id] > 280 {
			t.Fatalf("分散攻击应大致均匀：%v", spread)
		}
	}
}

// 设置攻击方式的命令要能落到整条编队上。
func TestSetStrikeDispersionAppliesToTask(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("攻击编队分配失败：%v", err)
	}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	if err := ValidateAirOrder(state, "B", Order{Type: "SET_STRIKE_DISPERSION", AircraftID: taskID, Disperse: true}); err != nil {
		t.Fatalf("设置分散攻击被拒绝：%v", err)
	}
	ApplyAirOrder(&state, Order{Type: "SET_STRIKE_DISPERSION", AircraftID: taskID, Disperse: true})
	for _, aircraft := range taskAircraft(state, taskID) {
		if !aircraft.Disperse {
			t.Fatalf("算子 %s 没有切换到分散攻击", aircraft.ID)
		}
	}
}

// 按任务 ID 下达的攻击方式命令要真正落到成员上：任务 ID 常与首架飞机 ID 相同。
func TestDispersionOrderByTaskIDReachesMembers(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	cmd := Order{Type: "SET_STRIKE_DISPERSION", AircraftID: taskID, Disperse: true}
	if err := ValidateOrder(state, "B", cmd); err != nil {
		t.Fatalf("按任务 ID 设置攻击方式被拒绝：%v", err)
	}
	ApplyAirOrder(&state, cmd)
	for _, aircraft := range taskAircraft(state, taskID) {
		if !aircraft.Disperse {
			t.Fatalf("成员 %s 没有打开分散攻击", aircraft.ID)
		}
	}
}

// 尚未起飞的攻击编队也要能改攻击方式。
func TestDispersionOnPreparingTask(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_STRIKE", AircraftIDs: []string{"b-cv-01-dive_bomber-x1", "b-cv-01-torpedo_bomber-x1"}}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	taskID := state.Aircraft["b-cv-01-dive_bomber-x1"].TaskID
	if state.Aircraft["b-cv-01-dive_bomber-x1"].Status != "service" {
		t.Fatalf("应当还在机库整备：%s", state.Aircraft["b-cv-01-dive_bomber-x1"].Status)
	}
	cmd := Order{Type: "SET_STRIKE_DISPERSION", AircraftID: taskID, Disperse: true}
	if IsAirfieldOrder(state, cmd) {
		t.Fatal("不应被判为机场命令")
	}
	events := ApplyAirOrder(&state, cmd)
	if len(events) == 0 {
		t.Fatal("准备中的编队改攻击方式没有产生事件")
	}
	for _, aircraft := range taskAircraft(state, taskID) {
		if !aircraft.Disperse {
			t.Fatalf("成员 %s 没有打开分散攻击", aircraft.ID)
		}
	}
}
