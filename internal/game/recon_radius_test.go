package game

import "testing"

// 侦察半径按"实际会挂的那套"折半（recon + 副油箱），不是转场挂载。
// 用转场航程算出来的半径飞机根本飞不到：燃油会在半路把它叫回来，
// 面板上写着 12 hex、实际只能扫 10 hex。
func TestReconRadiusUsesActualLoadout(t *testing.T) {
	// 基础航程 12 的水机：侦察挂载 16（12×1.3），半径 8，往返正好 16。
	aircraft := AirUnit{Kind: "recon", Seaplane: true, MaxRange: 16, BaseRange: 12}
	if got, want := ReconRadiusLimit(aircraft), 8; got != want {
		t.Fatalf("水机侦察半径应为侦察挂载航程的一半：got %d want %d", got, want)
	}
	// 半径不随当前油量变化：同一机型在快照里必须只有一个上限。
	aircraft.Fuel = Fuel{Tank: 6, Reserve: 4}
	if got, want := ReconRadiusLimit(aircraft), 8; got != want {
		t.Fatalf("半径上限不应随燃油账本变化：got %d want %d", got, want)
	}
}

// 半径必须能被飞满：往返距离不超过满油航程，否则玩家看到的上限是假的。
func TestReconRadiusIsFlyable(t *testing.T) {
	state := sandboxState(t)
	checked := 0
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || !canSearch(*aircraft) {
			continue
		}
		loadout := LoadoutForTask(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, "recon", StrikeTargetShip)
		sortie := SortieBaseRange(aircraft.BaseRange, loadout)
		limit := ReconRadiusLimit(*aircraft)
		if limit*2 > sortie {
			t.Fatalf("%s 半径 %d（往返 %d）超过满油航程 %d", id, limit, limit*2, sortie)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("沙盒剧本没有可搜索的机型")
	}
}
