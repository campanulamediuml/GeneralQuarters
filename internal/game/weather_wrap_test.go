package game

import "testing"

// 云飘出地图后从对面进来。旧实现只在图内挑邻格：边上的云无处可去只能留在原地，
// 于是所有靠边的云都往边缘挤，"飘到莫名其妙的位置"就是这么来的。
func TestWrapCellWrapsAcrossEdges(t *testing.T) {
	cases := []struct{ row, col, rows, cols, wantRow, wantCol int }{
		{0, 3, 6, 6, 6, 3},  // 从北边飘出去 → 南边同列
		{7, 3, 6, 6, 1, 3},  // 从南边飘出去 → 北边同列
		{3, 7, 6, 6, 3, 1},  // 从东边飘出去 → 西边同行
		{3, 0, 6, 6, 3, 6},  // 从西边飘出去 → 东边同行
		{0, 7, 6, 6, 6, 1},  // 角落斜着飘出去，行列一起折
		{6, 6, 6, 6, 6, 6},  // 图内坐标原样返回
		{1, 1, 6, 6, 1, 1},
	}
	for _, tc := range cases {
		row, col := wrapCell(tc.row, tc.col, tc.rows, tc.cols)
		if row != tc.wantRow || col != tc.wantCol {
			t.Fatalf("wrapCell(%d,%d) = (%d,%d)，应为 (%d,%d)", tc.row, tc.col, row, col, tc.wantRow, tc.wantCol)
		}
	}
}

// 边上的每一格，无论风向怎么吹，目标都必须是图内、且是折回后的相邻格；
// 以前图外的方向直接返回 -1，云就留在原地。
func TestCloudTargetOnEdgeStaysInsideMap(t *testing.T) {
	const rows, cols = 52, 64
	state := State{Map: MapDefinition{Rows: rows, Cols: cols}, WindField: newWindField("SE", 3)}
	edges := [][2]int{{1, 1}, {1, 32}, {1, 64}, {2, 1}, {2, 64}, {26, 1}, {26, 64}, {52, 1}, {52, 32}, {52, 64}}
	for seed := uint64(1); seed <= 120; seed++ {
		state.RNGState = seed
		for _, cell := range edges {
			index := (cell[0]-1)*cols + cell[1] - 1
			for turn := 0; turn < 20; turn++ {
				state.Turn = turn * weatherMoveTurns
				target := cloudTarget(&state, cell[0], cell[1], index, turn%weatherStormLayers)
				if target < 0 || target >= rows*cols {
					t.Fatalf("seed=%d 回合%d 的云落到了图外：%d", seed, state.Turn, target)
				}
				if target == index {
					t.Fatalf("seed=%d 回合%d 的云没有移动，还停在 %s", seed, state.Turn, Coord(cell[0], cell[1]))
				}
				adjacent := false
				for _, neighbor := range cloudNeighborCells(cell[0], cell[1]) {
					wrappedRow, wrappedCol := wrapCell(neighbor[0], neighbor[1], rows, cols)
					if (wrappedRow-1)*cols+(wrappedCol-1) == target {
						adjacent = true
						break
					}
				}
				if !adjacent {
					t.Fatalf("seed=%d 回合%d：%s 的云跳到了不相邻的 %s", seed, state.Turn, Coord(cell[0], cell[1]), Coord(target/cols+1, target%cols+1))
				}
			}
		}
	}
}

// 盛行风从西边吹来时，最东边一列的云大部分会绕到最西边一列，
// 而不是顺着边缘往南/北滑。随机分支占 25%，所以门槛放低到六成。
func TestEastEdgeCloudWrapsToWestColumn(t *testing.T) {
	const rows, cols = 52, 64
	state := State{Map: MapDefinition{Rows: rows, Cols: cols}, WindField: newWindField("W", 11)}
	const total = 400
	crossed := 0
	for seed := uint64(1); seed <= total; seed++ {
		state.RNGState = seed
		state.Turn = int(seed * weatherMoveTurns)
		index := (26-1)*cols + (cols - 1)
		target := cloudTarget(&state, 26, cols, index, 0)
		if target%cols == 0 {
			crossed++
		}
	}
	if crossed < total*6/10 {
		t.Fatalf("只有 %d/%d 层云从东边绕到了西边，环绕没生效", crossed, total)
	}
}
