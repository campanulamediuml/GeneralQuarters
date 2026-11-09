package game

func EnsureDamage(unit *Unit) *ShipDamage {
	if unit.Damage == nil {
		unit.Damage = &ShipDamage{}
	}
	if unit.Damage.Flooding == nil {
		unit.Damage.Flooding = []int{}
	}
	return unit.Damage
}

func EffectiveSpeedClass(unit *Unit) string {
	damage := EnsureDamage(unit)
	if damage.Disabled || len(damage.Flooding) > unit.Card.DamageControl {
		return "disabled"
	}
	rank := map[string]int{"very_slow": 0, "slow": 1, "normal": 2, "fast": 3}[unit.Card.SpeedClass]
	rank -= len(damage.Flooding)
	if rank <= 0 {
		return "very_slow"
	}
	return []string{"very_slow", "slow", "normal", "fast"}[rank]
}

func FleetMoveInterval(state State, unit *Unit) int {
	if unit == nil {
		return 24
	}
	fleetID := FleetKey(unit)
	pureFastDD := true
	interval := 0
	for _, member := range state.Units {
		if FleetKey(member) != fleetID {
			continue
		}
		speed := EffectiveSpeedClass(member)
		memberInterval := map[string]int{"normal": 3, "fast": 3, "slow": 5, "very_slow": 10, "disabled": 24}[speed]
		if memberInterval > interval {
			interval = memberInterval
		}
		if member.Type != "DD" || speed != "fast" {
			pureFastDD = false
		}
	}
	if pureFastDD && interval > 0 {
		return 2
	}
	if interval == 0 {
		return 24
	}
	return interval
}

func FleetMoveCooldown(state State, fleetID string) int {
	cooldown := 0
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID && unit.MoveCooldown > cooldown {
			cooldown = unit.MoveCooldown
		}
	}
	return cooldown
}

func FleetConfusion(state State, fleetID string) int {
	confusion := 0
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID && unit.Confusion > confusion {
			confusion = unit.Confusion
		}
	}
	return confusion
}

func LockFleetMovement(state *State, fleetID string, turns int) {
	for _, unit := range state.Units {
		if FleetKey(unit) == fleetID && unit.MoveCooldown < turns {
			unit.MoveCooldown = turns
		}
	}
}

func AdvanceMovementCooldown(state *State) {
	for _, unit := range state.Units {
		unit.Moved = false
		if unit.MoveCooldown > 0 {
			unit.MoveCooldown--
		}
		if unit.Confusion > 0 {
			unit.Confusion--
		}
	}
}
