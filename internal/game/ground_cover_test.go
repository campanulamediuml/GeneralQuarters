package game

import (
	"encoding/json"
	"strings"
	"testing"
)

// 陆基战斗机在 GroundAir 里执勤，防守判定只扫 state.Aircraft：
// 投影之后必须能被看到，投影撤销后必须消失（不能残留成幽灵飞机）。
func TestGroundCAPIsProjectedForDefense(t *testing.T) {
	state := coralCombatState(t)
	plane := state.GroundAir["af-a-moresby-p40-1"]
	if plane == nil {
		t.Fatal("找不到莫尔兹比 P-40")
	}
	plane.Status = "cap"
	plane.TaskID = "cap-moresby"
	plane.Coord = "1611"
	plane.TargetCoord = "1611"
	plane.OriginCoord = "1611"

	if len(airfieldCAPGroups(state, "A", "1611")) != 0 {
		t.Fatal("投影之前不应该可见（前提变了，测试需要重写）")
	}
	restore := projectDefendingGroundAir(&state, "A", "1611", 1)
	if len(airfieldCAPGroups(state, "A", "1611")) == 0 {
		restore()
		t.Fatal("投影之后机场防空应当看到陆基 CAP")
	}
	if len(strikeCAPGroups(state, "A", "1611")) == 0 {
		restore()
		t.Fatal("投影之后舰队防空也应当看到陆基 CAP")
	}
	restore()
	if len(airfieldCAPGroups(state, "A", "1611")) != 0 || state.Aircraft[plane.ID] != nil {
		t.Fatal("投影撤销后不应残留")
	}
	// 距离之外不投影。
	restore = projectDefendingGroundAir(&state, "A", "3000", 1)
	if state.Aircraft[plane.ID] != nil {
		restore()
		t.Fatal("距离超过 1 格时不应投影")
	}
	restore()
}

// 端到端：陆基 CAP 参与舰队防守，目标上空不再报"没有 CAP"。
func TestGroundCAPJoinsFleetDefense(t *testing.T) {
	build := func(capCoord string) (State, string) {
		state := coralCombatState(t)
		// 找一对"陆地 hex + 相邻海 hex"：CAP 飞机停在陆地上空，舰队在海格里，
		// 距离正好 1 格，符合"覆盖目标周围 1 hex"的判定。
		sea := ""
		landNext := ""
		for _, cell := range state.Map.Cells {
			for _, coord := range Neighbors(cell.Coord) {
				if terrainAt(state.Map, coord) == TerrainWater {
					sea, landNext = coord, cell.Coord
					break
				}
			}
			if sea != "" {
				break
			}
		}
		if sea == "" {
			t.Fatal("地图上找不到相邻的陆地/海面格")
		}
		for _, unit := range state.Units {
			if unit.Faction == "A" {
				if sea != "" {
					unit.Coord = sea
				}
				break
			}
		}
		plane := state.GroundAir["af-a-moresby-p40-1"]
		plane.Status = "cap"
		plane.TaskID = "cap-moresby"
		plane.Coord = capCoord
		if capCoord == "1611" {
			// 覆盖到海格：停在陆地格上，靠 1 hex 的判定覆盖旁边的舰队。
			plane.Coord = landNext
		}
		plane.TargetCoord = plane.Coord
		plane.OriginCoord = "1611"
		attacker := &AirUnit{
			ID: "atk-1", Faction: "B", Kind: "dive_bomber", Model: "D3A1 九九舰爆",
			Status: "attack", Strength: 1, Coord: sea, TaskID: "atk-task",
		}
		state.Aircraft[attacker.ID] = attacker
		return state, sea
	}

	state, sea := build("1611")
	for _, event := range resolveAirStrike(&state, "atk-task", sea) {
		if event.Type == "air_cap_unavailable" {
			t.Fatalf("陆基 CAP 在目标格时不应报没有 CAP：%s", event.Text)
		}
	}

	far, seaFar := build("3000")
	sawNote := false
	for _, event := range resolveAirStrike(&far, "atk-task", seaFar) {
		if event.Type == "air_cap_unavailable" {
			sawNote = true
		}
	}
	if !sawNote {
		t.Fatal("陆基 CAP 离得远时应当提示目标上空没有 CAP")
	}
}

// 空中掩护只能由战斗机执行，且能挂对空载荷起飞到掩护 hex 上空。
func TestGroundCoverMission(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 17
	UpdateDayNight(&state)

	bad := Order{Type: "LAUNCH_COVER", AircraftIDs: []string{"af-a-moresby-b25-1"}, Coord: "1611"}
	if err := ValidateGroundAirOrder(state, bad); err == nil {
		t.Fatal("轰炸机不应该能执行空中掩护")
	}

	order := Order{Type: "LAUNCH_COVER", AircraftIDs: []string{"af-a-moresby-p40-1", "af-a-moresby-p40-2"}, Coord: "1511"}
	if err := ValidateGroundAirOrder(state, order); err != nil {
		t.Fatalf("战斗机应当能执行空中掩护：%v", err)
	}
	ApplyGroundAirOrder(&state, order)

	airborne := false
	for turn := 0; turn < 30; turn++ {
		ResolveOrders(&state, nil)
		plane := state.GroundAir["af-a-moresby-p40-1"]
		if plane == nil {
			t.Fatal("P-40 不见了")
		}
		if plane.Status == "cover" {
			airborne = true
		}
		if plane.Status == "cover" && plane.Coord == "1511" {
			if plane.Loadout.Weapon() != WeaponAir {
				t.Fatalf("掩护任务应当挂对空载荷，实际 %s", plane.Loadout)
			}
			return
		}
	}
	t.Fatalf("30 回合内没有在掩护 hex 上空就位（airborne=%v）", airborne)
}

// 机场算子被打成"摧毁"后必须真的消失，而不是投影一撤就复活。
func TestGroundAircraftDestroyedInProjectionIsRemoved(t *testing.T) {
	state := coralCombatState(t)
	plane := state.GroundAir["af-a-moresby-p40-1"]
	plane.Status = "cap"
	plane.TaskID = "cap-moresby"
	plane.Coord = "1611"
	plane.TargetCoord = "1611"
	restore := projectDefendingGroundAir(&state, "A", "1611", 1)
	if state.Aircraft[plane.ID] == nil {
		restore()
		t.Fatal("投影失败")
	}
	// 模拟 applyAircraftHit 的"摧毁"：从 state.Aircraft 删掉。
	delete(state.Aircraft, plane.ID)
	restore()
	if state.GroundAir[plane.ID] != nil {
		t.Fatal("被击落的机场算子不应留在 GroundAir")
	}
}

// 机场飞机损失要进损失台账（每架 0.25 分的那一项）。
func TestGroundAircraftLossIsRecorded(t *testing.T) {
	state := coralCombatState(t)
	snapshotAircraftRoster(&state)
	before := ledgerFor(&state, "A").AircraftTotal
	delete(state.GroundAir, "af-a-moresby-p40-1")
	settleAircraftLosses(&state)
	if got := ledgerFor(&state, "A").AircraftTotal; got != before+1 {
		t.Fatalf("机场飞机被毁应记 1 架损失，实际 %d → %d", before, got)
	}
}

// CAP 会一直守在目标上空，直到被击中或弹药耗尽才退出——同回合第二波空袭照样参战。
func TestCAPKeepsFightingUntilHitOrOutOfAmmo(t *testing.T) {
	state := coralCombatState(t)
	plane := state.GroundAir["af-a-moresby-p40-1"]
	plane.Status = "cap"
	plane.TaskID = "cap-x"
	plane.Coord = "1611"
	plane.TargetCoord = "1611"
	plane.OriginCoord = "1611"
	restore := projectDefendingGroundAir(&state, "A", "1611", 1)
	defer restore()
	unit := state.Aircraft[plane.ID]
	if unit == nil {
		t.Fatal("投影失败")
	}
	// 已经拦截过一波：仍然要出现在目标上空。
	unit.InterceptedThisTurn = true
	if len(strikeCAPGroups(state, "A", "1611")) == 0 {
		t.Fatal("拦截过一波的 CAP 仍然应该守在空中")
	}
	if len(capGroupsWithin(state, "A", "1611", 2)) == 0 {
		t.Fatal("前出拦截判定同样不应该因为拦截过就排除")
	}
	// 被打中（返航）才退出。
	unit.Returning = true
	if len(strikeCAPGroups(state, "A", "1611")) != 0 {
		t.Fatal("返航中的 CAP 不应继续参战")
	}
	if len(capGroupsWithin(state, "A", "1611", 2)) != 0 {
		t.Fatal("返航中的 CAP 不应再被拉去前出拦截")
	}
}

// 空战/空袭战报要带算子图片，前端动画才不用回退到旧矢量剪影。
func TestCombatReportCarriesAircraftPictures(t *testing.T) {
	state := coralCombatState(t)
	planes := []*AirUnit{}
	for _, aircraft := range state.Aircraft {
		planes = append(planes, aircraft)
		if len(planes) == 3 {
			break
		}
	}
	if len(planes) == 0 {
		t.Fatal("没有可用舰载机")
	}
	models, pictures := aircraftModelCards(planes)
	if len(models) != len(pictures) {
		t.Fatalf("机型名和图片数量不一致：%d / %d", len(models), len(pictures))
	}
	found := false
	for _, picture := range pictures {
		if picture != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("舰载机算子上没有图片路径：%+v", planes[0])
	}
	if got := pictureCardsFor(planes); len(got) != len(models) {
		t.Fatalf("pictureCardsFor 数量不对：%d vs %d", len(got), len(models))
	}
}

// 战报序列化后必须真的带 attacker_pictures 字段（前端按这个字段渲染算子图）。
func TestCombatReportJSONHasPictures(t *testing.T) {
	state := coralCombatState(t)
	var planes []*AirUnit
	for _, aircraft := range state.Aircraft {
		planes = append(planes, aircraft)
		break
	}
	models, pictures := aircraftModelCards(planes)
	report := AirCombatReport{AttackerModels: models, AttackerPictures: pictures}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "attacker_pictures") {
		t.Fatalf("战报里没有图片字段：%s", raw)
	}
}
