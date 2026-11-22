package game

import "fmt"

// —— 挂载（规则书 43–63、80–83 行）——
//
// 挂载不是飞机类型，而是同一架飞机在某个任务下的武装配置，由两个维度组成：
//   1. 武器：侦察 / 对空 / 鱼雷 / 炸弹 / 转场；
//   2. 是否挂副油箱：除"重型炸弹"外都可手动选择。
//
// 哪些组合可选由机种决定（攻击机没有对空选项、战斗机没有炸弹鱼雷选项、
// 水上飞机只有侦察与转场），见 LoadoutOptions。

// Weapon 是一次出击的武器配置（不含副油箱）。
type Weapon string

const (
	WeaponRecon    Weapon = "recon"    // 侦察：无武装，靠眼睛
	WeaponAir      Weapon = "air"      // 对空：战斗机空战/护航
	WeaponTorpedo  Weapon = "torpedo"  // 鱼雷
	WeaponBomb     Weapon = "bomb"     // 炸弹（反舰/对地）
	WeaponTransfer Weapon = "transfer" // 转场：不携带任何武器（战斗机除外）
)

// Loadout 是武器 + 副油箱的组合。
type Loadout string

const (
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
	}
	return WeaponTransfer
}

// HasTank 返回该挂载是否挂副油箱。
func (l Loadout) HasTank() bool {
	return l == LoadoutReconTank || l == LoadoutAirTank || l == LoadoutTorpedoTank || l == LoadoutBombTank || l == LoadoutTransfer
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
	names := map[Weapon]string{WeaponRecon: "侦察", WeaponAir: "对空", WeaponTorpedo: "鱼雷", WeaponBomb: "炸弹", WeaponTransfer: "转场"}
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
// 战斗机没有炸弹鱼雷；水上飞机只有侦察与转场。
// 空地两用的攻击机（能挂炸弹）额外保留重型炸弹与裸炸弹选项。
func LoadoutOptions(kind string, seaplane bool) []Loadout {
	if seaplane {
		return []Loadout{LoadoutReconTank, LoadoutTransfer}
	}
	switch kind {
	case "fighter":
		return []Loadout{LoadoutAirTank, LoadoutAir, LoadoutTransfer}
	case "dive_bomber":
		// 俯冲轰炸机只能挂炸弹：D3A1/SBD 都没有鱼雷挂架。
		return []Loadout{LoadoutReconTank, LoadoutBombTank, LoadoutBomb, LoadoutTransfer}
	case "torpedo_bomber":
		// 攻击机可以带侦察载荷：没有专用侦察机时，俯冲/鱼雷轰炸机就是搜索主力。
		return []Loadout{LoadoutReconTank, LoadoutTorpedoTank, LoadoutBombTank, LoadoutBomb, LoadoutTorpedo, LoadoutTransfer}
	case "recon":
		return []Loadout{LoadoutReconTank, LoadoutTransfer}
	}
	return []Loadout{LoadoutTransfer}
}

// DefaultLoadout 按任务给出默认挂载，所有默认都挂副油箱。
//
// 规则：
//   - 对舰攻击：有鱼雷可挂就挂鱼雷，否则挂炸弹；战斗机挂对空。
//   - 对地攻击：挂炸弹；战斗机挂对空。
//   - 侦察：侦察挂载。
//   - CAP / 护航：对空挂载。
//   - 转场：转场挂载。
//
// 对地与对舰的区别由 targetKind 表达（StrikeTargetShip / StrikeTargetAirfield）。
func DefaultLoadout(kind string, seaplane bool, taskKind string, targetKind StrikeTarget) Loadout {
	options := LoadoutOptions(kind, seaplane)
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
		// 对舰攻击：鱼雷机优先鱼雷，俯冲轰炸机只能挂炸弹。
		// 以前是"能挂鱼雷就挂鱼雷"，而俯冲轰炸机的可选挂载里也列了鱼雷，
		// 于是 D3A1/SBD 出击时会顶着鱼雷出发——现实中这两型都挂不了鱼雷。
		if kind == "dive_bomber" {
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

// ValidateLoadout 校验机种是否能挂这个组合。
func ValidateLoadout(kind string, seaplane bool, loadout Loadout) error {
	for _, option := range LoadoutOptions(kind, seaplane) {
		if option == loadout {
			return nil
		}
	}
	return fmt.Errorf("%s无法挂载%s", kind, loadout.Label())
}

// SortieBaseRange 返回某种挂载下的"正常航程"（不含备用油）。
//
//   - 转场：base × 1.3（副油箱）× 1.3（转场加成），规则书 59 行；
//   - 重型炸弹（炸弹且不挂副油箱）：base × 0.7，规则书 82 行；
//   - 其余：挂了副油箱就是 base × 1.3，没挂就是 base。
func SortieBaseRange(base int, loadout Loadout) int {
	if base < 0 {
		base = 0
	}
	if loadout == LoadoutTransfer {
		// 规则书只要求最终结果四舍五入，中间不取整，
		// 否则 9×1.3×1.3 会被两步取整抬成 16，而规则书给的是 15。
		return roundRatio(base, 169, 100)
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
	if aircraft == nil || aircraft.Launched || aircraft.LoadoutManual {
		return
	}
	targetKind := strikeTargetKind(state, aircraft)
	loadout := LoadoutForTask(aircraft.Kind, aircraft.Seaplane, aircraft.TaskKind, targetKind)
	applyLoadoutChoice(aircraft, loadout)
}

// strikeTargetKind 判断这次攻击是对舰还是对地：目标 hex 上有敌方机场就是对地。
func strikeTargetKind(state State, aircraft *AirUnit) StrikeTarget {
	if aircraft == nil || aircraft.TargetCoord == "" {
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
func LoadoutForTask(kind string, seaplane bool, taskKind string, targetKind StrikeTarget) Loadout {
	return DefaultLoadout(kind, seaplane, taskKind, targetKind)
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

// dropBombsForEscort 让没护航的战斗轰炸机抛弃炸弹转当护航（规则书 813 行）。
// 抛弃挂载后不能再投弹，但当前剩余航程仍按"轻载"结算（规则书 83 行：攻击后变轻载），
// 因为这架飞机确实把自己的炸弹扔掉了。
func dropBombsForEscort(aircraft *AirUnit) {
	if aircraft == nil || !aircraft.Loadout.CarriesWeapon() {
		return
	}
	// 抛弃弹药后是纯空挂：没有炸弹也没有鱼雷，空战值按原机型（战斗轰炸机）结算。
	// 注意这里不能调 markBombed——规则书 83 行的轻载 +20% 只奖励"真的投过弹"，
	// 把炸弹扔海里的护航机不该额外获得航程。
	aircraft.Loadout = LoadoutAir
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
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Faction != faction {
			return fmt.Errorf("航空算子不存在或不属于当前玩家")
		}
		if aircraft.Launched || airborne(aircraft) {
			return fmt.Errorf("已经升空的算子不能更换挂载")
		}
		if err := ValidateLoadout(aircraft.Kind, aircraft.Seaplane, loadout); err != nil {
			return err
		}
	}
	return nil
}

// ApplyLoadoutOrder 按指定挂载重建飞机的满油航程。
func ApplyLoadoutOrder(state *State, order Order) []EngineEvent {
	loadout := Loadout(order.Loadout)
	events := []EngineEvent{}
	count := 0
	for _, id := range airOrderIDs(order) {
		aircraft := state.Aircraft[id]
		if aircraft == nil || aircraft.Launched || airborne(aircraft) {
			continue
		}
		applyLoadoutChoice(aircraft, loadout)
		// 手动选择要打上标记：之后分配任务不再用默认挂载覆盖它（规则书 51 行）。
		aircraft.LoadoutManual = true
		count++
	}
	if count == 0 {
		return nil
	}
	return append(events, EngineEvent{Type: "loadout_changed", Audience: orderFactionFromIDs(*state, order), Text: fmt.Sprintf("%d个算子更换挂载为%s", count, loadout.Label())})
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

// LoadoutOption 是前端展示用的一个挂载选项。
func LoadoutOptionList(kind string, seaplane bool) []map[string]string {
	result := []map[string]string{}
	for _, loadout := range LoadoutOptions(kind, seaplane) {
		result = append(result, map[string]string{"id": string(loadout), "label": loadout.Label()})
	}
	return result
}

// —— 机场飞机的挂载：与舰载机共用挂载规则，接口一致 ——

// applyGroundLoadout 按任务给机场飞机换上默认挂载。
// 与舰载机一致：玩家手动选过（LoadoutManual）就不再覆盖。
func applyGroundLoadout(state State, aircraft *AirfieldAirUnit) {
	if aircraft == nil || aircraft.LoadoutManual || airfieldAirborne(aircraft) {
		return
	}
	targetKind := StrikeTargetShip
	if aircraft.TaskKind == "attack" && aircraft.TargetCoord != "" {
		for _, airfield := range state.Airfields {
			if airfield != nil && airfield.Coord == aircraft.TargetCoord && airfield.Faction != aircraft.Faction {
				targetKind = StrikeTargetAir
				break
			}
		}
	}
	loadout := DefaultLoadout(aircraft.Kind, false, aircraft.TaskKind, targetKind)
	applyGroundLoadoutChoice(aircraft, loadout)
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
