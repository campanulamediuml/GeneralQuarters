package game

import (
	"strings"
	"testing"
)

// advanceGroundUntil 推进机场地面作业与空中作业，直到算子进入期望状态或用尽回合。
func advanceGroundUntil(state *State, id, status string, turns int) []EngineEvent {
	events := []EngineEvent{}
	for turn := 0; turn < turns; turn++ {
		events = append(events, AdvanceAirfields(state)...)
		events = append(events, AdvanceGroundAir(state)...)
		if aircraft := state.GroundAir[id]; aircraft != nil && aircraft.Status == status {
			return events
		}
	}
	return events
}

func hasEvent(events []EngineEvent, kind string) bool {
	for _, event := range events {
		if event.Type == kind {
			return true
		}
	}
	return false
}

// 规则书 188 行：机场侦察机落地后必须带着原航向/扇面/半径重新整备、再次出动。
// 这是用户报的那个 bug：机场算子压根没有循环，落地之后任务参数被清空就再也飞不起来。
func TestGroundReconCycleRelaunchesWithSearchParams(t *testing.T) {
	state := coralCombatState(t)
	// 时间窗单独测，这里只验循环本身。
	state.Timeline = nil
	pby := state.GroundAir["af-a-vila-pby-1"]
	if pby == nil {
		t.Fatal("找不到维拉港 PBY")
	}
	airfield := airfieldFor(&state, pby.AirfieldID)
	pby.Status = "recon"
	pby.TaskID = "recon-vila"
	pby.TaskKind = "recon"
	pby.CycleKind = "recon"
	pby.CycleTaskID = "recon-vila"
	pby.CycleHeading = "W"
	pby.CycleSector = 60
	pby.Heading = "W"
	pby.SectorWidth = 60
	pby.ReconRadius = GroundReconRadius(pby)
	pby.Coord = airfield.Coord
	pby.OriginCoord = airfield.Coord
	pby.Returning = true
	pby.OnCycle = true
	pby.SetRange(2)

	if !hasEvent(advanceGroundUntil(&state, pby.ID, "service", 8), "air_cycle_restart") {
		t.Fatalf("机场侦察降落后应自动重新整备：status=%s", pby.Status)
	}
	if !pby.OnCycle || pby.TaskKind != "recon" || pby.TaskID != "recon-vila" {
		t.Fatalf("重新整备应恢复侦察任务与循环开关：%+v", pby)
	}
	limit := GroundReconRadius(pby)
	if pby.Heading != "W" || pby.SectorWidth != 60 || pby.ReconRadius != limit {
		t.Fatalf("循环丢了搜索参数（期望航向 W / 扇面 60 / 半径 %d）：%+v", limit, pby)
	}
	// 继续推进：它得真的再次升空，而不是永远停在跑道上整备。
	if !hasEvent(advanceGroundUntil(&state, pby.ID, "recon", 6), "air_launched") {
		t.Fatalf("循环中的机场侦察机应再次升空：status=%s", pby.Status)
	}
	if pby.OriginCoord != airfield.Coord {
		t.Fatalf("再次出动的起点应是原机场：%s", pby.OriginCoord)
	}
}

// 机场 CAP 的循环：落回跑道后自动整备，并按原巡逻空域再飞回去。
func TestGroundCAPCycleKeepsPatrolCoord(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	p40 := state.GroundAir["af-a-moresby-p40-1"]
	if p40 == nil {
		t.Fatal("找不到莫尔兹比 P-40")
	}
	airfield := airfieldFor(&state, p40.AirfieldID)
	p40.Status = "cap"
	p40.TaskID = "af-" + airfield.ID + "-cap"
	p40.TaskKind = "cap"
	p40.CycleKind = "cap"
	p40.CycleTaskID = p40.TaskID
	p40.TargetCoord = airfield.Coord
	p40.CycleTarget = airfield.Coord
	p40.Coord = airfield.Coord
	p40.OriginCoord = airfield.Coord
	p40.Returning = true
	p40.OnCycle = true
	p40.SetRange(2)

	advanceGroundUntil(&state, p40.ID, "service", 8)
	if !p40.OnCycle || p40.TaskKind != "cap" {
		t.Fatalf("机场 CAP 落回跑道后应自动重新整备：%+v", p40)
	}
	if p40.TargetCoord != airfield.Coord {
		t.Fatalf("循环应回到原巡逻空域 %s：%s", airfield.Coord, p40.TargetCoord)
	}
	if !hasEvent(advanceGroundUntil(&state, p40.ID, "cap", 6), "air_launched") {
		t.Fatalf("循环中的机场 CAP 应再次升空：status=%s", p40.Status)
	}
}

// 规则书 191 行：机场算子也能从界面/接口开关循环，关掉之后落地不再自己出动。
func TestGroundCycleToggleCommand(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	pby := state.GroundAir["af-a-vila-pby-1"]
	pby.Status = "hangar"
	pby.TaskID = "recon-vila"
	pby.TaskKind = "recon"
	pby.Heading = "SW"
	pby.SectorWidth = 120

	order := Order{Type: "SET_AIR_CYCLE", AircraftID: pby.ID, Cycle: true}
	// 分派必须落到机场校验（airOrderIsGround 靠算子 ID 判断），否则命令会被当成舰载机。
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("机场算子的循环命令被拒绝：%v", err)
	}
	ApplyGroundAirOrder(&state, order)
	if !pby.OnCycle || pby.CycleKind != "recon" || pby.CycleHeading != "SW" || pby.CycleSector != 120 {
		t.Fatalf("开启循环应当记住侦察参数：%+v", pby)
	}

	off := Order{Type: "SET_AIR_CYCLE", AircraftID: pby.ID}
	if err := ValidateGroundAirOrder(state, off); err != nil {
		t.Fatalf("取消循环不应被拒绝：%v", err)
	}
	ApplyGroundAirOrder(&state, off)
	if pby.OnCycle || !pby.CycleOptOut {
		t.Fatalf("命令应当关闭循环并记下玩家选择：%+v", pby)
	}

	// 降落后不许自己恢复循环。
	pby.Status = "recon"
	pby.Returning = true
	pby.Coord = airfieldFor(&state, pby.AirfieldID).Coord
	advanceGroundUntil(&state, pby.ID, "hangar", 6)
	for turn := 0; turn < 3; turn++ {
		AdvanceAirfields(&state)
		AdvanceGroundAir(&state)
	}
	if pby.OnCycle || pby.Status != "hangar" {
		t.Fatalf("取消循环后不应自己再出动：status=%s on_cycle=%v", pby.Status, pby.OnCycle)
	}
}

// 没有可循环任务（例如停在机库、没编任务的战斗机）要明确拒绝，而不是默默什么都不做。
func TestGroundCycleRejectsNonCyclicTask(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	fighter := state.GroundAir["af-a-moresby-p40-1"]
	fighter.Status = "hangar"
	fighter.TaskID = ""
	fighter.TaskKind = ""
	fighter.CycleKind = ""
	order := Order{Type: "SET_AIR_CYCLE", AircraftID: fighter.ID, Cycle: true}
	err := ValidateGroundAirOrder(state, order)
	if err == nil || !strings.Contains(err.Error(), "没有可循环的任务") {
		t.Fatalf("没有任务时开启循环应被拒绝：%v", err)
	}
}

// 机场被打掉、跑道打光之后循环必须收手，并留下一条能看懂的说明。
func TestGroundCycleEndsWhenAirfieldLost(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	pby := state.GroundAir["af-a-vila-pby-1"]
	pby.Status = "hangar"
	pby.OnCycle = true
	pby.CycleKind = "recon"
	pby.CycleHeading = "W"
	pby.CycleSector = 60
	state.Airfields[pby.AirfieldID].Strength = 0

	events := AdvanceGroundAir(&state)
	if pby.OnCycle {
		t.Fatal("跑道被打光后循环应当结束")
	}
	if !hasEvent(events, "air_cycle_end") {
		t.Fatal("循环结束应当留一条战报")
	}
}

// 陆基 CAP 任务卡的位置必须跟着飞机走：以前固定写机场坐标，
// 地图上那块 CAP 牌永远挂在机场上，看不出它到底飞去哪个空域值班。
func TestGroundCAPTaskFollowsAircraft(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	p40 := state.GroundAir["af-a-moresby-p40-1"]
	p40.Status = "cap"
	p40.TaskID = "af-af-a-moresby-cap"
	p40.TaskKind = "cap"
	p40.Coord = "1711"
	p40.TargetCoord = "1811"
	p40.OriginCoord = "1611"

	view := GroundAviationView(state, "A")
	for _, task := range view.Tasks {
		if task.ID != p40.TaskID {
			continue
		}
		if task.CurrentCoord != "1711" {
			t.Fatalf("CAP 任务位置应跟着飞机走：%s", task.CurrentCoord)
		}
		if task.TargetCoord != "1811" {
			t.Fatalf("CAP 任务应带上巡逻空域：%s", task.TargetCoord)
		}
		return
	}
	t.Fatal("找不到陆基 CAP 任务")
}
