package game

import "testing"

// 规则书 59 行：转场航程 = base × 1.3 × 1.3，例如基础 9 → 15。
func TestTransferRangeAddsThirtyPercentTwice(t *testing.T) {
	if got := SortieBaseRange(9, LoadoutTransfer); got != 15 {
		t.Fatalf("转场航程错误：%d（期望15）", got)
	}
	if got := SortieBaseRange(10, LoadoutTransfer); got != 17 {
		t.Fatalf("转场航程错误：%d（期望17）", got)
	}
}

// 规则书 82 行：炸弹且不挂副油箱 = 重型炸弹，航程 -30%，对地/对舰 +2。
func TestHeavyBombIsBombWithoutTank(t *testing.T) {
	if !LoadoutBomb.IsHeavyBomb() {
		t.Fatal("炸弹且不挂副油箱应当识别为重型炸弹")
	}
	if LoadoutBombTank.IsHeavyBomb() {
		t.Fatal("挂副油箱的炸弹不是重型炸弹")
	}
	if got := SortieBaseRange(10, LoadoutBomb); got != 7 {
		t.Fatalf("重型炸弹航程错误：%d（期望7）", got)
	}
	fuel := MountedFuel(10, LoadoutBomb)
	if fuel.Tank != 0 {
		t.Fatalf("重型炸弹不该有副油箱里程：%+v", fuel)
	}
	if AttackBonus(LoadoutBomb) != 2 {
		t.Fatal("重型炸弹应当对地/对舰 +2")
	}
	if AttackBonus(LoadoutBombTank) != 0 {
		t.Fatal("挂副油箱的炸弹不应有攻击加成")
	}
}

// 挂副油箱的挂载都应当有副油箱里程；不挂的没有（规则书 46、51 行）。
func TestTankPresenceFollowsLoadout(t *testing.T) {
	for _, loadout := range LoadoutOrder {
		fuel := MountedFuel(10, loadout)
		if loadout.HasTank() && loadout != LoadoutTransfer && fuel.Tank <= 0 {
			t.Fatalf("%s 应当有副油箱里程：%+v", loadout, fuel)
		}
		if !loadout.HasTank() && fuel.Tank != 0 {
			t.Fatalf("%s 不应有副油箱里程：%+v", loadout, fuel)
		}
	}
	// 不挂副油箱时航程就是基础航程
	if got := SortieBaseRange(10, LoadoutAir); got != 10 {
		t.Fatalf("不挂副油箱时航程应为基础航程：%d", got)
	}
}

// 各机种的可用挂载：攻击机无对空、战斗机无炸弹鱼雷、水上飞机只有侦察与转场。
func TestLoadoutOptionsByAircraftKind(t *testing.T) {
	has := func(options []Loadout, want Loadout) bool {
		for _, option := range options {
			if option == want {
				return true
			}
		}
		return false
	}
	// 俯冲轰炸机只能挂炸弹：D3A1/SBD 都没有鱼雷挂架。
	diveBomber := LoadoutOptions("dive_bomber", false, 2, 2)
	if has(diveBomber, LoadoutAirTank) || has(diveBomber, LoadoutAir) {
		t.Fatalf("攻击机不应有对空选项：%v", diveBomber)
	}
	if !has(diveBomber, LoadoutBombTank) {
		t.Fatalf("俯冲轰炸机应当能挂炸弹：%v", diveBomber)
	}
	if has(diveBomber, LoadoutTorpedoTank) || has(diveBomber, LoadoutTorpedo) {
		t.Fatalf("俯冲轰炸机不应有鱼雷选项：%v", diveBomber)
	}
	torpedoBomber := LoadoutOptions("torpedo_bomber", false, 2, 2)
	if !has(torpedoBomber, LoadoutBombTank) || !has(torpedoBomber, LoadoutTorpedoTank) {
		t.Fatalf("鱼雷机应当能挂炸弹和鱼雷：%v", torpedoBomber)
	}
	fighter := LoadoutOptions("fighter", false, 0, 0)
	if has(fighter, LoadoutBomb) || has(fighter, LoadoutTorpedo) {
		t.Fatalf("战斗机不应有炸弹鱼雷选项：%v", fighter)
	}
	if !has(fighter, LoadoutAirTank) || !has(fighter, LoadoutAir) {
		t.Fatalf("战斗机应当有对空选项：%v", fighter)
	}
	seaplane := LoadoutOptions("recon", true, 0, 0)
	if len(seaplane) != 2 || !has(seaplane, LoadoutReconTank) || !has(seaplane, LoadoutTransfer) {
		t.Fatalf("水上飞机只能挂侦察与转场：%v", seaplane)
	}
}

// 弹药挂载只看飞机能力：对地/对舰都为 0 时，连攻击机也不给炸弹/鱼雷。
func TestLoadoutAmmoRequiresGroundOrShipValue(t *testing.T) {
	has := func(options []Loadout, want Weapon) bool {
		for _, option := range options {
			if option.Weapon() == want {
				return true
			}
		}
		return false
	}
	blank := LoadoutOptions("dive_bomber", false, 0, 0)
	if has(blank, WeaponBomb) || has(blank, WeaponTorpedo) {
		t.Fatalf("对地0、对舰0的飞机不应有炸弹/鱼雷：%v", blank)
	}
	groundOnly := LoadoutOptions("fighter", false, 1, 0)
	if !has(groundOnly, WeaponBomb) {
		t.Fatalf("有对地值的战斗机应当能挂炸弹：%v", groundOnly)
	}
	if has(groundOnly, WeaponTorpedo) {
		t.Fatalf("对舰0的战斗机不应能挂鱼雷：%v", groundOnly)
	}
	shipOnly := LoadoutOptions("torpedo_bomber", false, 0, 2)
	if !has(shipOnly, WeaponBomb) || !has(shipOnly, WeaponTorpedo) {
		t.Fatalf("有对舰值的鱼雷机应当能挂炸弹和鱼雷：%v", shipOnly)
	}
}

// 默认挂载：对舰优先鱼雷，无法挂鱼雷则炸弹；对地一律炸弹；战斗机对空。
func TestDefaultLoadoutByTarget(t *testing.T) {
	cases := []struct {
		name       string
		kind       string
		antiGround int
		antiShip   int
		task       string
		target     StrikeTarget
		want       Loadout
	}{
		{"鱼雷机对舰", "torpedo_bomber", 2, 2, "attack", StrikeTargetShip, LoadoutTorpedoTank},
		{"俯冲轰炸机对舰", "dive_bomber", 2, 2, "attack", StrikeTargetShip, LoadoutBombTank},
		{"鱼雷机对地", "torpedo_bomber", 2, 2, "attack", StrikeTargetAir, LoadoutBombTank},
		{"俯冲轰炸机对地", "dive_bomber", 2, 2, "attack", StrikeTargetAir, LoadoutBombTank},
		{"战斗机护航对舰", "fighter", 0, 0, "attack", StrikeTargetShip, LoadoutAirTank},
		{"战斗机护航对地", "fighter", 0, 0, "attack", StrikeTargetAir, LoadoutAirTank},
		{"战斗机CAP", "fighter", 0, 0, "cap", StrikeTargetShip, LoadoutAirTank},
		{"侦察机侦察", "recon", 0, 0, "recon", StrikeTargetShip, LoadoutReconTank},
		{"转场", "fighter", 0, 0, "transfer", StrikeTargetShip, LoadoutTransfer},
	}
	for _, item := range cases {
		if got := DefaultLoadout(item.kind, false, item.antiGround, item.antiShip, item.task, item.target); got != item.want {
			t.Fatalf("%s 默认挂载错误：%s（期望%s）", item.name, got, item.want)
		}
	}
}

// 默认挂载一律带副油箱（航空队的正常出击配置）。
func TestDefaultLoadoutAlwaysCarriesTank(t *testing.T) {
	for _, kind := range []string{"fighter", "dive_bomber", "torpedo_bomber", "recon"} {
		for _, task := range []string{"cap", "recon", "attack", "transfer"} {
			for _, target := range []StrikeTarget{StrikeTargetShip, StrikeTargetAir} {
				got := DefaultLoadout(kind, false, 2, 2, task, target)
				if !got.HasTank() {
					t.Fatalf("%s/%s/%s 默认挂载应当带副油箱：%s", kind, task, target, got)
				}
			}
		}
	}
}

// 机库里手动选过挂载之后，分配任务不会覆盖它。
func TestManualLoadoutSurvivesTaskAssignment(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-01-dive_bomber-x1"]
	if err := ValidateOrder(state, "B", Order{Type: "SET_LOADOUT", AircraftIDs: []string{aircraft.ID}, Loadout: string(LoadoutBomb)}); err != nil {
		t.Fatalf("机库换挂载被拒绝：%v", err)
	}
	ApplyLoadoutOrder(&state, Order{Type: "SET_LOADOUT", AircraftIDs: []string{aircraft.ID}, Loadout: string(LoadoutBomb)})
	if aircraft.Loadout != LoadoutBomb {
		t.Fatalf("手动挂载没有生效：%s", aircraft.Loadout)
	}
	// 分配对舰攻击任务：默认是鱼雷，但不应覆盖手动选的重型炸弹
	configureAirTask(&state, []*AirUnit{aircraft}, Order{Type: "LAUNCH_STRIKE", Coord: "0202"})
	if aircraft.Loadout != LoadoutBomb {
		t.Fatalf("分配任务覆盖了手动挂载：%s", aircraft.Loadout)
	}
}

// 非法组合被拒绝。
func TestValidateLoadoutRejectsMismatch(t *testing.T) {
	if err := ValidateLoadout("fighter", false, 0, 0, LoadoutBombTank); err == nil {
		t.Fatal("对地0、对舰0的战斗机不应能挂炸弹")
	}
	if err := ValidateLoadout("fighter", false, 0, 0, LoadoutTorpedo); err == nil {
		t.Fatal("对地0、对舰0的战斗机不应能挂鱼雷")
	}
	if err := ValidateLoadout("dive_bomber", false, 2, 2, LoadoutAirTank); err == nil {
		t.Fatal("攻击机不应能选对空挂载")
	}
	if err := ValidateLoadout("dive_bomber", false, 2, 2, LoadoutTorpedo); err == nil {
		t.Fatal("俯冲轰炸机不应能挂鱼雷")
	}
	if err := ValidateLoadout("recon", true, 0, 0, LoadoutTorpedoTank); err == nil {
		t.Fatal("水上飞机不应能挂鱼雷")
	}
	if err := ValidateLoadout("dive_bomber", false, 2, 2, LoadoutBombTank); err != nil {
		t.Fatalf("合法组合被拒绝：%v", err)
	}
	if err := ValidateLoadout("fighter", false, 1, 0, LoadoutBombTank); err != nil {
		t.Fatalf("有对地值的战斗机应当能挂炸弹：%v", err)
	}
}

// 空战爆发后抛弃副油箱：只损失还没烧掉的那部分（规则书 48 行）。
func TestDogfightDropsTank(t *testing.T) {
	state := sandboxState(t)
	attacker := state.Aircraft["b-cv-01-dive_bomber-x1"]
	attacker.Status = "attack"
	attacker.TaskKind = "attack"
	applyLoadout(state, attacker)
	before := attacker.Range()
	tank := attacker.Fuel.Tank
	if tank <= 0 {
		t.Fatalf("攻击编队应当带副油箱：%+v", attacker.Fuel)
	}
	report := &AirCombatReport{}
	resolveDogfight(&state, []*AirUnit{attacker}, nil, report)
	if !attacker.TankDropped {
		t.Fatal("空战后应当抛弃副油箱")
	}
	// "没有护航就抛弹转护航"这条规则已经取消：轰炸机带着炸弹进空战，不该被结算悄悄改派。
	if !attacker.Loadout.CarriesWeapon() {
		t.Fatal("空战不应让轰炸机抛弃炸弹")
	}
	// 抛弃副油箱只减少损失（丢掉副油箱剩余量），不会凭空加航程。
	if got := attacker.Range(); got != before-tank {
		t.Fatalf("抛弃副油箱后航程错误：%d → %d（期望%d，副油箱%d）", before, got, before-tank, tank)
	}
}

// 有护航战斗机时，俯冲轰炸机保留炸弹、只抛弃副油箱（规则书 811–813 行）。
func TestDogfightKeepsBombsWhenEscorted(t *testing.T) {
	state := sandboxState(t)
	bomber := state.Aircraft["b-cv-01-dive_bomber-x1"]
	bomber.Status = "attack"
	bomber.TaskKind = "attack"
	applyLoadout(state, bomber)
	if !bomber.Loadout.CarriesWeapon() {
		t.Fatalf("俯冲轰炸机攻击挂载应带弹：%s", bomber.Loadout)
	}
	escort := &AirUnit{ID: "escort-x", Faction: bomber.Faction, Kind: "fighter", Model: "F6F", Status: "attack", AirCombat: 5, Strength: 3}
	state.Aircraft[escort.ID] = escort
	resolveDogfight(&state, []*AirUnit{bomber, escort}, nil, &AirCombatReport{})
	if !bomber.Loadout.CarriesWeapon() {
		t.Fatalf("有护航时不应抛弃炸弹：%s", bomber.Loadout)
	}
	if !bomber.TankDropped {
		t.Fatal("有护航时仍应抛弃副油箱（规则书 811 行）")
	}
}

// 投弹后剩余航程 +20%（规则书 83 行），且不重复叠加。
func TestBombingGivesLightLoadBonus(t *testing.T) {
	aircraft := &AirUnit{Loadout: LoadoutBombTank, Fuel: Fuel{Internal: 10}}
	before := aircraft.Range()
	markBombed(aircraft)
	if got := aircraft.Range(); got != 12 {
		t.Fatalf("轻载加成错误：%d → %d（期望12）", before, got)
	}
	markBombed(aircraft)
	if got := aircraft.Range(); got != 12 {
		t.Fatalf("轻载加成重复叠加了：%d", got)
	}
}

// 转场不变，侦察不带武器，所以都不触发轻载加成。
func TestLightLoadOnlyForWeapons(t *testing.T) {
	for _, loadout := range []Loadout{LoadoutReconTank, LoadoutTransfer, LoadoutAirTank} {
		if loadout.CarriesWeapon() {
			t.Fatalf("%s 不应带弹药", loadout)
		}
	}
	for _, loadout := range []Loadout{LoadoutBombTank, LoadoutBomb, LoadoutTorpedoTank, LoadoutTorpedo} {
		if !loadout.CarriesWeapon() {
			t.Fatalf("%s 应当带弹药", loadout)
		}
	}
}
