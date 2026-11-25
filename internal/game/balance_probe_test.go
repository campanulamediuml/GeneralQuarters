package game

import (
	"fmt"
	"sort"
	"testing"
)

// 空袭体检用的统计桶。
type probeTally struct {
	trials        int
	aaHits        int
	attackerOut   int
	bombs         int
	hits          int
	damageHits    int
	deckDamaged   int
	deckDestroyed int
	strengthLoss  int
	shipsHurt     int
	allocCV       int
	allocOther    int
	hitCV         int
	hitOther      int
	byKind        map[string]*kindTally
}

type kindTally struct {
	attacks  int
	attempts int
	hits     int
}

func newProbeTally() probeTally {
	return probeTally{byKind: map[string]*kindTally{}}
}

func (t *probeTally) report(name string) string {
	f := func(v int) float64 { return float64(v) / float64(t.trials) }
	cvPct := 0.0
	if t.allocCV+t.allocOther > 0 {
		cvPct = 100 * float64(t.allocCV) / float64(t.allocCV+t.allocOther)
	}
	line := fmt.Sprintf("%s（%d次平均）：投弹架次 %.0f%% 打航母；防空命中 %.1f，攻击方退出 %.1f；"+
		"投弹 %.1f 次、命中 %.1f 次（航母 %.1f / 其它 %.1f），其中有效损伤 %.1f 次；"+
		"甲板受损 %.1f、甲板摧毁 %.1f；掉强度舰船 %.1f 艘，强度损失 %.1f。",
		name, t.trials, cvPct, f(t.aaHits), f(t.attackerOut),
		f(t.bombs), f(t.hits), f(t.hitCV), f(t.hitOther), f(t.damageHits),
		f(t.deckDamaged), f(t.deckDestroyed), f(t.shipsHurt), f(t.strengthLoss))
	kinds := make([]string, 0, len(t.byKind))
	for k := range t.byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		v := t.byKind[k]
		line += fmt.Sprintf(" %s：分配 %.1f 架、投弹 %.1f 次、命中 %.1f 次。", k, f(v.attacks), f(v.attempts), f(v.hits))
	}
	return line
}

// runStrikeProbe 用同一批飞机对同一支航母编队打若干次，返回统计。
func runStrikeProbe(t *testing.T, scenario ScenarioDefinition, gameMap MapDefinition, catalog UnitCatalog, aircraftIDs []string, carriers []string, trials int) probeTally {
	t.Helper()
	tally := newProbeTally()
	tally.trials = trials
	for seed := uint64(1); seed <= uint64(trials); seed++ {
		state, err := NewState(scenario, gameMap, catalog, seed)
		if err != nil {
			t.Fatal(err)
		}
		target := state.Units[carriers[0]].Coord
		before := map[string]int{}
		shipKinds := map[string]string{}
		for _, unit := range unitsAt(state, target) {
			before[unit.ID] = unit.Strength
			shipKinds[unit.ID] = unit.Type
		}
		airKind := map[string]string{}
		for _, id := range aircraftIDs {
			a := state.Aircraft[id]
			if a == nil {
				t.Fatalf("缺少%s", id)
			}
			airKind[id] = a.Kind
			a.Status = "attack"
			a.TaskID = "T-ATK"
			a.TaskKind = "attack"
			a.TargetCoord = target
			a.Coord = target
			a.Engaged = true
			a.Launched = true
			switch a.Kind {
			case "dive_bomber":
				applyLoadoutChoice(a, LoadoutBombTank)
			case "torpedo_bomber":
				applyLoadoutChoice(a, LoadoutTorpedoTank)
			default:
				applyLoadoutChoice(a, LoadoutAirTank)
			}
		}
		resolveAirStrike(&state, "T-ATK", target)
		if len(state.AirCombats) == 0 {
			t.Fatal("没有生成空袭报告")
		}
		report := state.AirCombats[len(state.AirCombats)-1]
		tally.aaHits += report.AAHits
		tally.attackerOut += report.AttackerLosses
		tally.bombs += report.BombAttempts
		tally.hits += report.StrikeHits
		tally.damageHits += report.DamageHits
		for _, alloc := range report.TargetAllocations {
			kind := airKind[alloc.Attacker]
			entry := tally.byKind[kind]
			if entry == nil {
				entry = &kindTally{}
				tally.byKind[kind] = entry
			}
			entry.attacks++
			entry.attempts += alloc.Attempts
			entry.hits += alloc.Hits
			shipKind := shipKinds[alloc.ShipID]
			if CarrierEquivalentType(shipKind) == "CV" {
				tally.allocCV++
				tally.hitCV += alloc.Hits
			} else {
				tally.allocOther++
				tally.hitOther += alloc.Hits
			}
		}
		for id := range before {
			unit := state.Units[id]
			if unit == nil {
				tally.strengthLoss += before[id]
				tally.shipsHurt++
				continue
			}
			if unit.Strength < before[id] {
				tally.strengthLoss += before[id] - unit.Strength
				tally.shipsHurt++
			}
			if unit.Damage != nil {
				if unit.Damage.DeckDestroyed {
					tally.deckDestroyed++
				} else if unit.Damage.DeckDamaged {
					tally.deckDamaged++
				}
			}
		}
	}
	return tally
}

func carrierIDs(prefix string) []string {
	return []string{prefix + "-01", prefix + "-02", prefix + "-03", prefix + "-04"}
}

func waveIDs(carrier string, dive, torp, fighter int) []string {
	ids := []string{}
	for n := 1; n <= dive; n++ {
		ids = append(ids, fmt.Sprintf("%s-dive_bomber-x%d", carrier, n))
	}
	for n := 1; n <= torp; n++ {
		ids = append(ids, fmt.Sprintf("%s-torpedo_bomber-x%d", carrier, n))
	}
	for n := 1; n <= fighter; n++ {
		ids = append(ids, fmt.Sprintf("%s-fighter-x%d", carrier, n))
	}
	return ids
}

// deckStrike32 是一轮"全甲板打击"的标准配比：32 架 = 6 架护航战斗机 + 14 俯冲 + 12 鱼雷。
// 每艘航母出一个甲板批次 8 架，四舰合计 32，正好一回合放飞完；双方配比一致。
func deckStrike32(prefix string) []string {
	plan := [][3]int{{2, 3, 3}, {2, 3, 3}, {1, 4, 3}, {1, 4, 3}}
	ids := []string{}
	for index, carrier := range carrierIDs(prefix) {
		part := plan[index]
		for n := 1; n <= part[0]; n++ {
			ids = append(ids, fmt.Sprintf("%s-fighter-x%d", carrier, n))
		}
		for n := 1; n <= part[1]; n++ {
			ids = append(ids, fmt.Sprintf("%s-dive_bomber-x%d", carrier, n))
		}
		for n := 1; n <= part[2]; n++ {
			ids = append(ids, fmt.Sprintf("%s-torpedo_bomber-x%d", carrier, n))
		}
	}
	return ids
}

// TestAirStrikeBalanceProbe 是参数体检：
// 单航母 12 机波次 与 四航母全甲板打击（32 俯冲 + 24 鱼雷 + 12 护航）分别打美/日航母编队。
func TestAirStrikeBalanceProbe(t *testing.T) {
	scenario, gameMap, catalog := loadSandboxScenario(t)
	type direction struct {
		name          string
		carrierPrefix string
		targets       []string
	}
	for _, dir := range []direction{
		{name: "美军打日本航母编队", carrierPrefix: "a2-cv", targets: carrierIDs("b-cv")},
		{name: "日军打美军航母编队", carrierPrefix: "b-cv", targets: carrierIDs("a2-cv")},
	} {
		dive, torpedo, fighters := 6, 3, 3
		if dir.carrierPrefix == "b-cv" {
			dive, torpedo, fighters = 5, 3, 2
		}
		small := runStrikeProbe(t, scenario, gameMap, catalog, waveIDs(dir.carrierPrefix+"-01", dive, torpedo, fighters), dir.targets, 50)
		t.Log(small.report(dir.name + "·单航母12机波次"))
		deckIDs := deckStrike32(dir.carrierPrefix)
		full := runStrikeProbe(t, scenario, gameMap, catalog, deckIDs, dir.targets, 50)
		t.Log(full.report(fmt.Sprintf("%s·四航母全甲板%d机", dir.name, len(deckIDs))))
	}
}
