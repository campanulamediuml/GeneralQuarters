package game

import "testing"

// 侦察半径不可设置：不管命令里写多少，执行时一律用该机型的最大搜索半径。
// 同一机型在同一个剧本里必须只有一个上限（目录按阵营分键，SB2C 和 D3A1 基础航程不同）。
func TestReconRadiusIsAlwaysTheLimit(t *testing.T) {
	state := sandboxState(t)
	seen := map[string]int{}
	checked := 0
	// 每支舰队最多两个侦察组：每个阵营只实测两架，别撞上组数上限。
	perFaction := map[string]int{}
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || !canSearch(*aircraft) || aircraft.Seaplane {
			continue
		}
		if perFaction[aircraft.Faction] >= 2 {
			continue
		}
		perFaction[aircraft.Faction]++
		limit := ReconRadiusLimit(*aircraft)
		key := aircraft.Faction + "/" + aircraft.DefinitionID
		if previous, ok := seen[key]; ok && previous != limit {
			t.Fatalf("%s 同一机型算出了两个半径上限：%d vs %d", key, previous, limit)
		}
		seen[key] = limit
		// 半径没有入参：命令里不再有这个字段，执行时只能取机型上限。
		order := Order{Type: "LAUNCH_RECON", AircraftID: id, Heading: "W", SectorWidth: 60}
		if err := ValidateAirOrder(state, aircraft.Faction, order); err != nil {
			t.Fatalf("%s 侦察命令不应因为半径被拒：%v", id, err)
		}
		ApplyAirOrder(&state, order)
		if got := state.Aircraft[id].ReconRadius; got != limit {
			t.Fatalf("%s 半径应固定为 %d，实际 %d", id, limit, got)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("沙盒剧本没有可搜索的舰载机")
	}
}

// TestReconRadiusProjectionMatchesLimit 快照投影里的上限必须等于校验上限。
func TestReconRadiusProjectionMatchesLimit(t *testing.T) {
	state := sandboxState(t)
	for _, faction := range []string{"A", "B"} {
		view := VisibleAviation(state, faction)
		for _, carrier := range view.Carriers {
			for _, projected := range carrier.Aircraft {
				aircraft := state.Aircraft[projected.ID]
				if aircraft == nil || !canSearch(*aircraft) {
					continue
				}
				want := ReconRadiusLimit(*aircraft)
				if projected.ReconRadiusLimit != want {
					t.Fatalf("%s 投影上限 %d ≠ 实际上限 %d", projected.ID, projected.ReconRadiusLimit, want)
				}
			}
		}
	}
}
