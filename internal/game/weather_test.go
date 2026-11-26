package game

import (
	"strings"
	"testing"
)

// 未开启天气系统时保持静态：不铺云，也不动老存档里的静态天气格。
func TestWeatherStaticWithoutCycle(t *testing.T) {
	state := State{Turn: 12, Map: MapDefinition{Rows: 2, Cols: 2, Cells: []MapCell{{Coord: "0101", Weather: "rain"}}}, RNGState: 7}
	StepWeather(&state)
	if state.Map.Clouds != "" {
		t.Fatal("没开天气系统不该铺云")
	}
	if weatherAt(state.Map, "0101") != "rain" {
		t.Fatal("未配置 WeatherTurns 时静态天气不应变化")
	}
}

// 初始撒云：70% 一层、20% 再加一层、10% 再加一层，叠成雨和风暴。
func TestCloudSeedingDistribution(t *testing.T) {
	state := State{WeatherTurns: 6, RNGState: 42, Map: MapDefinition{Rows: 52, Cols: 64}}
	EnsureWeather(&state)
	if len(state.Map.Clouds) != 52*64 {
		t.Fatalf("天气层长度不对：%d", len(state.Map.Clouds))
	}
	counts := [4]int{}
	for index := 0; index < len(state.Map.Clouds); index++ {
		layers := int(state.Map.Clouds[index] - '0')
		if layers < 0 || layers > 3 {
			t.Fatalf("非法云层数 %d", layers)
		}
		counts[layers]++
	}
	total := float64(len(state.Map.Clouds))
	clear := float64(counts[0]) / total
	rain := float64(counts[2]) / total
	storm := float64(counts[3]) / total
	if clear < 0.15 || clear > 0.30 {
		t.Fatalf("晴天比例应接近 22%%，实际 %.1f%%", clear*100)
	}
	if rain < 0.10 || rain > 0.28 {
		t.Fatalf("降雨比例应接近 19%%，实际 %.1f%%", rain*100)
	}
	if storm <= 0 || storm > 0.05 {
		t.Fatalf("风暴比例应接近 1.4%%，实际 %.1f%%", storm*100)
	}
	if counts[1] == 0 {
		t.Fatal("应该有多云格")
	}
}

// 云每 3 回合整体移动一次；其他回合不动。
func TestCloudsMoveEveryThreeTurns(t *testing.T) {
	state := State{Turn: 1, WeatherTurns: 6, RNGState: 9, Map: MapDefinition{Rows: 8, Cols: 8}}
	EnsureWeather(&state)
	seeded := state.Map.Clouds
	for _, turn := range []int{1, 2, 4, 5} {
		state.Turn = turn
		StepWeather(&state)
		if state.Map.Clouds != seeded {
			t.Fatalf("第 %d 回合不该移动云层", turn)
		}
	}
	state.Turn = 3
	StepWeather(&state)
	if state.Map.Clouds == seeded {
		t.Fatal("第 3 回合云层应该移动")
	}
}

// 云层总数在移动时守恒（目标叠满时的消散除外，小图里基本不会发生）。
func TestCloudsConservedWhenMoving(t *testing.T) {
	state := State{Turn: 3, WeatherTurns: 6, RNGState: 5, Wind: "E", Map: MapDefinition{Rows: 10, Cols: 10}}
	EnsureWeather(&state)
	before := totalClouds(state.Map.Clouds)
	state.Turn = 3
	StepWeather(&state)
	after := totalClouds(state.Map.Clouds)
	if before != after {
		t.Fatalf("云层总数不守恒：%d -> %d", before, after)
	}
}

func totalClouds(cells string) int {
	total := 0
	for index := 0; index < len(cells); index++ {
		total += int(cells[index] - '0')
	}
	return total
}

// 75% 顺风、25% 随机：单朵云在固定东风下，大部分时候应该往风吹向（西）挪。
func TestCloudsMostlyDriftDownwind(t *testing.T) {
	const trials = 200
	downwind := 0
	for trial := 0; trial < trials; trial++ {
		state := State{Turn: 3, WeatherTurns: 6, Wind: "E", RNGState: uint64(trial*7919 + 1), Map: MapDefinition{Rows: 12, Cols: 12}}
		cells := make([]byte, 12*12)
		for index := range cells {
			cells[index] = '0'
		}
		cells[(6-1)*12+(6-1)] = '1' // 中心一朵云
		state.Map.Clouds = string(cells)
		StepWeather(&state)
		row, col := 0, 0
		for index := 0; index < len(state.Map.Clouds); index++ {
			if state.Map.Clouds[index] == '1' {
				row, col = index/12+1, index%12+1
			}
		}
		// 东风（从东吹向西）：吹向是西，所以列号应该变小或不变；随机方向时可能变大。
		if row == 6 && col < 6 {
			downwind++
		}
	}
	if downwind < trials*60/100 {
		t.Fatalf("顺风移动比例太低：%d/%d", downwind, trials)
	}
}

// 云层数直接映射成天气名：1 多云、2 降雨、3 风暴。
func TestWeatherAtReadsLayers(t *testing.T) {
	state := State{Map: MapDefinition{Rows: 2, Cols: 2, Clouds: "0123"}}
	want := map[string]string{"0101": "", "0102": WeatherCloud, "0201": WeatherRain, "0202": WeatherStorm}
	for coord, expected := range want {
		if got := weatherAt(state.Map, coord); got != expected {
			t.Fatalf("%s 天气应为 %q，实际 %q", coord, expected, got)
		}
	}
}

// 老存档没有云层时退回 MapCell.Weather。
func TestWeatherFallsBackToStaticCells(t *testing.T) {
	gameMap := MapDefinition{Rows: 2, Cols: 2, Cells: []MapCell{{Coord: "0101", Weather: "RAIN"}}}
	if got := weatherAt(gameMap, "0101"); got != "rain" {
		t.Fatalf("老存档静态天气应小写返回：%q", got)
	}
	if got := weatherAt(gameMap, "0102"); got != "" {
		t.Fatalf("没有天气的格子应为晴：%q", got)
	}
}

// 逐格风向表里每个字符都合法；同种子结果稳定。
func TestWeatherLockedToSeed(t *testing.T) {
	first := State{WeatherTurns: 6, RNGState: 123, Map: MapDefinition{Rows: 6, Cols: 6}}
	second := State{WeatherTurns: 6, RNGState: 123, Map: MapDefinition{Rows: 6, Cols: 6}}
	EnsureWeather(&first)
	EnsureWeather(&second)
	if first.Map.Clouds != second.Map.Clouds {
		t.Fatal("同种子铺出来的云层不一致")
	}
	if strings.Trim(first.Map.Clouds, "0123") != "" {
		t.Fatalf("天气层里有非法字符：%q", first.Map.Clouds)
	}
}
