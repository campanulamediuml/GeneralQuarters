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
	if aircraft.Returning {
		return advanceAircraftReturn(state, aircraft)
	}
	members := []*AirUnit{aircraft}
	if aircraft.TaskID != "" {
		members = taskAircraft(*state, aircraft.TaskID)
	}
	if aircraft.TargetCoord == "" {
		for _, member := range members {
			member.RemainingRange--
		}
		return nil
	}
	arrived := aircraft.Coord == aircraft.TargetCoord
	for _, member := range members {
		for steps := 0; steps < 2 && member.Coord != member.TargetCoord; steps++ {
			next := nextCoordToward(state.Map, member.Coord, member.TargetCoord)
			if next == "" {
				break
			}
			member.Coord = next
			member.RemainingRange--
		}
	}
	if aircraft.TaskID != "" && aircraft.Coord == aircraft.TargetCoord {
		return resolveAirStrike(state, aircraft.TaskID, aircraft.TargetCoord)
	}
	if !arrived && aircraft.Coord == aircraft.TargetCoord {
		return []EngineEvent{{Type: "attack_arrived", Audience: aircraft.Faction, Text: fmt.Sprintf("%s抵达%s目标上空，等待后续空袭结算", aircraft.Model, aircraft.TargetCoord)}}
	}
	return nil
}
