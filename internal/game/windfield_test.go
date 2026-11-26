package game

import (
	"encoding/json"
	"testing"
)

// 每 hex 风向必须是连续的：相邻格最多差一档（60°）。
// 这条比"看起来顺眼"重要——甲板修正按 60° 一档，相邻格跨两档就等于凭空换了风。
func TestWindFieldIsSmoothAcrossHexes(t *testing.T) {
	field := newWindField("SE", 7)
	if field == nil {
		t.Fatal("有盛行风就应该生成风场")
	}
	mapDef := MapDefinition{Rows: 52, Cols: 64}
	index := func(coord string) int {
		for position, candidate := range windDirections {
			if candidate == field.DirectionAt(coord) {
				return position
			}
		}
		return -1
	}
	seen := map[string]bool{}
	for row := 1; row <= mapDef.Rows; row++ {
		for col := 1; col <= mapDef.Cols; col++ {
			coord := Coord(row, col)
			own := index(coord)
			seen[windDirections[own]] = true
			if own < 0 {
				t.Fatalf("%s 的风向不在六方位里", coord)
			}
			for _, neighbor := range Neighbors(coord) {
				nrow, ncol, err := ParseCoord(neighbor)
				if err != nil || nrow < 1 || nrow > mapDef.Rows || ncol < 1 || ncol > mapDef.Cols {
					continue
				}
				other := index(neighbor)
				diff := (own - other + 6) % 6
				if diff > 3 {
					diff = 6 - diff
				}
				if diff > 1 {
					t.Fatalf("%s(%s) 与 %s(%s) 风向差了两档以上：%d", coord, windDirections[own], neighbor, windDirections[other], diff)
				}
			}
		}
	}
	if len(seen) < 2 {
		t.Fatalf("整张图只有一个风向，风场没有空间变化：%v", seen)
	}
}

// 天气推进不能让任何一个格子跨两档：盛行风 ±2°、每层相位 ±2°。
func TestWindFieldStepStaysSmooth(t *testing.T) {
	field := newWindField("NE", 11)
	rng := uint64(2024)
	mapDef := MapDefinition{Rows: 52, Cols: 64}
	index := func(coord string) int {
		for position, candidate := range windDirections {
			if candidate == field.DirectionAt(coord) {
				return position
			}
		}
		return -1
	}
	coords := make([]string, 0, mapDef.Rows*mapDef.Cols)
	for row := 1; row <= mapDef.Rows; row++ {
		for col := 1; col <= mapDef.Cols; col++ {
			coords = append(coords, Coord(row, col))
		}
	}
	before := make([]int, len(coords))
	for position, coord := range coords {
		before[position] = index(coord)
	}
	for step := 0; step < 40; step++ {
		field.Step(&rng)
		for position, coord := range coords {
			after := index(coord)
			diff := (after - before[position] + 6) % 6
			if diff > 3 {
				diff = 6 - diff
			}
			if diff > 1 {
				t.Fatalf("第%d次推进后 %s 从 %s 跳到 %s（跨了 %d 档）", step+1, coord, windDirections[before[position]], windDirections[after], diff)
			}
			before[position] = after
		}
	}
}

// 同种子同场、JSON 往返不丢参数：存档读回来每个格子的风必须一致。
func TestWindFieldIsDeterministicAndSerializable(t *testing.T) {
	first := newWindField("NW", 99)
	second := newWindField("NW", 99)
	for row := 1; row <= 12; row++ {
		for col := 1; col <= 12; col++ {
			coord := Coord(row, col)
			if first.DegreesAt(coord) != second.DegreesAt(coord) {
				t.Fatalf("%s 同种子生成的风场不一致", coord)
			}
		}
	}
	raw, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var restored WindField
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for row := 1; row <= 12; row++ {
		for col := 1; col <= 12; col++ {
			coord := Coord(row, col)
			if first.DegreesAt(coord) != restored.DegreesAt(coord) {
				t.Fatalf("%s 序列化往返后风向变了", coord)
			}
		}
	}
}

// 甲板修正必须按船所在格的风算：同一艘航母放在两个不同风区、同一朝向，
// 修正可以不一样。
func TestDeckModifierUsesLocalWind(t *testing.T) {
	state := State{WindField: newWindField("SE", 5), Map: MapDefinition{Rows: 52, Cols: 64}}
	state.Units = map[string]*Unit{}
	// 找一对"同一朝向、修正不同"的格子。
	heading, first, second := "", "", ""
	for _, candidate := range windDirections {
		for row := 1; row <= 52 && second == ""; row++ {
			for col := 1; col <= 64; col++ {
				coord := Coord(row, col)
				modifier := windDeckModifier(state.WindField.DirectionAt(coord), candidate)
				if first == "" {
					first = coord
					continue
				}
				if modifier != windDeckModifier(state.WindField.DirectionAt(first), candidate) {
					second = coord
					break
				}
			}
		}
		if second != "" {
			heading = candidate
			break
		}
	}
	if second == "" {
		t.Fatal("风场里找不到两个修正不同的格子")
	}
	unitA := &Unit{ID: "a1", Faction: "A", Type: "CV", Coord: first, Heading: heading, Deck: &FlightDeck{BaseLength: 6}}
	unitB := &Unit{ID: "a2", Faction: "A", Type: "CV", Coord: second, Heading: heading, Deck: &FlightDeck{BaseLength: 6}}
	state.Units[unitA.ID] = unitA
	state.Units[unitB.ID] = unitB
	if got, want := deckLengthFor(state, unitA), 6+windDeckModifier(state.WindField.DirectionAt(first), heading); got != want {
		t.Fatalf("%s 甲板修正没有按本地风算：%d（期望%d）", first, got, want)
	}
	if got, want := deckLengthFor(state, unitB), 6+windDeckModifier(state.WindField.DirectionAt(second), heading); got != want {
		t.Fatalf("%s 甲板修正没有按本地风算：%d（期望%d）", second, got, want)
	}
	if unitA.Deck.BaseLength != 6 {
		t.Fatal("基础甲板长度被改写")
	}
}

// 视图里的逐格风向表：行优先、每格一个字符、下标和 windDirections 对得上。
func TestWindCellsMatchesDirections(t *testing.T) {
	state := State{WindField: newWindField("E", 3), Map: MapDefinition{Rows: 5, Cols: 7}}
	cells := WindCells(state)
	if len(cells) != 5*7 {
		t.Fatalf("逐格风向表长度错误：%d", len(cells))
	}
	for row := 1; row <= 5; row++ {
		for col := 1; col <= 7; col++ {
			char := cells[(row-1)*7+(col-1)]
			if char < '0' || char > '5' {
				t.Fatalf("非法风向字符 %q", char)
			}
			if want := state.WindField.DirectionAt(Coord(row, col)); windDirections[int(char-'0')] != want {
				t.Fatalf("%s 的风向表是 %s，单点查是 %s", Coord(row, col), windDirections[int(char-'0')], want)
			}
		}
	}
}

// 没有风场的剧本仍然是"无风、无甲板修正"。
func TestNoWindFieldMeansNoModifier(t *testing.T) {
	state := State{Wind: "", Map: MapDefinition{Rows: 4, Cols: 4}}
	if _, ok := state.WindDirectionAt("0101"); ok {
		t.Fatal("没设风向的剧本不应该有本地风")
	}
	if WindCells(state) != "" {
		t.Fatal("没有风场时逐格风向表应为空")
	}
}

// 风向场按 6–12 回合的节奏推进：不是每回合都动，也不会一直不动。
func TestWindStepCadence(t *testing.T) {
	state := sandboxState(t)
	if state.WindField == nil {
		t.Fatal("沙盒剧本应该有风场")
	}
	next := state.WindNextTurn
	if next < 7 || next > 13 {
		t.Fatalf("首次推进回合应在 7–13（1 + 6~12），实际 %d", next)
	}
	before := state.WindField.DegreesAt("0101")
	for turn := 1; turn < next; turn++ {
		state.Turn = turn
		if events := StepWind(&state); len(events) > 0 {
			t.Fatalf("第 %d 回合不该推进风场（下次 %d）", turn, next)
		}
	}
	if got := state.WindField.DegreesAt("0101"); got != before {
		t.Fatalf("还没到推进回合，风向就变了：%d -> %d", before, got)
	}
	state.Turn = next
	StepWind(&state)
	if got := state.WindField.DegreesAt("0101"); got == before {
		t.Fatalf("第 %d 回合应该推进风场", next)
	}
	if state.WindNextTurn <= state.Turn || state.WindNextTurn > state.Turn+12 {
		t.Fatalf("下一次推进回合不对：%d（当前 %d）", state.WindNextTurn, state.Turn)
	}
}
