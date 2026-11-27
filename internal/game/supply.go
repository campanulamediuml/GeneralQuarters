package game

import "fmt"

// —— 弹药供给 ——
//
// 挂载表过去只看"飞机能不能挂"：机型是鱼雷机、对舰值 > 0，机库里就永远能选鱼雷。
// 但弹药是补给点给的。护航航母（CVE）的弹药库没有鱼雷保障，停在它上面的 TBF
// 只能挂炸弹或侦察；同一架飞机转场到舰队航母（CV/CVL）之后，鱼雷选项就回来。
//
// 所以供给属于补给点（航母看所在舰的卡，陆基看所在机场），不属于机型：
// 同一个机种换母舰，挂载表跟着变。现在只记"有没有"，以后要记库存数量
// （比如鱼雷 4 条、炸弹不限额）就在 Supply 上加计数字段，挂载表 / 校验 / 界面
// 都还是这几个入口，不用再改一遍调用链。

// Supply 是补给点能提供什么。零值 = 弹药不限，全都给。
type Supply struct {
	// Blocked 是不提供的武器类别（torpedo / bomb / recon / air）。
	// 用"拉黑"而不是"白名单"：老数据、老存档不写这个字段就是全供，不会因为
	// 忘了补一条配置就把机库锁死。
	Blocked []Weapon `json:"blocked,omitempty"`
}

// Allows 判断这个补给点能不能提供某类武器。
func (s Supply) Allows(weapon Weapon) bool {
	for _, blocked := range s.Blocked {
		if blocked == weapon {
			return false
		}
	}
	return true
}

// Blocks 判断一个具体挂载会不会被补给点卡住。
// 空载和转场永远放行：转场不是弹药，补给点不给也得让飞机回家。
func (s Supply) Blocks(loadout Loadout) bool {
	if loadout == LoadoutNone || loadout.Weapon() == WeaponTransfer {
		return false
	}
	return !s.Allows(loadout.Weapon())
}

// SupplyOfAircraft 返回这架舰载机的补给点：按它所在的那条船算。
//
// 不按"整个舰队"合并：同一格里有 CV 也有 CVE 时，CVE 上的 TBF 不该蹭到
// 舰队航母的鱼雷——飞机是从自己母舰的弹药库挂弹的。
// 找不到母舰（母舰已沉、老存档缺 CarrierID）时返回空供给，不额外拦人。
func SupplyOfAircraft(state State, aircraft *AirUnit) Supply {
	if aircraft == nil || aircraft.CarrierID == "" {
		return Supply{}
	}
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		return Supply{}
	}
	return carrier.Card.Supply
}

// SupplyOfAirfield 返回机场的补给能力。
func SupplyOfAirfield(state State, airfieldID string) Supply {
	if airfieldID == "" {
		return Supply{}
	}
	airfield := state.Airfields[airfieldID]
	if airfield == nil {
		return Supply{}
	}
	return airfield.Supply
}

// SupplyOfGroundAircraft 返回陆基飞机所在机场的补给能力。
func SupplyOfGroundAircraft(state State, aircraft *AirfieldAirUnit) Supply {
	if aircraft == nil {
		return Supply{}
	}
	return SupplyOfAirfield(state, aircraft.AirfieldID)
}

// SupplySourceLabel 给补给点起个人话名字，用于拒绝提示：
// "护航航母「马尼拉湾」" / "莱城机场"。找不到来源时返回空串。
func SupplySourceLabel(state State, aircraft *AirUnit) string {
	if aircraft == nil || aircraft.CarrierID == "" {
		return ""
	}
	carrier := state.Units[aircraft.CarrierID]
	if carrier == nil {
		return ""
	}
	names := map[string]string{"CV": "舰队航母", "CVL": "轻航母", "CVE": "护航航母"}
	kind := names[carrier.Type]
	if kind == "" {
		kind = carrier.Type
	}
	return fmt.Sprintf("%s「%s」", kind, carrier.Name)
}

// SupplySourceLabelGround 是陆基版本的补给点名字："莱城机场"。
func SupplySourceLabelGround(state State, aircraft *AirfieldAirUnit) string {
	if aircraft == nil || aircraft.AirfieldID == "" {
		return ""
	}
	airfield := state.Airfields[aircraft.AirfieldID]
	if airfield == nil {
		return ""
	}
	return airfield.Name
}

// supplyRefusal 在挂载被补给点拒绝时说明缺什么、该去哪儿，比笼统的"无法挂载"有用。
// 返回 nil 表示补给点不拦这个挂载。
func supplyRefusal(supply Supply, loadout Loadout, where string) error {
	if !supply.Blocks(loadout) {
		return nil
	}
	if where == "" {
		where = "这个补给点"
	}
	weapon := weaponLabel(loadout.Weapon())
	return fmt.Errorf("%s不提供%s挂载：只能挂其他弹药，换到能保障%s的母舰或机场再挂", where, weapon, weapon)
}

// weaponLabel 是武器类别的中文名。
func weaponLabel(weapon Weapon) string {
	switch weapon {
	case WeaponTorpedo:
		return "鱼雷"
	case WeaponBomb:
		return "炸弹"
	case WeaponRecon:
		return "侦察"
	case WeaponAir:
		return "对空"
	}
	return "转场"
}
