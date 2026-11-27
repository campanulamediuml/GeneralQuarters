package game

import "testing"

// 敌舰队的航速属于情报：情报 4 级已经有完整编成和卡面，编队航速本来就是这些数据折出来的
// （最慢那艘 + 编队规模惩罚），所以照实投影；1–3 级连舰种都不齐，视图里不给。
func TestEnemyFleetSpeedFollowsIntelLevel(t *testing.T) {
	state := sandboxState(t)
	members := []Unit{}
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-B" {
			members = append(members, *unit)
		}
	}
	if len(members) == 0 {
		t.Fatal("夹具里没有 TG-B")
	}
	leader := members[0]
	want := FleetCycleKnots(state, &leader)
	if want <= 0 {
		t.Fatalf("TG-B 折出来的航速应该大于 0，实际 %d", want)
	}
	state.Contacts["A"] = map[string]*FleetContact{
		"TG-B": {FleetID: "TG-B", Faction: "B", Level: 4, Coord: leader.Coord, DisplayCoord: leader.Coord, Count: len(members), Members: members},
	}
	fleet := fleetByID(VisibleFleets(state, "A"), "TG-B")
	if fleet == nil {
		t.Fatal("情报 4 级的 TG-B 没有进 A 的视图")
	}
	if fleet.MoveKnots != want || fleet.MoveInterval != FleetMoveInterval(state, &leader) {
		t.Fatalf("情报 4 级的敌舰队航速应为 %d 节 / %d 回合一格，实际 %d 节 / %d 回合",
			want, FleetMoveInterval(state, &leader), fleet.MoveKnots, fleet.MoveInterval)
	}

	// 情报降到 2 级：只剩"有敌舰、多大"，航速不该跟着露出去。
	state.Contacts["A"]["TG-B"].Level = 2
	fleet = fleetByID(VisibleFleets(state, "A"), "TG-B")
	if fleet == nil {
		t.Fatal("情报 2 级的 TG-B 应该还在视图里")
	}
	if fleet.MoveKnots != 0 || fleet.MoveInterval != 0 {
		t.Fatalf("情报 2 级不该给出敌舰队航速，实际 %d 节 / %d 回合", fleet.MoveKnots, fleet.MoveInterval)
	}

	// 自己家的舰队不受情报等级影响，航速一直都在。
	own := fleetByID(VisibleFleets(state, "A"), "TG-A")
	if own == nil || own.MoveKnots <= 0 {
		t.Fatalf("己方舰队应当始终带航速，实际 %+v", own)
	}
}

func fleetByID(fleets []FleetGroup, id string) *FleetGroup {
	for index := range fleets {
		if fleets[index].FleetID == id {
			return &fleets[index]
		}
	}
	return nil
}
