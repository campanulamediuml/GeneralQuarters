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
	// 甲板只有 1 位时，第二架落到同编队的另一艘航母上（同一 hex），
	// 既不硬塞进这条甲板，也不转场去几十格外的小机场。
	if current != 2 {
		t.Fatalf("两架都应落地（第二架换一艘航母）：实际降落 %d", current)
	}
	if deckUsed(state, carrier.ID) != 1 {
		t.Fatalf("航母甲板只应接收 1 架：%d", deckUsed(state, carrier.ID))
	}
	if second.CarrierID == carrier.ID {
		t.Fatalf("第二架不该塞进已满的 %s", carrier.ID)
	}
	if diverted := state.GroundAir[second.ID]; diverted != nil {
		t.Fatalf("甲板满不该把飞机送去机场：%+v", diverted)
	}
	second.CarrierID = carrier.ID
	state.GroundAir[second.ID] = &AirfieldAirUnit{ID: second.ID, AirfieldID: "af-b-sentry", Faction: "B", Status: "recovery"}
	view := VisibleAviation(state, "B")
	// 剩余侦察机应把任务组保留下来，并显示已着舰等待下降的算子。
	if len(view.Tasks) != 1 || !strings.HasPrefix(view.Tasks[0].Name, "扇面侦察") {
		t.Fatalf("剩余侦察机应把任务组保留下来：%+v", view.Tasks)
	}
	landedInTask := false
	for _, item := range view.Tasks[0].Aircraft {
		if item.Status == "recovery" {
			landedInTask = true
		}
	}
	if !landedInTask {
		t.Fatalf("已着舰的算子应留在任务组里并显示 recovery：%+v", view.Tasks[0].Aircraft)
	}
}
