package game

import (
	"strings"
	"testing"
)

// 规则书 222 行：飞机不能在无跑道的机场起飞，但可以降落，降落时 60% 迫降失败进入损伤状态。
// 这里顺带守住另一端：跑道被打光不能顺手把召回/取消任务也一起锁死。
func TestAirfieldWithoutRunwaysBlocksTakeoffButAllowsLanding(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-a-lighthouse"]
	if airfield == nil {
		t.Fatal("缺少灯塔岛机场")
	}
	airfield.Strength = 0
	var parked *AirfieldAirUnit
	var airborne *AirfieldAirUnit
	for _, aircraft := range state.GroundAir {
		if aircraft.AirfieldID != airfield.ID {
			continue
		}
		if parked == nil {
			parked = aircraft
			continue
		}
		airborne = aircraft
		break
	}
	if parked == nil || airborne == nil {
		t.Fatal("灯塔岛机场缺少驻机")
	}
	airborne.Status = "cap"
	airborne.TaskID = "cap-test"
	airborne.TaskKind = "cap"

	// 起飞类命令被拒绝。
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_CAP", AircraftIDs: []string{parked.ID}})
	if err == nil || !strings.Contains(err.Error(), "已经没有可用跑道") {
		t.Fatalf("无跑道机场的起飞命令应被拒绝，实际 %v", err)
	}
	// 召回/取消任务只涉及降落或地面调度，不该被跑道锁死。
	if err := ValidateGroundAirOrder(state, Order{Type: "RECOVER_AIR", AircraftIDs: []string{airborne.ID}}); err != nil {
		t.Fatalf("RECOVER_AIR 在无跑道机场仍应允许：%v", err)
	}
	if err := ValidateGroundAirOrder(state, Order{Type: "RETURN_TO_HANGAR", AircraftIDs: []string{parked.ID}}); err != nil {
		t.Fatalf("RETURN_TO_HANGAR 在无跑道机场仍应允许：%v", err)
	}
}

// 无跑道迫降判定：60% 进入损伤状态但不直接摧毁机体。
func TestLandingWithoutRunwaysUsesDamageRoll(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-a-lighthouse"]
	airfield.Strength = 0
	var aircraft *AirfieldAirUnit
	for _, unit := range state.GroundAir {
		if unit.AirfieldID == airfield.ID {
			aircraft = unit
			break
		}
	}
	if aircraft == nil {
		t.Fatal("灯塔岛机场没有驻机")
	}
	aircraft.Status = "cap"
	state.RNGState = 1 // 28.08 < 60：迫降失败
	events := landGroundAircraft(&state, aircraft, airfield)
	if !aircraft.Damaged {
		t.Fatal("迫降失败时应进入损伤状态")
	}
	if !hasEvent(events, "hard_landing") {
		t.Fatal("迫降失败应产生 hard_landing 战报")
	}

	aircraft.Damaged = false
	state.RNGState = 3 // 84.25 >= 60：安全落地
	events = landGroundAircraft(&state, aircraft, airfield)
	if aircraft.Damaged {
		t.Fatal("迫降判定通过时不应损伤")
	}
	if !hasEvent(events, "air_landed") {
		t.Fatal("安全落地应产生 air_landed 战报")
	}
}
