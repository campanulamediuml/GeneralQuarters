package game

import (
	"strings"
	"testing"
)

// 规则书 211/212 行：CAP 只掩护自己所在 hex 及周围 1 hex；
// "派去其他友军上空"对应的是空中掩护任务（LAUNCH_COVER），会跟随被掩护舰队。
// 机场 CAP 不接受远程坐标，否则会出现"飞到远处守一个不会跟的空格"。
func TestGroundCAPRejectsRemotePatrolHex(t *testing.T) {
	state := sandboxState(t)
	var fighter *AirfieldAirUnit
	var airfield *Airfield
	for _, aircraft := range state.GroundAir {
		if aircraft.Kind != "fighter" || aircraft.TaskID != "" {
			continue
		}
		fighter = aircraft
		airfield = state.Airfields[aircraft.AirfieldID]
		break
	}
	if fighter == nil || airfield == nil {
		t.Fatal("沙盒里没有可用的战斗机")
	}

	// 远程 CAP 被拒绝，并提示改用 LAUNCH_COVER。
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_CAP", AircraftIDs: []string{fighter.ID}, Coord: "0101"})
	if err == nil || !strings.Contains(err.Error(), "LAUNCH_COVER") {
		t.Fatalf("远程 CAP 应被拒绝并提示空中掩护，实际 %v", err)
	}
	// 守本机场（留空或填本机场坐标）照常允许。
	if err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_CAP", AircraftIDs: []string{fighter.ID}}); err != nil {
		t.Fatalf("本机场 CAP 应允许：%v", err)
	}
	if err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_CAP", AircraftIDs: []string{fighter.ID}, Coord: airfield.Coord}); err != nil {
		t.Fatalf("本机场坐标 CAP 应允许：%v", err)
	}
}
