package game

import "testing"

// 未开启天气系统时，地图天气保持静态。
func TestWeatherStaticWithoutCycle(t *testing.T) {
	state := State{Turn: 12, Map: MapDefinition{Cells: []MapCell{{Coord: "0101", Weather: "rain"}}}, RNGState: 7}
	StepWeather(&state)
	if state.Map.Cells[0].Weather != "rain" {
		t.Fatal("未配置 WeatherTurns 时天气不应变化")
	}
}

// 开启天气系统后，天气格会随时间扩散变化。
func TestWeatherEvolvesWithCycle(t *testing.T) {
	cells := []MapCell{}
	for row := 1; row <= 4; row++ {
		for col := 1; col <= 4; col++ {
			cells = append(cells, MapCell{Coord: Coord(row, col)})
		}
	}
	cells[0].Weather = "storm"
	state := State{Turn: 3, WeatherTurns: 3, Map: MapDefinition{Rows: 4, Cols: 4, Cells: cells}, RNGState: 99, Wind: "NE"}
	events := StepWeather(&state)
	if len(events) == 0 {
		t.Fatal("天气系统应当产生变化事件")
	}
	stormy := 0
	for _, cell := range state.Map.Cells {
		if weatherRank(cell.Weather) >= 2 {
			stormy++
		}
	}
	if stormy == 0 {
		t.Fatal("初始风暴格不应整片消失")
	}
}

// 天气只在间隔回合推进：第 3 回合刷新时 Turn=3 生效，Turn=4 不生效。
func TestWeatherOnlyStepsOnInterval(t *testing.T) {
	cells := []MapCell{{Coord: "0101", Weather: "storm"}}
	state := State{Turn: 4, WeatherTurns: 3, Map: MapDefinition{Rows: 1, Cols: 1, Cells: cells}, RNGState: 5}
	StepWeather(&state)
	if state.Map.Cells[0].Weather != "storm" {
		t.Fatal("非刷新回合不应改变天气")
	}
}

// 风向关系表：顺风 -2、逆风 +2、侧顺 +1、侧逆 -1。
func TestWindDirectionsCoverAllHeadings(t *testing.T) {
	for _, heading := range []string{"NE", "E", "SE", "SW", "W", "NW"} {
		if got := windDeckModifier("NE", heading); got == 0 && heading != "NE" {
			t.Fatalf("朝向 %s 应当有风向修正", heading)
		}
	}
	// 风向 NE = 风从东北来：朝 NE 是逆风 +2，朝 SW 是顺风 -2。
	if windDeckModifier("NE", "SW") != -2 {
		t.Fatal("顺风应 -2")
	}
	if windDeckModifier("NE", "NE") != 2 {
		t.Fatal("逆风应 +2")
	}
}
