package game

import (
	"fmt"
	"sort"
	"strings"
)

// —— 挂载（规则书 43–63、80–83 行）——
//
// 挂载不是飞机类型，而是同一架飞机在某个任务下的武装配置，由两个维度组成：
//   1. 武器：侦察 / 对空 / 鱼雷 / 炸弹 / 转场；
//   2. 是否挂副油箱：除"重型炸弹"外都可手动选择。
//
// 哪些组合可选由机种和飞机的对地/对舰能力共同决定：攻击机没有对空选项，
// 水上飞机只有侦察与转场；炸弹/鱼雷还要看飞机本身有没有对地/对舰值，见 LoadoutOptions。

// Weapon 是一次出击的武器配置（不含副油箱）。
type Weapon string

const (
	WeaponRecon   Weapon = "recon"   // 侦察：无武装，靠眼睛
	WeaponAir     Weapon = "air"     // 对空：战斗机空战/护航
	WeaponTorpedo Weapon = "torpedo" // 鱼雷
	WeaponBomb    Weapon = "bomb"    // 炸弹（反舰/对地）
	// WeaponDepthCharge 是反潜深水炸弹：只有反潜巡逻用得上，母舰的弹药库提供。
	WeaponDepthCharge Weapon = "depth_charge"
	WeaponTransfer    Weapon = "transfer" // 转场：不携带任何武器（战斗机除外）
)

// Loadout 是武器 + 副油箱的组合。
type Loadout string

const (
	// LoadoutNone 是"空载"：机库里的默认状态，没有弹、没有副油箱，航程就是机种基础航程。
	// 空载算子不会自己开始整备：玩家在机库里选一套挂载才是整备的开始。
	LoadoutNone Loadout = ""
	// 侦察
	LoadoutReconTank Loadout = "recon+tank"
	LoadoutRecon     Loadout = "recon"
	// 对空
	LoadoutAirTank Loadout = "air+tank"
	LoadoutAir     Loadout = "air"
	// 鱼雷
	LoadoutTorpedoTank Loadout = "torpedo+tank"
	LoadoutTorpedo     Loadout = "torpedo"
	// 炸弹
	LoadoutBombTank Loadout = "bomb+tank"
	LoadoutBomb     Loadout = "bomb"
	// 反潜深弹
	LoadoutDepthChargeTank Loadout = "depth_charge+tank"
	LoadoutDepthCharge     Loadout = "depth_charge"
	// 转场：规则书 59 行，始终挂副油箱
	LoadoutTransfer Loadout = "transfer+tank"
)

// Weapon 返回挂载对应的武器。
func (l Loadout) Weapon() Weapon {
	switch l {
	case LoadoutReconTank, LoadoutRecon:
		return WeaponRecon
	case LoadoutAirTank, LoadoutAir:
		return WeaponAir
	case LoadoutTorpedoTank, LoadoutTorpedo:
		return WeaponTorpedo
	case LoadoutBombTank, LoadoutBomb:
		return WeaponBomb
	case LoadoutDepthChargeTank, LoadoutDepthCharge:
		return WeaponDepthCharge
	}
	return WeaponTransfer
}

// HasTank 返回该挂载是否挂副油箱。
func (l Loadout) HasTank() bool {
	return l == LoadoutReconTank || l == LoadoutAirTank || l == LoadoutTorpedoTank || l == LoadoutBombTank || l == LoadoutDepthChargeTank || l == LoadoutTransfer
}

// CarriesWeapon 判断挂载是否带弹药（决定投弹后能否获得轻载加成）。
func (l Loadout) CarriesWeapon() bool {
	weapon := l.Weapon()
	return weapon == WeaponBomb || weapon == WeaponTorpedo
}

// IsHeavyBomb 判断是否为"重型炸弹"挂载：炸弹 + 不挂副油箱（规则书 82 行）。
func (l Loadout) IsHeavyBomb() bool {
	return l == LoadoutBomb
}

// Label 是给玩家看的名字。
func (l Loadout) Label() string {
	if l == LoadoutNone {
		return "无挂载"
	}
	names := map[Weapon]string{WeaponRecon: "侦察", WeaponAir: "对空", WeaponTorpedo: "鱼雷", WeaponBomb: "炸弹", WeaponDepthCharge: "反潜深弹", WeaponTransfer: "转场"}
	name := names[l.Weapon()]
	if l.IsHeavyBomb() {
		return "重型炸弹"
	}
	if l.HasTank() && l.Weapon() != WeaponTransfer {
		return name + "（副油箱）"
	}
	if l.Weapon() == WeaponTransfer {
		return "转场（副油箱）"
	}
	return name
}

// LoadoutOrder 是界面上的展示顺序（规则书要求的排列）。
var LoadoutOrder = []Loadout{
	LoadoutDepthChargeTank,
	LoadoutReconTank,
	LoadoutAirTank,
	LoadoutTorpedoTank,
	LoadoutBombTank,
	LoadoutBomb,
	LoadoutTorpedo,
	LoadoutAir,
	LoadoutTransfer,
}

// LoadoutOptions 返回某个机种可选的挂载。
//
// 攻击机（俯冲/鱼雷轰炸机）没有对空选项，但可以挂侦察载荷去搜索；
// 水上飞机只有侦察与转场。
//
// 弹药挂载由飞机自己的对地/对舰值把关，而不是只看机种：
//   - 炸弹：对地 > 0 或对舰 > 0 才能挂（炸弹既能炸机场也能炸船）；
//   - 鱼雷：只有鱼雷机且对舰 > 0 才能挂（没有对舰值的飞机挂鱼雷没有意义）。
//
// 所以对地 0、对舰 0 的纯制空战斗机只会拿到对空/转场，不会出现炸弹选项；
// 反过来，有对地/对舰值的战斗轰炸型可以挂炸弹。
func LoadoutOptions(kind string, seaplane bool, antiGround, antiShip int) []Loadout {
	return LoadoutOptionsFor(kind, seaplane, antiGround, antiShip, false)
}

// LoadoutOptionsFor 在 LoadoutOptions 的基础上显式传入"这型飞机有没有鱼雷挂架"。
// B-26 是水平轰炸机，但能挂鱼雷；B-25 / B-17 没有这个能力，仍然只有炸弹。
func LoadoutOptionsFor(kind string, seaplane bool, antiGround, antiShip int, torpedoCapable bool) []Loadout {
	if seaplane {
		// 水上侦察机/飞行艇也能挂深弹做反潜巡逻（PBY 这类就是大西洋上的反潜主力）。
		return []Loadout{LoadoutReconTank, LoadoutDepthChargeTank, LoadoutDepthCharge, LoadoutTransfer}
	}
	canBomb := antiGround > 0 || antiShip > 0
	canTorpedo := antiShip > 0 && (kind == "torpedo_bomber" || torpedoCapable)
	var options []Loadout
	switch kind {
	case "fighter":
		options = []Loadout{LoadoutAirTank, LoadoutAir, LoadoutTransfer}
		if canBomb {
			// 战斗轰炸型：有对地/对舰值的战斗机可以带炸弹，但默认仍是护航对空。
			options = []Loadout{LoadoutAirTank, LoadoutAir, LoadoutBombTank, LoadoutBomb, LoadoutTransfer}
		}
	case "dive_bomber":
		// 俯冲轰炸机只能挂炸弹或反潜深弹：D3A1/SBD 都没有鱼雷挂架。
		options = []Loadout{LoadoutReconTank, LoadoutDepthChargeTank, LoadoutDepthCharge, LoadoutBombTank, LoadoutBomb, LoadoutTransfer}
	case "level_bomber", "medium_bomber", "heavy_bomber", "strategic_bomber":
		// 水平轰炸机默认挂炸弹；只有显式带鱼雷挂架的机型（B-26）才多出鱼雷选项。
		// B-25 / B-17 没有鱼雷挂架，不能因为同样是水平轰炸机就跟着沾上。
		if torpedoCapable && kind != "strategic_bomber" {
			options = []Loadout{LoadoutReconTank, LoadoutDepthChargeTank, LoadoutDepthCharge, LoadoutTorpedoTank, LoadoutBombTank, LoadoutBomb, LoadoutTorpedo, LoadoutTransfer}
		} else {
			options = []Loadout{LoadoutReconTank, LoadoutDepthChargeTank, LoadoutDepthCharge, LoadoutBombTank, LoadoutBomb, LoadoutTransfer}
		}
	case "torpedo_bomber":
		// 攻击机可以带侦察载荷：没有专用侦察机时，俯冲/鱼雷轰炸机就是搜索主力。
		options = []Loadout{LoadoutReconTank, LoadoutDepthChargeTank, LoadoutDepthCharge, LoadoutTorpedoTank, LoadoutBombTank, LoadoutBomb, LoadoutTorpedo, LoadoutTransfer}
	case "recon":
		options = []Loadout{LoadoutDepthChargeTank, LoadoutDepthCharge, LoadoutReconTank, LoadoutTransfer}
	default:
		options = []Loadout{LoadoutTransfer}
	}
	return filterAmmoLoadouts(options, canBomb, canTorpedo)
}

// LoadoutOptionsWithSupply 在 LoadoutOptionsFor 之上再叠一层补给限制。
//
// 挂载表过去只看"飞机能不能挂"：机型是鱼雷机、对舰值 > 0，机库里就永远能选鱼雷。
// 但弹药是补给点给的——护航航母（CVE）的弹药库没有鱼雷，停在它上面的 TBF 只能挂炸弹；
// 同一架飞机转场到舰队航母（CV/CVL）之后，鱼雷选项就回来了。
// 所以限制跟着补给点走，不写进机型定义。
func LoadoutOptionsWithSupply(kind string, seaplane bool, antiGround, antiShip int, torpedoCapable bool, supply Supply) []Loadout {
	options := LoadoutOptionsFor(kind, seaplane, antiGround, antiShip, torpedoCapable)
	if len(supply.Blocked) == 0 {
		return options
	}
	kept := make([]Loadout, 0, len(options))
	for _, option := range options {
		// 转场不是弹药，任何补给点都得放行。
		if supply.Blocks(option) {
			continue
		}
		kept = append(kept, option)
	}
	return kept
}

// filterAmmoLoadouts 按飞机能力删掉挂不了的弹药，其余选项原样保留。
// 转场永远保留：就算飞机没有武装，也得能飞回后方。
func filterAmmoLoadouts(options []Loadout, canBomb, canTorpedo bool) []Loadout {
	filtered := make([]Loadout, 0, len(options))
	for _, option := range options {
		switch option.Weapon() {
		case WeaponBomb:
			if !canBomb {
				continue
			}
		case WeaponTorpedo:
			if !canTorpedo {
				continue
			}
		}
		filtered = append(filtered, option)
	}
	return filtered
}

// DefaultLoadout 按任务给出默认挂载，所有默认都挂副油箱。
//
// 规则：
//   - 对舰攻击：鱼雷机优先鱼雷；俯冲/水平轰炸机优先炸弹；B-26 的鱼雷需要手动选。
//   - 对地攻击：挂炸弹；战斗机挂对空。
//   - 侦察：侦察挂载。
//   - CAP / 护航：对空挂载。
//   - 转场：转场挂载。
//
// 对地与对舰的区别由 targetKind 表达（StrikeTargetShip / StrikeTargetAirfield）。
// antiGround / antiShip 用来排除飞机根本挂不了的弹药（对地 0、对舰 0 的飞机没有炸弹）。
func DefaultLoadout(kind string, seaplane bool, antiGround, antiShip int, taskKind string, targetKind StrikeTarget) Loadout {
	return DefaultLoadoutFor(kind, seaplane, antiGround, antiShip, taskKind, targetKind, false)
}

// DefaultLoadoutFor 是带鱼雷挂架能力的默认挂载。
func DefaultLoadoutFor(kind string, seaplane bool, antiGround, antiShip int, taskKind string, targetKind StrikeTarget, torpedoCapable bool) Loadout {
	return DefaultLoadoutWithSupply(kind, seaplane, antiGround, antiShip, taskKind, targetKind, torpedoCapable, Supply{})
}

// DefaultLoadoutWithSupply 再带上补给点：护航航母上的鱼雷机默认挂炸弹，
// 不会因为机种是 torpedo_bomber 就被自动配成补给点根本给不出的鱼雷。
func DefaultLoadoutWithSupply(kind string, seaplane bool, antiGround, antiShip int, taskKind string, targetKind StrikeTarget, torpedoCapable bool, supply Supply) Loadout {
	options := LoadoutOptionsWithSupply(kind, seaplane, antiGround, antiShip, torpedoCapable, supply)
	if len(options) == 0 {
		return LoadoutNone
	}
	preferred := preferredWeapon(options, kind, taskKind, targetKind)
	if pick, ok := bestLoadout(options, preferred); ok {
		return pick
	}
	return options[len(options)-1]
}

// preferredWeapon 决定某次任务应该优先用什么武器。
func preferredWeapon(options []Loadout, kind, taskKind string, targetKind StrikeTarget) Weapon {
	hasWeapon := func(weapon Weapon) bool {
		_, ok := bestLoadout(options, weapon)
		return ok
	}
	switch taskKind {
	case "cap":
		// 机库待命（还没有任务）不是转场：按机种的主武器给一个合法挂载，
		// 否则轰炸机会一直顶着"转场"挂载，航程虚高 30%，飞到一半就以为回不了家。
		switch kind {
		case "fighter":
			return WeaponAir
		case "dive_bomber":
			if hasWeapon(WeaponBomb) {
				return WeaponBomb
			}
		case "torpedo_bomber":
			if hasWeapon(WeaponTorpedo) {
				return WeaponTorpedo
			}
		case "recon":
			return WeaponRecon
		}
		if hasWeapon(WeaponAir) {
			return WeaponAir
		}
		return WeaponRecon
	case "recon":
		return WeaponRecon
	case "asw":
		// 反潜巡逻默认挂深弹；侦察机没有深弹挂架时退回侦察载荷。
		if hasWeapon(WeaponDepthCharge) {
			return WeaponDepthCharge
		}
		return WeaponRecon
	case "transfer":
		return WeaponTransfer
	case "attack":
		// 战斗机在攻击编队里是护航，仍然挂对空。
		if kind == "fighter" {
			return WeaponAir
		}
		// 对地攻击一律炸弹。
		if targetKind == StrikeTargetAir {
			if hasWeapon(WeaponBomb) {
				return WeaponBomb
			}
			return WeaponAir
		}
		// 对舰攻击：鱼雷机优先鱼雷；俯冲轰炸机和水平轰炸机默认挂炸弹。
		// B-26 能挂鱼雷，但它的本职仍是中型轰炸机，鱼雷作为手动选项，
		// 不把默认挂载直接改成鱼雷——否则一换任务就绕过玩家选择。
		if kind == "dive_bomber" || kind == "level_bomber" || kind == "medium_bomber" || kind == "heavy_bomber" || kind == "strategic_bomber" {
			if hasWeapon(WeaponBomb) {
				return WeaponBomb
			}
			return WeaponAir
		}
		if hasWeapon(WeaponTorpedo) {
			return WeaponTorpedo
		}
		if hasWeapon(WeaponBomb) {
			return WeaponBomb
		}
		return WeaponAir
	}
	// cap / 护航
	if hasWeapon(WeaponAir) {
		return WeaponAir
	}
	return WeaponRecon
}

// bestLoadout 在候选里找指定武器的挂载，优先带副油箱。
func bestLoadout(options []Loadout, weapon Weapon) (Loadout, bool) {
	for _, option := range options {
		if option.Weapon() == weapon && option.HasTank() {
			return option, true
		}
	}
	for _, option := range options {
		if option.Weapon() == weapon {
			return option, true
		}
	}
	return "", false
}

// ValidateLoadout 校验机种和飞机能力是否能挂这个组合。
func ValidateLoadout(kind string, seaplane bool, antiGround, antiShip int, loadout Loadout) error {
	return ValidateLoadoutFor(kind, seaplane, antiGround, antiShip, loadout, false)
}

// ValidateLoadoutFor 校验带鱼雷挂架能力的机型。
func ValidateLoadoutFor(kind string, seaplane bool, antiGround, antiShip int, loadout Loadout, torpedoCapable bool) error {
	return ValidateLoadoutWithSupply(kind, seaplane, antiGround, antiShip, loadout, torpedoCapable, Supply{})
}

// ValidateLoadoutWithSupply 再校验补给点的弹药：补给点不给的东西挂不上。
// "该去哪儿挂"的说明由调用方用 SupplyRefusal 补，那里才知道母舰/机场叫什么。
func ValidateLoadoutWithSupply(kind string, seaplane bool, antiGround, antiShip int, loadout Loadout, torpedoCapable bool, supply Supply) error {
	// 空载永远合法：把挂载清空等于"卸弹回机库"，是玩家的一个合法选择。
	if loadout == LoadoutNone {
		return nil
	}
	for _, option := range LoadoutOptionsWithSupply(kind, seaplane, antiGround, antiShip, torpedoCapable, supply) {
		if option == loadout {
			return nil
		}
	}
	return fmt.Errorf("%s无法挂载%s", kind, loadout.Label())
}

// SortieBaseRange 返回某种挂载下的"正常航程"（不含备用油）。
//
//   - 转场：base × 1.3 × 1.7 = base × 2.21（副油箱与转场加成是乘算，规则书 59 行）；
//   - 重型炸弹（炸弹且不挂副油箱）：base × 0.7，规则书 82 行；
//   - 其余：挂了副油箱就是 base × 1.3，没挂就是 base。
func SortieBaseRange(base int, loadout Loadout) int {
	if base < 0 {
		base = 0
	}
	if loadout == LoadoutTransfer {
		// 转场：副油箱 1.3 × 转场加成 1.7 = 2.21，只做一次最终四舍五入。
		// 中间不取整，否则 9×1.3=11.7→12、12×1.7=20.4→20 会把两步误差叠起来。
		return roundRatio(base, 221, 100)
	}
	if loadout.IsHeavyBomb() {
		return roundRatio(base, 7, 10)
	}
	if loadout.HasTank() {
		return roundRatio(base, 13, 10)
	}
	return base
}

// MountedFuel 按挂载建立满油账本：副油箱里程、机内里程分开记账。
func MountedFuel(base int, loadout Loadout) Fuel {
	if base < 0 {
		base = 0
	}
	total := SortieBaseRange(base, loadout)
	internal := base
	if loadout.IsHeavyBomb() {
		// 重型炸弹没有副油箱，而且总航程被砍了 30%：全部里程按机内记账。
		internal = total
	}
	fuel := Fuel{Internal: internal, Reserve: ReserveRange}
	if !loadout.IsHeavyBomb() {
		if extra := total - internal; extra > 0 {
			fuel.Tank = extra
		}
	}
	return fuel
}

// roundRatio 计算 round(value × num / den)，对非负数使用四舍五入。
func roundRatio(value, num, den int) int {
	if den <= 0 {
		return value
	}
	product := value * num
	if product >= 0 {
		return (product + den/2) / den
	}
	return -((-product + den/2) / den)
}

// AttackBonus 返回挂载带来的对地/对舰攻击值修正（规则书 82 行：重型炸弹 +2）。
func AttackBonus(loadout Loadout) int {
	if loadout.IsHeavyBomb() {
		return 2
	}
	return 0
}

// EffectiveAntiShip 返回当前挂载实际能用于对舰攻击的值。
// 炸弹正常结算并保留重型炸弹加成，鱼雷保留基础对舰值，其余挂载不能攻击舰船。
func EffectiveAntiShip(loadout Loadout, antiShip int) int {
	switch loadout.Weapon() {
	case WeaponBomb:
		return antiShip + AttackBonus(loadout)
	case WeaponTorpedo:
		return antiShip
	default:
		return 0
	}
}

// EffectiveAntiGround 返回当前挂载实际能用于对地攻击的值。
// 只有炸弹能攻击地面目标；侦察、对空、反潜、转场、鱼雷和空载均为 0。
func EffectiveAntiGround(loadout Loadout, antiGround int) int {
	if loadout.Weapon() != WeaponBomb {
		return 0
	}
	return antiGround + AttackBonus(loadout)
}

// withLoadoutAttack 返回"折过挂载加成"的算子副本，只给玩家视图用。
//
// state 里存的是基础攻击值，结算时自己会加一次 AttackBonus（对舰见 resolveBombingOn、
// 对地见 resolveGroundStrike）。视图是另一条路：玩家把挂载切成重型炸弹，卡片上的
// 对地/对舰就该当场变成 +2，否则只能看到航程缩水，攻击值一点不动，很像没生效。
// 加成绝不能写进 state：结算再加一次就会变成 +4，还会被状态存档带下去。
func withLoadoutAttack(unit AirUnit) AirUnit {
	unit.AntiShip = EffectiveAntiShip(unit.Loadout, unit.AntiShip)
	unit.AntiGround = EffectiveAntiGround(unit.Loadout, unit.AntiGround)
	return unit
}

// withLoadoutAttackGround 是同一个加成在机场算子上的版本：机场那套投影
// 直接下发 AirfieldAirUnit 本体，不经过 AirUnit。
func withLoadoutAttackGround(unit AirfieldAirUnit) AirfieldAirUnit {
	unit.AntiShip = EffectiveAntiShip(unit.Loadout, unit.AntiShip)
	unit.AntiGround = EffectiveAntiGround(unit.Loadout, unit.AntiGround)
	return unit
}

// LightLoadRange 是投弹后的"轻载"加成：当前剩余航程 +20%（规则书 83 行）。
func LightLoadRange(current int) int {
	if current <= 0 {
		return current
	}
	return roundRatio(current, 12, 10)
}

// applyLoadout 按任务类型换上默认挂载，并重算满油航程。
// 只在"尚未起飞"时调用：挂载决定的是这次出击能飞多远、能带多少弹。
//
// 已经手动选过挂载的算子（LoadoutManual）不会被覆盖 —— 机库整备时可以自己定。
func applyLoadout(state State, aircraft *AirUnit) {
	if aircraft == nil || aircraft.Launched {
		return
	}
	// 规则书 59 行：转场不携带任何武器，只挂副油箱再吃那 30% 转场加成。
	// 手动挂载在转场任务里必须让位：以前被 LoadoutManual 一挡，
	// 校验按 2.21× 放行、飞机实际只带 1.3× 的油，飞到半路就透支。
	manual := aircraft.LoadoutManual
	if manual && aircraft.TaskKind != "transfer" {
		return
	}
	targetKind := strikeTargetKind(state, aircraft)
	// 补给点给不出鱼雷时（例如停在护航航母上），默认挂载也不能是鱼雷。
	supply := SupplyOfAircraft(state, aircraft)
	loadout := LoadoutForTaskWithSupply(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, aircraft.TaskKind, targetKind, aircraft.TorpedoCapable, supply)
	applyLoadoutChoice(aircraft, loadout)
	if manual {
		// 武器已经卸下来了，这次手动挂载跟着作废：转场结束后按任务自动配。
		aircraft.LoadoutManual = false
	}
}

// strikeTargetKind 判断这次攻击是对舰还是对地：目标 hex 上有敌方机场就是对地。
func strikeTargetKind(state State, aircraft *AirUnit) StrikeTarget {
	if aircraft == nil || aircraft.TargetCoord == "" {
		return StrikeTargetShip
	}
	if aircraft.TargetFleetID != "" {
		return StrikeTargetShip
	}
	for _, airfield := range state.Airfields {
		if airfield != nil && airfield.Coord == aircraft.TargetCoord && airfield.Faction != aircraft.Faction {
			return StrikeTargetAir
		}
	}
	return StrikeTargetShip
}

// applyLoadoutChoice 用指定挂载重建满油航程。
func applyLoadoutChoice(aircraft *AirUnit, loadout Loadout) {
	if aircraft == nil {
		return
	}
	base := aircraft.BaseRange
	if base <= 0 {
		base = aircraft.MaxRange
	}
	aircraft.Loadout = loadout
	aircraft.TankDropped = false
	aircraft.Bombed = false
	fuel := MountedFuel(base, loadout)
	aircraft.Fuel = fuel
	aircraft.MaxRange = fuel.Available()
	aircraft.RemainingRange = fuel.Available()
}

// LoadoutForTask 是 DefaultLoadout 的便捷包装（保留旧调用点）。
func LoadoutForTask(kind string, seaplane bool, antiGround, antiShip int, taskKind string, targetKind StrikeTarget) Loadout {
	return LoadoutForTaskFor(kind, seaplane, antiGround, antiShip, taskKind, targetKind, false)
}

// LoadoutForTaskFor 是带鱼雷挂架能力的任务挂载。
func LoadoutForTaskFor(kind string, seaplane bool, antiGround, antiShip int, taskKind string, targetKind StrikeTarget, torpedoCapable bool) Loadout {
	return DefaultLoadoutFor(kind, seaplane, antiGround, antiShip, taskKind, targetKind, torpedoCapable)
}

// LoadoutForTaskWithSupply 再带上补给点的弹药限制。
func LoadoutForTaskWithSupply(kind string, seaplane bool, antiGround, antiShip int, taskKind string, targetKind StrikeTarget, torpedoCapable bool, supply Supply) Loadout {
	return DefaultLoadoutWithSupply(kind, seaplane, antiGround, antiShip, taskKind, targetKind, torpedoCapable, supply)
}

// DroppedTank 模拟空战爆发后抛弃副油箱（规则书 48 行）：
// 只损失副油箱里还没烧掉的那部分；已经烧干时抛弃不会减少剩余航程。
func dropTank(aircraft *AirUnit) int {
	if aircraft == nil || aircraft.TankDropped {
		return 0
	}
	aircraft.TankDropped = true
	wasted := aircraft.Fuel.DropTank()
	aircraft.RemainingRange = aircraft.Range()
	return wasted
}

// markBombed 记录投弹并给剩余航程加 20%（规则书 83 行）。
func markBombed(aircraft *AirUnit) {
	if aircraft == nil || aircraft.Bombed || !aircraft.Loadout.CarriesWeapon() {
		return
	}
	aircraft.Bombed = true
	delta := LightLoadRange(aircraft.Range()) - aircraft.Range()
	if delta > 0 {
		aircraft.Fuel.Internal += delta
	}
	aircraft.RemainingRange = aircraft.Range()
}

// validateLoadoutOrder 校验换挂载命令：只能给机库/整备/甲板待命的飞机换挂载，
// 已经起飞的不行（规则书 258 行：飞机已在甲板上时换挂载需要放回机库）。
func validateLoadoutOrder(state State, faction string, order Order) error {
	loadout := Loadout(order.Loadout)
	ids := airOrderIDs(order)
	if len(ids) == 0 {
		return fmt.Errorf("请选择要更换挂载的飞机")
	}
	for _, id := range ids {
		if ground := state.GroundAir[id]; ground != nil {
			// 机场算子换挂载：地面上待命的陆基机同样可以换弹，只是不能用舰载机那套记账。
			if ground.Faction != faction {
				return fmt.Errorf("航空算子不存在或不属于当前玩家")
			}
			if airfieldAirborne(ground) {
				return fmt.Errorf("已经升空的算子不能更换挂载")
			}
			supply := SupplyOfGroundAircraft(state, ground)
			if err := ValidateLoadoutWithSupply(ground.Kind, false, ground.AntiGround, ground.AntiShip, loadout, ground.TorpedoCapable, supply); err != nil {
				// 补给点不给的东西，提示要说到"这个机场缺什么"，而不是笼统的"无法挂载"。
				if refused := supplyRefusal(supply, loadout, SupplySourceLabelGround(state, ground)); refused != nil {
					return refused
				}
				return err
			}
			continue
		}
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Faction != faction {
			return fmt.Errorf("航空算子不存在或不属于当前玩家")
		}
		if aircraft.Launched || airborne(aircraft) {
			return fmt.Errorf("已经升空的算子不能更换挂载")
		}
		if aircraft.Status == "elevator" {
			return fmt.Errorf("%s正在升降机上，等这一轮提升/下降走完再换挂载", aircraft.Model)
		}
		if aircraft.Status == "recovery" {
			return fmt.Errorf("%s还在排队下降，等它回到机库再换挂载", aircraft.Model)
		}
		supply := SupplyOfAircraft(state, aircraft)
		if err := ValidateLoadoutWithSupply(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, loadout, aircraft.TorpedoCapable, supply); err != nil {
			// 母舰的弹药保障不够时，说清楚是哪条船、缺什么、去哪儿能挂上。
			if refused := supplyRefusal(supply, loadout, SupplySourceLabel(state, aircraft)); refused != nil {
				return refused
			}
			return err
		}
	}
	return nil
}

// ApplyLoadoutOrder 给选中的算子换挂载；舰载机换挂载等于"开始整备"。
//
// 机库里选好挂载就开始整备（不需要先拖进任务栏），整备完在机库里待发；
// 空载算子拖上甲板后再选挂载，就在甲板上原地整备。选"无挂载"等于卸弹回机库。
func ApplyLoadoutOrder(state *State, order Order) []EngineEvent {
	loadout := Loadout(order.Loadout)
	events := []EngineEvent{}
	count := 0
	for _, id := range airOrderIDs(order) {
		if ground := state.GroundAir[id]; ground != nil {
			if airfieldAirborne(ground) {
				continue
			}
			applyGroundLoadoutChoice(ground, loadout)
			ground.LoadoutManual = loadout != LoadoutNone
			count++
			continue
		}
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Launched || airborne(aircraft) {
			continue
		}
		if aircraft.Status == "elevator" || aircraft.Status == "recovery" {
			continue
		}
		armAircraft(aircraft, loadout)
		count++
	}
	if count == 0 {
		return nil
	}
	return append(events, EngineEvent{Type: "loadout_changed", Audience: orderFactionFromIDs(*state, order), Text: fmt.Sprintf("%d个算子挂载改为%s", count, loadout.Label())})
}

// armAircraft 把机库/甲板上的舰载机换成指定挂载并重新开始整备。
//
// 整备位置跟着算子现在的位置走：机库里的在机库整备（status service），
// 甲板上的在甲板整备（status service + DeckService），整备完都变成待发。
// 空载（LoadoutNone）不是一次整备：卸掉弹和副油箱，状态退回 hangar/deck。
func armAircraft(aircraft *AirUnit, loadout Loadout) {
	if aircraft == nil {
		return
	}
	if loadout == LoadoutNone {
		applyLoadoutChoice(aircraft, LoadoutNone)
		aircraft.LoadoutManual = false
		aircraft.Armed = false
		aircraft.ServiceTurns = 0
		if aircraft.Status == "service" {
			if aircraft.DeckService {
				aircraft.Status = "deck"
			} else {
				aircraft.Status = "hangar"
			}
		}
		aircraft.DeckService = false
		return
	}
	// 同一套挂载、而且已经整备好了：不重新开工，避免"点一下下拉框又白等一回合"。
	if aircraft.Loadout == loadout && serviceComplete(aircraft) {
		aircraft.LoadoutManual = true
		return
	}
	applyLoadoutChoice(aircraft, loadout)
	// 手动选择要打上标记：之后分配任务不再用默认挂载覆盖它（规则书 51 行）。
	aircraft.LoadoutManual = true
	aircraft.Armed = false
	aircraft.ServiceTurns = 0
	if aircraft.Status == "deck" || aircraft.Status == "ready" {
		aircraft.Status = "service"
		aircraft.DeckService = true
		return
	}
	aircraft.Status = "service"
	aircraft.DeckService = false
}

// orderFactionFromIDs 从命令里的算子反推阵营。
func orderFactionFromIDs(state State, order Order) string {
	for _, id := range airOrderIDs(order) {
		if aircraft := state.Aircraft[id]; aircraft != nil {
			return aircraft.Faction
		}
	}
	return ""
}

// LoadoutOption 是给界面渲染的一个挂载选项（id + 中文名）。
//
// 挂载能不能选只在服务端算一次（机型能力 ∩ 补给点），前端拿这个列表直接渲染；
// 以前前端自己复制了一份"哪个机种能挂什么"的规则，加一条限制就得两边同步改。
type LoadoutOption struct {
	ID    Loadout `json:"id"`
	Label string  `json:"label"`
}

// withLoadoutView 给视图补齐挂载相关字段：可选清单、以及当前挂载的中文名。
// 挂载表、名字都在服务端算，前端只渲染——加一种弹药或一条补给限制时只用改这里。
// 必须在 withLoadoutAttack 之前调用：挂载加成会改对舰/对地值，选项要按机种原始值算。
func withLoadoutView(aircraft AirUnit, supply Supply) AirUnit {
	aircraft.LoadoutOptions = LoadoutOptionsView(aircraft.Kind, aircraft.Seaplane, aircraft.AntiGround, aircraft.AntiShip, aircraft.TorpedoCapable, supply)
	if aircraft.Loadout != LoadoutNone {
		aircraft.LoadoutLabel = aircraft.Loadout.Label()
	}
	return aircraft
}

// LoadoutOptionsView 返回下发给界面的挂载选项。
// 和 ValidateLoadoutWithSupply 走同一个 LoadoutOptionsWithSupply，界面与校验不会分叉。
func LoadoutOptionsView(kind string, seaplane bool, antiGround, antiShip int, torpedoCapable bool, supply Supply) []LoadoutOption {
	options := LoadoutOptionsWithSupply(kind, seaplane, antiGround, antiShip, torpedoCapable, supply)
	result := make([]LoadoutOption, 0, len(options))
	for _, loadout := range options {
		result = append(result, LoadoutOption{ID: loadout, Label: loadout.Label()})
	}
	return result
}

// —— 机场飞机的挂载：与舰载机共用挂载规则，接口一致 ——

// applyGroundLoadout 按任务给机场飞机换上默认挂载。
// 与舰载机一致：玩家手动选过（LoadoutManual）就不再覆盖。
func applyGroundLoadout(state State, aircraft *AirfieldAirUnit) {
	if aircraft == nil || airfieldAirborne(aircraft) {
		return
	}
	// 与舰载机一致：转场把武器全部卸下来，手动挂载不能留到转场任务里。
	manual := aircraft.LoadoutManual
	if manual && aircraft.TaskKind != "transfer" {
		return
	}
	targetKind := StrikeTargetShip
	if aircraft.TargetFleetID != "" {
		targetKind = StrikeTargetShip
	} else if aircraft.TaskKind == "attack" && aircraft.TargetCoord != "" {
		for _, airfield := range state.Airfields {
			if airfield != nil && airfield.Coord == aircraft.TargetCoord && airfield.Faction != aircraft.Faction {
				targetKind = StrikeTargetAir
				break
			}
		}
	}
	loadout := DefaultLoadoutWithSupply(aircraft.Kind, false, aircraft.AntiGround, aircraft.AntiShip, aircraft.TaskKind, targetKind, aircraft.TorpedoCapable, SupplyOfGroundAircraft(state, aircraft))
	applyGroundLoadoutChoice(aircraft, loadout)
	if manual {
		aircraft.LoadoutManual = false
	}
}

// refillFuelForLoadout 让非空中的算子回到满油状态。
// 必须按当前挂载重建"副油箱/机内油/备用油"三部分：早期这里用 SetRange(MaxRange)
// 直接写一个整数，副油箱里程被并进机内油、备用油被清零，结果 dropTank 永远没有
// 可抛弃的里程，"主油耗尽后还能飞 4 格"的备用油规则也永远不会触发负数。
func refillFuelForLoadout(aircraft *AirUnit) {
	if aircraft == nil {
		return
	}
	base := aircraft.BaseRange
	if base <= 0 {
		base = aircraft.MaxRange
	}
	fuel := MountedFuel(base, aircraft.Loadout)
	aircraft.Fuel = fuel
	aircraft.MaxRange = fuel.Available()
	aircraft.RemainingRange = fuel.Available()
	aircraft.TankDropped = false
}

// refillGroundFuelForLoadout 是机场飞机的同一套逻辑。
func refillGroundFuelForLoadout(aircraft *AirfieldAirUnit) {
	if aircraft == nil {
		return
	}
	base := aircraft.BaseRange
	if base <= 0 {
		base = aircraft.MaxRange
	}
	fuel := MountedFuel(base, aircraft.Loadout)
	aircraft.Fuel = fuel
	aircraft.MaxRange = fuel.Available()
	aircraft.RemainingRange = fuel.Available()
	aircraft.TankDropped = false
}

// taskTargetKind 返回这架飞机当前任务的目标类型：只有攻击任务区分对舰/对地。
func taskTargetKind(state State, aircraft *AirUnit) StrikeTarget {
	if aircraft != nil && aircraft.TaskKind == "attack" {
		return strikeTargetKind(state, aircraft)
	}
	return StrikeTargetShip
}

// loadoutTaskMismatch 返回"这架飞机的挂载干不了这个任务"的原因，空串表示没问题。
// 只提示、不拦命令：鱼雷打岸上、挂炸弹做 CAP、挂着炸弹/鱼雷转场都属于这类错配，
// 飞机照样能飞过去，只是投不出弹或者干不了这活。
func loadoutTaskMismatch(kind, taskKind string, targetKind StrikeTarget, loadout Loadout) string {
	if loadout == LoadoutNone {
		// 空载还没选弹：整备时会按任务配弹，不算错配。
		return ""
	}
	weapon := loadout.Weapon()
	switch taskKind {
	case "attack":
		if kind == "fighter" {
			// 攻击编队里的战斗机是护航，只带对空挂载。
			if weapon != WeaponAir {
				return "护航战斗机只能带对空挂载"
			}
			return ""
		}
		if targetKind == StrikeTargetAir {
			if weapon != WeaponBomb {
				return "鱼雷挂载打不了岸上目标"
			}
			return ""
		}
		if weapon != WeaponBomb && weapon != WeaponTorpedo {
			return "对舰攻击需要炸弹或鱼雷"
		}
		return ""
	case "cap", "cover":
		if weapon != WeaponAir {
			return "挂着炸弹/鱼雷执行不了CAP"
		}
		return ""
	case "transfer":
		// 转场一定要换成副油箱（航程不换挂载算不出来），所以不算错配：
		// 由任务自动卸弹、改挂副油箱，提示一句"已经帮你换了"就够了。
		return ""
	case "recon":
		if weapon != WeaponRecon {
			return "侦察任务需要侦察挂载"
		}
		return ""
	case "asw":
		// 反潜巡逻需要深弹：其他弹药挂上去只是占重量，潜艇可打不着。
		if weapon != WeaponDepthCharge {
			return "反潜巡逻需要挂载反潜深弹"
		}
		return ""
	}
	return ""
}

// strikeLoadoutMismatch 判断舰载机的挂载是否干不了当前任务。
func strikeLoadoutMismatch(state State, aircraft *AirUnit) string {
	if aircraft == nil {
		return ""
	}
	targetKind := StrikeTargetShip
	if aircraft.TaskKind == "attack" {
		targetKind = strikeTargetKind(state, aircraft)
	}
	return loadoutTaskMismatch(aircraft.Kind, aircraft.TaskKind, targetKind, aircraft.Loadout)
}

// groundTaskTargetKind 返回岸基算子当前任务的目标类型：只有攻击任务区分对舰/对地。
func groundTaskTargetKind(state State, aircraft *AirfieldAirUnit) StrikeTarget {
	if aircraft != nil && aircraft.TaskKind == "attack" && aircraft.TargetFleetID == "" {
		if airfield := strikeAirfieldAt(state, aircraft.Faction, aircraft.TargetCoord); airfield != nil {
			return StrikeTargetAir
		}
	}
	return StrikeTargetShip
}

// groundStrikeLoadoutMismatch 是机场算子的同一套判断。
func groundStrikeLoadoutMismatch(state State, aircraft *AirfieldAirUnit) string {
	if aircraft == nil {
		return ""
	}
	return loadoutTaskMismatch(aircraft.Kind, aircraft.TaskKind, groundTaskTargetKind(state, aircraft), aircraft.Loadout)
}

// strikeLoadoutMismatchModels 收集编队里挂载干不了当前任务的"机型：原因"，去重排序。
func strikeLoadoutMismatchModels(state State, members []*AirUnit) []string {
	entries := []string{}
	seen := map[string]bool{}
	for _, aircraft := range members {
		reason := strikeLoadoutMismatch(state, aircraft)
		if reason == "" {
			continue
		}
		entry := aircraft.Model + "：" + reason
		if seen[entry] {
			continue
		}
		seen[entry] = true
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return entries
}

// groundStrikeLoadoutMismatchModels 是机场编队的同一套收集。
func groundStrikeLoadoutMismatchModels(state State, members []*AirfieldAirUnit) []string {
	entries := []string{}
	seen := map[string]bool{}
	for _, aircraft := range members {
		reason := groundStrikeLoadoutMismatch(state, aircraft)
		if reason == "" {
			continue
		}
		entry := aircraft.Model + "：" + reason
		if seen[entry] {
			continue
		}
		seen[entry] = true
		entries = append(entries, entry)
	}
	sort.Strings(entries)
	return entries
}

// transferLoadoutNotice 是转场时"自动卸弹加油箱"的说明；本来就挂着副油箱时不提示。
func transferLoadoutNotice(loadout Loadout) string {
	if loadout == LoadoutNone {
		return ""
	}
	switch loadout.Weapon() {
	case WeaponBomb, WeaponTorpedo, WeaponAir:
		return "转场改为副油箱（炸弹/鱼雷已卸下）"
	}
	return ""
}

// strikeLoadoutMismatchHint 是"挂载不适合这个任务"的提示文案：只提示，不换弹，也不踢出编队。
func strikeLoadoutMismatchHint(entries []string) string {
	return "这个机队有些挂载不适合这个任务（" + strings.Join(entries, "；") + "）。飞过去也投不出弹，要换弹得先回机库/跑道重新整备。"
}

// applyGroundLoadoutChoice 用指定挂载重建机场飞机的满油航程。
func applyGroundLoadoutChoice(aircraft *AirfieldAirUnit, loadout Loadout) {
	if aircraft == nil {
		return
	}
	base := aircraft.BaseRange
	if base <= 0 {
		base = aircraft.MaxRange
	}
	aircraft.Loadout = loadout
	aircraft.TankDropped = false
	aircraft.Bombed = false
	fuel := MountedFuel(base, loadout)
	aircraft.Fuel = fuel
	aircraft.MaxRange = fuel.Available()
	aircraft.SetRange(fuel.Available())
}

// dropGroundTank 抛副油箱，返回浪费的里程。
func dropGroundTank(aircraft *AirfieldAirUnit) int {
	if aircraft == nil || aircraft.TankDropped {
		return 0
	}
	aircraft.TankDropped = true
	wasted := aircraft.Fuel.DropTank()
	aircraft.SetRange(aircraft.Range())
	return wasted
}

// markGroundBombed 投弹后的轻载加成。
func markGroundBombed(aircraft *AirfieldAirUnit) {
	if aircraft == nil || aircraft.Bombed || !aircraft.Loadout.CarriesWeapon() {
		return
	}
	aircraft.Bombed = true
	delta := LightLoadRange(aircraft.Range()) - aircraft.Range()
	if delta > 0 {
		aircraft.Fuel.Internal += delta
	}
	aircraft.SetRange(aircraft.Range())
}
