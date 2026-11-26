package game

import (
	"fmt"
	"strings"
	"testing"
)

// 机场停机上限：航母机库满了装不下飞机，机场以前完全没有上限，
// 转场可以把任意多飞机堆进同一个机场。目标机场满了就拒绝转场。
func TestAirfieldTransferRejectsFullHangar(t *testing.T) {
	state := State{
		Map:       MapDefinition{Rows: 12, Cols: 12},
		Airfields: map[string]*Airfield{},
		GroundAir: map[string]*AirfieldAirUnit{},
		Units:     map[string]*Unit{},
		Aircraft:  map[string]*AirUnit{},
	}
	state.Airfields["src"] = &Airfield{ID: "src", Faction: "A", Name: "源机场", Coord: "0101", Type: AirfieldLarge, Runways: 2, Strength: 10, MaxStrength: 10, HangarCapacity: 6}
	state.Airfields["dst"] = &Airfield{ID: "dst", Faction: "A", Name: "目标机场", Coord: "0103", Type: AirfieldLarge, Runways: 2, Strength: 10, MaxStrength: 10, HangarCapacity: 2}
	state.GroundAir["a1"] = &AirfieldAirUnit{ID: "a1", DefinitionID: "fighter", AirfieldID: "src", Faction: "A", Model: "F4F 野猫", Kind: "fighter", Status: "hangar", BaseRange: 20, MaxRange: 20}
	state.GroundAir["d1"] = &AirfieldAirUnit{ID: "d1", DefinitionID: "fighter", AirfieldID: "dst", Faction: "A", Model: "F4F 野猫", Kind: "fighter", Status: "hangar", BaseRange: 20, MaxRange: 20}
	state.GroundAir["d2"] = &AirfieldAirUnit{ID: "d2", DefinitionID: "fighter", AirfieldID: "dst", Faction: "A", Model: "F4F 野猫", Kind: "fighter", Status: "hangar", BaseRange: 20, MaxRange: 20}

	order := Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"a1"}, Coord: "0103"}
	err := ValidateGroundAirOrder(state, order)
	if err == nil {
		t.Fatal("目标机场机库已满（2/2），转场应当被拒绝")
	}
	if !strings.Contains(err.Error(), "停放机位已满") {
		t.Fatalf("拒绝理由应说明机库已满，实际：%v", err)
	}

	delete(state.GroundAir, "d2")
	if err := ValidateGroundAirOrder(state, order); err != nil {
		t.Fatalf("腾出一个停机位后转场应当放行：%v", err)
	}
}

// 机场没有机库：老存档没有 hangar_capacity 时补成"无上限"，不能因此拒绝任何转场。
func TestNormalizeAirfieldHangarDefaultsToUnlimited(t *testing.T) {
	state := State{Airfields: map[string]*Airfield{"af": {ID: "af", Runways: 3}}}
	NormalizeAirfieldHangar(&state)
	if got := state.Airfields["af"].HangarCapacity; got != AirfieldHangarUnlimited {
		t.Fatalf("机场默认应当无上限（%d），实际 %d", AirfieldHangarUnlimited, got)
	}
	if err := airfieldHangarFull(state, state.Airfields["af"], 50); err != nil {
		t.Fatalf("默认无上限时 50 架转场也应当放行：%v", err)
	}
}

// 备降机场也会挑满：停机位占满的机场不再接收舰载机，飞机只能去还有空位的机场。
func TestDivertSkipsFullAirfield(t *testing.T) {
	state := State{
		Map:       MapDefinition{Rows: 12, Cols: 12},
		Airfields: map[string]*Airfield{},
		GroundAir: map[string]*AirfieldAirUnit{},
	}
	state.Airfields["full"] = &Airfield{ID: "full", Faction: "A", Name: "满机场", Coord: "0102", Type: AirfieldLarge, Runways: 1, Strength: 5, MaxStrength: 5, HangarCapacity: 1}
	state.Airfields["open"] = &Airfield{ID: "open", Faction: "A", Name: "空机场", Coord: "0104", Type: AirfieldLarge, Runways: 1, Strength: 5, MaxStrength: 5, HangarCapacity: 6}
	state.GroundAir["x1"] = &AirfieldAirUnit{ID: "x1", AirfieldID: "full", Faction: "A", Status: "hangar"}
	plane := &AirUnit{ID: "p1", Faction: "A", Model: "F4F 野猫", Kind: "fighter", Coord: "0101"}

	if airfieldCanAccept(&state, state.Airfields["full"], plane) {
		t.Fatal("停机位已满的机场不该被选作备降点")
	}
	if !airfieldCanAccept(&state, state.Airfields["open"], plane) {
		t.Fatal("还有空位的机场应当可以备降")
	}
	if got := alternateAirfield(&state, plane); got == nil || got.ID != "open" {
		t.Fatalf("备降机场应跳过满的，实际：%+v", got)
	}
}

// 机库满了就不能再往回收：甲板空着也没用，飞机只能在舰队头顶盘旋。
func TestCarrierLandingBlockedWhenHangarFull(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["a2-cv-01"]
	carrier.Card.HangarCapacity = 4
	carrier.Deck.Operation = ""
	state.Aircraft = map[string]*AirUnit{}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("parked-%d", i)
		state.Aircraft[id] = &AirUnit{ID: id, CarrierID: carrier.ID, Faction: "A", Status: "hangar"}
	}

	if reason := landingBlockReason(state, carrier); !strings.Contains(reason, "机库已满") {
		t.Fatalf("机库已满时应当拒绝回收，实际：%q", reason)
	}
	delete(state.Aircraft, "parked-0")
	if reason := landingBlockReason(state, carrier); strings.Contains(reason, "机库已满") {
		t.Fatalf("腾出一个机位后应当可以回收，实际：%q", reason)
	}
}
