package game

import (
	"testing"
)

// cloudField 造一张 7x9 的云层图：只有给定的坐标压云，其余晴朗。
// 坐标沿用 "rrcc" 格式（02 行 02 列），和 state.Map.Clouds 的行优先布局一致。
func cloudField(rows, cols int, cells map[string]byte) string {
	buf := make([]byte, rows*cols)
	for coord, layers := range cells {
		row := int(coord[0]-'0')*10 + int(coord[1]-'0')
		col := int(coord[2]-'0')*10 + int(coord[3]-'0')
		if row < 1 || row > rows || col < 1 || col > cols {
			continue
		}
		buf[(row-1)*cols+(col-1)] = layers
	}
	return string(buf)
}

// 规则书 697–699 行：对舰攻击只有"多云仅战略轰炸机 -5%"和"降雨 -10%"两档，
// 普通机在云下照常投弹；风暴那一档对舰攻击表里没有。
func TestStrikeWeatherOnlyPunishesStrategicBombersUnderCloud(t *testing.T) {
	state := buildStrikeState()
	bb := addDefenderFleet(state, "bb", "BB", "0202", 0, 0)
	dive := addAttacker(state, "dive", "dive_bomber", 1, 4, LoadoutBombTank)
	strat := addAttacker(state, "strat", "strategic_bomber", 1, 4, LoadoutBombTank)

	// 晴天基准：普通机 25%，战略轰炸机吃高空减半 → 12.5%。
	if got := strikeHitChance(*state, dive, bb, false, true); got != 25 {
		t.Fatalf("晴天俯冲轰炸应 25%%，实际 %.1f", got)
	}
	if got := strikeHitChance(*state, strat, bb, false, true); got != 12.5 {
		t.Fatalf("晴天战略轰炸机应 12.5%%，实际 %.1f", got)
	}

	// 多云：普通机不受影响，战略轰炸机 -5% 后再减半。
	state.Map.Clouds = cloudField(7, 9, map[string]byte{"0202": 1})
	if got := strikeHitChance(*state, dive, bb, false, true); got != 25 {
		t.Fatalf("多云不该影响俯冲轰炸机：期望 25，实际 %.1f", got)
	}
	if got := strikeHitChance(*state, strat, bb, false, true); got != 10 {
		t.Fatalf("多云战略轰炸机应为 (25-5)/2=10，实际 %.1f", got)
	}

	// 降雨：所有机种 -10%。
	state.Map.Clouds = cloudField(7, 9, map[string]byte{"0202": 2})
	if got := strikeHitChance(*state, dive, bb, false, true); got != 15 {
		t.Fatalf("降雨俯冲轰炸机应为 15%%，实际 %.1f", got)
	}
	if got := strikeHitChance(*state, strat, bb, false, true); got != 7.5 {
		t.Fatalf("降雨战略轰炸机应为 (25-10)/2=7.5，实际 %.1f", got)
	}
}

// 规则书 679–681 行：对地攻击 多云仅战略轰炸机 -5%、降雨 -10%、风暴 -15%。
func TestGroundAttackWeatherModifiers(t *testing.T) {
	state := buildStrikeState()
	airfield := &Airfield{ID: "af", Coord: "0202", Faction: "B", Strength: 10, Runways: 1}
	medium := &AirfieldAirUnit{ID: "g1", AirfieldID: "af", Faction: "A", Kind: "level_bomber", Model: "B-25"}
	strategic := &AirfieldAirUnit{ID: "g2", AirfieldID: "af", Faction: "A", Kind: "strategic_bomber", Model: "B-17"}

	if got := groundHitChance(*state, airfield, medium, true); got != 20 {
		t.Fatalf("晴天对地基础应 20%%，实际 %.1f", got)
	}

	state.Map.Clouds = cloudField(7, 9, map[string]byte{"0202": 1})
	if got := groundHitChance(*state, airfield, medium, true); got != 20 {
		t.Fatalf("多云不该影响普通轰炸机：期望 20，实际 %.1f", got)
	}
	if got := groundHitChance(*state, airfield, strategic, true); got != 15 {
		t.Fatalf("多云战略轰炸机对地应 15%%，实际 %.1f", got)
	}
	// 机型号也能识别：没写 kind 的老存档同样吃这条修正。
	byModel := &AirfieldAirUnit{ID: "g3", AirfieldID: "af", Faction: "A", Model: "B-24"}
	if got := groundHitChance(*state, airfield, byModel, true); got != 15 {
		t.Fatalf("按机型号识别的战略轰炸机应 15%%，实际 %.1f", got)
	}

	state.Map.Clouds = cloudField(7, 9, map[string]byte{"0202": 2})
	if got := groundHitChance(*state, airfield, medium, true); got != 10 {
		t.Fatalf("降雨对地应 10%%，实际 %.1f", got)
	}

	state.Map.Clouds = cloudField(7, 9, map[string]byte{"0202": 3})
	if got := groundHitChance(*state, airfield, medium, true); got != 5 {
		t.Fatalf("风暴对地应 5%%（15-20 落到下限 5），实际 %.1f", got)
	}
}

// 同格接敌（200%）不吃天气，1 hex 外才乘天气系数（规则书 629–631 行）。
func TestStrikeEngagementWeatherFactor(t *testing.T) {
	if weatherFactor("") != 1 || weatherFactor("cloud") != 0.5 || weatherFactor("rain") != 0.35 || weatherFactor("storm") != 0.2 {
		t.Fatalf("接敌天气系数应为 1/0.5/0.35/0.2，实际 %v/%v/%v/%v",
			weatherFactor(""), weatherFactor("cloud"), weatherFactor("rain"), weatherFactor("storm"))
	}
	// 云层字符串里的三档都要能被 weatherAt 正确读出，否则判定拿不到天气。
	state := State{Map: MapDefinition{Rows: 1, Cols: 3, Clouds: string([]byte{0, 1, 3})}}
	if got := weatherAt(state.Map, "0101"); got != "" {
		t.Fatalf("晴格应返回空，实际 %q", got)
	}
	if got := weatherAt(state.Map, "0102"); got != "cloud" {
		t.Fatalf("1 层云应判为多云，实际 %q", got)
	}
	if got := weatherAt(state.Map, "0103"); got != "storm" {
		t.Fatalf("3 层云应判为风暴，实际 %q", got)
	}
}
