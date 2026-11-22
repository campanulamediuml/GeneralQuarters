package game

import "testing"

// 端到端：下午放出去的远程侦察必须自己回来，不会在 19:40 被"天黑未能返航"带走。
// 这是"飞机没有自动返航"那个恶性 bug 的回归用例。
func TestLongRangeReconReturnsBeforeNightfall(t *testing.T) {
	state := sandboxState(t)
	state.Timeline = &Timeline{TurnMinutes: 20, ServiceStart: "04:00", LaunchStart: "05:20", RecoverBy: "19:40", NightLoss: true}
	state.TurnsPerDay = 72
	state.DayHours = 0
	// 沙盒地图只有 7×9，坐标必须落在图内，否则"飞不回去"是地图的错不是引擎的错。
	carrier := state.Units["b-ca-01"]
	carrier.Coord = "0403"
	carrier.FleetID = "MO"
	plane := state.Aircraft["air-b-11"]
	if plane == nil {
		t.Fatal("缺少水上侦察算子")
	}
	plane.Status = "recon"
	plane.TaskKind = "recon"
	plane.TaskID = "recon-1"
	plane.Heading = "E"
	plane.SectorWidth = 60
	plane.ReconRadius = 4
	plane.ReconDistance = 0
	plane.Coord = carrier.Coord
	plane.OriginCoord = carrier.Coord
	plane.Seaplane = true
	plane.OnCycle = true
	plane.CycleKind = "recon"
	plane.SetRange(30)

	// 第 50 回合 = 16:20，距 19:40 只剩 10 回合：往返 8 回合刚好卡住，再晚就是送。
	state.Turn = 50
	for turn := 50; turn <= 64; turn++ {
		state.Turn = turn
		for _, event := range ResolveOrders(&state, nil) {
			if event.Type == "air_destroyed" {
				t.Fatalf("第 %d 回合出现航空损失：%s", turn, event.Text)
			}
		}
	}
	live := state.Aircraft[plane.ID]
	if live == nil {
		t.Fatal("远程侦察机在天黑前被损失了")
	}
	if airborne(live) {
		t.Fatalf("天黑时仍在天上：%s %s", live.Status, live.Coord)
	}
}
