package game

import (
	"fmt"
	"math"
	"sort"
)

type ShellHit struct {
	AttackerID string
	TargetID   string
	Torpedo    bool
}

func d10(state *State) int {
	return int(nextUint64(&state.RNGState)%10) + 1
}

func d6(state *State) int {
	return int(nextUint64(&state.RNGState)%6) + 1
}

func addFire(unit *Unit, fire string) {
	damage := EnsureDamage(unit)
	if damage.Fire == "small" && fire == "small" {
		damage.Fire = "burning"
		return
	}
	if damage.Fire == "" || fire == "burning" {
		damage.Fire = fire
	}
}

func sinkUnit(state *State, unit *Unit) []EngineEvent {
	for id, aircraft := range state.Aircraft {
		if aircraft.CarrierID == unit.ID {
			delete(state.Aircraft, id)
		}
	}
	delete(state.Units, unit.ID)
	return []EngineEvent{{Type: "unit_destroyed", Audience: "all", Text: unit.Name + "沉没，舰上航空算子一并移除"}}
}

func damageStrength(state *State, unit *Unit, amount int) []EngineEvent {
	unit.Strength -= amount
	if unit.Strength <= 0 {
		return sinkUnit(state, unit)
	}
	return nil
}

func AdvanceShipConditions(state *State) []EngineEvent {
	events := []EngineEvent{}
	for _, id := range sortedUnitIDs(state.Units) {
		unit := state.Units[id]
		if unit == nil {
			continue
		}
		damage := EnsureDamage(unit)
		if len(damage.Flooding) > unit.Card.DamageControl {
			damage.Disabled = true
		}
		if damage.Fire == "small" {
			damage.Fire = ""
		}
		if damage.DeckDamaged {
			damage.DeckDamaged = false
			events = append(events, EngineEvent{Type: "deck_repaired", Audience: unit.Faction, Text: unit.Name + "的飞行甲板损伤已修复"})
		}
		flooding := make([]int, 0, len(damage.Flooding))
		for _, timer := range damage.Flooding {
			if timer > 1 {
				flooding = append(flooding, timer-1)
			}
		}
		damage.Flooding = flooding
		if damage.Tilt && probabilityRoll(&state.RNGState) < float64(unit.Card.DamageControl*5) {
			damage.Tilt = false
			events = append(events, EngineEvent{Type: "tilt_repaired", Audience: unit.Faction, Text: unit.Name + "的进水倾斜已修复"})
		}
		if damage.Fire != "burning" {
			continue
		}
		first, second := d10(state), d10(state)
		if first <= unit.Card.DamageControl && second <= unit.Card.DamageControl {
			damage.Fire = ""
			events = append(events, EngineEvent{Type: "fire_extinguished", Audience: unit.Faction, Text: fmt.Sprintf("%s扑灭大火（%d/%d ≤ %d）", unit.Name, first, second, unit.Card.DamageControl)})
		} else if first > unit.Card.DamageControl && second > unit.Card.DamageControl {
			events = append(events, EngineEvent{Type: "fire_out_of_control", Audience: "all", Text: fmt.Sprintf("%s损管失败（%d/%d > %d）", unit.Name, first, second, unit.Card.DamageControl)})
			events = append(events, sinkUnit(state, unit)...)
		} else if d10(state) > unit.Card.DamageControl {
			events = append(events, severeDamage(state, unit)...)
		}
	}
	return events
}

func severeDamage(state *State, unit *Unit) []EngineEvent {
	table := []string{"small_fire", "burning", "flooding", "dc_down", "ammo_explosion", "oil_leak", "severe_flooding", "hull_damage", "aa_loss", "surface_radar_damage"}
	if IsCarrier(unit) {
		table = append(table, "deck_damage", "deck_destroyed", "elevator_damage", "avgas_explosion", "tilt", "hangar_damage")
	}
	if unit.Card.AirRadar {
		table = append(table, "air_radar_damage")
	}
	if unit.Card.SurfaceRadar {
		table = append(table, "surface_radar_damage")
	}
	if unit.Card.MainGunCount > 0 {
		table = append(table, "main_gun_damage")
	}
	if unit.Card.TorpedoSalvos > 0 {
		table = append(table, "torpedo_disabled", "torpedo_explosion")
	}
	if unit.Type == "DD" {
		table = append(table, "sonar_damage")
	}
	if unit.Type == "TRS" {
		table = append(table, "cargo_loss")
	}
	result := table[nextUint64(&state.RNGState)%uint64(len(table))]
	damage := EnsureDamage(unit)
	events := []EngineEvent{{Type: "critical_damage", Audience: "all", Text: fmt.Sprintf("%s严重伤害判定：%s", unit.Name, damageName(result))}}
	switch result {
	case "small_fire":
		addFire(unit, "small")
	case "burning":
		addFire(unit, "burning")
	case "flooding":
		damage.Flooding = append(damage.Flooding, 1)
	case "dc_down":
		if unit.Card.DamageControl > 0 {
			unit.Card.DamageControl--
		}
	case "ammo_explosion", "avgas_explosion", "torpedo_explosion":
		addFire(unit, "small")
		events = append(events, damageStrength(state, unit, 1)...)
	case "oil_leak":
		damage.OilLeak = true
	case "severe_flooding":
		events = append(events, damageStrength(state, unit, unit.Strength)...)
	case "hull_damage":
		events = append(events, damageStrength(state, unit, 1)...)
	case "aa_loss":
		unit.Card.HeavyAA *= 0.9
		unit.Card.LightAA *= 0.9
	case "surface_radar_damage":
		damage.SurfaceRadarDisabled = true
	case "air_radar_damage":
		damage.AirRadarDisabled = true
	case "deck_damage":
		damage.DeckDamaged = true
	case "deck_destroyed":
		damage.DeckDestroyed = true
	case "elevator_damage":
		if unit.Deck != nil && damage.ElevatorDamage < len(unit.Deck.Elevators) {
			damage.ElevatorDamage++
		}
	case "tilt":
		damage.Tilt = true
	case "hangar_damage":
		ids := []string{}
		for id, aircraft := range state.Aircraft {
			if aircraft.CarrierID == unit.ID && (aircraft.Status == "hangar" || aircraft.Status == "service") {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for index := 0; index < 2 && len(ids) > 0; index++ {
			selected := nextUint64(&state.RNGState) % uint64(len(ids))
			delete(state.Aircraft, ids[selected])
			unit.Card.Aircraft--
			ids = append(ids[:selected], ids[selected+1:]...)
		}
	case "main_gun_damage":
		if unit.Card.MainGunCount > 0 {
			unit.Card.MainGunCount--
			unit.Card.Gunnery = int(math.Round(unit.Card.MainGunCaliber * float64(unit.Card.MainGunCount) / 9))
		}
	case "torpedo_disabled":
		damage.TorpedoDisabled = true
	case "sonar_damage":
		damage.SonarDisabled = true
	case "cargo_loss":
		unit.Card.Cargo = int(math.Round(float64(unit.Card.Cargo) / 2))
	}
	return events
}

func damageName(id string) string {
	names := map[string]string{
		"small_fire": "小型火灾", "burning": "起火燃烧", "flooding": "进水", "dc_down": "损管能力降低", "ammo_explosion": "弹药爆炸", "oil_leak": "漏油", "severe_flooding": "严重进水", "hull_damage": "船体损伤", "aa_loss": "防空炮组伤亡", "surface_radar_damage": "对海雷达损坏", "air_radar_damage": "对空雷达损坏", "deck_damage": "飞行甲板损伤", "deck_destroyed": "飞行甲板摧毁", "elevator_damage": "升降机损坏", "avgas_explosion": "航空燃油爆炸", "tilt": "进水倾斜", "hangar_damage": "机库损伤", "main_gun_damage": "主炮毁坏", "torpedo_disabled": "鱼雷发射器毁坏", "torpedo_explosion": "鱼雷爆炸", "sonar_damage": "声呐损坏", "cargo_loss": "物资损失",
	}
	return names[id]
}

func applyShellHit(state *State, hit ShellHit) []EngineEvent {
	target := state.Units[hit.TargetID]
	if target == nil {
		return nil
	}
	target.Card.HeavyAA *= 0.9
	target.Card.LightAA *= 0.9
	if IsCarrier(target) && probabilityRoll(&state.RNGState) < 75 {
		damage := EnsureDamage(target)
		damage.DeckDamaged = true
		destroyChance := 1.0
		if state.Doctrines[target.Faction] == "japan" {
			destroyChance = 10
		}
		if probabilityRoll(&state.RNGState) < destroyChance {
			damage.DeckDestroyed = true
			damage.DeckDamaged = false
		}
	}
	if d10(state) > target.Card.DamageControl {
		return severeDamage(state, target)
	}
	return nil
}

func applyTorpedoHit(state *State, hit ShellHit) []EngineEvent {
	target := state.Units[hit.TargetID]
	if target == nil {
		return nil
	}
	events := damageStrength(state, target, 1)
	if target.Strength <= 0 {
		return events
	}
	if d10(state) > target.Card.DamageControl {
		EnsureDamage(target).Tilt = true
	}
	if d10(state) > target.Card.DamageControl {
		damage := EnsureDamage(target)
		damage.Flooding = append(damage.Flooding, 1)
	}
	if probabilityRoll(&state.RNGState) < 50 && d10(state) > target.Card.DamageControl {
		events = append(events, severeDamage(state, target)...)
	}
	return events
}

func applyHits(state *State, hits []ShellHit) []EngineEvent {
	events := []EngineEvent{}
	for _, hit := range hits {
		if hit.Torpedo {
			events = append(events, applyTorpedoHit(state, hit)...)
		} else {
			events = append(events, applyShellHit(state, hit)...)
		}
	}
	return events
}
