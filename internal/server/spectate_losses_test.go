package server

import (
	"testing"

	"generalquarters/internal/game"
)

// 观战台账要按阵营分别投影再合并：自己那份最全，对方的飞机损失在自己的视图里会被抹掉。
// 以前合并顺序让 B 的视图覆盖了 A 的结果，观战页显示 A 一架飞机都没丢过。
func TestSpectatorLossesKeepOwnAircraftLedger(t *testing.T) {
	state := game.State{Losses: map[string]*game.LossLedger{
		"A": {
			Ships:         []game.ShipLoss{{Name: "约克城"}},
			Aircraft:      []game.AircraftLoss{{Model: "SBD 无畏", Kind: "dive_bomber", Count: 5}},
			ShipTotal:     1,
			AircraftTotal: 5,
		},
		"B": {
			Ships:         []game.ShipLoss{{Name: "祥凤"}},
			Aircraft:      []game.AircraftLoss{{Model: "A6M2 零战", Kind: "fighter", Count: 2}},
			ShipTotal:     1,
			AircraftTotal: 2,
		},
	}}
	merged := spectatorLosses(state)
	if got := merged["A"].AircraftTotal; got != 5 {
		t.Fatalf("观战页 A 的飞机损失 = %d，应为 5（被对方视图覆盖了）", got)
	}
	if got := merged["B"].AircraftTotal; got != 2 {
		t.Fatalf("观战页 B 的飞机损失 = %d，应为 2", got)
	}
	if merged["A"].ShipTotal != 1 || merged["B"].ShipTotal != 1 {
		t.Fatalf("沉船台账丢了：%+v", merged)
	}
}
