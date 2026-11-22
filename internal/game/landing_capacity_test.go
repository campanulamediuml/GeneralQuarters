package game

import (
	"strings"
	"testing"
)

func TestDeckSizedRecoveryKeepsTaskGroup(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	carrier.Deck.Operation = ""
	carrier.Deck.Length = 1
	recon := &AirUnit{ID: "b-cv-04-dive_bomber-x1", CarrierID: carrier.ID, Faction: "B", DefinitionID: "recon", Kind: "recon", Model: "F6F-5P", Status: "recon", Strength: 1, TaskID: "b-cv-04-dive_bomber-x1", TaskKind: "recon", Heading: "E", SectorWidth: 60, ReconRadius: 3, OriginCoord: carrier.Coord, Coord: carrier.Coord, Returning: true, MaxRange: 16, RemainingRange: 6}
	second := &AirUnit{ID: "air-b-11", CarrierID: carrier.ID, Faction: "B", DefinitionID: "recon", Kind: "recon", Model: "F6F-5P", Status: "recon", Strength: 1, TaskID: "b-cv-04-dive_bomber-x1", TaskKind: "recon", Heading: "E", SectorWidth: 60, ReconRadius: 3, OriginCoord: carrier.Coord, Coord: carrier.Coord, Returning: true, MaxRange: 16, RemainingRange: 6}
	state.Aircraft = map[string]*AirUnit{recon.ID: recon, second.ID: second}
	current := 0
	for _, aircraft := range []*AirUnit{recon, second} {
		carrier.Deck.Operation = ""
		if landIfCapacity(&state, aircraft, carrier) {
			current++
		}
	}
	// 甲板只有 1 位时，第二架按规则备降到最近的可用机场：仍然算"降落成功"，但不占航母甲板。
	if current != 2 {
		t.Fatalf("第二架应备降机场而不是卡在空中：实际降落 %d", current)
	}
	if deckUsed(state, carrier.ID) != 1 {
		t.Fatalf("航母甲板只应接收 1 架：%d", deckUsed(state, carrier.ID))
	}
	if diverted := state.GroundAir[second.ID]; diverted == nil || diverted.AirfieldID == "" {
		t.Fatalf("第二架应转为机场地面算子：%+v", diverted)
	}
	second.CarrierID = carrier.ID
	state.GroundAir[second.ID] = &AirfieldAirUnit{ID: second.ID, AirfieldID: "af-b-sentry", Faction: "B", Status: "recovery"}
	view := VisibleAviation(state, "B")
	// leader 是按 ID 排序后的第一架（这里刚着舰，TaskKind 已被清空），因此用任务名判断类型。
	if len(view.Tasks) != 1 || !strings.HasPrefix(view.Tasks[0].Name, "扇面侦察") || view.Tasks[0].Aircraft[0].Status != "recovery" {
		t.Fatalf("剩余侦察机应把任务组保留下来，并显示已着舰等待下降的算子：%+v", view.Tasks)
	}
}
