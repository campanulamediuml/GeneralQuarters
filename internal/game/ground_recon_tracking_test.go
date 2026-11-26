package game

import "testing"

// 岸基侦察机发现舰队后必须转入持续跟踪：以前 searchGroundReconStep 只记录情报，
// 从不设置 TrackingFleetID，于是"点亮了但没人跟"——情报照常每 6/3 回合降一级直到消失。
func TestGroundReconStartsTrackingOnContact(t *testing.T) {
	state := coralCombatState(t)
	plane := state.GroundAir["af-a-moresby-b17-1"]
	if plane == nil {
		t.Fatal("找不到莫尔兹比的 B-17")
	}
	plane.Status = "recon"
	plane.OriginCoord = "1611"
	plane.Coord = "1611"
	plane.Heading = "E"
	plane.SectorWidth = 60
	plane.ReconRadius = 20

	fleet := firstEnemyShip(&state, "B")
	if fleet == nil {
		t.Fatal("靶场里没有敌方舰船")
	}
	fleetID := FleetKey(fleet)
	// 靶场先用 firstEnemyShip 挑船是随机的（遍历 map），挑中的阵位离机场多远每跑一次都不一样，
	// 而发现率随距离往下掉，于是"首抽很低"也不一定过线——这个测试以前就靠运气。
	// 现在把目标舰队挪到机场旁边一个空格：距离 ≤2，发现率 ≥25%，并且那一格里只有这一支舰队，
	// 所以第一次概率判定就是它的，种子一定能挑到。
	occupied := map[string]bool{}
	for _, unit := range state.Units {
		occupied[unit.Coord] = true
	}
	coord := ""
	for _, candidate := range []string{"1711", "1612", "1512", "1610", "1712"} {
		if !occupied[candidate] {
			coord = candidate
			break
		}
	}
	if coord == "" {
		t.Fatal("机场周围找不到空格放靶子")
	}
	for _, member := range fleetMembers(&state, fleetID) {
		unit := state.Units[member.ID]
		unit.Coord = coord
	}
	chance := groundReconChance(state, plane, fleetMembers(&state, fleetID))

	// 让这一次发现判定必定成功：概率判定的 roll 来自 RNG，挑一个首抽低于发现率的种子。
	seeded := false
	for seed := uint64(1); seed < 5000; seed++ {
		probe := seed
		if probabilityRoll(&probe) < chance {
			state.RNGState = seed
			seeded = true
			break
		}
	}
	if !seeded {
		t.Fatalf("发现率 %.1f%% 下没有找到可用的随机种子", chance)
	}

	searchGroundReconStep(&state, plane, []string{coord}, 0)
	if plane.TrackingFleetID != fleetID {
		t.Fatalf("发现舰队后应该开始持续跟踪 %s，实际 %q", fleetID, plane.TrackingFleetID)
	}
	if plane.Returning {
		t.Fatal("开始跟踪时不该处在返航状态")
	}
}

// 跟踪中的岸基侦察机每回合按自己的航速飞向目标，且这条接触不再按"脱离接触"衰减。
func TestGroundReconTrackingKeepsContactFresh(t *testing.T) {
	state := coralCombatState(t)
	plane := state.GroundAir["af-a-moresby-b17-1"]
	if plane == nil {
		t.Fatal("找不到莫尔兹比的 B-17")
	}
	plane.Status = "recon"
	plane.OriginCoord = "1611"
	plane.Coord = "1611"
	plane.SetRange(30)
	plane.Heading = "E"
	plane.SectorWidth = 60
	plane.ReconRadius = 20

	fleet := firstEnemyShip(&state, "B")
	if fleet == nil {
		t.Fatal("靶场里没有敌方舰船")
	}
	fleetID := FleetKey(fleet)
	members := fleetMembers(&state, fleetID)
	if len(members) == 0 {
		t.Fatal("目标舰队没有成员")
	}
	plane.TrackingFleetID = fleetID
	recordContact(&state, "A", fleetID, members)
	contact := state.Contacts["A"][fleetID]
	if contact == nil {
		t.Fatal("没有建立接触")
	}
	contact.Level = 4
	if state.IntelStaleTurns == nil {
		state.IntelStaleTurns = map[string]int{}
	}
	state.IntelStaleTurns["A|"+fleetID] = 99
	contact.StaleTurns = 99
	before := plane.Coord

	AdvanceGroundAir(&state)

	if plane.Coord == before {
		t.Fatalf("跟踪中的侦察机应该朝目标飞：%s 没有移动", before)
	}
	decayContactIntel(&state)
	contact = state.Contacts["A"][fleetID]
	if contact == nil || contact.Level != 4 {
		t.Fatalf("有飞机持续跟踪时情报不该衰减：%+v", contact)
	}
}
