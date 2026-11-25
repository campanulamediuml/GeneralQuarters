package game

import "testing"

// B-26：强度 2、对空 2、对地 2、对舰 2；陆基中型轰炸机，既能挂炸弹也能挂鱼雷。
func TestB26DefinitionAndLoadout(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 5)
	if err != nil {
		t.Fatal(err)
	}
	townsville := state.Airfields["af-a-townsville"]
	if townsville == nil || townsville.Coord != "4410" || townsville.Type != AirfieldLarge ||
		townsville.Runways != 3 || !townsville.Radar || townsville.HeavyAA != 10 || townsville.LightAA != 20 {
		t.Fatalf("汤斯维尔基地配置错误：%+v", townsville)
	}
	b26 := state.GroundAir["af-a-townsville-b26-1"]
	if b26 == nil || b26.Kind != "medium_bomber" || b26.Strength != 2 || b26.BaseRange != 24 ||
		b26.AirCombat != 2 || b26.AntiShip != 2 || b26.AntiGround != 2 ||
		!b26.AirfieldOnly || !b26.TorpedoCapable || !b26.HighAltitudeBomber {
		t.Fatalf("B-26 数据不对：%+v", b26)
	}
	if !containsLoadout(LoadoutOptionsFor("medium_bomber", false, 2, 2, true), LoadoutTorpedoTank) {
		t.Fatal("B-26 应该有鱼雷挂载选项")
	}
	if err := ValidateLoadoutFor("medium_bomber", false, 2, 2, LoadoutTorpedoTank, true); err != nil {
		t.Fatalf("B-26 应能挂鱼雷：%v", err)
	}
	if got := DefaultLoadoutFor("medium_bomber", false, 2, 2, "attack", StrikeTargetShip, true); got != LoadoutBombTank {
		t.Fatalf("B-26 对舰默认应挂炸弹，实际 %s", got)
	}

	// B-25 / B-17 都没有鱼雷挂架。B-17 即使被错误标上能力，也不能出现在挂载表里。
	b25 := state.GroundAir["af-a-moresby-b25-1"]
	if b25 == nil || b25.TorpedoCapable {
		t.Fatalf("B-25 不应有鱼雷挂架：%+v", b25)
	}
	b17 := state.GroundAir["af-a-moresby-b17-1"]
	if b17 == nil || b17.TorpedoCapable {
		t.Fatalf("B-17 不应有鱼雷挂架：%+v", b17)
	}
	if containsLoadout(LoadoutOptionsFor("strategic_bomber", false, 2, 2, true), LoadoutTorpedoTank) {
		t.Fatal("战略轰炸机不应因为能力标记而获得鱼雷选项")
	}
	if err := ValidateLoadoutFor("strategic_bomber", false, 2, 2, LoadoutTorpedoTank, false); err == nil {
		t.Fatal("B-17 不该能挂鱼雷")
	}
}

func containsLoadout(options []Loadout, want Loadout) bool {
	for _, option := range options {
		if option == want {
			return true
		}
	}
	return false
}

// B-26 挂炸弹时按高空轰炸机结算；挂鱼雷时按鱼雷机结算，不再享受高空特性。
func TestB26BombAndTorpedoModes(t *testing.T) {
	b26 := &AirUnit{
		ID: "b26", Kind: "medium_bomber", Model: "B-26 劫掠者",
		TorpedoCapable: true, HighAltitudeBomber: true, Loadout: LoadoutBombTank,
	}
	if !isHighAltitudeBomber(b26) {
		t.Fatal("B-26 挂炸弹时应按高空轰炸机结算")
	}
	if isTorpedoAttack(b26) {
		t.Fatal("B-26 挂炸弹时不应按鱼雷机结算")
	}
	b26.Loadout = LoadoutTorpedoTank
	if isHighAltitudeBomber(b26) {
		t.Fatal("B-26 挂鱼雷时不应再按高空轰炸机结算")
	}
	if !isTorpedoAttack(b26) {
		t.Fatal("B-26 挂鱼雷时应按鱼雷机结算")
	}
}

// 高空轰炸对舰命中率减半；B-26 挂鱼雷时回到普通鱼雷机的命中率。
func TestB26HighAltitudeShipPenalty(t *testing.T) {
	state := coralCombatState(t)
	ship := firstEnemyShip(&state, "A")
	if ship == nil {
		t.Fatal("找不到目标舰")
	}
	b17 := &AirUnit{Kind: "strategic_bomber", Model: "B-17 空中堡垒", Loadout: LoadoutBombTank}
	b26Bomb := &AirUnit{Kind: "medium_bomber", Model: "B-26 劫掠者", TorpedoCapable: true, HighAltitudeBomber: true, Loadout: LoadoutBombTank}
	b26Torpedo := &AirUnit{Kind: "medium_bomber", Model: "B-26 劫掠者", TorpedoCapable: true, HighAltitudeBomber: true, Loadout: LoadoutTorpedoTank}
	bombChance := strikeHitChance(state, b26Bomb, ship, false, true)
	if bombChance != strikeHitChance(state, b17, ship, false, true) {
		t.Fatalf("B-26 挂炸弹应与 B-17 同样吃高空惩罚：%.2f vs %.2f", bombChance, strikeHitChance(state, b17, ship, false, true))
	}
	torpedoChance := strikeHitChance(state, b26Torpedo, ship, false, true)
	diveChance := strikeHitChance(state, &AirUnit{Kind: "dive_bomber", Model: "SBD 无畏"}, ship, false, true)
	if torpedoChance != diveChance {
		t.Fatalf("B-26 挂鱼雷应按普通鱼雷机/轰炸机命中率结算：%.2f vs 基准 %.2f", torpedoChance, diveChance)
	}
	if torpedoChance <= bombChance {
		t.Fatalf("B-26 挂鱼雷的命中率应高于高空炸弹：%.2f vs %.2f", torpedoChance, bombChance)
	}
}

// 高空轰炸免疫轻防空；B-26 挂鱼雷时仍然正常挨轻防空（而且鱼雷机翻倍）。
func TestB26BombIgnoresLightAATorpedoDoesNot(t *testing.T) {
	state := coralCombatState(t)
	reference := firstEnemyShip(&state, "A")
	if reference == nil {
		t.Fatal("找不到参照舰")
	}
	ship := &Unit{ID: "b26-aa-target", Faction: "B", Type: "DD", Coord: reference.Coord, Card: ShipCard{LightAA: 9}}
	next := 0
	group := func(loadout Loadout) []*AirUnit {
		planes := []*AirUnit{}
		for i := 0; i < 4; i++ {
			next++
			plane := &AirUnit{
				ID: "b26-" + string(rune('a'+next)), Kind: "medium_bomber", Model: "B-26 劫掠者",
				TorpedoCapable: true, HighAltitudeBomber: true, Loadout: loadout,
				Status: "attack", Strength: 2, Coord: ship.Coord, Faction: "B",
			}
			state.Aircraft[plane.ID] = plane
			planes = append(planes, plane)
		}
		return planes
	}
	for seed := uint64(1); seed <= 20; seed++ {
		state.RNGState = seed
		report := &AirCombatReport{}
		resolveLightAAPool(&state, group(LoadoutBombTank), ship, false, report)
		if report.AAHits != 0 {
			t.Fatalf("种子%d：B-26 挂炸弹不应被轻防空命中，实际 %d 次", seed, report.AAHits)
		}
	}
	hit := false
	for seed := uint64(1); seed <= 20; seed++ {
		state.RNGState = seed
		report := &AirCombatReport{}
		resolveLightAAPool(&state, group(LoadoutTorpedoTank), ship, false, report)
		if report.AAHits > 0 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("B-26 挂鱼雷时仍然应该吃轻防空")
	}
}
