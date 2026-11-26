package game

import "testing"

// 敌方滩头物资还压在机场上：生命值归 0、跑道全废、修复停摆——等于机场正在被占领。
func TestAirfieldOccupiedByEnemySuppliesBlocksRepair(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	airfield.Strength = airfield.MaxStrength
	state.ShoreSupplies = map[string]int{"B|" + airfield.Coord: 6}

	events := AdvanceAirfields(&state)
	if airfield.Strength != 0 {
		t.Fatalf("被敌方滩头物资压制的机场生命值应为 0，实际 %d", airfield.Strength)
	}
	if RunwaysAvailable(airfield.Strength, airfield.Runways) != 0 {
		t.Fatal("被压制的机场不应还有可用跑道")
	}
	found := false
	for _, event := range events {
		if event.Type == "airfield_occupied" {
			found = true
		}
		if event.Type == "airfield_repaired" {
			t.Fatal("被压制期间不该出现修复事件")
		}
	}
	if !found {
		t.Fatal("机场被压制时应发一条 airfield_occupied 战报")
	}
}

// 清滩之后修复逻辑要能自己恢复，不用再等额外回合。
func TestAirfieldRepairsAfterSuppliesCleared(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	airfield := state.Airfields["af-a-moresby"]
	airfield.Strength = 1
	state.ShoreSupplies = map[string]int{"B|" + airfield.Coord: 6}
	AdvanceAirfields(&state)
	if airfield.Strength != 0 {
		t.Fatalf("压制期间生命值应为 0，实际 %d", airfield.Strength)
	}
	delete(state.ShoreSupplies, "B|"+airfield.Coord)
	for turn := 0; turn < 60 && airfield.Strength == 0; turn++ {
		AdvanceAirfields(&state)
	}
	if airfield.Strength == 0 {
		t.Fatal("清滩之后机场应恢复修复逻辑")
	}
	if airfield.Strength > airfield.MaxStrength {
		t.Fatalf("修复不应超过最大生命值：%d/%d", airfield.Strength, airfield.MaxStrength)
	}
}

// 前端/观战靠 AirfieldContact 上的 enemy_supplies 画红字，字段必须带出去。
func TestAirfieldContactExposesEnemySupplies(t *testing.T) {
	state := coralCombatState(t)
	airfield := state.Airfields["af-a-moresby"]
	state.ShoreSupplies = map[string]int{"B|" + airfield.Coord: 9}
	for _, contact := range VisibleAirfields(state, "A") {
		if contact.ID == airfield.ID {
			if contact.EnemySupplies != 9 {
				t.Fatalf("机场情报应带敌方滩头物资数量，实际 %d", contact.EnemySupplies)
			}
			return
		}
	}
	t.Fatal("没找到莫尔兹比机场的情报")
}
