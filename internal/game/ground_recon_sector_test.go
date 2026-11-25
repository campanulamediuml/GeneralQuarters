package game

import "testing"

// TestGroundReconTaskExposesSector 机场侦察任务必须带上扇面几何，
// 否则地图上画不出黄色搜索扇面，玩家不知道飞机在扫哪一片。
func TestGroundReconTaskExposesSector(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.GroundAir["af-b-dive_bomber-1"]
	if aircraft == nil {
		t.Fatal("缺少机场算子 af-b-dive_bomber-1")
	}
	aircraft.Status = "recon"
	aircraft.TaskID = "af-b-recon-1"
	aircraft.TaskKind = "recon"
	aircraft.Heading = "E"
	aircraft.SectorWidth = 60
	aircraft.ReconRadius = 4
	aircraft.ReconDistance = 1
	aircraft.OriginCoord = ""
	aircraft.Coord = "0303"

	view := GroundAviationView(state, "B")
	var recon *AirTask
	for index := range view.Tasks {
		if view.Tasks[index].Kind == "recon" {
			recon = &view.Tasks[index]
		}
	}
	if recon == nil {
		t.Fatal("机场侦察任务没有出现在任务列表里")
	}
	if len(recon.ReconSectors) != 1 {
		t.Fatalf("机场侦察任务应带一份扇面投影：%+v", recon.ReconSectors)
	}
	sector := recon.ReconSectors[0]
	if sector.Heading != "E" || sector.SectorWidth != 60 {
		t.Fatalf("扇面参数不对：%+v", sector)
	}
	if len(sector.SectorHexes) == 0 {
		t.Fatal("扇面没有展开成 hex 列表，地图无法渲染")
	}
	if sector.OriginCoord == "" {
		t.Fatal("扇面缺少顶点坐标")
	}
}
