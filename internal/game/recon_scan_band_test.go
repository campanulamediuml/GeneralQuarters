package game

import "testing"

// freezeFleets 让沙盒里的舰船不再移动，侦察原点才能在测试期间保持不变。
func freezeFleets(state *State) {
	for _, unit := range state.Units {
		unit.MoveCooldown = 99
	}
}

// TestReconScanBandMatchesActualScanning UI 显示的搜索区域必须等于实际会判定的格子。
// 判定是"每步扫自己 + 正前方（120° 再加两侧）"，所以投影的搜索带必须覆盖实际扫到的每一格，
// 否则玩家按 UI 规划会系统性落空。
func TestReconScanBandMatchesActualScanning(t *testing.T) {
	state := sandboxState(t)
	freezeFleets(&state)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	aircraft.Status = "recon"
	aircraft.Launched = true
	aircraft.TaskID = "band-probe"
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

	origin := aircraft.OriginCoord
	actuallyScanned := map[string]bool{}
	for turn := 0; turn < 12; turn++ {
		for _, event := range AdvanceAviation(&state) {
			if event.Type != "recon_step_searched" {
				continue
			}
			for _, coord := range scannedCoords(event.Text) {
				actuallyScanned[coord] = true
			}
		}
		if aircraft.Status != "recon" {
			break
		}
	}
	if len(actuallyScanned) == 0 {
		t.Fatal("侦察没有产生任何扫描记录")
	}
	band := ReconScanBandHexes(state.Map, origin, "W", 60, 3)
	bandSet := map[string]bool{}
	for _, coord := range band {
		bandSet[coord] = true
	}
	for coord := range actuallyScanned {
		if !bandSet[coord] {
			t.Fatalf("实际扫到 %s，但投影的搜索带里没有：%v", coord, band)
		}
	}
	// 搜索带不该退回"整块扇面"：格数要和航迹同量级。
	if len(band) > 3*len(actuallyScanned)+6 {
		t.Fatalf("搜索带 %d 格明显大于实际覆盖 %d 格，仍在展示扇面", len(band), len(actuallyScanned))
	}
}

// TestWideSectorCoversMoreThanNarrow 120° 每步比 60° 多扫前方左右两格，
// 但两者都只扫航迹带，不会退回整圈扇面。
func TestWideSectorCoversMoreThanNarrow(t *testing.T) {
	run := func(width int) map[string]bool {
		state := sandboxState(t)
		freezeFleets(&state)
		aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
		carrier := state.Units[aircraft.CarrierID]
		aircraft.Status = "recon"
		aircraft.Launched = true
		aircraft.TaskID = "sector-probe"
		aircraft.TaskKind = "recon"
		aircraft.Heading = "W"
		aircraft.SectorWidth = width
		aircraft.ReconRadius = 3
		aircraft.OriginCoord = carrier.Coord
		aircraft.Coord = carrier.Coord
		aircraft.ReconDistance = 0
		aircraft.Fuel = Fuel{Tank: 8, Internal: 8, Reserve: 4}
		aircraft.MaxRange = 16
		aircraft.SetRange(16)
		scanned := map[string]bool{}
		for turn := 0; turn < 12; turn++ {
			for _, event := range AdvanceAviation(&state) {
				if event.Type != "recon_step_searched" {
					continue
				}
				for _, coord := range scannedCoords(event.Text) {
					scanned[coord] = true
				}
			}
			if aircraft.Status != "recon" {
				break
			}
		}
		return scanned
	}
	narrow := run(60)
	wide := run(120)
	if len(narrow) < 4 {
		t.Fatalf("60° 一趟应覆盖航线上至少 4 格：%v", narrow)
	}
	if len(wide) <= len(narrow) {
		t.Fatalf("120° 应比 60° 覆盖更宽：60°=%d，120°=%d", len(narrow), len(wide))
	}
	// 60° 每步 2 格，航线 4 格往返，覆盖应在个位数，不该是整圈扇面的十几格。
	if len(narrow) > 10 {
		t.Fatalf("60° 覆盖 %d 格，已经不是航迹带了：%v", len(narrow), narrow)
	}
}

// TestReconCurrentRingIsArc 本回合高亮的必须是整段弧，而不是飞机所在的那一个格子。
// 实际判定扫的是"当前距离圈 + 下一距离圈 ∩ 扇面"，高亮必须和它一致。
func TestReconCurrentRingIsArc(t *testing.T) {
	state := sandboxState(t)
	freezeFleets(&state)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	aircraft.Status = "recon"
	aircraft.Heading = "W"
	aircraft.SectorWidth = 60
	aircraft.ReconRadius = 4
	aircraft.OriginCoord = carrier.Coord
	aircraft.Coord = "0606"

	sector := projectReconSector(state, aircraft)
	if len(sector.CurrentRingHexes) < 3 {
		t.Fatalf("当前回合应该高亮整段弧（至少 3 格），实际只有 %d 格：%v", len(sector.CurrentRingHexes), sector.CurrentRingHexes)
	}
	ring := Distance(carrier.Coord, "0606")
	for _, coord := range sector.CurrentRingHexes {
		got := Distance(carrier.Coord, coord)
		if got != ring && got != ring+1 {
			t.Fatalf("高亮格 %s 不在当前/下一距离圈上（距离 %d，应为 %d 或 %d）", coord, got, ring, ring+1)
		}
	}
	inside := map[string]bool{}
	for _, coord := range sector.SectorHexes {
		inside[coord] = true
	}
	for _, coord := range sector.CurrentRingHexes {
		if !inside[coord] {
			t.Fatalf("高亮格 %s 不在扇面覆盖范围内", coord)
		}
	}
}
