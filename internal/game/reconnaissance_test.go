package game

import (
	"math"
	"testing"
)

func containsCoord(coords []string, target string) bool {
	for _, coord := range coords {
		if coord == target {
			return true
		}
	}
	return false
}

func TestReconSectorGeometryUsesHeadingAndWidth(t *testing.T) {
	gameMap := MapDefinition{Rows: 7, Cols: 9}
	narrow := ReconSectorHexes(gameMap, "0405", "E", 60, 3)
	if !containsCoord(narrow, "0406") || containsCoord(narrow, "0404") {
		t.Fatalf("60度东向扇面方向错误：%v", narrow)
	}
	wide := ReconSectorHexes(gameMap, "0405", "E", 120, 3)
	if len(wide) <= len(narrow) {
		t.Fatalf("120度扇面没有扩大覆盖：60=%d 120=%d", len(narrow), len(wide))
	}
	for _, coord := range wide {
		if coord == "0405" || Distance("0405", coord) > 3 {
			t.Fatalf("扇面包含顶点或越界距离圈：%s", coord)
		}
	}
	west := ReconSectorHexes(gameMap, "0405", "W", 60, 3)
	if containsCoord(west, "0406") || !containsCoord(west, "0404") {
		t.Fatalf("西向扇面没有跟随中轴旋转：%v", west)
	}
	ring := ReconRingHexes(gameMap, "0405", "E", 60, 2)
	for _, coord := range ring {
		if Distance("0405", coord) != 2 || !containsCoord(narrow, coord) {
			t.Fatalf("距离圈筛选错误：%s", coord)
		}
	}
}

func TestReconDetectionChanceModifiers(t *testing.T) {
	state := State{Map: MapDefinition{Rows: 3, Cols: 5, Cells: []MapCell{{Coord: "0104", Weather: "rain"}}}}
	aircraft := &AirUnit{OriginCoord: "0101", SectorWidth: 60, Faction: "A"}
	members := []Unit{
		{ID: "b1", FleetID: "target", Faction: "B", Type: "BB", Coord: "0104"},
		{ID: "b2", FleetID: "target", Faction: "B", Type: "BB", Coord: "0104"},
		{ID: "b3", FleetID: "target", Faction: "B", Type: "DD", Coord: "0104"},
	}
	if chance := reconDetectionChance(state, aircraft, members); chance != 20 {
		t.Fatalf("舰船、距离和降雨修正错误：%.1f", chance)
	}
	state.Contacts = map[string]map[string]*FleetContact{"A": {"target": {Level: 1}}}
	if chance := reconDetectionChance(state, aircraft, members); chance != 25 {
		t.Fatalf("既有情报修正错误：%.1f", chance)
	}
	aircraft.SectorWidth = 120
	if chance := reconDetectionChance(state, aircraft, members); math.Abs(chance-12.5) > 0.001 {
		t.Fatalf("120度扇面没有将最终发现率减半：%.1f", chance)
	}
}

func TestLaunchReconValidationAndProjection(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-10", "air-b-11")
	valid := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60, ReconRadius: 6}
	if err := ValidateAirOrder(state, "B", valid); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Heading = "N"
	if err := ValidateAirOrder(state, "B", invalid); err == nil {
		t.Fatal("无效扇面航向未被拒绝")
	}
	invalid = valid
	invalid.SectorWidth = 90
	if err := ValidateAirOrder(state, "B", invalid); err == nil {
		t.Fatal("非规则扇面角度未被拒绝")
	}
	invalid = valid
	invalid.ReconRadius = 9
	if err := ValidateAirOrder(state, "B", invalid); err == nil {
		t.Fatal("超过返航燃料的侦察半径未被拒绝")
	}
	invalid = valid
	invalid.AircraftID = "air-b-01"
	if err := ValidateAirOrder(state, "B", invalid); err == nil {
		t.Fatal("单座战斗机被允许执行专用侦察")
	}
	events := ApplyAirOrder(&state, valid)
	aircraft := state.Aircraft["air-b-10"]
	if len(events) != 1 || aircraft.Status != "recon" || aircraft.Coord != "0208" || aircraft.OriginCoord != "0208" || aircraft.SectorWidth != 60 || aircraft.ReconRadius != 6 {
		t.Fatalf("侦察起飞状态错误：%+v", aircraft)
	}
	if state.Units["b-cv-04"].Card.Aircraft != 1 {
		t.Fatal("侦察起飞未扣减机库占用")
	}
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 1 || view.Tasks[0].Kind != "recon" || len(view.Tasks[0].SectorHexes) == 0 {
		t.Fatalf("侦察任务投影错误：%+v", view.Tasks)
	}
	seaplane := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-11", Heading: "W", SectorWidth: 120, ReconRadius: 4}
	if err := ValidateAirOrder(state, "B", seaplane); err != nil {
		t.Fatalf("巡洋舰水上飞机不能起飞侦察：%v", err)
	}
	ApplyAirOrder(&state, seaplane)
	AdvanceAviation(&state)
	if state.Aircraft["air-b-10"].ReconDistance != 2 || state.Aircraft["air-b-11"].ReconDistance != 1 {
		t.Fatalf("普通机和水机每回合速度错误：普通=%d 水机=%d", state.Aircraft["air-b-10"].ReconDistance, state.Aircraft["air-b-11"].ReconDistance)
	}
}

func TestReconDetectionContactAndReturn(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-10", "air-b-11")
	for _, unit := range state.Units {
		if unit.Faction == "A" {
			unit.Type = "BB"
		}
	}
	for index := 0; index < 6; index++ {
		id := "a-extra-" + string(rune('a'+index))
		state.Units[id] = &Unit{ID: id, FleetID: "TG-A", FleetName: "第一战列舰队", Name: id, Type: "BB", Faction: "A", Coord: "0202", Strength: 4, MaxStrength: 4}
	}
	order := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60, ReconRadius: 6}
	ApplyAirOrder(&state, order)
	AdvanceAviation(&state)
	if state.Aircraft["air-b-10"].Coord != "0206" || state.Aircraft["air-b-10"].RemainingRange != 14 {
		t.Fatalf("侦察机第一回合移动或耗油错误：%+v", state.Aircraft["air-b-10"])
	}
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	aircraft := state.Aircraft["air-b-10"]
	if aircraft.TrackingFleetID != "TG-A" || aircraft.Coord != "0202" || aircraft.ReconDistance != 6 {
		t.Fatalf("发现后没有跟踪最高发现率舰队：%+v", aircraft)
	}
	contact := state.Contacts["B"]["TG-A"]
	if contact == nil || contact.Level != 1 || contact.Count != 20 {
		t.Fatalf("侦察情报未由服务器记录：%+v", contact)
	}
	fleets := VisibleFleets(state, "B")
	var enemy *FleetGroup
	for index := range fleets {
		if fleets[index].Faction == "A" {
			enemy = &fleets[index]
		}
	}
	if enemy == nil || enemy.Count != 20 || enemy.Coord != "0202" {
		t.Fatalf("情报1敌方舰队投影错误：%+v", enemy)
	}
	for _, member := range enemy.Members {
		if member.Type != "" || member.Name != "未知舰船" || member.Card.MainGunCount != 0 {
			t.Fatalf("情报1泄露了舰船细节：%+v", member)
		}
	}

	aircraft.RemainingRange = Distance(aircraft.Coord, "0208")
	AdvanceAviation(&state)
	if aircraft.TrackingFleetID != "" || !aircraft.Returning || aircraft.Coord != "0204" {
		t.Fatalf("燃料余量没有触发返航：%+v", aircraft)
	}
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	if aircraft.Status != "hangar" || aircraft.Coord != "" || aircraft.ReconRadius != 0 || aircraft.Returning {
		t.Fatalf("返航后未自动落舰并清理任务：%+v", aircraft)
	}
	if state.Units["b-cv-04"].Card.Aircraft != 2 {
		t.Fatal("返航后机库占用未恢复")
	}
	if state.Contacts["B"]["TG-A"] == nil {
		t.Fatal("返航不应清除已经取得的情报")
	}
}

func TestReconRadiusDefaultsToAircraftLimit(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "air-b-10", "air-b-11")
	order := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-10", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	if state.Aircraft["air-b-10"].ReconRadius != 8 {
		t.Fatalf("省略半径时未使用服务端默认值：%+v", state.Aircraft["air-b-10"])
	}
}
