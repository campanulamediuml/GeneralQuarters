package game

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// HitCause 是这次命中的来源。后面按规则分流全靠它：
// 整装待发飞机的损失是炸弹专属，装甲航母 25% 甲板损坏是炸弹专属，漏油只有鱼雷才可能抽到。
// HitFire 不是命中，是"起火燃烧每回合的蔓延判定"，和命中分开是因为它能抽到的表也不一样。
type HitCause uint8

const (
	HitShell   HitCause = iota // 舰炮、岸轰
	HitBomb                    // 航空炸弹
	HitTorpedo                 // 鱼雷（舰载/航空）
	HitFire                    // 大火失控等非命中来源
)

func (cause HitCause) String() string {
	switch cause {
	case HitBomb:
		return "炸弹"
	case HitTorpedo:
		return "鱼雷"
	case HitFire:
		return "火灾"
	default:
		return "炮弹"
	}
}

type Hit struct {
	AttackerID string
	TargetID   string
	Cause      HitCause
	// Warned 表示本次空袭已被目标方提前发现，DCT 临时 +1（规则书 408 行）
	Warned bool
}

func d10(state *State) int {
	return int(nextUint64(&state.RNGState)%10) + 1
}

func d6(state *State) int {
	return int(nextUint64(&state.RNGState)%6) + 1
}

func addFire(unit *Unit, fire string) {
	damage := EnsureDamage(unit)
	if fire == "burning" {
		damage.Fire = "burning"
		damage.SmallFireHits = 0
		return
	}
	// 小型火灾：同一回合内再次触发就升级为起火燃烧（规则书 432 行）。
	// 回合开始小火自动扑灭并清零计数，所以跨回合的两把小火不会叠加成大火。
	if damage.Fire == "small" {
		damage.SmallFireHits++
		if damage.SmallFireHits >= 2 {
			damage.Fire = "burning"
			damage.SmallFireHits = 0
		}
		return
	}
	if damage.Fire == "" {
		damage.Fire = "small"
		damage.SmallFireHits = 1
	}
}

// sinkUnit 让舰船沉没：先记战果台账，再把舰上/空中的航空算子按规则处理，最后移出战场。
//
// 对舰攻击结算期间（DeferredSink）不立即移除：规则书规定"一轮投弹里所有炸弹都会砸上去"，
// 半路打沉不能打断这一轮，沉没只影响它后面的那轮轻防空和后续攻击。等这一轮投完
// 再由 flushDeferredSinks 统一移除。
func sinkUnit(state *State, unit *Unit) []EngineEvent {
	if state.DeferredSink {
		for _, pending := range state.PendingSinks {
			if pending != nil && pending.ID == unit.ID {
				return nil
			}
		}
		unit.Strength = 0
		state.PendingSinks = append(state.PendingSinks, unit)
		return nil
	}
	// 沉没即记账，战果页面直接读这份台账；不需要等到回合结束对账。
	RecordShipLoss(state, unit)
	for id, aircraft := range state.Aircraft {
		if aircraft.CarrierID != unit.ID {
			continue
		}
		if airborne(aircraft) {
			// 已经在编队里的攻击机不能因为母舰此刻沉没就中止任务：
			// 同一回合双方的空袭是依次结算的，先结算的一方把对手母舰打沉，
			// 对手还没结算的编队就会被整队改成回收、一架都投不出弹——等于白送先手。
			// 让它们按原任务打完，返航时再走"失去母舰"的转场/备降流程。
			if aircraft.Status == "attack" || aircraft.Status == "attack_holding" {
				continue
			}
			// 母舰沉没不等于飞机当场坠毁：把空中算子转成"失去母舰"的回收状态，
			// 由下一回合的回收流程按真实战况结算；已经在舰上的算子随舰沉没一起消失。
			aircraft.Status = "recovery"
			aircraft.Launched = false
			aircraft.Returning = true
			continue
		}
		delete(state.Aircraft, id)
	}
	delete(state.Units, unit.ID)
	return []EngineEvent{{Type: "unit_destroyed", Audience: "all", Text: unit.Name + "沉没，舰上航空算子一并移除"}}
}

// flushDeferredSinks 把这一轮投弹期间攒下的沉没一次性结算掉（移除、记录、发事件）。
func flushDeferredSinks(state *State) []EngineEvent {
	if len(state.PendingSinks) == 0 {
		return nil
	}
	pending := state.PendingSinks
	state.PendingSinks = nil
	state.DeferredSink = false
	events := []EngineEvent{}
	for _, unit := range pending {
		if unit == nil || state.Units[unit.ID] == nil {
			continue
		}
		events = append(events, sinkUnit(state, unit)...)
	}
	return events
}

// sweepSunkUnits 把还留在 state.Units 里的 0 生命值舰船按沉没收尾。
// 正常路径 sinkUnit 会直接移除，但延迟沉没（PendingSinks）一旦中途没被 flush，
// 就会留下 Strength=0 的幽灵舰：舰船面板显示 0/1 还挂着"沉没"标记，编队人数、
// 重防空合计、舰船上限都还会把它算进去。每回合开回合前扫一遍清掉。
func sweepSunkUnits(state *State) []EngineEvent {
	if state.DeferredSink {
		state.DeferredSink = false
	}
	events := flushDeferredSinks(state)
	ids := make([]string, 0)
	for id, unit := range state.Units {
		if unit != nil && unit.Strength <= 0 && unit.MaxStrength > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if unit := state.Units[id]; unit != nil {
			events = append(events, sinkUnit(state, unit)...)
		}
	}
	return events
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
			damage.SmallFireHits = 0
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
		// 漏油：每 3 回合掷一次 d10，≤ 当前损管值即修复，失败就再等 3 回合。
		if damage.OilLeak {
			if damage.OilLeakTurns > 0 {
				damage.OilLeakTurns--
			}
			if damage.OilLeakTurns <= 0 {
				if d10(state) <= unit.Card.DamageControl {
					damage.OilLeak = false
					damage.OilLeakTurns = 0
					events = append(events, EngineEvent{Type: "oil_leak_repaired", Audience: unit.Faction, Text: unit.Name + "的漏油已修复"})
				} else {
					damage.OilLeakTurns = 3
				}
			}
		}
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
			events = append(events, severeDamage(state, unit, HitFire)...)
		}
	}
	return events
}

// severeDamage 抽一条严重伤害。表按命中来源和船只装备拼（见 severeDamageTable）。
func severeDamage(state *State, unit *Unit, cause HitCause) []EngineEvent {
	return severeDamageFrom(state, unit, cause, nil)
}

// severeDamageFrom 带攻击方抽表：炮弹有口径限制，鱼雷和火灾传 nil 不设限。
func severeDamageFrom(state *State, unit *Unit, cause HitCause, attacker *Unit) []EngineEvent {
	table := severeDamageTableFrom(cause, unit, attacker)
	result := table[nextUint64(&state.RNGState)%uint64(len(table))]
	return applySevereDamageResult(state, unit, result)
}

// severeDamageTable 按命中来源和船只装备拼出这次能抽到的严重伤害表。
// 全局表只放任何船都可能中的条目；雷达相关的两条都挂在"装备对海雷达"的额外表里，
// 否则没有雷达的驱逐舰会白掷到一条毫无效果的"火控雷达损坏"。
// 漏油只有鱼雷和炮弹命中才可能抽到——炸弹和火灾抽到"漏油"等于白给一条结果。
func severeDamageTable(cause HitCause, unit *Unit) []string {
	return severeDamageTableFrom(cause, unit, nil)
}

// severeDamageTableFrom 多带一个攻击方：小口径的炮弹打不穿大船的船体，
// 目标比攻击方高 2 档以上抽不到「严重进水」「弹药爆炸」，高 3 档以上连「船体损伤」也抽不到。
// 只有炮弹受这条限制，而且航母不经打、不吃这个豁免；鱼雷和火灾（attacker == nil）照旧。
func severeDamageTableFrom(cause HitCause, unit *Unit, attacker *Unit) []string {
	table := []string{"small_fire", "burning", "flooding", "dc_down", "ammo_explosion"}
	if cause == HitTorpedo || cause == HitShell {
		table = append(table, "oil_leak")
	}
	table = append(table, "severe_flooding", "hull_damage", "aa_loss")
	// 漏油的船到处是燃油：任何一次损伤判定都更容易点着，
	// 所以火灾条目各多一份（小型火灾、起火燃烧各 +1）。
	if unit.Damage != nil && unit.Damage.OilLeak {
		table = append(table, "small_fire", "burning")
	}
	if IsCarrier(unit) {
		table = append(table, "deck_damage", "deck_destroyed", "elevator_damage", "avgas_explosion", "tilt", "hangar_damage")
	}
	if unit.Card.AirRadar {
		table = append(table, "air_radar_damage")
	}
	if unit.Card.SurfaceRadar {
		table = append(table, "fire_control_radar_damage", "surface_radar_damage")
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
	// 航母没有装甲防护，不享受"小口径打不穿"的豁免：驱逐舰的炮弹照样能打穿
	// 航母的严重进水和弹药爆炸。这一条只保护有船体装甲的船。
	if cause == HitShell && attacker != nil && !IsCarrier(unit) {
		gap := shellDamageRank(unit.Type) - shellDamageRank(attacker.Type)
		if gap <= -2 {
			// 小口径打不穿大船：既打不出致命进水，也点不着弹药库。
			table = dropDamageResults(table, "severe_flooding", "ammo_explosion")
		}
		if gap <= -3 {
			table = dropDamageResults(table, "hull_damage")
		}
	}
	return table
}

// dropDamageResults 从伤害表里剔掉指定条目：口径不够的炮弹砸不穿战列舰的船体，
// 与其让它抽到再改成"无效果"，不如直接不进表。
func dropDamageResults(table []string, drop ...string) []string {
	kept := make([]string, 0, len(table))
	for _, entry := range table {
		skip := false
		for _, blocked := range drop {
			if entry == blocked {
				skip = true
				break
			}
		}
		if !skip {
			kept = append(kept, entry)
		}
	}
	return kept
}

// applySevereDamageResult 把一条严重伤害落到船上；抽表和落表拆开，方便逐条测试效果。
func applySevereDamageResult(state *State, unit *Unit, result string) []EngineEvent {
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
		if damage.OilLeakTurns <= 0 {
			damage.OilLeakTurns = 3
		}
	case "severe_flooding":
		events = append(events, damageStrength(state, unit, unit.Strength)...)
	case "hull_damage":
		events = append(events, damageStrength(state, unit, 1)...)
	case "aa_loss":
		unit.Card.HeavyAA *= 0.9
		unit.Card.LightAA *= 0.9
	case "fire_control_radar_damage":
		damage.FireControlRadarDisabled = true
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
		events = append(events, applyTilt(state, unit)...)
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
			before := unit.Card.MainGunCount
			unit.Card.MainGunCount--
			// 按剩余炮塔比例削减炮击值：这样 JSON 里显式写的 gunnery 也不会被折算公式覆盖，
			// 拆掉一座炮塔就按 "剩下的炮塔/原来的炮塔" 等比掉。
			unit.Card.Gunnery = int(math.Round(float64(unit.Card.Gunnery) * float64(unit.Card.MainGunCount) / float64(before)))
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
		"small_fire": "小型火灾", "burning": "起火燃烧", "flooding": "进水", "dc_down": "损管能力降低", "ammo_explosion": "弹药爆炸", "oil_leak": "漏油", "severe_flooding": "严重进水", "hull_damage": "船体损伤", "aa_loss": "防空炮组伤亡", "fire_control_radar_damage": "火控雷达损坏", "surface_radar_damage": "对海搜索雷达损坏", "air_radar_damage": "对空雷达损坏", "deck_damage": "飞行甲板损伤", "deck_destroyed": "飞行甲板摧毁", "elevator_damage": "升降机损坏", "avgas_explosion": "航空燃油爆炸", "tilt": "进水倾斜", "hangar_damage": "机库损伤", "main_gun_damage": "主炮毁坏", "torpedo_disabled": "鱼雷发射器毁坏", "torpedo_explosion": "鱼雷爆炸", "sonar_damage": "声呐损坏", "cargo_loss": "物资损失",
	}
	return names[id]
}

// dctValue 返回本次命中结算时使用的损管值。
// 被空袭时如果来袭机群已被提前发现（雷达/预警），本次空袭的所有 DCT 临时 +1（规则书 408 行）。
func dctValue(state *State, target *Unit, warned bool) int {
	value := target.Card.DamageControl
	if warned && state != nil {
		value++
	}
	return value
}

// readyAircraft 返回航母上整装待发的飞机，按规则书 419–423 行拆成机库与甲板两批：
// 机库 = 已经整备完、还在机库里的（hangar+ready/armed）；甲板 = 提升上来等起飞或已在甲板的。
func readyAircraft(state State, carrierID string) (hangar, deck []*AirUnit) {
	for _, id := range sortedAirUnitIDs(state) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.CarrierID != carrierID || !aircraft.Armed {
			continue
		}
		switch {
		case aircraft.Status == "service" && aircraft.DeckService:
			// 甲板上整备的算子就在甲板作业面上，被炸时按甲板那一批算。
			deck = append(deck, aircraft)
		case aircraft.Status == "hangar", aircraft.Status == "service":
			hangar = append(hangar, aircraft)
		case aircraft.Status == "deck", aircraft.Status == "ready":
			deck = append(deck, aircraft)
		}
	}
	return hangar, deck
}

// destroyAircraftGroup 随机摧毁一组飞机里的 n 个算子（n 至多 2，规则书 421–422 行）。
func destroyAircraftGroup(state *State, carrier *Unit, group []*AirUnit, count int) []string {
	names := []string{}
	remaining := append([]*AirUnit{}, group...)
	for index := 0; index < count && len(remaining) > 0; index++ {
		pick := int(nextUint64(&state.RNGState) % uint64(len(remaining)))
		victim := remaining[pick]
		names = append(names, victim.Model)
		carrier.Card.Aircraft--
		delete(state.Aircraft, victim.ID)
		remaining = append(remaining[:pick], remaining[pick+1:]...)
	}
	return names
}

// hangarExplosionTable 是机库存放整装待发飞机、被炸弹命中且 DCT 失败时的小表。
// 这里不抽全局严重伤害表：挂弹飞机连环爆只有三种结果，也正是这一条让整装待发的航母
// 有可能被一发炸弹炸出油气爆炸。甲板上没有这个结算（飞机在甲板上不构成封闭空间）。
var hangarExplosionTable = []string{"avgas_explosion", "ammo_explosion", "small_fire"}

// hangarDCTRoll 是机库爆炸判定的 d10：卡面带 HangarAdvantage 的航母取优势（两投取小），
// 其余取劣势（两投取大，油气积聚更容易炸）。这是单舰属性，和阵营无关。
// DCT 失败的条件是骰点大于损管值。
//
// sealed 表示机库被装甲甲板封死（装甲航母被打穿）：油气排不出去，爆炸判定强制劣势，
// 就算卡面上有 hangar_advantage 也不给优势。
func hangarDCTRoll(state *State, unit *Unit, sealed bool) int {
	// 封闭机库（装甲航母被打穿）强制劣势，卡面上的优势档位不让用。
	if sealed || unit.Card.HangarAdvantage <= 0 {
		return max(d10(state), d10(state))
	}
	// 2 骰优势投两次取小，3 骰优势投三次取小。
	dice := int(unit.Card.HangarAdvantage)
	best := d10(state)
	for index := 1; index < dice; index++ {
		if roll := d10(state); roll < best {
			best = roll
		}
	}
	return best
}

// bombHangarDeckEffect 是炸弹命中航母时的"整装待发飞机"结算（规则书 421–423 行）：
// 机库组和甲板组各自结算——摧毁至多 2 个算子并引起小型火灾；
// 机库组额外掷一次 DCT，失败时从 hangarExplosionTable 抽一条（航空燃油爆炸/弹药爆炸/小型火灾）。
// 装甲航母被炸弹命中时只损伤甲板飞机，机库飞机不受影响（规则书 486–489 行），
// 因此这里只在普通航母上调用。
func bombHangarDeckEffect(state *State, target *Unit, warned bool) []EngineEvent {
	events := []EngineEvent{}
	hangar, deck := readyAircraft(*state, target.ID)
	if len(hangar) > 0 {
		if names := destroyAircraftGroup(state, target, hangar, 2); len(names) > 0 {
			events = append(events, EngineEvent{Type: "hangar_aircraft_lost", Audience: target.Faction, Text: fmt.Sprintf("炸弹命中%s机库，摧毁整装待发的%s共%d个算子", target.Name, strings.Join(names, "、"), len(names))})
		}
		addFire(target, "small")
		// 机库里是挂好弹的飞机：失败就从那张三选一的小表里抽。
		if hangarDCTRoll(state, target, false) > dctValue(state, target, warned) {
			result := hangarExplosionTable[nextUint64(&state.RNGState)%uint64(len(hangarExplosionTable))]
			events = append(events, applySevereDamageResult(state, target, result)...)
		}
	}
	if len(deck) > 0 {
		if names := destroyAircraftGroup(state, target, deck, 2); len(names) > 0 {
			events = append(events, EngineEvent{Type: "deck_aircraft_lost", Audience: target.Faction, Text: fmt.Sprintf("炸弹命中%s飞行甲板，摧毁整装待发的%s共%d个算子", target.Name, strings.Join(names, "、"), len(names))})
		}
		addFire(target, "small")
	}
	return events
}

// armoredCarrierBombEffect 是炸弹命中装甲航母时的"整装待发飞机"结算（规则书 520 行）：
// 机库藏在装甲甲板下面，普通命中打不到——先额外过一次 DCT，失败（骰点大于损管值）才算
// 把装甲甲板打穿。打穿之后机库是封闭的，油气排不出去，机库爆炸那次 DCT 强制劣势（取大）。
// 甲板上的整装待发飞机不受装甲保护，照常结算（装甲甲板挡不住摆在甲板上的飞机）。
func armoredCarrierBombEffect(state *State, target *Unit, warned bool) []EngineEvent {
	events := []EngineEvent{}
	hangar, deck := readyAircraft(*state, target.ID)
	if len(deck) > 0 {
		if names := destroyAircraftGroup(state, target, deck, 2); len(names) > 0 {
			events = append(events, EngineEvent{Type: "deck_aircraft_lost", Audience: target.Faction, Text: fmt.Sprintf("炸弹命中%s飞行甲板，摧毁整装待发的%s共%d个算子", target.Name, strings.Join(names, "、"), len(names))})
		}
		addFire(target, "small")
	}
	if len(hangar) == 0 {
		return events
	}
	// 击穿装甲甲板本身要过一次 DCT：损管成功＝甲板扛住了，机库里的飞机没事。
	if d10(state) <= dctValue(state, target, warned) {
		events = append(events, EngineEvent{Type: "armored_deck_held", Audience: target.Faction, Text: fmt.Sprintf("%s的装甲甲板挡住了炸弹，机库里的整装待发飞机未受损失", target.Name)})
		return events
	}
	events = append(events, EngineEvent{Type: "armored_deck_penetrated", Audience: target.Faction, Text: fmt.Sprintf("炸弹击穿%s的装甲甲板", target.Name)})
	if names := destroyAircraftGroup(state, target, hangar, 2); len(names) > 0 {
		events = append(events, EngineEvent{Type: "hangar_aircraft_lost", Audience: target.Faction, Text: fmt.Sprintf("炸弹打进%s机库，摧毁整装待发的%s共%d个算子", target.Name, strings.Join(names, "、"), len(names))})
	}
	addFire(target, "small")
	// 封闭机库：油气排不出去，机库爆炸判定强制劣势。
	if hangarDCTRoll(state, target, true) > dctValue(state, target, warned) {
		result := hangarExplosionTable[nextUint64(&state.RNGState)%uint64(len(hangarExplosionTable))]
		events = append(events, applySevereDamageResult(state, target, result)...)
	}
	return events
}

// shellDamageRank 把舰种映射到"装甲档次"，数字越小越扛揍：
// 1 = BB、2 = BC、3 = CA / CV（航母和重巡同级）、4 = CL / CVL（轻母和轻巡同级）、
// 5 = DD、6 = CVE / TRS（护航航母和运输船同级，垫底）。
func shellDamageRank(unitType string) int {
	switch unitType {
	case "BB":
		return 1
	case "BC":
		return 2
	case "CA", "CV":
		return 3
	case "CL", "CVL":
		return 4
	case "CVE", "TRS":
		return 6
	default:
		return 5
	}
}

// shellDamageChance 是这一发炮弹直接打掉 1 点生命值的概率（%），只看双方档次差：
// 同级 1%、低一档 2%、低两档 4%、低三档 8%、低四档 10%、低五档 12%。
// 打比自己大（或同为高一档以上）的目标是 0%——这种命中只能去摇 DCT 的伤害表。
func shellDamageChance(attacker, target *Unit) float64 {
	chances := []float64{1, 2, 4, 8, 10, 12}
	diff := 0
	if attacker != nil {
		diff = shellDamageRank(target.Type) - shellDamageRank(attacker.Type)
	}
	if diff < 0 {
		return 0
	}
	if diff >= len(chances) {
		return chances[len(chances)-1]
	}
	return chances[diff]
}

// applyGunOrBombHit 结算炮弹和炸弹命中：两者共用防空、甲板与 DCT 流程，
// 差别在装甲航母的甲板损坏概率（炸弹 25%，规则书 490 行），
// 以及"整装待发飞机被摧毁"只有炸弹才会发生（规则书 421–423 行）。
func applyGunOrBombHit(state *State, hit Hit) []EngineEvent {
	target := state.Units[hit.TargetID]
	if target == nil {
		return nil
	}
	target.Card.HeavyAA *= 0.9
	target.Card.LightAA *= 0.9
	if IsCarrier(target) && probabilityRoll(&state.RNGState) < deckDamageChance(hit.Cause, target) {
		damage := EnsureDamage(target)
		damage.DeckDamaged = true
		destroyChance := rulesFor(*state, target.Faction).DeckDestroyChance
		if probabilityRoll(&state.RNGState) < destroyChance {
			damage.DeckDestroyed = true
			damage.DeckDamaged = false
		}
	}
	events := []EngineEvent{}
	if d10(state) > dctValue(state, target, hit.Warned) {
		events = append(events, severeDamageFrom(state, target, hit.Cause, state.Units[hit.AttackerID])...)
	}
	// 只有炸弹会打击整装待发的飞机：炮弹打穿机库再点着飞机是另一条规则，这里没有。
	if hit.Cause == HitBomb && IsCarrier(target) && state.Units[target.ID] != nil {
		if target.Card.ArmoredCarrier {
			events = append(events, armoredCarrierBombEffect(state, target, hit.Warned)...)
		} else {
			events = append(events, bombHangarDeckEffect(state, target, hit.Warned)...)
		}
	}
	if state.Units[target.ID] == nil {
		return events
	}
	// 炮弹直击要害：不管 DCT 过不过，都按双方档次差再判一次直接扣 1 点生命值。
	// 大口径砸薄壳（BB 打 DD）能一炮带走，小船还击战列舰则只有摇伤害表这条路。
	if hit.Cause == HitShell {
		attacker := state.Units[hit.AttackerID]
		if chance := shellDamageChance(attacker, target); chance > 0 &&
			probabilityRoll(&state.RNGState) < chance {
			events = append(events, damageStrength(state, target, 1)...)
		}
	}
	return events
}

// deckDamageChance 返回一次命中损坏飞行甲板的概率（%）。
// 装甲航母的甲板只对炸弹硬（25%，规则书 490 行）；炮弹命中仍按普通 75% 判。
func deckDamageChance(cause HitCause, target *Unit) float64 {
	if cause == HitBomb && target.Card.ArmoredCarrier {
		return 25
	}
	return 75
}

// applyTilt 施加倾斜。已经处在倾斜状态（还没修好）又中一次时，不再重复记倾斜：
// 这次判定转为"多一层进水 + 生命值 -1"（规则书 435 行）。
func applyTilt(state *State, unit *Unit) []EngineEvent {
	damage := EnsureDamage(unit)
	if damage.Tilt {
		damage.Flooding = append(damage.Flooding, 1)
		return damageStrength(state, unit, 1)
	}
	damage.Tilt = true
	return nil
}

func applyTorpedoHit(state *State, hit Hit) []EngineEvent {
	target := state.Units[hit.TargetID]
	if target == nil {
		return nil
	}
	events := damageStrength(state, target, 1)
	if target.Strength <= 0 {
		return events
	}
	if d10(state) > dctValue(state, target, hit.Warned) {
		events = append(events, applyTilt(state, target)...)
	}
	if d10(state) > dctValue(state, target, hit.Warned) {
		damage := EnsureDamage(target)
		damage.Flooding = append(damage.Flooding, 1)
	}
	// 先掷 50% 严重伤害判定，再掷损管判定；不能依赖 && 的短路求值，否则两次判定共用一次 RNG，
	// 固定随机种子的回归测试也就失去可复现性。
	if probabilityRoll(&state.RNGState) < 50 {
		if d10(state) > dctValue(state, target, hit.Warned) {
			events = append(events, severeDamage(state, target, HitTorpedo)...)
		}
	}
	return events
}

func applyHits(state *State, hits []Hit) []EngineEvent {
	events := []EngineEvent{}
	for _, hit := range hits {
		if hit.Cause == HitTorpedo {
			events = append(events, applyTorpedoHit(state, hit)...)
			continue
		}
		events = append(events, applyGunOrBombHit(state, hit)...)
	}
	return events
}
