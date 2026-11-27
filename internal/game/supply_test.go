package game

import "testing"

// 补给点（护航航母）没有鱼雷：停在它上面的鱼雷机挂载表里不该出现鱼雷，
// 默认挂载要退回炸弹，SET_LOADOUT 也要被明确拒绝。转场到舰队航母后全部恢复。
func TestEscortCarrierSupplyBlocksTorpedo(t *testing.T) {
	state := State{
		Units: map[string]*Unit{
			"cve": {ID: "cve", Faction: "A", Type: "CVE", Name: "马尼拉湾", Card: ShipCard{HangarCapacity: 6, Supply: Supply{Blocked: []Weapon{WeaponTorpedo}}}},
			"cv":  {ID: "cv", Faction: "A", Type: "CV", Name: "企业", Card: ShipCard{HangarCapacity: 12}},
		},
		Aircraft: map[string]*AirUnit{
			"tbf": {ID: "tbf", CarrierID: "cve", Faction: "A", Kind: "torpedo_bomber", Model: "TBF 复仇者", Status: "hangar", BaseRange: 9, MaxRange: 9, AntiShip: 5, AntiGround: 2},
		},
	}
	tbf := state.Aircraft["tbf"]

	// 1. 挂载表：CVE 上没有鱼雷，但有炸弹和侦察。
	options := LoadoutOptionsWithSupply(tbf.Kind, tbf.Seaplane, tbf.AntiGround, tbf.AntiShip, tbf.TorpedoCapable, SupplyOfAircraft(state, tbf))
	for _, option := range options {
		if option.Weapon() == WeaponTorpedo {
			t.Fatalf("护航航母上的挂载表不该有鱼雷：%v", options)
		}
	}
	if !containsLoadout(options, LoadoutBombTank) {
		t.Fatalf("护航航母上的鱼雷机仍然要能挂炸弹：%v", options)
	}

	// 2. 默认挂载：对舰攻击退到炸弹，而不是被补给点给不出的鱼雷顶上。
	if got := LoadoutForTaskWithSupply(tbf.Kind, tbf.Seaplane, tbf.AntiGround, tbf.AntiShip, "attack", StrikeTargetShip, tbf.TorpedoCapable, SupplyOfAircraft(state, tbf)); got != LoadoutBombTank {
		t.Fatalf("护航航母上的默认对舰挂载应为炸弹，实际 %s", got)
	}
	// applyLoadout 走的是同一条路：分配打击任务时不能自动配鱼雷。
	tbf.TaskKind = "attack"
	tbf.TargetCoord = "0808"
	applyLoadout(state, tbf)
	if tbf.Loadout.Weapon() == WeaponTorpedo {
		t.Fatalf("护航航母上分配打击任务却挂上了鱼雷：%s", tbf.Loadout)
	}

	// 3. 校验：SET_LOADOUT 挂鱼雷要被拒，并且提示点名是哪条船、缺什么。
	order := Order{Type: "SET_LOADOUT", AircraftIDs: []string{"tbf"}, Loadout: string(LoadoutTorpedoTank)}
	err := validateLoadoutOrder(state, "A", order)
	if err == nil {
		t.Fatal("护航航母上挂鱼雷应当被拒绝")
	}
	if !containsText(err.Error(), "马尼拉湾") || !containsText(err.Error(), "鱼雷") {
		t.Fatalf("拒绝提示应当点名母舰和缺失的弹药，实际：%v", err)
	}

	// 4. 转场到舰队航母：鱼雷选项和校验都回来。
	tbf.CarrierID = "cv"
	if err := validateLoadoutOrder(state, "A", order); err != nil {
		t.Fatalf("舰队航母上应当能挂鱼雷：%v", err)
	}
	restored := LoadoutOptionsWithSupply(tbf.Kind, tbf.Seaplane, tbf.AntiGround, tbf.AntiShip, tbf.TorpedoCapable, SupplyOfAircraft(state, tbf))
	if !containsLoadout(restored, LoadoutTorpedoTank) {
		t.Fatalf("转场到舰队航母后应当恢复鱼雷挂载：%v", restored)
	}
}

// 机场也能配"不提供某种挂载"：陆基鱼雷机在没鱼雷的机场只能挂炸弹。
func TestAirfieldSupplyBlocksTorpedo(t *testing.T) {
	state := State{
		Airfields: map[string]*Airfield{
			"af": {ID: "af", Faction: "B", Name: "莱城机场", Supply: Supply{Blocked: []Weapon{WeaponTorpedo}}},
		},
		GroundAir: map[string]*AirfieldAirUnit{
			"g4m": {ID: "g4m", AirfieldID: "af", Faction: "B", Kind: "torpedo_bomber", Model: "G4M 一式陆攻", Status: "hangar", AntiShip: 5, AntiGround: 2, BaseRange: 12, MaxRange: 12},
		},
	}
	order := Order{Type: "SET_LOADOUT", AircraftIDs: []string{"g4m"}, Loadout: string(LoadoutTorpedoTank)}
	err := validateLoadoutOrder(state, "B", order)
	if err == nil {
		t.Fatal("没有鱼雷保障的机场不该允许挂鱼雷")
	}
	if !containsText(err.Error(), "莱城机场") || !containsText(err.Error(), "鱼雷") {
		t.Fatalf("拒绝提示应当点名机场和缺失的弹药，实际：%v", err)
	}
	if err := validateLoadoutOrder(state, "B", Order{Type: "SET_LOADOUT", AircraftIDs: []string{"g4m"}, Loadout: string(LoadoutBombTank)}); err != nil {
		t.Fatalf("同一机场挂炸弹应当放行：%v", err)
	}
}

// 视图要把"能挂什么 + 当前挂的是什么"都算好下发：前端只渲染，
// 不再维护机种规则和 id→中文名映射表。
func TestWithLoadoutViewSendsOptionsAndLabel(t *testing.T) {
	aircraft := AirUnit{Kind: "torpedo_bomber", AntiShip: 5, AntiGround: 2, Loadout: LoadoutBombTank}
	view := withLoadoutView(aircraft, Supply{Blocked: []Weapon{WeaponTorpedo}})
	if view.LoadoutLabel != "炸弹（副油箱）" {
		t.Fatalf("当前挂载中文名应由服务端给，实际 %q", view.LoadoutLabel)
	}
	hasOption := func(id Loadout) bool {
		for _, option := range view.LoadoutOptions {
			if option.ID == id {
				return true
			}
		}
		return false
	}
	if hasOption(LoadoutTorpedoTank) {
		t.Fatal("护航航母上的选项里不该有鱼雷")
	}
	if !hasOption(LoadoutBombTank) {
		t.Fatal("炸弹选项应当还在")
	}
	for _, option := range view.LoadoutOptions {
		if option.Label == "" {
			t.Fatalf("选项 %s 没有中文名：前端会直接显示 id", option.ID)
		}
	}
}

// 马里亚纳的 16 艘护航航母必须在数据里带"不提供鱼雷"，
// 否则引擎那条规则永远不会被触发，机库里照样能选鱼雷。
func TestMarianasEscortCarriersBlockTorpedoes(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_marianas")
	if err != nil {
		t.Fatalf("加载剧本失败：%v", err)
	}
	state, err := NewState(scenario, gameMap, catalog, 42)
	if err != nil {
		t.Fatalf("建局失败：%v", err)
	}
	cves, tbfs := 0, 0
	for _, unit := range state.Units {
		if unit.Type != "CVE" {
			continue
		}
		cves++
		if unit.Card.Supply.Allows(WeaponTorpedo) {
			t.Fatalf("护航航母%s（%s）没有配置鱼雷限制", unit.ID, unit.Name)
		}
	}
	for _, aircraft := range state.Aircraft {
		carrier := state.Units[aircraft.CarrierID]
		if carrier == nil || carrier.Type != "CVE" || aircraft.Kind != "torpedo_bomber" {
			continue
		}
		tbfs++
		options := LoadoutOptionsWithSupply(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, aircraft.TorpedoCapable, SupplyOfAircraft(state, aircraft))
		for _, option := range options {
			if option.Weapon() == WeaponTorpedo {
				t.Fatalf("%s 停在护航航母上却有鱼雷挂载：%v", aircraft.ID, options)
			}
		}
	}
	if cves == 0 || tbfs == 0 {
		t.Fatalf("马里亚纳应当有护航航母和鱼雷机：CVE=%d TBF=%d", cves, tbfs)
	}
}
