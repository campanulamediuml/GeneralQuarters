package game

import "testing"

// 回归：目标还在情报里跟着走的时候，打击编队每回合都要把"改追目标"的战报发出来。
// advanceAttack 末尾曾经直接 return nil，攒下来的 strike_retarget / attack_arrived
// 全被丢掉，玩家看不到任何改目标提示。
func TestAdvanceAttackKeepsRetargetEvents(t *testing.T) {
	state := retargetFixture()
	leader := state.Aircraft["l"]
	member := state.Aircraft["m"]
	// 先认领舰队（正常流程里这一步发生在 SET_STRIKE_TARGET）。
	leader.TargetFleetID = "B1"
	member.TargetFleetID = "B1"
	state.Contacts["A"]["B1"].Coord = "0205"
	events := advanceAttack(&state, leader)
	found := false
	for _, event := range events {
		if event.Type == "strike_retarget" {
			found = true
		}
	}
	if !found {
		t.Fatalf("编队飞行途中改追目标却没有下发 strike_retarget：%+v", events)
	}
	if leader.TargetCoord != "0205" || member.TargetCoord != "0205" {
		t.Fatalf("目标没有改追：leader=%s member=%s", leader.TargetCoord, member.TargetCoord)
	}
}

// 回归：一回合的空袭结算会 delete 掉被摧毁的算子，而 AdvanceAviation 遍历的是回合开始时的
// id 快照。之前这里会 nil 解引用 panic，服务端 recover 后把整回合事件丢弃、状态却照旧落库，
// 于是"空袭打完了但战报一片空白"。
func TestAdvanceAviationSurvivesAircraftDestroyedDuringStrike(t *testing.T) {
	state := sandboxState(t)
	state.Aircraft = map[string]*AirUnit{}
	ids := []string{"strike-a", "strike-b", "strike-c", "strike-d"}
	for _, id := range ids {
		state.Aircraft[id] = &AirUnit{
			ID: id, CarrierID: "a2-cv-01", Faction: "A", Model: "SB2C", Kind: "dive_bomber",
			Status: "attack", TaskID: "strike-a", TaskKind: "attack",
			Coord: "0608", TargetCoord: "0608", Engaged: true,
			Strength: 1, AirCombat: 1, AntiShip: 4, MaxRange: 12, RemainingRange: 12,
		}
	}
	state.Units["b-cv-01"].Coord = "0608"
	// 防空值拉到夸张，保证本回合一定摧毁至少两架攻击机，逼出"快照里有已删除算子"的情形。
	state.Units["b-cv-01"].Card.HeavyAA = 4000
	state.Units["b-cv-01"].Card.LightAA = 4000
	// 这个用例测的是"快照里有已删除算子"的崩溃，不是日军重防空的新算法；
	// 给防守方套美军规则，保证整队重防空在投弹前就打出大量击落。
	if state.Doctrines == nil {
		state.Doctrines = map[string]string{}
	}
	state.Doctrines["B"] = DoctrineUS
	events := AdvanceAviation(&state)
	if len(state.Aircraft) == len(ids) {
		t.Fatalf("测试前提不成立，没有任何攻击机被摧毁")
	}
	resolved := false
	for _, event := range events {
		if event.Type == "air_combat_resolved" {
			resolved = true
		}
	}
	if !resolved {
		t.Fatalf("空袭结算的事件被丢弃了：%+v", events)
	}
}
