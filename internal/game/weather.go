package game

import (
	"fmt"
	"sort"
)

// —— 动态天气与风向（规则书 313–314、512、610、638–645、701–702、717–722、815–825 行）——
//
// 剧本可以开启天气系统（TurnsPerDay > 0 时生效）：每 WeatherTurns 回合重新掷一次天气，
// 按地图上已有的天气格做一次平滑扩散——相邻格子的天气会互相影响，风向也会随机转向。
//
// 剧本没有配置 WeatherTurns 时保持静态：地图里的天气格与剧本风向一概不变。

// WeatherKind 是三种会影响判定的天气。
const (
	WeatherClear = ""
	WeatherCloud = "cloud"
	WeatherRain  = "rain"
	WeatherStorm = "storm"
)

var weatherKinds = []string{WeatherClear, WeatherCloud, WeatherRain, WeatherStorm}

// windDirections 是八方位风向，与 Chapter 风向 的四档关系一致。
var windDirections = []string{"NE", "E", "SE", "SW", "W", "NW"}

// weatherRank 用于扩散：风暴比降雨更"重"，会压过相邻的多云。
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

// StepWeather 在需要时推进一次天气变化。返回产生的事件。
func StepWeather(state *State) []EngineEvent {
	if state.WeatherTurns <= 0 || state.Turn%state.WeatherTurns != 0 {
		return nil
	}
	events := []EngineEvent{}
	if state.Map.Cells == nil {
		return nil
	}
	// 1) 先按当前格子掷一次基础天气。
	next := map[string]string{}
	for _, cell := range state.Map.Cells {
		next[cell.Coord] = stepCellWeather(&state.RNGState, cell.Weather)
	}
	// 2) 再让相邻格互相扩散：取邻居里最"重"的天气和自身平均。
	for index := range state.Map.Cells {
		cell := &state.Map.Cells[index]
		own := weatherRank(next[cell.Coord])
		neighborSum := 0
		neighborCount := 0
		for _, coord := range Neighbors(cell.Coord) {
			if rank, ok := next[coord]; ok {
				neighborSum += weatherRank(rank)
				neighborCount++
			}
		}
		if neighborCount == 0 {
			continue
		}
		average := float64(neighborSum) / float64(neighborCount)
		pick := own
		switch {
		case average >= float64(own)+1.2:
			pick = own + 1
		case average <= float64(own)-1.2 && own > 0:
			pick = own - 1
		}
		if pick < 0 {
			pick = 0
		}
		if pick > 3 {
			pick = 3
		}
		next[cell.Coord] = weatherKinds[pick]
	}
	changed := 0
	for index := range state.Map.Cells {
		cell := &state.Map.Cells[index]
		if cell.Weather != next[cell.Coord] {
			changed++
			cell.Weather = next[cell.Coord]
		}
	}
	if changed > 0 {
		events = append(events, EngineEvent{Type: "weather_changed", Audience: "all", Text: fmt.Sprintf("%d格天气发生变化", changed)})
	}
	// 3) 风向：有风场时由 StepWind 按自己的节奏推进（见 windfield.go），这里只保留
	// 老存档（没有风场）的旧逻辑，免得测试和沙盒剧本行为突变。
	if state.WindField == nil && len(windDirections) > 0 && probabilityRoll(&state.RNGState) < 16.67 {
		current := state.Wind
		order := windDirections
		pick := order[nextUint64(&state.RNGState)%uint64(len(order))]
		if pick != current {
			state.Wind = pick
			RefreshDeckLengths(state)
			events = append(events, EngineEvent{Type: "wind_changed", Audience: "all", Text: "风向转为" + pick})
		}
	}
	return events
}

// stepCellWeather 让单个格子的天气做一次随机游走：晴天更容易变多云，风暴更容易转晴。
func stepCellWeather(rng *uint64, weather string) string {
	roll := probabilityRoll(rng)
	rank := weatherRank(weather)
	switch {
	case roll < 25:
		rank++
	case roll < 60:
		// 保持
	case roll < 90:
		rank--
	default:
		rank = 0
	}
	if rank < 0 {
		rank = 0
	}
	if rank > 3 {
		rank = 3
	}
	return weatherKinds[rank]
}

// VisibleWeather 返回某个阵营可见的天气（目前是全局可见）。
func VisibleWeather(state State) map[string]string {
	result := map[string]string{}
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
