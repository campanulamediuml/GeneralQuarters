package game

import "testing"

// TestGroundAircraftCanChangeLoadout 机场待命的陆基机也能换挂载：
// 以前 SET_LOADOUT 只查舰载机表，陆攻选挂载会被判"航空算子不存在"，
// 前端下拉框因为缺字段干脆是空的。
func TestGroundAircraftCanChangeLoadout(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatal(err)
	}
	StartBattle(&state)
	const id = "af-b-rabaul-g4m-1"
	ground := state.GroundAir[id]
	if ground == nil {
		t.Fatalf("缺少陆攻算子 %s", id)
	}
	ground.Status = "hangar"
	before := ground.Loadout

	order := Order{Type: "SET_LOADOUT", AircraftID: id, Loadout: string(LoadoutTorpedo)}
	if err := ValidateOrder(state, "B", order); err != nil {
		t.Fatalf("陆基机换挂载应被接受：%v", err)
	}
	ApplyLoadoutOrder(&state, order)
	if ground.Loadout == before {
		t.Fatalf("挂载没有变化，仍是 %s", ground.Loadout)
	}
	if !ground.LoadoutManual {
		t.Fatal("手动选的挂载应打上标记，避免分配任务时被默认值覆盖")
	}
	if ground.Range() <= 0 {
		t.Fatalf("换挂载后应重算航程：%d", ground.Range())
	}
}
