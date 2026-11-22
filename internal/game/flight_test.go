package game

import "testing"

// TestReconConsumesFuelWhileSearching 回归：侦察机在扇面搜索时每移动一格都要扣航程。
func TestReconConsumesFuelWhileSearching(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "a2-cv-04-dive_bomber-x1", CarrierID: "a2-cv-01", Faction: "A", Kind: "recon", Model: "F6F-5P", Status: "recon",
		Strength: 1, TaskID: "a2-cv-04-dive_bomber-x1", TaskKind: "recon", Heading: "E", SectorWidth: 60, ReconRadius: 4,
		OriginCoord: "0602", Coord: "0602", RemainingRange: 10, MaxRange: 16}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.Units["a2-cv-01"].Deck.Operation = ""
	before := aircraft.Range()
	advanceRecon(&state, aircraft)
	if aircraft.Range() >= before {
		t.Fatalf("侦察搜索应消耗航程：%d -> %d", before, aircraft.Range())
	}
}

// TestFlightPathCrossesIsland 回归：飞机在空中可以飞越岛屿，只受地图边界限制。
// 早期版本复用了舰队的水面通行规则，导致飞往机场（岛屿 hex）的编队永远到不了目标。
func TestFlightPathCrossesIsland(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "probe", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Coord: "0608"}
	// 0607 是岛屿，但飞机可以直接穿过它飞往 0606
	next := nextFlightCoord(state, aircraft, "0606")
	if next != "0607" {
		t.Fatalf("飞机应当直接穿越岛屿飞向目标：得到 %s", next)
	}
	if !flightCanEnter(state, aircraft, "0607") {
		t.Fatal("岛屿对飞机应当是可通行的")
	}
	// 地图边界仍然不可飞
	if flightCanEnter(state, aircraft, "9999") {
		t.Fatal("地图外不应可飞")
	}
}

// 飞往岛屿上的机场时，BFS 必须能找到路径（否则对地空袭根本出发不了）。
func TestFlightReachesAirfieldOnIsland(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "bomb", CarrierID: "b-cv-01", Faction: "B", Kind: "dive_bomber", Coord: "0208"}
	// 0304 是 A 方机场（岛屿），B 方编队必须能规划出航线
	next := nextFlightCoord(state, aircraft, "0304")
	if next == "" {
		t.Fatal("飞往岛屿机场的航线不应为空")
	}
}

// TestRadarDetectionChanceTable 雷达距离表：美军航母 90/70/50/30/10，未装雷达 30/0/0/0/0。
func TestRadarDetectionChanceTable(t *testing.T) {
	state := sandboxState(t)
	cv := state.Units["a2-cv-01"]
	for distance, want := range map[int]float64{1: 90, 2: 70, 3: 50, 4: 30, 5: 10} {
		if got := radarDetectionChance(cv, distance); got != want {
			t.Fatalf("航母雷达 %d hex 发现率应为 %.0f，实际 %.0f", distance, want, got)
		}
	}
	dd := state.Units["a-dd-01"]
	if got := radarDetectionChance(dd, 1); got != 30 {
		t.Fatalf("无雷达单位 1 hex 发现率应为 30，实际 %.0f", got)
	}
	if got := radarDetectionChance(dd, 2); got != 0 {
		t.Fatalf("无雷达单位 2 hex 应为 0，实际 %.0f", got)
	}
}

// TestEngagementChance 接敌率：同 hex 基础 200%（上限 100），相邻 1 hex 基础 100% 乘天气系数。
func TestEngagementChance(t *testing.T) {
	state := sandboxState(t)
	attacker := &AirUnit{ID: "a", Faction: "A", CarrierID: "a2-cv-01", Coord: "0608"}
	same := engageChance(state, "A", "0608", "0608", []*AirUnit{attacker})
	if same < 100 {
		t.Fatalf("同 hex 接敌率应封顶 100，实际 %.0f", same)
	}
	far := engageChance(state, "A", "0608", "0607", []*AirUnit{attacker})
	if far > 100 || far < 0 {
		t.Fatalf("相邻 hex 接敌率应在 0–100，实际 %.0f", far)
	}
	if engageChance(state, "A", "0608", "0605", []*AirUnit{attacker}) != 0 {
		t.Fatal("目标超过 1 hex 时不应直接接敌")
	}
}

// TestAttackRequiresEngagementBeforeStrike 回归：抵达目标 hex 当回合只做接敌判定，下一回合才空袭。
func TestAttackRequiresEngagementBeforeStrike(t *testing.T) {
	state := sandboxState(t)
	// 目标 hex 上放一支 B 舰队
	aircraft := &AirUnit{ID: "sb", CarrierID: "a2-cv-01", Faction: "A", Kind: "dive_bomber", Model: "SB2C", Status: "attack",
		Strength: 2, AirCombat: 1, AntiShip: 4, TaskID: "sb", TaskKind: "attack", Coord: "0608", TargetCoord: "0608", RemainingRange: 10, MaxRange: 12}
	state.Aircraft = map[string]*AirUnit{aircraft.ID: aircraft}
	state.RNGState = 3
	advanceAttack(&state, aircraft)
	if !aircraft.Engaged {
		t.Fatalf("抵达目标 hex 时应完成接敌判定")
	}
	if len(state.AirCombats) != 0 {
		t.Fatal("接敌当回合不应结算空袭")
	}
	state.Units["b-cv-01"].Deck.Operation = ""
	advanceAttack(&state, aircraft)
	if aircraft.Engaged && len(state.AirCombats) == 0 {
		t.Fatal("已完成接敌的下一回合应发起空袭")
	}
}
