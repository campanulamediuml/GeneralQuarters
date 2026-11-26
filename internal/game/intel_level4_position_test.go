package game

import "testing"

// levelContactState 造一个"我方 A 有一支舰队、敌方 B 一支舰队"的最小状态。
// 情报里记的坐标故意和真实坐标不同，用来验证刷新规则。
func levelContactState(level int) State {
	return State{
		Units: map[string]*Unit{
			"a1": {ID: "a1", FleetID: "TF1", FleetName: "TF1", Faction: "A", Coord: "0304", Name: "旗舰", Type: "CA", Strength: 4, MaxStrength: 4},
			"b1": {ID: "b1", FleetID: "MO", FleetName: "MO", Faction: "B", Coord: "0808", Name: "瑞鹤", Type: "CV", Strength: 4, MaxStrength: 4},
		},
		Contacts: map[string]map[string]*FleetContact{
			"A": {"MO": {
				FleetID: "MO", Faction: "B", Level: level,
				Coord: "0707", DisplayCoord: "0707",
				Members: []Unit{{ID: "b1", FleetID: "MO", FleetName: "MO", Faction: "B", Coord: "0707", Name: "瑞鹤", Type: "CV"}},
			}},
		},
	}
}

// TestLevel4ContactTracksRealPosition 4 级情报是"编成与状态已知"：位置也要按真实舰队刷新。
// 以前 refreshContactMembers 把坐标钉在最后发现的那一格，4 级情报的舰队在敌情面板上
// 越跑越远（实测差 4 hex），打击校验也还按旧 hex 放行。
func TestLevel4ContactTracksRealPosition(t *testing.T) {
	state := levelContactState(intelMaxLevel)
	refreshContactMembers(&state)
	contact := state.Contacts["A"]["MO"]
	if contact.Coord != "0808" || contact.DisplayCoord != "0808" {
		t.Fatalf("4 级情报应跟到真实位置 0808：Coord=%s DisplayCoord=%s", contact.Coord, contact.DisplayCoord)
	}
	if len(contact.Members) != 1 || contact.Members[0].Coord != "0808" {
		t.Fatalf("4 级情报的舰船投影也应在真实位置：%+v", contact.Members)
	}
	units := projectedContactUnits(*contact)
	if len(units) != 1 || units[0].Coord != "0808" {
		t.Fatalf("地图投影应画在真实位置：%+v", units)
	}
}

// TestLowLevelContactStaysFrozen 1–3 级仍然停在最后发现位置（误差按等级收敛），
// 否则"情报不全"就没有代价了。
func TestLowLevelContactStaysFrozen(t *testing.T) {
	state := levelContactState(2)
	refreshContactMembers(&state)
	contact := state.Contacts["A"]["MO"]
	if contact.DisplayCoord != "0707" || contact.Members[0].Coord != "0707" {
		t.Fatalf("2 级情报应停在最后发现位置 0707，实际 DisplayCoord=%s 舰船=%s", contact.DisplayCoord, contact.Members[0].Coord)
	}
}
