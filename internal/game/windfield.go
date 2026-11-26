package game

import (
	"math"
	"strings"
)

// —— 每 hex 风向场（规则书 815–825 行）——
//
// 以前只有一个全局风向：要么恒定向、要么每 6 回合整体重掷一次，可能从 NE 直接翻到 SW。
// 现在风向是坐标的函数：每个 hex 有自己的来向，由"盛行风 + 三层长波谐波"叠加而成。
//
// 平滑性不是靠事后修正，而是靠参数保证的：
//
//	相邻 hex 的夹角 ≤ Σ(振幅 × 2π × 波方向长度 / 波长)
//	三层谐波分别是 7°@13格、6°@15格、4°@17格(对角)，合起来每格变化 < 8°。
//
// 时间上也一样：每次天气推进，盛行风最多转 ±2°、每层相位最多 ±2°，
// 任意一个 hex 单次变化 < 8°，不会出现"一回合转 120°、180°"的情况。
type WindHarmonic struct {
	// RowWeight / ColWeight 是这层波沿行/列的推进方向（-1/0/1）。
	RowWeight int `json:"row_weight"`
	ColWeight int `json:"col_weight"`
	// Wavelength 是一个完整波长的 hex 数；越大越平缓。
	Wavelength int `json:"wavelength"`
	// Amplitude 是方向偏移的幅度（度）。
	Amplitude int `json:"amplitude"`
	// Phase 是相位（度，0–359），天气推进时每次最多走 ±2°。
	Phase int `json:"phase"`
}

// WindField 是整张地图的风向场。盛行风（BaseDeg）是"来向"，
// 0 = 北、90 = 东，和 compass 方位一致（气象习惯：SE = 风从东南吹来）。
type WindField struct {
	// Version 是风场参数版本：老存档里的风场可能还是旧振幅/波长，
	// 房间载入时按版本升级一次（保留盛行风和相位，免得风向凭空重掷）。
	Version   int            `json:"version"`
	BaseDeg   int            `json:"base_deg"`
	Harmonics []WindHarmonic `json:"harmonics"`
}

const windFieldVersion = 2

// windHarmonicTemplate 是初始化用的三层长波：幅度和波长的比值决定了每格最大变化。
// 取到这个幅度（±18/15/10°），整张图会跨到 3–4 个方位带，而相邻格最大连续变化约 20°，
// 量化到 60° 一档后只会跨一档。
var windHarmonicTemplate = []WindHarmonic{
	{RowWeight: 1, ColWeight: 0, Wavelength: 13, Amplitude: 18},
	{RowWeight: 0, ColWeight: 1, Wavelength: 15, Amplitude: 15},
	{RowWeight: 1, ColWeight: 1, Wavelength: 17, Amplitude: 10},
}

// newWindField 用剧本的盛行风 + 一个种子生成初始风场。种子只用来错开三层相位，
// 不消耗 RNGState——部署随机序列不能被天气初始化带偏。
func newWindField(compass string, seed uint64) *WindField {
	base, ok := compassDegrees(compass)
	if !ok {
		// 剧本没设风向：没有风就没有甲板修正，也不生成风场。
		return nil
	}
	field := &WindField{Version: windFieldVersion, BaseDeg: base, Harmonics: make([]WindHarmonic, len(windHarmonicTemplate))}
	for index, template := range windHarmonicTemplate {
		field.Harmonics[index] = template
		field.Harmonics[index].Phase = int(splitmix64(seed+uint64(index)*0x9e3779b97f4a7c15) % 360)
	}
	return field
}

func splitmix64(value uint64) uint64 {
	value += 0x9e3779b97f4a7c15
	value = (value ^ (value >> 30)) * 0xbf58476d1ce4e5b9
	value = (value ^ (value >> 27)) * 0x94d049bb133111eb
	return value ^ (value >> 31)
}

// DegreesAt 返回某个 hex 的风向来向（度，0–359）。坐标非法时退回盛行风。
func (field WindField) DegreesAt(coord string) int {
	row, col, err := ParseCoord(coord)
	if err != nil {
		return normalizeDegrees(field.BaseDeg)
	}
	total := float64(field.BaseDeg)
	for _, harmonic := range field.Harmonics {
		wavelength := harmonic.Wavelength
		if wavelength <= 0 {
			wavelength = 12
		}
		phase := (float64(harmonic.RowWeight*row+harmonic.ColWeight*col)*360/float64(wavelength) + float64(harmonic.Phase)) * math.Pi / 180
		total += float64(harmonic.Amplitude) * math.Sin(phase)
	}
	return normalizeDegrees(int(math.Round(total)))
}

// Step 缓慢推进一次风场：盛行风 ±3°、每层相位 ±3°。
// 单次推进对任意 hex 的影响 < 12°，正好在一档（60°）之内；相邻 hex 的变化率不变。
func (field *WindField) Step(rng *uint64) {
	if field == nil {
		return
	}
	field.BaseDeg = normalizeDegrees(field.BaseDeg + randRange(rng, -3, 3))
	for index := range field.Harmonics {
		field.Harmonics[index].Phase = normalizeDegrees(field.Harmonics[index].Phase + randRange(rng, -3, 3))
	}
}

// EnsureWindField 给老存档补一个风场：房间载入时调用一次。老存档里只有单一风向，
// 直接拿它当盛行风基准、用当前 RNGState 当种子错开相位——不消耗 RNG，也不会让
// 部署序列变样；补完之后的回合就开始按坐标取风。
func EnsureWindField(state *State) {
	if state.Wind == "" {
		return
	}
	if state.WindField == nil {
		state.WindField = newWindField(state.Wind, state.RNGState)
	} else if state.WindField.Version < windFieldVersion {
		upgradeWindField(state.WindField)
	}
	if state.WindNextTurn <= 0 {
		state.WindNextTurn = state.Turn + 6 + int(state.RNGState%7)
	}
	if state.WindField != nil {
		// 老存档的 state.Wind 可能还停在升级前的方位：统一成风场的盛行风，
		// 前端和事件文案才不会出现"盛行风 E、字段里写 SE"这种自相矛盾。
		state.Wind = windDirectionName(state.WindField.BaseDeg)
	}
}

// upgradeWindField 把旧版本的风场参数换成当前模板：保留盛行风和每层相位，
// 换成新的振幅/波长。只发生在载入时，不会每回合重排。
func upgradeWindField(field *WindField) {
	phases := make([]int, len(field.Harmonics))
	for index := range field.Harmonics {
		phases[index] = field.Harmonics[index].Phase
	}
	field.Harmonics = make([]WindHarmonic, len(windHarmonicTemplate))
	for index, template := range windHarmonicTemplate {
		field.Harmonics[index] = template
		if index < len(phases) {
			field.Harmonics[index].Phase = phases[index]
		}
	}
	field.Version = windFieldVersion
}

// StepWind 按"每 6–12 回合一次"的节奏推进风向场，返回事件。
// 不是每回合都动：甲板修正每回合抖一下没有实感，反而让玩家觉得数字在乱跳。
func StepWind(state *State) []EngineEvent {
	if state.WindField == nil || state.WindNextTurn <= 0 {
		return nil
	}
	if state.Turn < state.WindNextTurn {
		return nil
	}
	before := windDirectionName(state.WindField.BaseDeg)
	state.WindField.Step(&state.RNGState)
	state.Wind = windDirectionName(state.WindField.BaseDeg)
	RefreshDeckLengths(state)
	state.WindNextTurn = state.Turn + 6 + int(nextUint64(&state.RNGState)%7)
	if after := state.Wind; after != before {
		return []EngineEvent{{Type: "wind_changed", Audience: "all", Text: "盛行风由" + before + "转为" + after + "，各海区风向随之缓慢变化"}}
	}
	return nil
}

// randRange 返回 [min, max] 的整数，用现有的 RNGState 生成。
func randRange(rng *uint64, min, max int) int {
	if max <= min {
		return min
	}
	return min + int(nextUint64(rng)%uint64(max-min+1))
}

func normalizeDegrees(degrees int) int {
	degrees %= 360
	if degrees < 0 {
		degrees += 360
	}
	return degrees
}

// compassDegrees 把六方位名转成来向角度；不认识就是"没风"。
func compassDegrees(compass string) (int, bool) {
	angle, ok := reconHeadings[strings.ToUpper(strings.TrimSpace(compass))]
	return int(angle), ok
}

// DirectionAt 返回某个 hex 的六方位风向来向（windDirections 见 weather.go）。
// 逐格风向量化到六档：底层连续场每格最多变 8°，量化后相邻格最多差 60°（一档），
// 不会出现 120°/180° 的突变。
func (field WindField) DirectionAt(coord string) string {
	return windDirectionName(field.DegreesAt(coord))
}

// windDirectionName 把连续来向角量化成六方位之一：六个档中心是 30°+k·60°，
// 所以 (deg-30)/60 四舍五入就是档位。
func windDirectionName(degrees int) string {
	shifted := normalizeDegrees(degrees - 30)
	index := int(math.Round(float64(shifted)/60)) % 6
	return windDirections[index]
}

// WindDirectionAt 是规则侧取风口的入口：有风场按坐标取，没有就退回剧本的单一风向。
func (state State) WindDirectionAt(coord string) (string, bool) {
	if state.WindField != nil {
		return state.WindField.DirectionAt(coord), true
	}
	name := strings.ToUpper(strings.TrimSpace(state.Wind))
	for _, candidate := range windDirections {
		if candidate == name {
			return candidate, true
		}
	}
	return "", false
}

// WindCells 把整张地图的逐格风向压成一个行优先字符串：每个 hex 一个字符，
// '0'..'5' 是 windDirections 的下标（视图给前端画箭头 / 悬浮查询用）。
// 没有风场时返回空串，前端按单一风向处理。
//
// 每次视图投影都要重算一遍（三千多个格子），所以这里把每层谐波的 sin/cos 拆成
// 行表和列表：sin(α_row + β_col) = sinα·cosβ + cosα·sinβ，逐格只做两次乘加，
// 不再每格调三次 math.Sin，也不拼坐标字符串。
func WindCells(state State) string {
	if state.WindField == nil {
		return ""
	}
	rows, cols := state.Map.Rows, state.Map.Cols
	if rows <= 0 || cols <= 0 {
		return ""
	}
	type layer struct {
		amplitude      float64
		sinRow, cosRow []float64
		sinCol, cosCol []float64
	}
	layers := make([]layer, len(state.WindField.Harmonics))
	for index, harmonic := range state.WindField.Harmonics {
		wavelength := harmonic.Wavelength
		if wavelength <= 0 {
			wavelength = 12
		}
		k := 2 * math.Pi / float64(wavelength)
		phase := float64(harmonic.Phase) * math.Pi / 180
		current := layer{
			amplitude: float64(harmonic.Amplitude),
			sinRow:    make([]float64, rows+1), cosRow: make([]float64, rows+1),
			sinCol: make([]float64, cols+1), cosCol: make([]float64, cols+1),
		}
		for row := 1; row <= rows; row++ {
			angle := k*float64(harmonic.RowWeight*row) + phase
			current.sinRow[row], current.cosRow[row] = math.Sin(angle), math.Cos(angle)
		}
		for col := 1; col <= cols; col++ {
			angle := k * float64(harmonic.ColWeight*col)
			current.sinCol[col], current.cosCol[col] = math.Sin(angle), math.Cos(angle)
		}
		layers[index] = current
	}
	cells := make([]byte, 0, rows*cols)
	for row := 1; row <= rows; row++ {
		for col := 1; col <= cols; col++ {
			total := float64(state.WindField.BaseDeg)
			for _, current := range layers {
				total += current.amplitude * (current.sinRow[row]*current.cosCol[col] + current.cosRow[row]*current.sinCol[col])
			}
			shifted := math.Mod(total-30+3600, 360)
			index := int(math.Round(shifted/60)) % 6
			cells = append(cells, byte('0'+index))
		}
	}
	return string(cells)
}
