package game

import "math"

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
	// 进水的每一层降一档；倾斜本身也降一档（规则书 435 行），修好之后自动恢复。
	rank -= len(damage.Flooding)
	if damage.Tilt {
		rank--
	}
	if rank <= 0 {
		return "very_slow"
	}
	return []string{"very_slow", "slow", "normal", "fast"}[rank]
}

// classTurns 是老四档对应的"每几回合走一格"：25 海里/hex、20 分钟/回合换算过来，
// normal 3 回合≈25 节，slow 6 回合≈12.5 节，very_slow 9 回合≈8.3 节，disabled 24 回合≈停在原地。
func classTurns(class string) int {
	switch class {
	case "slow":
		return 6
	case "very_slow":
		return 9
	case "disabled":
		return 24
	default:
		return 3
	}
}

// ShipMoveCycle 返回单舰的移动节奏：一圈里第 i 格要等 cycle[i] 回合才能走下一格。
// 卡面配了 move_cycle 就按它来，进水每层、倾斜各 +1 回合；没配的老卡仍走四档，
// 保证旧存档和旧剧本的行为一点不变。
func ShipMoveCycle(unit *Unit) []int {
	damage := EnsureDamage(unit)
	disabled := damage.Disabled || len(damage.Flooding) > unit.Card.DamageControl
	if len(unit.Card.MoveCycle) == 0 {
		if disabled {
			return []int{24}
		}
		return []int{classTurns(EffectiveSpeedClass(unit))}
	}
	if disabled {
		return []int{24}
	}
	penalty := len(damage.Flooding)
	if damage.Tilt {
		penalty++
	}
	cycle := make([]int, len(unit.Card.MoveCycle))
	for i, turn := range unit.Card.MoveCycle {
		turn += penalty
		if turn < 1 {
			turn = 1
		}
		if turn > 24 {
			turn = 24
		}
		cycle[i] = turn
	}
	return cycle
}

// CycleForKnots 是反查：给定航速（节），找一串周期，让牌面结数（75/平均回合数向下取整）
// 最接近它。同样接近时优先短的周期，再优先平均回合数更贴近的——和 tools/fill_move_cycles.py
// 用同一套规则，所以脚本填出来的周期和引擎算出来的编队节奏是一回事。
func CycleForKnots(knots int) []int {
	if knots <= 0 {
		return []int{24}
	}
	n := 75.0 / float64(knots)
	base := int(math.Floor(n))
	if base < 1 {
		base = 1
	}
	var best []int
	bestScore := 0.0
	for length := 1; length <= 8; length++ {
		for extra := 0; extra <= length; extra++ {
			avg := float64(base*length+extra) / float64(length)
			model := 75 / avg
			// 先保证牌面结数（向下取整）对得上，再比真实结数误差，最后才比周期长短。
			score := math.Abs(math.Floor(model)-float64(knots)) +
				math.Abs(model-float64(knots))/100 +
				float64(length)/10000
			if best == nil || score < bestScore {
				bestScore = score
				best = buildCycle(base, length, extra)
			}
		}
	}
	return best
}

// buildCycle 把 extra 个 base+1 均匀撒进 length 个位置，避免走走停停成块。
func buildCycle(base, length, extra int) []int {
	if extra <= 0 {
		return []int{base}
	}
	if extra >= length {
		return []int{base + 1}
	}
	cycle := make([]int, 0, length)
	for i := 0; i < length; i++ {
		if (i+1)*extra/length > i*extra/length {
			cycle = append(cycle, base+1)
		} else {
			cycle = append(cycle, base)
		}
	}
	return cycle
}

// FleetMoveCycle 取编队里最慢的那艘船当全队节奏：平均回合数最大的那个周期说了算，
// 所以拖一条运输船的航母编队会跟着运输船走。两艘以上的编队还要再 -1 节：保持队形、
// 规避机动都要吃掉一点航速，单舰跑自己的最高航速。
func FleetMoveCycle(state State, unit *Unit) []int {
	if unit == nil {
		return []int{24}
	}
	fleetID := FleetKey(unit)
	legacy := true
	pureFastDD := true
	members := 0
	var slowest []int
	slowestAvg := 0.0
	for _, member := range state.Units {
		if FleetKey(member) != fleetID {
			continue
		}
		members++
		if len(member.Card.MoveCycle) > 0 {
			legacy = false
		}
		if cycle := ShipMoveCycle(member); cycleAverage(cycle) > slowestAvg {
			slowestAvg = cycleAverage(cycle)
			slowest = cycle
		}
		if member.Type != "DD" || EffectiveSpeedClass(member) != "fast" {
			pureFastDD = false
		}
	}
	if members == 0 || len(slowest) == 0 {
		return []int{24}
	}
	// 老卡（没有 move_cycle）的纯高速驱逐舰编队仍是 2 回合 1 hex，这是旧规则里
	// 唯一比四档细的一档，留着让旧存档行为不变。
	if legacy && pureFastDD {
		return []int{2}
	}
	// 编队规模惩罚：保持队形、规避机动都要吃掉一点航速，队伍越大越难。单舰不罚；
	// 失去动力的船不参与（24 回合本来就是"拖也拖不动"）；老卡（纯四档）的编队也不罚，
	// 免得旧存档里一支舰队的节奏凭空变掉。
	if !legacy && slowestAvg < 24 {
		if penalty := formationSpeedPenalty(members); penalty > 0 {
			target := CycleKnots(slowest) - penalty
			if target < 1 {
				target = 1
			}
			return CycleForKnots(target)
		}
	}
	return slowest
}

// formationSpeedPenalty 按编队规模扣航速：小型（1–2 艘）不扣，中型（3–5 艘）−1 节，
// 大型（6 艘以上）−2 节。档位和情报里显示的"编队规模"共用同一套阈值，避免出现
// "情报说大型编队、跑起来却和两条船一样快"。
func formationSpeedPenalty(members int) int {
	switch contactSizeClass(members) {
	case "medium":
		return 1
	case "large":
		return 2
	}
	return 0
}

func cycleAverage(cycle []int) float64 {
	if len(cycle) == 0 {
		return 24
	}
	sum := 0
	for _, turn := range cycle {
		sum += turn
	}
	return float64(sum) / float64(len(cycle))
}

// CycleKnots 把节奏折算成平均航速（节），向下取整：牌面写 27 节就不能按 28 节算航程。
func CycleKnots(cycle []int) int {
	avg := cycleAverage(cycle)
	if avg <= 0 {
		return 0
	}
	return int(math.Floor(75 / avg))
}

// FleetMoveInterval 是节奏折成的平均回合数，面板上显示"几回合一格"用它。
func FleetMoveInterval(state State, unit *Unit) int {
	return int(math.Round(cycleAverage(FleetMoveCycle(state, unit))))
}

func FleetCycleKnots(state State, unit *Unit) int {
	return CycleKnots(FleetMoveCycle(state, unit))
}

// ApplyMoveCooldown 在舰队走完一格之后调一次：按节奏取这一格要等几回合，并把周期指针推进。
// 冷却在回合结算开始时递减，所以 N 回合走一格要挂 N-1。全队共用同一个指针，
// 所以按领队调一次就够，队内所有船一起写。
func ApplyMoveCooldown(state *State, leader *Unit) int {
	cycle := FleetMoveCycle(*state, leader)
	pos := leader.CyclePos
	if pos < 0 || pos >= len(cycle) {
		pos = 0
	}
	cooldown := cycle[pos] - 1
	if cooldown < 0 {
		cooldown = 0
	}
	next := (pos + 1) % len(cycle)
	fleetID := FleetKey(leader)
	for _, unit := range state.Units {
		if FleetKey(unit) != fleetID {
			continue
		}
		if cooldown > unit.MoveCooldown {
			unit.MoveCooldown = cooldown
		}
		unit.CyclePos = next
	}
	return cooldown
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
