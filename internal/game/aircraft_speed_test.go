package game

import "testing"

// 规则书 200 行：执行任务时水上飞机/双翼机 1 hex，其余 2 hex。
func TestAircraftSpeedSeaplane(t *testing.T) {
	if got := aircraftSpeed(&AirUnit{Seaplane: true}); got != 1 {
		t.Fatalf("水上飞机每回合应移动 1 hex，实际 %d", got)
	}
	if got := aircraftSpeed(&AirUnit{}); got != 2 {
		t.Fatalf("普通飞机每回合应移动 2 hex，实际 %d", got)
	}
}
