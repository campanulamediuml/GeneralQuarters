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
	// 四档速度：fast > normal > slow > very_slow。没被进水拖慢时原样返回自身档位——
	// 之前 very_slow 被映射到 0、再经 rank<=0 兜底，进水一档就直接跳到"极慢"，
	// 极慢本身反而永远拿不到自己该有的档位。
	rank := map[string]int{"fast": 3, "normal": 2, "slow": 1, "very_slow": 0}[unit.Card.SpeedClass]
	if _, ok := map[string]bool{"fast": true, "normal": true, "slow": true, "very_slow": true}[unit.Card.SpeedClass]; !ok {
		rank = 2
	}
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

// movementCooldownAfterMove 把"每 N 回合移动一次"换算成移动后要挂的冷却值。
// 冷却在回合结算开始时递减，所以 N 回合周期要挂 N-1：普通舰队挂 2，
// 第 1 回合动完，第 2、3 回合递减，第 4 回合（即第 3 个回合后）重新可动。
func movementCooldownAfterMove(interval int) int {
	if interval <= 1 {
		return 0
	}
	return interval - 1
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
