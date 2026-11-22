package game

import "testing"

// 回归：机场侦察机分配侦察任务时必须拿到侦察半径。
// 以前这里不赋值，ReconRadius 停在 0，起飞当回合就满足"扫完扇面"直接返航，
// 图拉吉的 4 架 H6K（半径 31）因此完全用不了。
func TestGroundReconGetsRadiusOnAssignment(t *testing.T) {
	state := sandboxState(t)
	unit := state.GroundAir["af-b-fighter-1"]
	_ = unit
	var recon *AirfieldAirUnit
	for _, item := range state.GroundAir {
		if item.Faction == "B" && item.Kind == "dive_bomber" {
			recon = item
			break
		}
	}
	if recon == nil {
		t.Skip("沙盒没有可搜索的机场轰炸机")
	}
	order := Order{Type: "LAUNCH_RECON", AircraftID: recon.ID, Heading: "W", SectorWidth: 60}
	if err := ValidateGroundAirOrder(state, order); err != nil {
		t.Fatalf("侦察命令被拒：%v", err)
	}
	ApplyGroundAirOrder(&state, order)
	if recon.ReconRadius <= 0 {
		t.Fatalf("侦察半径没有赋值：%+v", recon)
	}
	if recon.ReconRadius != recon.ReconRadiusLimit {
		t.Fatalf("侦察半径应等于机型上限 %d，实际 %d", recon.ReconRadiusLimit, recon.ReconRadius)
	}
}
