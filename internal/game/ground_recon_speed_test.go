package game

import "testing"

func reconPlane(state *State, id string) *AirfieldAirUnit {
	aircraft := state.GroundAir[id]
	base := airfieldFor(state, aircraft.AirfieldID)
	aircraft.Coord = base.Coord
	aircraft.Status = "recon"
	aircraft.TaskKind = "recon"
	aircraft.OriginCoord = aircraft.Coord
	// 用 SE 航向：拉包尔在 0224，往 NE 飞一格就到地图北边界（row 0 不是合法 hex）。
	aircraft.Heading = "SE"
	aircraft.SectorWidth = 60
	aircraft.ReconRadius = 20
	aircraft.Returning = false
	return aircraft
}

// 规则书 200 行：水上飞机执行任务每回合移动 1 hex。岸基搜索以前写死 2 格，
// 飞行艇实际比规则快一倍，这局日军的远距搜索就是这么排出来的。
func TestGroundReconSeaplaneMovesOneHex(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	h6k := reconPlane(&state, "af-b-shortland-h6k-1")
	start := h6k.Coord
	advanceGroundRecon(&state, h6k)
	if got := Distance(start, h6k.Coord); got != 1 {
		t.Fatalf("飞行艇每回合应只前进 1 hex，实际 %d", got)
	}
}

// 两格速度的飞机一回合走两步：当前圈 + 下一圈不能重复判定，否则中间那圈被掷两次骰。
func TestReconRingsToScanDedupesWithinTurn(t *testing.T) {
	scanned := map[int]bool{}
	first := reconRingsToScan(3, 20, scanned)
	if len(first) != 2 || first[0] != 3 || first[1] != 4 {
		t.Fatalf("第一步应判定第 3、4 圈，实际 %v", first)
	}
	second := reconRingsToScan(4, 20, scanned)
	if len(second) != 1 || second[0] != 5 {
		t.Fatalf("第二步只该补判第 5 圈，实际 %v", second)
	}
	if third := reconRingsToScan(5, 20, scanned); len(third) != 1 || third[0] != 6 {
		t.Fatalf("第三步应判定第 6 圈，实际 %v", third)
	}
}

func TestGroundReconBomberMovesTwoHex(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	g4m := reconPlane(&state, "af-b-rabaul-g4m-1")
	start := g4m.Coord
	advanceGroundRecon(&state, g4m)
	if got := Distance(start, g4m.Coord); got != 2 {
		t.Fatalf("陆攻每回合应前进 2 hex，实际 %d", got)
	}
}
