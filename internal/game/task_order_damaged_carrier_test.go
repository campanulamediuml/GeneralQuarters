package game

import "testing"

// 编队级命令（改目标/改攻法）不能被编队里某一条船的损伤挡掉：
// 翔鹤甲板被摧毁，不影响给瑞鹤那几架在空中的飞机改目标。
func TestStrikeTargetAllowedWithDamagedGroupMember(t *testing.T) {
	state := sandboxState(t)
	damaged := state.Units["b-cv-01"]
	damaged.Coord = "0303"
	EnsureDamage(damaged).DeckDestroyed = true
	healthy := state.Units["b-cv-02"]
	healthy.Coord = "0303"

	leader := state.Aircraft["b-cv-01-dive_bomber-x1"]
	leader.Status = "attack_holding"
	leader.TaskKind = "attack"
	leader.TaskID = "strike-x"
	leader.TargetCoord = "0708"
	leader.Coord = "0303"

	member := state.Aircraft["b-cv-02-dive_bomber-x1"]
	member.Status = "attack_holding"
	member.TaskKind = "attack"
	member.TaskID = "strike-x"
	member.TargetCoord = "0708"
	member.Coord = "0303"

	// 目标要有一份真实情报，否则会被"不能盲打"拦下来（那是另一条规则，应该保留）。
	state.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Faction: "A", Level: 4, Coord: "0708", Count: 1,
			Members: []Unit{{ID: "a-cv-01", Faction: "A", Coord: "0708", Type: "CV"}}},
	}

	order := Order{Type: "SET_STRIKE_TARGET", AircraftID: "strike-x", TaskID: "strike-x", Coord: "0708"}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("编队级命令不应被甲板损伤挡掉：%v", err)
	}
	// 但真正需要起飞的命令仍然要被拦。
	launch := Order{Type: "LAUNCH_STRIKE", AircraftID: "b-cv-01-dive_bomber-x2"}
	if err := ValidateOrder(state, "B", launch); err == nil {
		t.Fatal("甲板被摧毁的母舰不应允许分配新的出击任务")
	}
}
