package game

import "testing"

// 阵营规则表锁定：这些数字是规则书里的国家差异，重构（比如把散落的 if 收进
// FactionRules）时必须逐项保持原样，不能顺手"统一"掉。
func TestFactionRuleTablesAreLocked(t *testing.T) {
	us := rulesForDoctrine("us", 1944, false)
	jp := rulesForDoctrine("japan", 1942, true)

	if us.RepairChance != 10 || jp.RepairChance != 3 {
		t.Fatalf("修复率 us=%v jp=%v", us.RepairChance, jp.RepairChance)
	}
	if us.RadioPenaltyPerHex != 10 || jp.RadioPenaltyPerHex != 20 {
		t.Fatalf("无线电惩罚 us=%v jp=%v", us.RadioPenaltyPerHex, jp.RadioPenaltyPerHex)
	}
	if us.AirfieldRadar != [5]float64{100, 80, 60, 40, 20} {
		t.Fatalf("美军机场雷达表 %v", us.AirfieldRadar)
	}
	if jp.AirfieldRadar != [5]float64{80, 60, 40, 20, 0} {
		t.Fatalf("日军机场雷达表 %v", jp.AirfieldRadar)
	}
	if us.CapitalShipRadar != [5]float64{90, 70, 50, 30, 10} {
		t.Fatalf("美军主力舰雷达表 %v", us.CapitalShipRadar)
	}
	if jp.CapitalShipRadar != [5]float64{85, 65, 45, 25, 5} {
		t.Fatalf("日军主力舰雷达表 %v", jp.CapitalShipRadar)
	}
	// 弹药曲线：日军 50% 那一档的边界是"兵力比≥1"，美军是">1/2"，照规则书各写各的。
	if got := us.AmmunitionDepletionChance(1, 2); got != 75 {
		t.Fatalf("美军 1:2 弹药消耗率 %v", got)
	}
	if got := jp.AmmunitionDepletionChance(1, 2); got != 75 {
		t.Fatalf("日军 1:2 弹药消耗率 %v", got)
	}
	if got := jp.AmmunitionDepletionChance(1, 1); got != 50 {
		t.Fatalf("日军 1:1 弹药消耗率（含边界）%v", got)
	}
	if got := us.AmmunitionDepletionChance(6, 1); got != 10 {
		t.Fatalf("美军 6:1 弹药消耗率 %v", got)
	}
	if got := jp.AmmunitionDepletionChance(6, 1); got != 10 {
		t.Fatalf("日军 6:1 弹药消耗率 %v", got)
	}
	if jp.DeckDestroyChance != 10 || us.DeckDestroyChance != 1 {
		t.Fatalf("甲板报废概率 us=%v jp=%v", us.DeckDestroyChance, jp.DeckDestroyChance)
	}
	if jp.ShortRangeEngagePenalty != 50 || us.ShortRangeEngagePenalty != 0 {
		t.Fatalf("航程不足惩罚 us=%v jp=%v", us.ShortRangeEngagePenalty, jp.ShortRangeEngagePenalty)
	}
	if !us.FleetLightAAMerge || jp.FleetLightAAMerge {
		t.Fatalf("航母轻防空并入：us=%v jp=%v", us.FleetLightAAMerge, jp.FleetLightAAMerge)
	}
	// 1942 年及以前的日军精锐机组：机型数据不动，出手时 +1，侦察 +10%。
	if jp.ElitePilotBonus != 1 || jp.EarlyWarReconBonus != 10 || jp.SurpriseEliteBonus != 1 {
		t.Fatalf("日军精锐机组加成丢失：%+v", jp)
	}
	if jp.SurpriseNightBonus != 1 || jp.SurpriseTorpedoBonus != 1 {
		t.Fatalf("日军夜战/雷击队加成丢失：%+v", jp)
	}
	// 1944 年的美军：雷达火控惩罚消失，后期重型防空与年份加成生效。
	if us.EarlyRadarPenalty != 0 || us.HeavyAALateBonus != 2 || us.SurpriseYearBonus != 1 {
		t.Fatalf("1944 年美军加成不对：%+v", us)
	}
	if us.AirRadarReconDetectBonus != 20 || us.ReconInCAPBonus != 20 {
		t.Fatalf("美军雷达抓侦察机的加成不对：%+v", us)
	}
}

// 年份/天色要进入规则解析：同一支军队在 1942 年和 1944 年不是同一套规则。
func TestFactionRuleYearAndNightGates(t *testing.T) {
	if early := rulesForDoctrine("japan", 1943, false); early.ElitePilotBonus != 0 || early.EarlyWarReconBonus != 0 {
		t.Fatalf("1943 年日军不该还有精锐机组加成：%+v", early)
	}
	if pre := rulesForDoctrine("us", 1943, false); pre.EarlyRadarPenalty != 20 {
		t.Fatalf("1943 年美军应有雷达火控惩罚，实际 %v", pre.EarlyRadarPenalty)
	}
	if day := rulesForDoctrine("japan", 1942, false); day.SurpriseNightBonus != 0 {
		t.Fatal("白天不该有夜战加成")
	}
	// 没配 doctrine 的势力走中立表：既没有美日特权，也不套用任何一方数值。
	neutral := rulesForDoctrine("", 1944, true)
	if neutral.ElitePilotBonus != 0 || neutral.SurpriseNightBonus != 0 || neutral.SurpriseYearBonus != 0 ||
		neutral.HeavyAALateBonus != 0 || neutral.FleetLightAAMerge || neutral.EarlyRadarPenalty != 0 {
		t.Fatalf("中立规则不该带阵营加成：%+v", neutral)
	}
	if neutral.AirfieldRadar != [5]float64{60, 40, 20, 0, 0} {
		t.Fatalf("中立机场雷达表 %v", neutral.AirfieldRadar)
	}
}

// 对手阵营只在一处算：以前每个文件各写一遍 "B"; if faction=="B" {"A"}。
func TestOpposingFaction(t *testing.T) {
	if got := opposingFaction("A"); got != "B" {
		t.Fatalf("A 的对手应为 B，实际 %q", got)
	}
	if got := opposingFaction("B"); got != "A" {
		t.Fatalf("B 的对手应为 A，实际 %q", got)
	}
	if got := opposingFaction("C"); got != "" {
		t.Fatalf("未知阵营不该凭空造出对手，实际 %q", got)
	}
}
