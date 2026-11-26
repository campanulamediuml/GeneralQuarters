package game

import (
	"fmt"
	"testing"
)

// B-25 是中型轰炸机，走水平轰炸；B-17 是战略轰炸机，同样水平轰炸但按规则书除以 2。
// 两者都必须能挂炸弹起飞——LoadoutOptions 里漏掉机型分支时，DefaultLoadout 会退化成
// "只有转场"，飞机出不了击。
func TestLevelBomberArmament(t *testing.T) {
	if got := DefaultLoadout("level_bomber", false, 2, 1, "attack", StrikeTargetShip); got.Weapon() != WeaponBomb {
		t.Fatalf("中型轰炸机对舰出击应挂炸弹，实际 %s", got)
	}
	if got := DefaultLoadout("strategic_bomber", false, 2, 1, "attack", StrikeTargetShip); got.Weapon() != WeaponBomb {
		t.Fatalf("战略轰炸机对舰出击应挂炸弹，实际 %s", got)
	}
	for _, kind := range []string{"level_bomber", "strategic_bomber"} {
		if err := ValidateLoadout(kind, false, 2, 1, LoadoutBombTank); err != nil {
			t.Fatalf("%s 应能挂炸弹（副油箱）：%v", kind, err)
		}
		if err := ValidateLoadout(kind, false, 2, 1, LoadoutTorpedo); err == nil {
			t.Fatalf("%s 不应该能挂鱼雷", kind)
		}
	}
}

// 水平轰炸对舰命中率打折：中型 0.75、战略 0.5（且不低于 5%）。
func TestLevelBomberShipHitPenalty(t *testing.T) {
	state := coralCombatState(t)
	ship := firstEnemyShip(&state, "A")
	if ship == nil {
		t.Fatal("找不到目标舰")
	}
	baseline := strikeHitChance(state, &AirUnit{Kind: "dive_bomber", Model: "SBD 无畏"}, ship, false, true)
	level := strikeHitChance(state, &AirUnit{Kind: "level_bomber", Model: "B-25 米切尔"}, ship, false, true)
	strategic := strikeHitChance(state, &AirUnit{Kind: "strategic_bomber", Model: "B-17 空中堡垒"}, ship, false, true)
	wantLevel := baseline * levelBomberShipPenalty
	if diff := level - wantLevel; diff > 0.001 || diff < -0.001 {
		t.Fatalf("中型轰炸机对舰命中率应为 %.2f，实际 %.2f（基准 %.2f）", wantLevel, level, baseline)
	}
	if strategic > level {
		t.Fatalf("战略轰炸机命中率应低于中型：%.2f vs %.2f", strategic, level)
	}
	if strategic < 5 {
		t.Fatalf("战略轰炸机单次命中率不应低于 5%%，实际 %.2f", strategic)
	}
}

// 规则书 719 行：轻型防空值不影响战略轰炸机。骰子照掷，选中高空机时这一发作废。
func TestStrategicBomberIgnoresLightAA(t *testing.T) {
	state := coralCombatState(t)
	reference := firstEnemyShip(&state, "A")
	if reference == nil {
		t.Fatal("找不到参照舰")
	}
	// 用一艘轻防空 9 的驱逐舰当靶子：单次判定 3%，9 颗骰子跑 20 个种子几乎必然出命中，
	// 这样"中型挨打、战略不挨打"的对照才有意义。真实剧本里的舰船轻防空偏小，
	// 跑两三个种子可能一个命中都没有，测试会假通过。
	ship := &Unit{ID: "aa-target", Faction: "B", Type: "DD", Coord: reference.Coord,
		Card: ShipCard{LightAA: 9}}
	// 参战算子必须登记在 state.Aircraft 里，否则 operationalAircraft 会把整队当成
	// 已经不在了，轻防空一轮都掷不出去——这样测试会假通过。
	next := 0
	makeGroup := func(kind, model string) []*AirUnit {
		group := []*AirUnit{}
		for i := 0; i < 4; i++ {
			next++
			unit := &AirUnit{
				ID: fmt.Sprintf("t%d", next), Kind: kind, Model: model,
				Status: "attack", Strength: 3,
				Coord: ship.Coord, Faction: "B",
			}
			state.Aircraft[unit.ID] = unit
			group = append(group, unit)
		}
		return group
	}
	for seed := uint64(1); seed <= 20; seed++ {
		state.RNGState = seed
		report := &AirCombatReport{}
		resolveLightAAPool(&state, makeGroup("strategic_bomber", "B-17 空中堡垒"), ship, false, report)
		if report.AAHits != 0 {
			t.Fatalf("种子%d：战略轰炸机不应被轻型防空命中，实际命中 %d 次", seed, report.AAHits)
		}
	}
	hit := false
	for seed := uint64(1); seed <= 20; seed++ {
		state.RNGState = seed
		report := &AirCombatReport{}
		resolveLightAAPool(&state, makeGroup("level_bomber", "B-25 米切尔"), ship, false, report)
		if report.AAHits > 0 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatal("同样的骰子对中型轰炸机应当能打出命中，否则说明判定被整体关掉了")
	}
}

// 回归：一式陆攻是鱼雷机，不能被当成战略轰炸机把雷击命中率腰斩。
func TestG4MIsNotStrategicBomber(t *testing.T) {
	if isStrategicBomber(&AirUnit{Kind: "torpedo_bomber", Model: "G4M 一式陆攻"}) {
		t.Fatal("G4M 是鱼雷机，不应算战略轰炸机")
	}
	if !isStrategicBomber(&AirUnit{Kind: "strategic_bomber", Model: "B-17 空中堡垒"}) {
		t.Fatal("B-17 应被识别为战略轰炸机")
	}
	if !isStrategicBomber(&AirUnit{Kind: "", Model: "B-24 解放者"}) {
		t.Fatal("没有机种常量时应按型号关键字识别 B-24")
	}
}

// 规则书 98 行：中型机场不能起降战略轰炸机，大型机场可以。
func TestStrategicBomberNeedsMajorAirfield(t *testing.T) {
	if (Airfield{Type: AirfieldMedium}).HardensFor("strategic_bomber", false) {
		t.Fatal("中型机场不应能起降战略轰炸机")
	}
	if !(Airfield{Type: AirfieldLarge}).HardensFor("strategic_bomber", false) {
		t.Fatal("大型机场应能起降战略轰炸机")
	}
	if !(Airfield{Type: AirfieldMedium}).HardensFor("level_bomber", false) {
		t.Fatal("中型轰炸机可以在中型机场起降")
	}
}

// 水上飞机基地只能操作水上飞机：反过来，轮式飞机也不能进水机基地。
func TestSeaplaneBaseOnlyTakesSeaplanes(t *testing.T) {
	base := Airfield{Type: AirfieldSeaplane}
	if !base.HardensFor("recon", true) {
		t.Fatal("水上飞机基地应能起降水上飞机")
	}
	if base.HardensFor("fighter", false) {
		t.Fatal("水上飞机基地不能起降轮式战斗机")
	}
	if base.HardensFor("torpedo_bomber", false) {
		t.Fatal("水上飞机基地不能起降陆攻")
	}
	// 反过来：陆上机场收不了水上飞机。
	for _, kind := range []AirfieldType{AirfieldSmall, AirfieldMedium, AirfieldLarge} {
		if (Airfield{Type: kind}).HardensFor("recon", true) {
			t.Fatalf("%s 不该接收水上飞机", kind)
		}
	}
	if !(Airfield{Type: AirfieldMedium}).HardensFor("recon", false) {
		t.Fatal("中型机场应能起降陆基侦察机")
	}
}

// 旧剧本/旧存档里的 field/heavy/major 要按规模归一：野战=小、重型=中、大型=大。
func TestNormalizeLegacyAirfieldTypes(t *testing.T) {
	cases := map[AirfieldType]AirfieldType{
		"field": AirfieldSmall, "heavy": AirfieldMedium, "major": AirfieldLarge,
		"small": AirfieldSmall, "medium": AirfieldMedium, "large": AirfieldLarge,
		"seaplane": AirfieldSeaplane, " Major ": AirfieldLarge,
	}
	for input, want := range cases {
		if got := NormalizeAirfieldType(input); got != want {
			t.Fatalf("%q 应归一为 %s，实际 %s", input, want, got)
		}
	}
}

// 维拉港 4 架卡特里娜、凯恩斯 4 架 P-40 + 4 架 F4F + 4 架 B-17。
func TestCoralRealVilaAndCairns(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 9)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]map[string]int{}
	for _, aircraft := range state.GroundAir {
		if groups[aircraft.AirfieldID] == nil {
			groups[aircraft.AirfieldID] = map[string]int{}
		}
		groups[aircraft.AirfieldID][aircraft.DefinitionID]++
	}
	want := map[string]map[string]int{
		"af-a-vila": {"pby": 4},
		// 凯恩斯的 F4F 用的就是美军现成的 fighter 卡（模型 F4F 野猫），不再单开一张。
		"af-a-cairns": {"p40": 4, "fighter": 4, "b17": 4},
	}
	for airfieldID, expected := range want {
		for kind, count := range expected {
			if got := groups[airfieldID][kind]; got != count {
				t.Fatalf("%s 的 %s 应有 %d 架，实际 %d", airfieldID, kind, count, got)
			}
		}
	}
	vila := state.Airfields["af-a-vila"]
	if vila == nil || vila.Coord != "3960" || vila.Type != AirfieldSeaplane {
		t.Fatalf("维拉港水上飞机基地部署错误：%+v", vila)
	}
	cairns := state.Airfields["af-a-cairns"]
	if cairns == nil || cairns.Coord != "3707" || cairns.Type != AirfieldLarge {
		t.Fatalf("凯恩斯机场部署错误：%+v", cairns)
	}
	b17 := state.GroundAir["af-a-cairns-b17-1"]
	if b17 == nil || b17.Kind != "strategic_bomber" || b17.Strength != 3 || b17.AirCombat != 3 ||
		b17.AntiShip != 1 || b17.AntiGround != 2 || b17.BaseRange != 37 {
		t.Fatalf("B-17 数据不对：%+v", b17)
	}
	pby := state.GroundAir["af-a-vila-pby-1"]
	if pby == nil || pby.Kind != "recon" || pby.BaseRange != 40 || !pby.AirfieldOnly {
		t.Fatalf("PBY 数据不对：%+v", pby)
	}
}

// coralCombatState 造一个能跑空战判定的珊瑚海局面。
func coralCombatState(t *testing.T) State {
	t.Helper()
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 3)
	if err != nil {
		t.Fatal(err)
	}
	// 命中率用例要的是"天气中性"的靶场：云层会随机落在目标格上、
	// 把两种攻击一起压到命中下限，那样比不出机种差异。
	state.WeatherTurns = 0
	state.Map.Clouds = ""
	// 战斗用例要的是开战后的状态：直接跳过开局部署阶段。
	StartBattle(&state)
	return state
}

// firstEnemyShip 找一艘指定阵营的船当靶子。
func firstEnemyShip(state *State, faction string) *Unit {
	for _, unit := range state.Units {
		if unit.Faction == faction && !isCarrierType(unit.Type) && unit.Type != "TRS" {
			return unit
		}
	}
	return nil
}
