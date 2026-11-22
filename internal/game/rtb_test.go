package game

import "testing"

// 返航距离要按实际飞行路径算：绕开岛屿时比直线距离更远。
func TestRTBDistanceUsesFlightPath(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "probe", Faction: "B", CarrierID: "b-cv-01", Coord: "0303"}
	// b-cv-01 在 0208。无论障碍如何，路径长度都不会小于直线距离。
	path := rtbDistance(state, aircraft)
	straight := Distance("0303", state.Units["b-cv-01"].Coord)
	if path < straight {
		t.Fatalf("路径距离不应小于直线距离：路径%d 直线%d", path, straight)
	}
	if path > straight+4 {
		t.Fatalf("路径距离异常偏大：路径%d 直线%d", path, straight)
	}
}

// 剩余航程 == 返航距离时就应该判定为需要返航（规则书 618 行"小于或等于"）。
func TestShouldRTBAtExactDistance(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "probe", Faction: "B", CarrierID: "b-cv-01", Coord: "0202"}
	need := rtbDistance(state, aircraft)
	aircraft.SetRange(need + 1)
	if shouldRTB(state, aircraft) {
		t.Fatal("航程充足时不应返航")
	}
	aircraft.SetRange(need)
	if !shouldRTB(state, aircraft) {
		t.Fatal("剩余航程等于返航距离时必须返航")
	}
	aircraft.SetRange(need - 1)
	if !shouldRTB(state, aircraft) {
		t.Fatal("剩余航程不足时必须返航")
	}
}

// 没有返航目的地的编队不做阈值判定。
func TestShouldRTBSkipsWithoutDestination(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{ID: "orphan", Faction: "B", Coord: "0202", RemainingRange: 1}
	if shouldRTB(state, aircraft) {
		t.Fatal("没有母舰也没有起飞点时不应当判定返航")
	}
}

// 搜索方向：正前 + 两个斜前。
func TestSearchHeadingsAreForwardAndTwoFlanks(t *testing.T) {
	headings := searchHeadings("E")
	if len(headings) != 3 || headings[0] != "E" {
		t.Fatalf("搜索方向错误：%v", headings)
	}
	set := map[string]bool{}
	for _, heading := range headings {
		set[heading] = true
	}
	if !set["SE"] || !set["NE"] {
		t.Fatalf("两个斜前方应当包含 NE 和 SE：%v", headings)
	}
}

// 接敌失败后编队会移动到相邻格，并且仍然没有接敌。
func TestFailedEngageSearchesAndMoves(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "search", Faction: "B", Model: "D3A", Kind: "dive_bomber", Status: "attack",
		Coord: "0404", OriginCoord: "0208", TargetCoord: "0404", Heading: "W",
		RemainingRange: 12, MaxRange: 12,
	}
	state.Aircraft[aircraft.ID] = aircraft
	before := aircraft.Coord
	events := searchAfterFailedEngage(&state, aircraft)
	if aircraft.Coord == before {
		t.Fatalf("搜索失败后应当移动：%+v", aircraft)
	}
	found := false
	for _, event := range events {
		if event.Type == "attack_searching_move" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应当产生搜索移动事件：%+v", events)
	}
	if aircraft.Engaged {
		t.Fatal("搜索移动本身不应判定接敌成功")
	}
}

// 搜索位移会消耗航程，且航程不足时不移动、直接返航。
func TestSearchRespectsRTBThreshold(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "search2", Faction: "B", CarrierID: "b-cv-01", Model: "D3A",
		Coord: "0404", OriginCoord: "0208", TargetCoord: "0404", Heading: "W",
		RemainingRange: 1,
	}
	state.Aircraft[aircraft.ID] = aircraft
	searchAfterFailedEngage(&state, aircraft)
	if !aircraft.Returning {
		t.Fatal("航程不足以返航时搜索编队应当直接返航")
	}
	if aircraft.Coord != "0404" {
		t.Fatal("航程不足时不应继续搜索移动")
	}
}

// 已经停在目标 hex 的编队：本回合不再推进，也不重复扣航程，只做接敌/搜索。
func TestOnTargetFormationDoesNotDoubleConsume(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "ontarget", Faction: "B", CarrierID: "b-cv-01", Model: "D3A", Kind: "dive_bomber",
		Status: "attack", TaskID: "task-x", Coord: "0404", OriginCoord: "0208", TargetCoord: "0404",
		Heading: "W", RemainingRange: 8, MaxRange: 12,
	}
	state.Aircraft[aircraft.ID] = aircraft
	before := aircraft.Range()
	advanceAttack(&state, aircraft)
	consumed := before - aircraft.Range()
	// 搜索移动最多消耗 1 点；不应该出现「推进 2 格 + 搜索 1 格」的重复消耗。
	if consumed > 1 {
		t.Fatalf("目标上空只应消耗搜索里程，实际消耗%d", consumed)
	}
}

// 同一编队一回合里只做一次接敌判定。
func TestEngagementTriedOncePerTurn(t *testing.T) {
	state := sandboxState(t)
	state.EngageTried = map[string]bool{}
	aircraft := &AirUnit{ID: "e1", Faction: "B", TaskID: "task-e", Coord: "0404", TargetCoord: "0404", RemainingRange: 8}
	state.Aircraft[aircraft.ID] = aircraft
	if engagedThisTurn(&state, aircraft) {
		t.Fatal("新回合不应标记为已判定")
	}
	tryEngage(&state, aircraft, "0404")
	if !engagedThisTurn(&state, aircraft) {
		t.Fatal("判定之后应当标记本回合已判定")
	}
}
