package game

import "testing"

// 已经挂着鱼雷的飞机拖进"打机场"的编队：保留原挂载、保留整备进度，
// 只回一条"挂载不适合这个任务"的提示——不自动换弹，也不拦命令。
func TestDragArmedPlaneIntoMismatchedStrikeKeepsLoadoutAndHints(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "tb-hit", CarrierID: "b-cv-01", Faction: "B", Model: "B5N2", Kind: "torpedo_bomber",
		Status: "ready", ServiceTurns: 1, Armed: true,
		Loadout: LoadoutTorpedo, AntiGround: 2, AntiShip: 4, BaseRange: 10, MaxRange: 10, RemainingRange: 10,
	}
	state.Aircraft[aircraft.ID] = aircraft
	// 0304 是 af-a-lighthouse（美方机场），对 B 阵营就是对地目标。
	events := configureAirTask(&state, []*AirUnit{aircraft}, Order{Type: "LAUNCH_STRIKE", AircraftID: aircraft.ID, Coord: "0304"})
	if aircraft.Loadout != LoadoutTorpedo {
		t.Fatalf("错配时不该自动换挂载，实际 %s", aircraft.Loadout)
	}
	if aircraft.ServiceTurns != 1 || !aircraft.Armed {
		t.Fatalf("错配不该重置整备进度：service=%d armed=%v", aircraft.ServiceTurns, aircraft.Armed)
	}
	hinted := false
	for _, event := range events {
		if event.Type == "air_loadout_mismatch" {
			hinted = true
		}
	}
	if !hinted {
		t.Fatalf("挂鱼雷打机场应提示挂载不适合：%+v", events)
	}
}

// 空载飞机拖进任务栏：按任务自动配弹，并重新走整备流程。
func TestDragEmptyPlaneIntoStrikeAutoArms(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "tb-empty", CarrierID: "b-cv-01", Faction: "B", Model: "B5N2", Kind: "torpedo_bomber",
		Status: "hangar", Loadout: LoadoutNone, AntiGround: 2, AntiShip: 4, BaseRange: 10, MaxRange: 10,
	}
	state.Aircraft[aircraft.ID] = aircraft
	events := configureAirTask(&state, []*AirUnit{aircraft}, Order{Type: "LAUNCH_STRIKE", AircraftID: aircraft.ID, Coord: "0304"})
	if aircraft.Loadout == LoadoutNone {
		t.Fatal("空载拖进任务栏应自动配弹")
	}
	if aircraft.Loadout.Weapon() != WeaponBomb {
		t.Fatalf("打机场应自动挂炸弹，实际 %s", aircraft.Loadout)
	}
	if aircraft.ServiceTurns != 0 || aircraft.Armed {
		t.Fatalf("自动配弹应重新走整备流程：service=%d armed=%v", aircraft.ServiceTurns, aircraft.Armed)
	}
	for _, event := range events {
		if event.Type == "air_loadout_mismatch" {
			t.Fatalf("空载自动配弹不该弹错配提示：%s", event.Text)
		}
	}
}

// 转场会自动卸弹换副油箱，这件事也要说一声，但不当作错配拦着。
func TestTransferAutoTankIsReported(t *testing.T) {
	state := sandboxState(t)
	aircraft := &AirUnit{
		ID: "tb-transfer", CarrierID: "b-cv-01", Faction: "B", Model: "B5N2", Kind: "torpedo_bomber",
		Status: "ready", ServiceTurns: 1, Armed: true,
		Loadout: LoadoutTorpedo, AntiGround: 2, AntiShip: 4, BaseRange: 10, MaxRange: 10, RemainingRange: 10,
	}
	state.Aircraft[aircraft.ID] = aircraft
	events := configureAirTask(&state, []*AirUnit{aircraft}, Order{Type: "LAUNCH_TRANSFER", AircraftID: aircraft.ID, Coord: "0304"})
	if aircraft.Loadout.Weapon() != WeaponTransfer {
		t.Fatalf("转场应换成副油箱挂载，实际 %s", aircraft.Loadout)
	}
	hinted := false
	for _, event := range events {
		if event.Type == "air_loadout_mismatch" {
			hinted = true
		}
	}
	if !hinted {
		t.Fatalf("自动卸弹转场应提示一句：%+v", events)
	}
}
