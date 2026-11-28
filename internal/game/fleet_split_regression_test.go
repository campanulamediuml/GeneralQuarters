package game

import "testing"

func TestSplitFleetPreservesEnemyContact(t *testing.T) {
	state := submarineFixture()
	state.Contacts = map[string]map[string]*FleetContact{
		"B": {
			"TF-A": {
				FleetID: "TF-A", Faction: "A", Level: 3, Coord: "0505", DisplayCoord: "0505", Count: 2,
				Members: []Unit{*state.Units["a-ca"], *state.Units["a-dd"]},
			},
		},
	}
	ApplySplitFleet(&state, Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-dd"}})

	newFleetID := state.Units["a-dd"].FleetID
	contact := state.Contacts["B"][newFleetID]
	if contact == nil {
		t.Fatalf("拆出的舰队 %s 应继承敌方情报，不能从侦察中消失", newFleetID)
	}
	if contact.Level != 3 || contact.Coord != "0505" {
		t.Fatalf("拆出的舰队情报等级或位置丢失：%+v", contact)
	}
	if len(contact.Members) != 1 || contact.Members[0].ID != "a-dd" {
		t.Fatalf("新舰队的情报成员应只有拆出的舰船：%+v", contact.Members)
	}
	oldContact := state.Contacts["B"]["TF-A"]
	if oldContact == nil || len(oldContact.Members) != 1 || oldContact.Members[0].ID != "a-ca" {
		t.Fatalf("原舰队情报不应继续包含已拆出的舰船：%+v", oldContact)
	}
}

func TestSplitFleetPreservesFleetLevelTimers(t *testing.T) {
	state := submarineFixture()
	state.FleetSessions = map[string]int{"TF-A": 2}
	state.FleetStays = map[string]string{"TF-A": "0505"}
	state.SurpriseLock = map[string]int{"TF-A": 4}
	state.HeadingChanged = map[string]bool{"TF-A": true}
	ApplySplitFleet(&state, Order{Type: "SPLIT_FLEET", UnitID: "a-ca", SplitIDs: []string{"a-dd"}})

	newFleetID := state.Units["a-dd"].FleetID
	if got := SessionOf(state, newFleetID); got != 2 {
		t.Fatalf("拆分不应重置卸载/停留计时： got %d, want 2", got)
	}
	if got := state.FleetStays[newFleetID]; got != "0505" {
		t.Fatalf("拆分不应重置岸轰停留位置： got %q, want 0505", got)
	}
	if got := state.SurpriseLock[newFleetID]; got != 4 {
		t.Fatalf("拆分不应清除突袭冷却： got %d, want 4", got)
	}
	if !state.HeadingChanged[newFleetID] {
		t.Fatal("拆分不应让同一批舰船获得额外转向机会")
	}
}
