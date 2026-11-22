package game

import (
	"math"
	"testing"
)

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
	reconInitial := state.Units["b-cv-04"].Card.Aircraft
	setAircraftReady(&state, "b-cv-04-dive_bomber-x1", "air-b-11")
	valid := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 60}
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
	// 侦察半径不可设置：Order 里已经没有这个字段，客户端多传也只是被忽略。
	invalid = valid
	invalid.AircraftID = "b-cv-01-fighter-x1"
	if err := ValidateAirOrder(state, "B", invalid); err == nil {
		t.Fatal("单座战斗机被允许执行专用侦察")
	}
	events := ApplyAirOrder(&state, valid)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	if want := ReconRadiusLimit(*aircraft); len(events) != 1 || aircraft.Status != "recon" || aircraft.Coord != "0208" || aircraft.OriginCoord != "0208" || aircraft.SectorWidth != 60 || aircraft.ReconRadius != want {
		t.Fatalf("侦察起飞状态错误（期望半径 %d）：%+v", want, aircraft)
	}
	if state.Units["b-cv-04"].Card.Aircraft != reconInitial-1 {
		t.Fatal("侦察起飞未扣减机库占用")
	}
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 1 || view.Tasks[0].Kind != "recon" || len(view.Tasks[0].SectorHexes) == 0 {
		t.Fatalf("侦察任务投影错误：%+v", view.Tasks)
	}
	seaplane := Order{Type: "LAUNCH_RECON", AircraftID: "air-b-11", Heading: "W", SectorWidth: 120}
	if err := ValidateAirOrder(state, "B", seaplane); err != nil {
		t.Fatalf("巡洋舰水上飞机不能起飞侦察：%v", err)
	}
	ApplyAirOrder(&state, seaplane)
	AdvanceAviation(&state)
	if state.Aircraft["b-cv-04-dive_bomber-x1"].ReconDistance != 2 || state.Aircraft["air-b-11"].ReconDistance != 1 {
		t.Fatalf("普通机和水机每回合速度错误：普通=%d 水机=%d", state.Aircraft["b-cv-04-dive_bomber-x1"].ReconDistance, state.Aircraft["air-b-11"].ReconDistance)
	}
}

func TestReconSectorsArePerAircraftAndEmbarkedPositionFollowsCarrier(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "b-cv-01-dive_bomber-x1")
	first := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	ApplyAirOrder(&state, first)
	firstCarrier := state.Units[state.Aircraft["b-cv-01-dive_bomber-x1"].CarrierID]
	firstCarrier.Coord = "0308"
	firstCarrier.Deck.Operation = ""
	setAircraftReady(&state, "b-cv-01-torpedo_bomber-x1")
	second := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-torpedo_bomber-x1", TaskID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	ApplyAirOrder(&state, second)
	ApplyAirOrder(&state, Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 120})
	movingCarrier := state.Units[state.Aircraft["b-cv-04-dive_bomber-x1"].CarrierID]
	destination := LegalMoves(state, movingCarrier)[0]
	ApplyImmediateMove(&state, Order{Type: "SET_MOVE", UnitID: movingCarrier.ID, Coord: destination})

	if state.Aircraft["b-cv-04-dive_bomber-x1"].Coord != destination {
		t.Fatalf("舰上侦察机没有跟随母舰：%+v", state.Aircraft["b-cv-04-dive_bomber-x1"])
	}
	if state.Aircraft["b-cv-01-dive_bomber-x1"].Coord != "0208" || state.Aircraft["b-cv-01-torpedo_bomber-x1"].Coord != "0308" {
		t.Fatalf("已升空侦察机被错误跟随母舰：%s / %s", state.Aircraft["b-cv-01-dive_bomber-x1"].Coord, state.Aircraft["b-cv-01-torpedo_bomber-x1"].Coord)
	}
	view := VisibleAviation(state, "B")
	if len(view.Tasks) != 2 {
		t.Fatalf("侦察任务数量错误：%+v", view.Tasks)
	}
	var joined *AirTask
	var embarked *AirTask
	for index := range view.Tasks {
		task := &view.Tasks[index]
		if task.ID == "b-cv-01-dive_bomber-x1" {
			joined = task
		} else {
			embarked = task
		}
	}
	if joined == nil || len(joined.ReconSectors) != 2 {
		t.Fatalf("同组侦察机没有逐机投影：%+v", joined)
	}
	origins := map[string]string{}
	for _, sector := range joined.ReconSectors {
		origins[sector.AircraftID] = sector.OriginCoord
		if len(sector.SectorHexes) == 0 || sector.CurrentCoord != sector.OriginCoord || sector.DistanceReference == nil {
			t.Fatalf("独立侦察扇面或位置缺失：%+v", sector)
		}
	}
	if origins["b-cv-01-dive_bomber-x1"] != "0208" || origins["b-cv-01-torpedo_bomber-x1"] != "0308" {
		t.Fatalf("逐机扇面顶点错误：%+v", origins)
	}
	if embarked == nil || len(embarked.ReconSectors) != 1 {
		t.Fatalf("舰上侦察机投影缺失：%+v", embarked)
	}
	pending := embarked.ReconSectors[0]
	if pending.Status != "service" || pending.CurrentCoord != destination || pending.OriginCoord != destination || len(pending.SectorHexes) != 0 {
		t.Fatalf("舰上侦察机没有使用母舰当前位置：%+v", pending)
	}
}

func TestReconDetectionContactAndReturn(t *testing.T) {
	state := sandboxState(t)
	reconInitial := state.Units["b-cv-04"].Card.Aircraft
	setAircraftReady(&state, "b-cv-04-dive_bomber-x1", "air-b-11")
	for _, unit := range state.Units {
		if unit.Faction == "A" {
			unit.Type = "BB"
		}
	}
	for index := 0; index < 6; index++ {
		id := "a-extra-" + string(rune('a'+index))
		state.Units[id] = &Unit{ID: id, FleetID: "TG-A", FleetName: "第一战列舰队", Name: id, Type: "BB", Faction: "A", Coord: "0202", Strength: 4, MaxStrength: 4}
	}
	order := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	ApplyAirOrder(&state, order)
	AdvanceAviation(&state)
	// 主燃油 = 基础航程 × 副油箱系数，飞 2 格消耗 2 点；副油箱优先消耗（规则书 49 行）。
	// Range() 不含备用油。
	recon := state.Aircraft["b-cv-04-dive_bomber-x1"]
	fullRange := SortieBaseRange(recon.BaseRange, recon.Loadout)
	if recon.Coord != "0206" || recon.Range() != fullRange-2 {
		t.Fatalf("侦察机第一回合移动或耗油错误（期望航程%d）：%+v", fullRange-2, recon)
	}
	if recon.Fuel.Tank != fullRange-recon.BaseRange-2 || recon.Fuel.Internal != recon.BaseRange {
		t.Fatalf("副油箱应当优先消耗：%+v", recon.Fuel)
	}
	AdvanceAviation(&state)
	AdvanceAviation(&state)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
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
	// 情报等级 1 只显示一个算子，位置误差 5 hex 以内（规则书 144 行）。
	if enemy == nil || enemy.Count != 1 || len(enemy.Members) != 1 {
		t.Fatalf("情报1敌方舰队投影错误：%+v", enemy)
	}
	if offset := Distance(enemy.Coord, "0202"); offset > 5 {
		t.Fatalf("情报1的位置误差超出上限：显示%s，真实0202，误差%d", enemy.Coord, offset)
	}
	for _, member := range enemy.Members {
		if member.Type != "" || member.Card.MainGunCount != 0 || member.Strength != 0 {
			t.Fatalf("情报1泄露了舰船细节：%+v", member)
		}
	}

	// 本用例只验证一次出动和返航落舰：玩家点掉自动循环（循环行为有专项用例）。
	cancel := Order{Type: "SET_AIR_CYCLE", AircraftID: aircraft.ID}
	if err := ValidateAirOrder(state, "B", cancel); err != nil {
		t.Fatalf("取消循环的命令被拒绝：%v", err)
	}
	ApplyAirOrder(&state, cancel)
	aircraft.SetRange(Distance(aircraft.Coord, "0208"))
	AdvanceAviation(&state)
	if aircraft.TrackingFleetID != "" || !aircraft.Returning {
		t.Fatalf("燃料余量没有触发返航：%+v", aircraft)
	}
	// 返航要飞若干回合（每回合 2 格），一直推进到落舰。
	for turn := 0; turn < 12 && aircraft.Status != "hangar"; turn++ {
		AdvanceAviation(&state)
	}
	if aircraft.Status != "hangar" || aircraft.Coord != state.Units[aircraft.CarrierID].Coord || aircraft.ReconRadius != 0 || aircraft.Returning {
		t.Fatalf("返航后未自动落舰并清理任务：%+v", aircraft)
	}
	if state.Units["b-cv-04"].Card.Aircraft != reconInitial {
		t.Fatal("返航后机库占用未恢复")
	}
	if state.Contacts["B"]["TG-A"] == nil {
		t.Fatal("返航不应清除已经取得的情报")
	}
}

func TestReconRadiusDefaultsToAircraftLimit(t *testing.T) {
	state := sandboxState(t)
	setAircraftReady(&state, "b-cv-04-dive_bomber-x1", "air-b-11")
	order := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-04-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", order); err != nil {
		t.Fatal(err)
	}
	ApplyAirOrder(&state, order)
	if want := ReconRadiusLimit(*state.Aircraft["b-cv-04-dive_bomber-x1"]); state.Aircraft["b-cv-04-dive_bomber-x1"].ReconRadius != want {
		t.Fatalf("省略半径时未使用服务端默认值（期望%d）：%+v", want, state.Aircraft["b-cv-04-dive_bomber-x1"])
	}
}

func TestAttackAircraftCanLaunchSectorSearch(t *testing.T) {
	for _, test := range []struct {
		id      string
		kind    string
		model   string
		carrier string
	}{
		{id: "b-cv-02-dive_bomber-x1", kind: "dive_bomber", model: "SB2C 地狱俯冲者", carrier: "b-cv-02"},
		{id: "b-cv-03-torpedo_bomber-x1", kind: "torpedo_bomber", model: "TBF 复仇者", carrier: "b-cv-03"},
	} {
		t.Run(test.model, func(t *testing.T) {
			state := sandboxState(t)
			setAircraftReady(&state, test.id)
			var projected *AirUnit
			for _, carrier := range VisibleAviation(state, "B").Carriers {
				for index := range carrier.Aircraft {
					if carrier.Aircraft[index].ID == test.id {
						projected = &carrier.Aircraft[index]
					}
				}
			}
			if projected == nil || projected.ReconRadiusLimit != ReconRadiusLimit(*state.Aircraft[test.id]) || projected.ReconRadiusLimit == 0 {
				t.Fatalf("攻击机缺少服务端搜索半径：%+v", projected)
			}
			order := Order{Type: "LAUNCH_RECON", AircraftID: test.id, Heading: "W", SectorWidth: 60}
			if err := ValidateAirOrder(state, "B", order); err != nil {
				t.Fatalf("攻击机不能分配搜索任务：%v", err)
			}
			initialAircraft := state.Units[test.carrier].Card.Aircraft
			ApplyAirOrder(&state, order)
			aircraft := state.Aircraft[test.id]
			if aircraft.Kind != test.kind || aircraft.Status != "recon" || aircraft.TaskKind != "recon" || aircraft.ReconRadius != projected.ReconRadiusLimit {
				t.Fatalf("攻击机搜索任务状态错误：%+v", aircraft)
			}
			tasks := VisibleAviation(state, "B").Tasks
			if len(tasks) != 1 || tasks[0].Kind != "recon" || len(tasks[0].SectorHexes) == 0 || state.Units[test.carrier].Card.Aircraft != initialAircraft-1 {
				t.Fatalf("攻击机搜索投影或载机数错误：%+v", tasks)
			}
			AdvanceAviation(&state)
			if aircraft.ReconDistance != 2 {
				t.Fatalf("攻击机没有按普通飞机速度搜索：%+v", aircraft)
			}
		})
	}
}
