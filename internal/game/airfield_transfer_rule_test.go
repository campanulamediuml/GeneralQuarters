package game

import (
	"strings"
	"testing"
)

// 水上飞机基地只能操作水上飞机：轮式飞机不能转场进水机基地。
func TestTransferToSeaplaneBaseRejected(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	fighter := state.GroundAir["af-b-rabaul-fighter-1"]
	if fighter == nil {
		t.Fatal("找不到拉包尔的零战")
	}
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{fighter.ID}, Coord: "0831"})
	if err == nil {
		t.Fatal("轮式战斗机不该能转场到肖特兰水上飞机基地")
	}
	if !strings.Contains(err.Error(), "无法起降") && !strings.Contains(err.Error(), "水上飞机基地") {
		t.Fatalf("错误信息应说明是水机基地拒收：%v", err)
	}
}

// 反过来：水上飞机转场到陆上机场也要被拒。
func TestTransferSeaplaneToLandAirfieldRejected(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	flying := state.GroundAir["af-b-shortland-h6k-1"]
	if flying == nil {
		t.Fatal("找不到肖特兰的飞行艇")
	}
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{flying.ID}, Coord: "0224"})
	if err == nil {
		t.Fatal("水上飞机不该能转场到陆上机场")
	}
	if !strings.Contains(err.Error(), "水上飞机基地") && !strings.Contains(err.Error(), "无法起降") {
		t.Fatalf("错误信息应说明陆上机场收不了水机：%v", err)
	}
}

// 中型机场收不了 B-17，大型机场可以：拉包尔仍是中型，莫尔兹比港已升为大型。
func TestTransferRespectsAirfieldSize(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	bomber := state.GroundAir["af-a-townsville-b17-1"]
	if bomber == nil {
		t.Fatal("找不到汤斯维尔的 B-17")
	}
	moresby := state.Airfields["af-a-moresby"]
	moresby.Type = AirfieldMedium
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{bomber.ID}, Coord: "1611"})
	if err == nil {
		t.Fatal("B-17 不该能转场到中型机场")
	}
	if !strings.Contains(err.Error(), "无法起降") {
		t.Fatalf("错误信息应说明机场无法起降 B-17：%v", err)
	}
	moresby.Type = AirfieldLarge
	if err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{bomber.ID}, Coord: "1611"}); err != nil {
		t.Fatalf("B-17 应能转场到大型的莫尔兹比港：%v", err)
	}
}
