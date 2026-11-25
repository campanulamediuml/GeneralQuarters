package game

import "testing"

// 陆攻认的是"那支舰队"，不是出航时那一格 hex：情报一更新，编队就该改向新的位置。
// 以前只有舰载打击编队每回合校正目标，机场编队要飞到旧 hex 扑空之后才改追，
// 一整段航程白扔，还经常追不上。
func TestGroundStrikeChasesUpdatedContact(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	leader := state.GroundAir["af-b-rabaul-g4m-1"]
	if leader == nil {
		t.Fatal("找不到拉包尔陆攻")
	}
	leader.Status = "attack"
	leader.TaskID = "g-strike"
	leader.TaskKind = "attack"
	leader.Coord = "0223"
	leader.OriginCoord = "0223"
	leader.TargetCoord = "0410"
	leader.TargetFleetID = "US-1"
	leader.SetRange(30)
	mate := *leader
	mate.ID = "af-b-rabaul-g4m-1-mate"
	state.GroundAir[mate.ID] = &mate
	// 情报里这支舰队已经跑到 0610。
	state.Contacts = map[string]map[string]*FleetContact{
		"B": {"US-1": {FleetID: "US-1", Faction: "A", Level: 4, Coord: "0610", DisplayCoord: "0610"}},
	}

	events := AdvanceGroundAir(&state)
	if leader.TargetCoord != "0610" {
		t.Fatalf("编队目标应改到最新情报位置：%s", leader.TargetCoord)
	}
	if !hasEvent(events, "strike_retarget") {
		t.Fatalf("改向应当留下战报：%+v", events)
	}
	if leader.Coord == "0223" {
		t.Fatal("编队应当朝新目标前进了")
	}
	if mate.TargetCoord != "0610" {
		t.Fatalf("同编队成员的目标要一起更新：%s", mate.TargetCoord)
	}
}

// 打固定机场的编队不能被情报拽走：它没有 TargetFleetID，也不该产生改向事件。
func TestGroundStrikeOnAirfieldKeepsHexTarget(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	leader := state.GroundAir["af-b-rabaul-g4m-1"]
	leader.Status = "attack"
	leader.TaskID = "g-airfield"
	leader.TaskKind = "attack"
	leader.Coord = "0223"
	leader.OriginCoord = "0223"
	leader.TargetCoord = "1611"
	leader.TargetFleetID = ""
	leader.SetRange(30)
	state.Contacts = map[string]map[string]*FleetContact{
		"B": {"US-1": {FleetID: "US-1", Faction: "A", Level: 4, Coord: "0610", DisplayCoord: "0610"}},
	}

	events := AdvanceGroundAir(&state)
	if leader.TargetCoord != "1611" {
		t.Fatalf("对机场的打击不该被敌舰队情报拽走：%s", leader.TargetCoord)
	}
	if hasEvent(events, "strike_retarget") {
		t.Fatalf("不该产生改向战报：%+v", events)
	}
}
