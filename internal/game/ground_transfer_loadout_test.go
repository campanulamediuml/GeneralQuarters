package game

import "testing"

// 陆基机同样如此：转场任务把手动挂载换成副油箱，否则校验按 1.7× 放行、
// 飞机实际只带 1.3× 的油，飞到半路就透支。
func TestGroundTransferDropsManualLoadout(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	const id = "af-b-rabaul-g4m-1"
	// 剧本从 00:00 开始，整备窗口还没开；这里只测挂载，把时间线摘掉绕开时间窗。
	state.Timeline = nil
	ground := state.GroundAir[id]
	if ground == nil {
		t.Fatalf("缺少陆攻算子 %s", id)
	}
	ground.Status = "hangar"
	ground.TaskID = ""
	ground.LoadoutManual = true
	applyGroundLoadoutChoice(ground, LoadoutTorpedoTank)

	order := Order{Type: "LAUNCH_TRANSFER", AircraftID: id}
	if err := ValidateGroundAirOrder(state, order); err != nil {
		t.Fatalf("建立转场任务失败：%v", err)
	}
	ApplyGroundAirOrder(&state, order)

	if ground.Loadout != LoadoutTransfer {
		t.Fatalf("陆基转场没有换成副油箱：loadout=%s", ground.Loadout)
	}
	if ground.LoadoutManual {
		t.Fatal("陆基转场卸掉武器后，手动挂载标记没有清掉")
	}
	if want := SortieBaseRange(ground.BaseRange, LoadoutTransfer); ground.MaxRange != want {
		t.Fatalf("陆基转场满油航程 = %d，应为 %d", ground.MaxRange, want)
	}
}
