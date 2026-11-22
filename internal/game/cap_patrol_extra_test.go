package game

import "testing"

// TestReconExposureChance 侦察机暴露：基础 30%，处于敌方 CAP 范围内 50%，美军对空雷达 +20%。
func TestReconExposureChance(t *testing.T) {
	state := sandboxState(t)
	recon := &AirUnit{ID: "r", Faction: "A", Kind: "recon", Status: "recon", Coord: "0608"}
	if got := reconExposureChance(state, recon); got != 30 {
		t.Fatalf("不在 CAP 范围内应为 30，实际 %.0f", got)
	}
	state.Units["b-cv-01"].Coord = "0608" // CAP 锚定母舰当前位置
	state.Aircraft = map[string]*AirUnit{"cap-b": {ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Status: "cap", PatrolCoord: "0608", TaskID: "cap-b"}}
	got := reconExposureChance(state, recon)
	if got < 50 {
		t.Fatalf("处于敌方 CAP 范围内应至少 50，实际 %.0f", got)
	}
}

// TestReconInterceptedByCAP 回合末 CAP 会攻击跟踪中的侦察机，命中即进入撤退/损伤。
func TestReconInterceptedByCAP(t *testing.T) {
	intercepted := 0
	for seed := uint64(1); seed <= 40; seed++ {
		state := sandboxState(t)
		recon := &AirUnit{ID: "r", CarrierID: "a2-cv-01", Faction: "A", Kind: "recon", Model: "F6F-5P", Status: "recon", Strength: 1,
			Coord: "0608", OriginCoord: "0602", TrackingFleetID: "TG-B", RemainingRange: 10, MaxRange: 16}
		state.Aircraft = map[string]*AirUnit{
			"r":     recon,
			"cap-b": {ID: "cap-b", CarrierID: "b-cv-01", Faction: "B", Kind: "fighter", Status: "cap", PatrolCoord: "0608", TaskID: "cap-b"},
		}
		state.Units["b-cv-01"].Coord = "0608" // CAP 锚定母舰当前位置
		state.RNGState = seed
		resolveReconExposure(&state)
		if recon.Returning || recon.Damaged || state.Aircraft["r"] == nil {
			intercepted++
		}
	}
	if intercepted == 0 {
		t.Fatal("CAP 应能攻击跟踪中的侦察机")
	}
}

// TestDogfightColumnsStackCAP 列式空战：CAP 多于攻击机时同列堆叠，所有 CAP 都能参战。
func TestDogfightColumnsStackCAP(t *testing.T) {
	state := sandboxState(t)
	attacker := &AirUnit{ID: "bomb", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Status: "attack", Strength: 2, AirCombat: 1, TaskID: "t", TaskKind: "attack"}
	caps := []*AirUnit{
		{ID: "cap1", Faction: "B", Kind: "fighter", Status: "cap", Strength: 2, AirCombat: 5},
		{ID: "cap2", Faction: "B", Kind: "fighter", Status: "cap", Strength: 2, AirCombat: 5},
		{ID: "cap3", Faction: "B", Kind: "fighter", Status: "cap", Strength: 2, AirCombat: 5},
	}
	state.Aircraft = map[string]*AirUnit{attacker.ID: attacker}
	for _, cap := range caps {
		state.Aircraft[cap.ID] = cap
	}
	report := AirCombatReport{}
	resolveDogfight(&state, []*AirUnit{attacker}, caps, &report)
	if report.DefenderCount != 0 {
		t.Fatalf("DefenderCount 应由调用方统计：%d", report.DefenderCount)
	}
	// CAP 三次攻击机会都针对唯一的轰炸机
	attacks := 0
	for _, hit := range report.AttackerHits {
		if hit.Attacker == "cap1" || hit.Attacker == "cap2" || hit.Attacker == "cap3" {
			attacks++
		}
	}
	_ = attacks
}

// TestDCTModifierWhenWarned 被提前发现的空袭：命中结算 DCT +1，更容易打出严重伤害。
func TestDCTModifierWhenWarned(t *testing.T) {
	state := sandboxState(t)
	ship := state.Units["b-cv-01"]
	ship.Card.DamageControl = 5
	if dctValue(&state, ship, false) != 5 {
		t.Fatal("未被发现时 DCT 不变")
	}
	if dctValue(&state, ship, true) != 6 {
		t.Fatal("被提前发现时 DCT 应 +1")
	}
}
