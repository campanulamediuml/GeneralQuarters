package game

import "testing"

// TestCarrierFleetCannotEnterAirfieldHex 剧本规则：含 CV/CVL/CVE 的舰队不能进入机场格（陆地 hex）。
func TestCarrierFleetCannotEnterAirfieldHex(t *testing.T) {
	state := sandboxState(t)
	// TG-A-CV 含有航母，把整支舰队放到 0206（机场 0207 西侧）
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A-CV" {
			unit.Coord = "0206"
		}
	}
	moves := LegalMoves(state, state.Units["a2-cv-01"])
	for _, coord := range moves {
		if coord == "0207" {
			t.Fatalf("航母舰队不应能进入机场 hex：%v", moves)
		}
	}
}

// TestSurfaceFleetCanEnterAirfieldHex 纯水面舰队可以进入机场格（靠岸岸轰）。
func TestSurfaceFleetCanEnterAirfieldHex(t *testing.T) {
	state := sandboxState(t)
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = "0206"
		}
	}
	found := false
	for _, coord := range LegalMoves(state, state.Units["a-bb-01"]) {
		if coord == "0207" {
			found = true
		}
	}
	if !found {
		t.Fatalf("水面舰队应能进入机场 hex 进行岸轰：%v", LegalMoves(state, state.Units["a-bb-01"]))
	}
}

// TestCarrierFleetCannotDeployOnIsland CV 不能部署在陆地 hex 上。
func TestCarrierFleetCannotDeployOnIsland(t *testing.T) {
	scenario, gameMap, catalog := loadSandboxScenario(t)
	for index := range scenario.Deployments {
		if scenario.Deployments[index].ID == "a2-cv-01" {
			scenario.Deployments[index].Coord = "0207"
		}
	}
	if _, err := NewState(scenario, gameMap, catalog, 1); err == nil {
		t.Fatal("航母部署在陆地 hex 应被拒绝")
	}
}

// TestShoreBombardmentFromAirfieldHex 水面舰队停在机场 hex 上、停留满 1 回合即可岸轰（配合上面的移动规则）。
func TestShoreBombardmentFromAirfieldHex(t *testing.T) {
	state := sandboxState(t)
	airfield := state.Airfields["af-b-sentry"]
	for _, unit := range state.Units {
		if FleetKey(unit) == "TG-A" {
			unit.Coord = airfield.Coord
		}
	}
	state.FleetStays = map[string]string{"TG-A": airfield.Coord}
	if canBombard(state, state.Units["a-bb-01"]) != airfield {
		t.Fatal("水面舰队停在机场 hex 且停留满 1 回合时应能岸轰")
	}
}
