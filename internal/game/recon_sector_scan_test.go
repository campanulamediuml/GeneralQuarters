package game

import "testing"

// TestWideSectorScansSideHexes 120° 扇面必须真的覆盖得更宽：除了正前方，还要扫前方左右斜向两格。
// 之前宽扇面只是把发现率减半、扫描范围完全一样，等于 120° 严格劣于 60°。
func TestWideSectorScansSideHexes(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	aircraft.Status = "recon"
	aircraft.Launched = true
	aircraft.TaskID = "sector-narrow"
	aircraft.TaskKind = "recon"
	aircraft.Heading = "W"
	aircraft.SectorWidth = 60
	aircraft.ReconRadius = 4
	aircraft.OriginCoord = carrier.Coord
	aircraft.Coord = carrier.Coord
	aircraft.ReconDistance = 0
	aircraft.Fuel = Fuel{Tank: 8, Internal: 8, Reserve: 4}
	aircraft.MaxRange = 16
	aircraft.SetRange(16)
	narrow := map[string]bool{}
	for turn := 0; turn < 8; turn++ {
		for _, event := range AdvanceAviation(&state) {
			if event.Type == "recon_step_searched" {
				for _, coord := range scannedCoords(event.Text) {
					narrow[coord] = true
				}
			}
		}
	}

	wide := sandboxState(t)
	plane := wide.Aircraft["b-cv-04-dive_bomber-x1"]
	mother := wide.Units[plane.CarrierID]
	plane.Status = "recon"
	plane.Launched = true
	plane.TaskID = "sector-wide"
	plane.TaskKind = "recon"
	plane.Heading = "W"
	plane.SectorWidth = 120
	plane.ReconRadius = 4
	plane.OriginCoord = mother.Coord
	plane.Coord = mother.Coord
	plane.ReconDistance = 0
	plane.Fuel = Fuel{Tank: 8, Internal: 8, Reserve: 4}
	plane.MaxRange = 16
	plane.SetRange(16)
	scannedWide := map[string]bool{}
	for turn := 0; turn < 8; turn++ {
		for _, event := range AdvanceAviation(&wide) {
			if event.Type == "recon_step_searched" {
				for _, coord := range scannedCoords(event.Text) {
					scannedWide[coord] = true
				}
			}
		}
	}

	if len(scannedWide) <= len(narrow) {
		t.Fatalf("120° 扇面应覆盖更多格子：窄 %d 个，宽 %d 个", len(narrow), len(scannedWide))
	}
}
