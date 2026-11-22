package game

import "testing"

// 侦察距离不可设置：不管客户端传什么，一律用该机型的最大搜索半径。
// 玩家只决定"搜哪个方向、多宽的扇面"。
func TestReconRadiusIsNotConfigurable(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-11", Heading: "E", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatalf("侦察命令不应因为半径被拒：%v", err)
	}
	ApplyAirOrder(&state, order)
	plane := state.Aircraft["air-b-11"]
	if plane == nil {
		t.Fatal("算子丢失")
	}
	want := ReconRadiusLimit(*plane)
	if plane.ReconRadius != want {
		t.Fatalf("侦察半径应固定为机型的最大值 %d，实际 %d", want, plane.ReconRadius)
	}
	if plane.ReconRadius <= 0 {
		t.Fatal("侦察半径没有赋值")
	}
}
