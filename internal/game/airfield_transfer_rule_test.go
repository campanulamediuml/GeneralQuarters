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
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{flying.ID}, Coord: "0323"})
	if err == nil {
		t.Fatal("水上飞机不该能转场到陆上机场")
	}
	if !strings.Contains(err.Error(), "水上飞机基地") && !strings.Contains(err.Error(), "无法起降") {
		t.Fatalf("错误信息应说明陆上机场收不了水机：%v", err)
	}
}

// 中型机场收得了战斗机、收不了 B-17：莫尔兹比港是中型的，凯恩斯的 B-17 转不过去。
func TestTransferRespectsAirfieldSize(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	// 珊瑚海里只有凯恩斯（大型）和莫尔兹比港（中型）两座陆上机场，相距 21 hex，
	// 战斗机挂副油箱也飞不过去，所以这里只测"中型机场收不了 B-17"这一侧。
	bomber := state.GroundAir["af-a-cairns-b17-1"]
	if bomber == nil {
		t.Fatal("找不到凯恩斯的 B-17")
	}
	err := ValidateGroundAirOrder(state, Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{bomber.ID}, Coord: "1611"})
	if err == nil {
		t.Fatal("B-17 不该能转场到中型机场")
	}
	if !strings.Contains(err.Error(), "无法起降") {
		t.Fatalf("错误信息应说明机场无法起降 B-17：%v", err)
	}
}
