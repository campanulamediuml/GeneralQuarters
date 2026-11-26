package game

import (
	"math"
	"sort"
	"strings"
)

// —— 动态天气（规则书 313–314、512、610、638–645、701–702、717–722 行）——
//
// 天气不再是"每格自己随机游走"，而是真实的云层：地图上每个 hex 记压了几层云，
//   0 层 = 晴，1 层 = 多云，2 层 = 降雨，3 层及以上 = 风暴。
// 初始撒点：10% 的 hex 是多云（1 层）、5% 是降雨（2 层）、1% 是风暴（3 层），
// 其余 84% 晴朗。开局只铺 1–3 层，是为了顺手造出四种天气；之后云完全自己飘、自己叠，
// 一层一层独立运动，叠到 4 层、5 层也还是风暴，不会因为"目标格已经有云"就消散。
// 每 3 回合所有云一起移动一格：75% 顺着当地风（吹向），25% 随机方向。
// 移动用的是哈希出来的伪随机数，不消耗 RNGState——天气不应该影响炮击/命中的随机序列。
//
// 地图按环绕处理：云从一边飘出去，就从对面那一列/一行进来。以前只在图内挑邻格，
// 边上的云无处可去只能留在原地，看着就像"云卡在边上慢慢聚成一坨"。

// WeatherKind 是三种会影响判定的天气。
const (
	WeatherClear = ""
	WeatherCloud = "cloud"
	WeatherRain  = "rain"
	WeatherStorm = "storm"
)

const (
	weatherMoveTurns = 3 // 云层每 3 回合移动一次
	// weatherStormLayers 是"到这一层就是风暴"的门槛：3 层、4 层、10 层都是风暴。
	weatherStormLayers = 3
	// cloudLayersMax 只是记账上限，不是天气上限：127 层以上不会再有新档位，
	// 而一个字节再往上写就成了非 ASCII，JSON 编码会把它替换成 U+FFFD。
	cloudLayersMax = 127
)

// windDirections 是六个方位（NE/E/SE/SW/W/NW），风向与舰艏关系的唯一定义来源。
var windDirections = []string{"NE", "E", "SE", "SW", "W", "NW"}

// wealthCloudSeed 把（房间种子、回合、格子、第几层云）混成一个稳定的伪随机数。
func cloudRoll(seed, turn uint64, index, layer int) uint64 {
	mixed := seed ^ (turn * 0x9e3779b97f4a7c15) ^ (uint64(index) * 0xbf58476d1ce4e5b9) ^ (uint64(layer+1) * 0x94d049bb133111eb)
	return splitmix64(mixed)
}

// EnsureWeather 在载入房间/新建房间时铺一次云。
// 只在天气系统开启（WeatherTurns > 0）且还没有天气层时生效；老存档里
// MapCell.Weather 的静态天气保留为回退，不会被覆盖。
func EnsureWeather(state *State) {
	if state.WeatherTurns <= 0 {
		return
	}
	normalizeCloudLayers(state)
	if state.Map.Clouds != "" {
		return
	}
	rows, cols := state.Map.Rows, state.Map.Cols
	if rows <= 0 || cols <= 0 {
		return
	}
	cells := make([]byte, rows*cols)
	for index := range cells {
		// 一次掷骰定档：1% 风暴、5% 降雨、10% 多云，剩下 84% 晴。
		// 档位直接对应云层数，所以下雨/风暴的格子照样是"叠了三层云"，会被风一起吹走。
		roll := cloudRoll(state.RNGState, 0, index, 0) % 100
		layers := 0
		switch {
		case roll < 1:
			layers = 3
		case roll < 6:
			layers = 2
		case roll < 16:
			layers = 1
		}
		cells[index] = byte(layers)
	}
	state.Map.Clouds = string(cells)
}

// normalizeCloudLayers 把旧存档的天气层迁成"每格一个字节的层数"。
// 旧编码每格是一个字符 '0'..'3'；不迁的话读出来每格都是 48–51 层，整张图直接变风暴。
func normalizeCloudLayers(state *State) {
	cells := state.Map.Clouds
	if len(cells) == 0 || len(cells) != state.Map.Rows*state.Map.Cols {
		return
	}
	for index := 0; index < len(cells); index++ {
		// 只有整串都落在 '0'..'3' 才当成旧编码。新编码里层数要凑到 48 才会撞上这个区间，
		// 那时这格早就是风暴，误判也不影响天气。
		if cells[index] < '0' || cells[index] > '3' {
			return
		}
	}
	layers := make([]byte, len(cells))
	for index := 0; index < len(cells); index++ {
		layers[index] = cells[index] - '0'
	}
	state.Map.Clouds = string(layers)
}

// CloudCells 是给前端/观战页的天气档位串：每格一个字符 '0'..'3'。
// 服务端内部记的是层数（4 层、5 层…），到了界面上一律按"3 = 风暴"显示，
// 因为规则里只看这三档，叠到第几层已经没有区别。
func CloudCells(gameMap MapDefinition) string {
	if len(gameMap.Clouds) == 0 {
		return ""
	}
	cells := make([]byte, len(gameMap.Clouds))
	for index := 0; index < len(gameMap.Clouds); index++ {
		layers := gameMap.Clouds[index]
		if layers > weatherStormLayers {
			layers = weatherStormLayers
		}
		cells[index] = '0' + layers
	}
	return string(cells)
}

// weatherFromLayers 把云层数变成规则用的天气名：3 层及以上都是风暴。
func weatherFromLayers(layers byte) string {
	switch {
	case layers <= 0:
		return WeatherClear
	case layers == 1:
		return WeatherCloud
	case layers == 2:
		return WeatherRain
	}
	return WeatherStorm
}

// weatherRank 用于和旧代码/测试兼容：晴 0 / 多云 1 / 降雨 2 / 风暴 3。
func weatherRank(weather string) int {
	switch weather {
	case WeatherCloud:
		return 1
	case WeatherRain:
		return 2
	case WeatherStorm:
		return 3
	}
	return 0
}

// moveClouds 把云层整体挪一格，返回移动了几个云层。
// 每一层单独找目标：叠到同一格就叠着，4 层、5 层都只是风暴更厚，不会消散。
func moveClouds(state *State) int {
	rows, cols := state.Map.Rows, state.Map.Cols
	if rows <= 0 || cols <= 0 || len(state.Map.Clouds) != rows*cols {
		return 0
	}
	next := make([]byte, rows*cols)
	moved := 0
	for row := 1; row <= rows; row++ {
		for col := 1; col <= cols; col++ {
			index := (row-1)*cols + (col - 1)
			layers := int(state.Map.Clouds[index])
			for layer := 0; layer < layers; layer++ {
				target := cloudTarget(state, row, col, index, layer)
				// 只有记账上限会让一层云停下（127 层，早就风暴了）：这时它就留在原地，
				// 不丢。目标格能不能再来一层，跟它是不是风暴无关。
				if target >= 0 && next[target] < cloudLayersMax {
					next[target]++
					moved++
					continue
				}
				if next[index] < cloudLayersMax {
					next[index]++
				}
			}
		}
	}
	state.Map.Clouds = string(next)
	return moved
}

// cloudNeighborCells 返回六个邻格的行列号，其中可能已经有格飘到地图外：
// 错位规则和 Neighbors 一致（偶数行左上/左下，奇数行右上/右下）。
// 这里刻意不裁掉图外格——云先按真实方向往外飘，再由 wrapCell 折回图内。
func cloudNeighborCells(row, col int) [][2]int {
	deltas := [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	if row%2 == 0 {
		deltas = append(deltas, [2]int{-1, -1}, [2]int{1, -1})
	} else {
		deltas = append(deltas, [2]int{-1, 1}, [2]int{1, 1})
	}
	cells := make([][2]int, 0, len(deltas))
	for _, delta := range deltas {
		cells = append(cells, [2]int{row + delta[0], col + delta[1]})
	}
	return cells
}

// wrapCell 把飘出地图的一格折回图内：从一边出去，就从对面进来。
// 行列各自按 1..N 循环，所以斜着飘出去会同时换行和换列。
func wrapCell(row, col, rows, cols int) (int, int) {
	wrap := func(value, size int) int {
		if size <= 0 {
			return value
		}
		// Go 的取模跟着被除数走号，负一格要自己补回去。1..size 原样返回。
		mod := value % size
		if mod <= 0 {
			mod += size
		}
		return mod
	}
	return wrap(row, rows), wrap(col, cols)
}

// cloudTarget 选这层云要挪到哪个相邻格：75% 顺风、25% 随机。
// 目标先按"地图无限大"算出来，再环绕折算回图内；返回目标格的行优先下标，
// -1 表示地图尺寸不合法、连一个邻格都算不出来。
func cloudTarget(state *State, row, col, index, layer int) int {
	rows, cols := state.Map.Rows, state.Map.Cols
	type candidate struct {
		row, col int // 没折回的坐标：算方位用它，方向才是真的
		index    int // 折回后的下标：云最终落在这里
	}
	candidates := make([]candidate, 0, 6)
	seen := make(map[int]bool, 6)
	for _, cell := range cloudNeighborCells(row, col) {
		wrappedRow, wrappedCol := wrapCell(cell[0], cell[1], rows, cols)
		target := (wrappedRow-1)*cols + (wrappedCol - 1)
		// 小地图/角落上两个方向可能折回同一格，去重，免得那一格权重翻倍。
		if seen[target] {
			continue
		}
		seen[target] = true
		candidates = append(candidates, candidate{row: cell[0], col: cell[1], index: target})
	}
	if len(candidates) == 0 {
		return -1
	}
	roll := cloudRoll(state.RNGState, uint64(state.Turn), index, layer)
	if roll%100 < 75 {
		// 顺着当地风：先取来向，再算吹向，然后挑最接近这个方位的邻格。
		// 方位必须按没折回的坐标算：东边要飘出去的云，方向仍然是东，
		// 折回后它落在西边——要是拿折回后的位置算方位，它会掉头往西飘。
		blow := 0.0
		if name, ok := state.WindDirectionAt(Coord(row, col)); ok {
			if from, okFrom := compassDegrees(name); okFrom {
				blow = float64((from + 180) % 360)
			}
		}
		x, y := hexCenter(row, col)
		best := candidates[0].index
		bestDiff := 360.0
		for _, candidate := range candidates {
			nx, ny := hexCenter(candidate.row, candidate.col)
			bearing := math.Atan2(nx-x, -(ny-y)) * 180 / math.Pi
			for bearing < 0 {
				bearing += 360
			}
			diff := math.Abs(bearing - blow)
			if diff > 180 {
				diff = 360 - diff
			}
			if diff < bestDiff {
				bestDiff, best = diff, candidate.index
			}
		}
		return best
	}
	return candidates[int((roll>>8)%uint64(len(candidates)))].index
}

// StepWeather 在需要时推进一次天气。返回产生的事件。
// 云层移动不写战报：地图上的云/雨/风暴就是提示，每 3 回合刷一条只会淹没真正的消息。
func StepWeather(state *State) []EngineEvent {
	if state.WeatherTurns <= 0 {
		return nil
	}
	EnsureWeather(state)
	if state.Map.Clouds == "" || state.Turn%weatherMoveTurns != 0 {
		return nil
	}
	moveClouds(state)
	return nil
}

// weatherAt 查一个 hex 的天气：优先用云层，老存档退回静态天气格。
func weatherAt(gameMap MapDefinition, coord string) string {
	if gameMap.Clouds != "" {
		if row, col, err := ParseCoord(coord); err == nil && row >= 1 && row <= gameMap.Rows && col >= 1 && col <= gameMap.Cols {
			if index := (row-1)*gameMap.Cols + (col - 1); index < len(gameMap.Clouds) {
				return weatherFromLayers(gameMap.Clouds[index])
			}
		}
	}
	for _, cell := range gameMap.Cells {
		if cell.Coord == coord {
			return strings.ToLower(cell.Weather)
		}
	}
	return ""
}

// VisibleWeather 返回所有非晴朗格的天气（调试 / 兼容旧接口）。
func VisibleWeather(state State) map[string]string {
	result := map[string]string{}
	if state.Map.Clouds != "" {
		for row := 1; row <= state.Map.Rows; row++ {
			for col := 1; col <= state.Map.Cols; col++ {
				coord := Coord(row, col)
				if weather := weatherAt(state.Map, coord); weather != WeatherClear {
					result[coord] = weather
				}
			}
		}
		return result
	}
	coords := make([]string, 0, len(state.Map.Cells))
	for _, cell := range state.Map.Cells {
		if cell.Weather != "" {
			coords = append(coords, cell.Coord)
		}
	}
	sort.Strings(coords)
	for _, coord := range coords {
		result[coord] = weatherAt(state.Map, coord)
	}
	return result
}
