package game

import "fmt"

func ValidateStrikeTarget(state State, faction, coord string) error {
	if coord == "" {
		return fmt.Errorf("打击任务必须选择目标")
	}
	if !inBounds(state.Map, coord) {
		return fmt.Errorf("打击目标不在地图内")
	}
	for _, unit := range VisibleUnits(state, faction) {
		if unit.Faction != faction && unit.Coord == coord {
			return nil
		}
	}
	return fmt.Errorf("目标不在当前情报范围内，请先侦察")
}

func advanceAttack(state *State, aircraft *AirUnit) []EngineEvent {
	if aircraft.TargetCoord == "" {
		aircraft.RemainingRange--
		return nil
	}
	arrived := aircraft.Coord == aircraft.TargetCoord
	for steps := 0; steps < 2 && aircraft.Coord != aircraft.TargetCoord; steps++ {
		next := nextCoordToward(state.Map, aircraft.Coord, aircraft.TargetCoord)
		if next == "" {
			break
		}
		aircraft.Coord = next
		aircraft.RemainingRange--
	}
	if !arrived && aircraft.Coord == aircraft.TargetCoord {
		return []EngineEvent{{Type: "attack_arrived", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s目标上空，等待后续空袭结算", aircraft.Model, aircraft.TargetCoord)}}
	}
	return nil
}
