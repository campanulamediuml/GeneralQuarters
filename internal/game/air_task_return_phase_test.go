package game

import "testing"

// 无线电召回会把打击编队的目标清空。目标为空时 taskPhase 以前一律判成
// "舰队上空等待编队"（assembling），于是已经出发的编队在召回后又冒回整编栏里，
// 玩家会在"还没出发"和"已经在天上"之间看到同一支编队两遍。
// 返航中的打击编队必须留在"已离场"这一侧，交给地图上的编队牌显示。
func TestRecalledAttackFormationStaysDeparted(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	carrier := state.Units["b-cv-01"]
	if carrier == nil {
		t.Fatal("找不到翔鹤")
	}
	plane := &AirUnit{ID: "strike-ret", CarrierID: carrier.ID, Faction: "B", Kind: "dive_bomber", Model: "D3A1",
		Status: "attack", Strength: 1, AirCombat: 1, AntiShip: 3,
		TaskID: "strike-ret", TaskKind: "attack", Coord: "1931", OriginCoord: carrier.Coord,
		TargetCoord: "", Returning: true}
	plane.SetRange(20)
	state.Aircraft = map[string]*AirUnit{plane.ID: plane}

	view := VisibleAviation(state, "B")
	var task *AirTask
	for index := range view.Tasks {
		if view.Tasks[index].ID == "strike-ret" {
			task = &view.Tasks[index]
		}
	}
	if task == nil {
		t.Fatal("返航中的打击编队不应该从任务列表里消失")
	}
	if task.Phase == "assembling" {
		t.Fatalf("返航中的编队不该被当成还没出发：%+v", task.Phase)
	}
	if task.Phase != "active" {
		t.Fatalf("返航中的编队的阶段应落在已离场一侧，实际 %s", task.Phase)
	}
	if !task.Returning {
		t.Fatal("任务投影要带 returning，地图上的编队牌据此写“返航中”")
	}
}

// 全体成员都还在舰队上空等待（没有目标、没有返航）时仍然是 assembling：
// 这支编队还得留在整编栏里让玩家选目标。
func TestAssemblingAttackFormationKeepsPhase(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	carrier := state.Units["b-cv-01"]
	if carrier == nil {
		t.Fatal("找不到翔鹤")
	}
	plane := &AirUnit{ID: "strike-wait", CarrierID: carrier.ID, Faction: "B", Kind: "dive_bomber", Model: "D3A1",
		Status: "attack", Strength: 1, AirCombat: 1, AntiShip: 3,
		TaskID: "strike-wait", TaskKind: "attack", Coord: carrier.Coord, OriginCoord: carrier.Coord,
		TargetCoord: ""}
	plane.SetRange(20)
	state.Aircraft = map[string]*AirUnit{plane.ID: plane}

	view := VisibleAviation(state, "B")
	for _, task := range view.Tasks {
		if task.ID != "strike-wait" {
			continue
		}
		if task.Phase != "assembling" {
			t.Fatalf("在舰队上空等编队、还没选目标的编队应当保持 assembling，实际 %s", task.Phase)
		}
		if task.Returning {
			t.Fatal("还没返航的编队不该标成返航中")
		}
		return
	}
	t.Fatal("等待编队的打击编队不应从任务列表里消失")
}
