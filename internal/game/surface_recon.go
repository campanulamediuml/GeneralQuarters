package game

import (
	"fmt"
	"math"
	"sort"
)

// —— 舰队侦查（规则书 575–585 行）——
//
// 水面舰队每回合对周围 1 hex 内的每个敌方舰队各掷一次，成功则情报等级 +1。
// 这条规则一直只写在规则书里没实现：2 hex 内目视自动发现、2 hex 外全靠侦察机，
// 中间的"挨着但看不清"根本不存在，带对海搜索雷达的舰队也没有对应的发现优势。
//
// 判定顺序按舰队 ID 排序，保证同种子两局事件顺序一致。

// surfaceReconChance 计算一次舰队侦查的成功率。observer 是侦查方的水面舰队（机场侦查时为空），
// target 是被侦查的敌方舰队。
func surfaceReconChance(state State, observer, target []Unit) float64 {
	chance := 50.0
	if len(target) > 0 {
		// 天气按目标上空的 hex 判定：雾里找船和被找是同一件事。
		switch weatherAt(state.Map, target[0].Coord) {
		case "cloud":
			chance -= 10
		case "rain":
			chance -= 15
		case "storm":
			chance -= 20
		}
	}
	if state.Night {
		chance -= 10
	}
	if hasSurfaceRadar(observer) {
		chance += 20
	}
	// 己方舰队船只每艘 +1%：编队越大，警戒面越宽。
	chance += float64(len(observer))
	for _, unit := range target {
		if isCapitalShip(unit.Type) {
			chance += 2
		} else {
			chance++
		}
		if unit.Damage == nil {
			continue
		}
		if unit.Damage.OilLeak {
			chance += 10
		}
		if unit.Damage.Fire != "" {
			chance += 20
		}
	}
	// 天气只按档扣点，不再乘系数：这个额外的 ×0.5/×0.35/×0.2 只给航空侦察，
	// 水面舰队的瞭望和雷达在同一片云底下不会同样"瞎掉"。
	return math.Max(0, math.Min(100, chance))
}

func fleetGroups(state *State) map[string][]Unit {
	groups := map[string][]Unit{}
	for _, unit := range state.Units {
		key := FleetKey(unit)
		groups[key] = append(groups[key], *unit)
	}
	for key := range groups {
		members := groups[key]
		sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
		groups[key] = members
	}
	return groups
}

func resolveSurfaceRecon(state *State) []EngineEvent {
	events := []EngineEvent{}
	groups := fleetGroups(state)
	ids := sortedFleetIDs(groups)
	for _, observerID := range ids {
		observer := groups[observerID]
		if len(observer) == 0 {
			continue
		}
		faction := observer[0].Faction
		for _, targetID := range ids {
			if targetID == observerID {
				continue
			}
			target := groups[targetID]
			if len(target) == 0 || target[0].Faction == faction {
				continue
			}
			// 只管相邻一圈：同格已经算接触，2 hex 内是目视范围，都不该再白掷一次。
			if Distance(observer[0].Coord, target[0].Coord) != 1 {
				continue
			}
			chance := surfaceReconChance(*state, observer, target)
			roll := probabilityRoll(&state.RNGState)
			if roll >= chance {
				// 失败不写日志：相邻舰队每回合都要掷一次，写出来会淹没真正的事件。
				continue
			}
			level := recordContact(state, faction, targetID, target)
			events = append(events, EngineEvent{Type: "surface_recon", Audience: faction, Text: fmt.Sprintf("舰队侦查发现%s在%s（%d艘），情报等级升至%d（成功率%.1f%%，判定%.1f）", target[0].FleetName, target[0].Coord, len(target), level, chance, roll)})
		}
	}
	// 机场侦查（规则书 575 行）：机场必须装备对海搜索雷达才能做这项工作。
	for _, airfield := range state.Airfields {
		if !airfield.Radar {
			continue
		}
		for _, targetID := range ids {
			target := groups[targetID]
			if len(target) == 0 || target[0].Faction == airfield.Faction {
				continue
			}
			if Distance(airfield.Coord, target[0].Coord) != 1 {
				continue
			}
			// 机场没有舰船，observer 传空：少掉"己方舰队船只每艘+1%"那一项。
			chance := surfaceReconChance(*state, nil, target)
			roll := probabilityRoll(&state.RNGState)
			if roll >= chance {
				continue
			}
			level := recordContact(state, airfield.Faction, targetID, target)
			events = append(events, EngineEvent{Type: "surface_recon", Audience: airfield.Faction, Text: fmt.Sprintf("%s对海雷达发现%s在%s（%d艘），情报等级升至%d", airfield.Name, target[0].FleetName, target[0].Coord, len(target), level)})
		}
	}
	return events
}
