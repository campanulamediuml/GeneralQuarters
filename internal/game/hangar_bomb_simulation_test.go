package game

import (
	"fmt"
	"testing"
)

// probeCarrierHangar 造一艘只有机库的航母：机库里摆满飞机。
// armed=true 时这些飞机是"整装待发"（有挂载 + 整备完成），false 时是空载未整备。
func probeCarrierHangar(count, dc int, armed bool, seed uint64) (*State, *Unit) {
	state := &State{Units: map[string]*Unit{}, Aircraft: map[string]*AirUnit{}, RNGState: seed}
	carrier := &Unit{
		ID: "cv", Faction: "B", Name: "翔鹤", Type: "CV", Strength: 12, MaxStrength: 12,
		Card: ShipCard{HangarCapacity: count, DeckCapacity: 8, DamageControl: dc},
	}
	state.Units[carrier.ID] = carrier
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("a%02d", index)
		plane := &AirUnit{
			ID: id, CarrierID: carrier.ID, Faction: "B", Model: "B5N2 九七舰攻",
			Kind: "torpedo_bomber", Status: "hangar", Strength: 1,
		}
		if armed {
			plane.Loadout = LoadoutTorpedoTank
			plane.LoadoutManual = true
			plane.ServiceTurns = 1
		}
		state.Aircraft[id] = plane
	}
	syncArmedAircraft(state, nil)
	carrier.Card.Aircraft = count
	return state, carrier
}

// 机库里"整装待发"的飞机挨航弹和"没整备"的飞机挨航弹，损失差多少。
//
// 规则（规则书 421–423 行）：只有 Armed（有挂载 + 整备完成 + 没损伤）的飞机会被
// "炸弹命中机库"摧毁 1–2 架，并触发机库油气爆炸表；空载飞机只剩全局严重伤害表里
// 那一条"机库损伤"的小概率顺带。这个测试就是把这套联动的量级钉住。
func TestHangarBombLossesDependOnAircraftReadiness(t *testing.T) {
	const runs = 20000
	type result struct {
		aircraft    float64
		strength    float64
		destroyRate float64
		burningRate float64
	}
	measure := func(armed bool) result {
		lost, strength, destroyed, burning := 0, 0, 0, 0
		for run := 0; run < runs; run++ {
			state, carrier := probeCarrierHangar(18, 3, armed, uint64(run)*2654435761+7)
			beforeAircraft, beforeStrength := carrier.Card.Aircraft, carrier.Strength
			applyGunOrBombHit(state, Hit{TargetID: carrier.ID, Cause: HitBomb})
			if carrier.Card.Aircraft < beforeAircraft {
				destroyed++
			}
			lost += beforeAircraft - carrier.Card.Aircraft
			strength += beforeStrength - carrier.Strength
			if EnsureDamage(carrier).Fire == "burning" {
				burning++
			}
		}
		return result{
			aircraft:    float64(lost) / float64(runs),
			strength:    float64(strength) / float64(runs),
			destroyRate: float64(destroyed) * 100 / float64(runs),
			burningRate: float64(burning) * 100 / float64(runs),
		}
	}
	armed := measure(true)
	empty := measure(false)
	t.Logf("DCT=3、机库 18 架，每发航弹：整装待发 毁机 %.2f 架（有毁伤 %.1f%%）结构 -%.2f 燃烧 %.1f%%；空载未整备 毁机 %.2f 架（有毁伤 %.1f%%）结构 -%.2f 燃烧 %.1f%%",
		armed.aircraft, armed.destroyRate, armed.strength, armed.burningRate,
		empty.aircraft, empty.destroyRate, empty.strength, empty.burningRate)
	// 整装待发的机库几乎每发都要掉 2 架；空载的只剩严重伤害表里"机库损伤"那条小概率。
	if armed.aircraft < 1.5 || armed.aircraft > 2.05 {
		t.Fatalf("整装待发机库每发应毁 2 架左右，实际 %.2f", armed.aircraft)
	}
	if empty.aircraft > 0.3 {
		t.Fatalf("没整备的飞机不该跟着吃整装待发那一套，实际每发毁 %.2f 架", empty.aircraft)
	}
	if armed.aircraft < empty.aircraft*8 {
		t.Fatalf("整装待发的损失应显著高于空载：%.2f vs %.2f", armed.aircraft, empty.aircraft)
	}
	// 机库油气爆炸给整装待发的那一侧额外叠结构损失和火灾。
	if armed.strength < empty.strength+0.3 {
		t.Fatalf("机库爆炸应额外造成结构损失：%.2f vs %.2f", armed.strength, empty.strength)
	}
	if armed.burningRate < 60 {
		t.Fatalf("整装待发机库应有很高的起火燃烧概率，实际 %.1f%%", armed.burningRate)
	}
	if empty.burningRate > 15 {
		t.Fatalf("空载机库不该动不动就燃烧，实际 %.1f%%", empty.burningRate)
	}
}
