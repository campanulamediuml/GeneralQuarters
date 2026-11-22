package game

import "testing"

// 取消自动航行：空坐标是合法的"清空"指令。
// 之前校验层把它当成"没选目的地"直接 400，取消按钮永远发不出去。
func TestClearAutoNavAcceptsEmptyCoord(t *testing.T) {
	state := sandboxState(t)
	unit := state.Units["b-cv-01"]
	order := Order{Type: "SET_AUTO_NAV", UnitID: unit.ID, Coord: "0505"}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("设置目的地应当通过：%v", err)
	}
	ApplyNavOrder(&state, order)
	if state.Units[unit.ID].Destination != "0505" {
		t.Fatalf("目的地未写入：%+v", state.Units[unit.ID].Destination)
	}

	clear := Order{Type: "SET_AUTO_NAV", UnitID: unit.ID, Coord: ""}
	if err := ValidateOrder(state, "B", clear); err != nil {
		t.Fatalf("取消自动航行不应被拒绝：%v", err)
	}
	events := ApplyNavOrder(&state, clear)
	if state.Units[unit.ID].Destination != "" {
		t.Fatalf("取消后目的地应清空：%q", state.Units[unit.ID].Destination)
	}
	if len(events) != 1 || events[0].Type != "nav_cleared" {
		t.Fatalf("应产生取消自动航行事件：%+v", events)
	}
}
