package game

import "testing"

// TestHeavyAAHitsEscortFighters 规则书 689 行：重型防空从“当前参与空袭的算子”里随机选目标，
// 护航战斗机不是免疫的。之前 resolveHeavyAA 只收到轰炸机列表，护航机怎么都挨不到打。
func TestHeavyAAHitsEscortFighters(t *testing.T) {
	state := sandboxState(t)
	fighter := &AirUnit{ID: "escort-1", CarrierID: "a2-cv-01", Faction: "A", Kind: "fighter", Model: "F6F", Status: "attack",
		Strength: 2, AirCombat: 6, TaskID: "strike-1", TaskKind: "attack", Coord: "0304"}
	bomber := &AirUnit{ID: "bomber-1", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack",
		Strength: 2, AirCombat: 1, AntiShip: 4, TaskID: "strike-1", TaskKind: "attack", Coord: "0304"}
	state.Aircraft = map[string]*AirUnit{fighter.ID: fighter, bomber.ID: bomber}

	escortHits := 0
	for seed := uint64(1); seed <= 12; seed++ {
		state.Aircraft = map[string]*AirUnit{fighter.ID: fighter, bomber.ID: bomber}
		fighter.Damaged, fighter.Returning, fighter.Strength = false, false, 2
		bomber.Damaged, bomber.Returning, bomber.Strength = false, false, 2
		ships := []Unit{*state.Units["b-cv-01"]}
		ships[0].Card.HeavyAA = 200 // 强制大量判定，避免测试受单次随机影响
		report := &AirCombatReport{}
		state.RNGState = seed
		resolveHeavyAA(&state, []*AirUnit{fighter, bomber}, ships, ships[0].Coord, report)
		if state.Aircraft[fighter.ID] == nil || fighter.Damaged || fighter.Returning || fighter.Strength < 2 {
			escortHits++
		}
	}
	if escortHits == 0 {
		t.Fatal("12 个种子里护航战斗机一次都没被打到，重型防空的目标池里没有护航机")
	}
}

// TestLightAANeverTargetsEscorts 规则书 685–697 行：轻型防空只对“以本舰为目标的飞机”结算，
// 护航战斗机不参与投弹，所以轻防空威胁不到它们；重防空才是全员平等。
func TestLightAANeverTargetsEscorts(t *testing.T) {
	state := sandboxState(t)
	coord := state.Units["b-cv-01"].Coord
	escorts := []*AirUnit{}
	all := map[string]*AirUnit{}
	for _, id := range []string{"a2-cv-01-fighter-x1", "a2-cv-01-fighter-x2", "a2-cv-01-fighter-x3"} {
		aircraft := state.Aircraft[id]
		aircraft.Status = "attack"
		aircraft.Launched = true
		aircraft.TaskID = "strike-1"
		aircraft.TaskKind = "attack"
		aircraft.Coord = coord
		aircraft.TargetCoord = coord
		aircraft.OriginCoord = coord
		all[id] = aircraft
		escorts = append(escorts, aircraft)
	}
	bomber := state.Aircraft["a2-cv-01-dive_bomber-x1"]
	bomber.Status = "attack"
	bomber.Launched = true
	bomber.TaskID = "strike-1"
	bomber.TaskKind = "attack"
	bomber.Coord = coord
	bomber.TargetCoord = coord
	bomber.OriginCoord = coord
	all[bomber.ID] = bomber
	state.Aircraft = all
	// 只留轻型防空，重防空清零，避免护航机被重防空打掉造成误判。
	for _, unit := range state.Units {
		if unit.Faction == "B" && unit.Coord == coord {
			unit.Card.HeavyAA = 0
			unit.Card.LightAA = 500
		}
	}
	state.RNGState = 11
	resolveAirStrike(&state, "strike-1", coord)
	combat := state.AirCombats[len(state.AirCombats)-1]
	if combat.AAHits == 0 {
		t.Fatalf("轻防空没有命中，测试前提不成立：%+v", combat)
	}
	escortIDs := map[string]bool{}
	for _, escort := range escorts {
		escortIDs[escort.ID] = true
	}
	for _, hit := range combat.AttackerHits {
		if escortIDs[hit.Attacker] {
			t.Fatalf("轻型防空打到了护航战斗机 %s（%s）：轻防空只结算投弹飞机", hit.Attacker, hit.Outcome)
		}
	}
	for _, escort := range escorts {
		if state.Aircraft[escort.ID] == nil || escort.Damaged || escort.Strength < 2 {
			t.Fatalf("护航战斗机不应被轻型防空打到：%+v（AAHits=%d）", escort, combat.AAHits)
		}
	}
}
