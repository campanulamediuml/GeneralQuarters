package game

import "testing"

// TestReconScansOwnAndForwardHexEveryStep 侦察每步判"自己所在格 + 正前方一格"：
// 去程和返程都会经过同一片海域，所以同一格会被扫到多次（规则书 571 行）。
func TestReconScansOwnAndForwardHexEveryStep(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	aircraft.Status = "recon"
	aircraft.Launched = true
	aircraft.TaskID = "recon-scan"
	aircraft.TaskKind = "recon"
	aircraft.Heading = "W"
	aircraft.SectorWidth = 60
	aircraft.ReconRadius = 3
	aircraft.OriginCoord = carrier.Coord
	aircraft.Coord = carrier.Coord
	aircraft.ReconDistance = 0
	aircraft.Fuel = Fuel{Tank: 8, Internal: 8, Reserve: 4}
	aircraft.MaxRange = 16
	aircraft.SetRange(16)

	scans := map[string]int{}
	for turn := 0; turn < 14; turn++ {
		for _, event := range AdvanceAviation(&state) {
			if event.Type != "recon_step_searched" {
				continue
			}
			for _, coord := range scannedCoords(event.Text) {
				scans[coord]++
			}
		}
	}
	// 半径 3、从母舰往西：0208→0207→0206→0205，再加返航，共 4 个格子。
	if len(scans) < 4 {
		t.Fatalf("侦察应该扫过航线上每个格子，实际只有 %d 个：%v", len(scans), scans)
	}
	// 起飞点与最远点之间的格子往返都会经过，至少被扫两次。
	repeated := 0
	for _, count := range scans {
		if count >= 2 {
			repeated++
		}
	}
	if repeated < 3 {
		t.Fatalf("往返应该重复扫到多个格子，实际只有 %d 个被扫了两次以上：%v", repeated, scans)
	}
}

// scannedCoords 从"XXXX扫描A、B"文本里取出被扫描的格子。
func scannedCoords(text string) []string {
	coords := []string{}
	for index := 0; index+4 <= len(text); index++ {
		chunk := text[index : index+4]
		if !isDigitRange(chunk) {
			continue
		}
		// 只收"扫描"之后、括号之前的坐标。
		coords = append(coords, chunk)
		index += 3
	}
	return coords
}

func isDigitRange(chunk string) bool {
	for index := 0; index < 4; index++ {
		if chunk[index] < '0' || chunk[index] > '9' {
			return false
		}
	}
	return true
}
