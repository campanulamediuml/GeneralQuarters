package game

import (
	"math"
	"sort"
	"strings"
)

// —— 动态天气（规则书 313–314、512、610、638–645、701–702、717–722 行）——
//
// 天气不再是"每格自己随机游走"，而是真实的云层：地图上每个 hex 记 0–3 层云，
//   0 层 = 晴，1 层 = 多云，2 层 = 降雨，3 层 = 风暴。
// 初始按 70% / 20% / 10% 三层独立撒点（每个 hex 每层各自掷一次，最多叠三层）。
// 之后每 3 回合所有云一起移动一格：75% 顺着当地风（吹向），25% 随机方向；
// 云叠在一起就变成雨、再叠出第三层就是风暴。移动用的是哈希出来的伪随机数，
// 不消耗 RNGState——天气不应该影响炮击/命中的随机序列。

// WeatherKind 是三种会影响判定的天气。
const (
	WeatherClear = ""
	WeatherCloud = "cloud"
	WeatherRain  = "rain"
	WeatherStorm = "storm"
)

const (
	weatherMoveTurns = 3 // 云层每 3 回合移动一次
	weatherMaxLayers = 3 // 三层 = 风暴
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
	if state.WeatherTurns <= 0 || state.Map.Clouds != "" {
		return
	}
	rows, cols := state.Map.Rows, state.Map.Cols
	if rows <= 0 || cols <= 0 {
		return
	}
	cells := make([]byte, rows*cols)
	for index := range cells {
		layers := 0
		if cloudRoll(state.RNGState, 0, index, 0)%100 < 70 {
			layers++
		}
		if cloudRoll(state.RNGState, 0, index, 1)%100 < 20 {
			layers++
		}
		if cloudRoll(state.RNGState, 0, index, 2)%100 < 10 {
			layers++
		}
		if layers > weatherMaxLayers {
			layers = weatherMaxLayers
		}
		cells[index] = byte('0' + layers)
	}
	state.Map.Clouds = string(cells)
}

// weatherFromLayers 把云层数变成规则用的天气名。
func weatherFromLayers(layers byte) string {
	switch layers {
	case '1':
		return WeatherCloud
	case '2':
		return WeatherRain
	case '3':
		return WeatherStorm
	}
	return WeatherClear
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
// 目标格已经叠满 3 层时这层云消散（很少发生，但避免无限堆叠）。
func moveClouds(state *State) int {
	rows, cols := state.Map.Rows, state.Map.Cols
	if rows <= 0 || cols <= 0 || len(state.Map.Clouds) != rows*cols {
		return 0
	}
	next := make([]byte, rows*cols)
	for index := range next {
		next[index] = '0'
	}
	moved := 0
	turn := uint64(state.Turn)
	for row := 1; row <= rows; row++ {
		for col := 1; col <= cols; col++ {
			index := (row-1)*cols + (col - 1)
			layers := int(state.Map.Clouds[index] - '0')
			for layer := 0; layer < layers; layer++ {
				target := cloudTarget(state, row, col, index, layer)
				if target >= 0 && next[target] < '0'+weatherMaxLayers {
					next[target]++
					moved++
					continue
				}
				// 没有可去的地方（出界 / 目标满）：留在原地。
				if next[index] < '0'+weatherMaxLayers {
					next[index]++
				}
			}
		}
	}
	state.Map.Clouds = string(next)
	_ = turn
	return moved
}

// cloudTarget 选这层云要挪到哪个相邻格：75% 顺风、25% 随机。
// 返回目标格的行优先下标；-1 表示没有可去的地方。
func cloudTarget(state *State, row, col, index, layer int) int {
	coord := Coord(row, col)
	neighbors := Neighbors(coord)
	valid := make([]int, 0, len(neighbors))
	for _, neighbor := range neighbors {
		nrow, ncol, err := ParseCoord(neighbor)
		if err != nil || nrow < 1 || nrow > state.Map.Rows || ncol < 1 || ncol > state.Map.Cols {
			continue
		}
		valid = append(valid, (nrow-1)*state.Map.Cols+(ncol-1))
	}
	if len(valid) == 0 {
		return -1
	}
	roll := cloudRoll(state.RNGState, uint64(state.Turn), index, layer)
	if roll%100 < 75 {
		// 顺着当地风：先取来向，再算吹向，然后挑最接近这个方位的邻格。
		blow := 0.0
		if name, ok := state.WindDirectionAt(coord); ok {
			if from, okFrom := compassDegrees(name); okFrom {
				blow = float64((from + 180) % 360)
			}
		}
		x, y := hexCenter(row, col)
		best := valid[0]
		bestDiff := 360.0
		for _, candidate := range valid {
			crow := candidate/state.Map.Cols + 1
			ccol := candidate%state.Map.Cols + 1
			nx, ny := hexCenter(crow, ccol)
			bearing := math.Atan2(nx-x, -(ny-y)) * 180 / math.Pi
			for bearing < 0 {
				bearing += 360
			}
			diff := math.Abs(bearing - blow)
			if diff > 180 {
				diff = 360 - diff
			}
			if diff < bestDiff {
				bestDiff, best = diff, candidate
			}
		}
		return best
	}
	return valid[int((roll>>8)%uint64(len(valid)))]
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
