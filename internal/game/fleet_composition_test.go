package game

import (
	"fmt"
	"testing"
)

// 规则书〈编队〉：每艘航母至少配 1 艘护航舰，每 2 艘运输船至少配 1 艘护航舰。
// 这些用例钉住整编与合并两条入口都走同一套校验。

func compositionFixture(types ...string) (State, Order) {
	state := State{Units: map[string]*Unit{}}
	assignments := map[string]string{}
	for index, unitType := range types {
		id := fmt.Sprintf("u%d", index)
		state.Units[id] = &Unit{ID: id, Faction: "A", Coord: "0303", FleetID: "TF1", Type: unitType}
		assignments[id] = "TF1"
	}
	return state, Order{Type: "REORGANIZE_HEX", Coord: "0303", Assignments: assignments}
}

func TestReorganizationRequiresEscortForCarrier(t *testing.T) {
	state, order := compositionFixture("CV", "CV")
	if err := validateReorganization(state, "A", order); err == nil {
		t.Fatal("两艘航母没有护航舰，整编应当被拒")
	}

	state, order = compositionFixture("CV", "DD")
	if err := validateReorganization(state, "A", order); err != nil {
		t.Fatalf("一艘护航舰配一艘航母应当被接受：%v", err)
	}

	state, order = compositionFixture("CV", "CV", "DD", "DD")
	if err := validateReorganization(state, "A", order); err != nil {
		t.Fatalf("两艘航母配两艘驱逐舰应当被接受：%v", err)
	}
}

func TestReorganizationRequiresEscortForTransports(t *testing.T) {
	state, order := compositionFixture("TRS", "TRS", "TRS", "DD")
	if err := validateReorganization(state, "A", order); err == nil {
		t.Fatal("三艘运输船只配一艘护航舰，整编应当被拒")
	}

	state, order = compositionFixture("TRS", "TRS", "DD")
	if err := validateReorganization(state, "A", order); err != nil {
		t.Fatalf("两艘运输船配一艘护航舰应当被接受：%v", err)
	}

	state, order = compositionFixture("TRS", "TRS", "TRS", "DD", "DD")
	if err := validateReorganization(state, "A", order); err != nil {
		t.Fatalf("三艘运输船配两艘护航舰应当被接受：%v", err)
	}
}

// 护航名额按"航母数 + 运输船数的一半（向上取整）"累加：一艘驱逐舰不能既替航母护航
// 又替运输船护航。
func TestReorganizationEscortQuotaAddsUp(t *testing.T) {
	state, order := compositionFixture("CV", "TRS", "TRS", "DD")
	if err := validateReorganization(state, "A", order); err == nil {
		t.Fatal("一艘航母加两艘运输船只配一艘护航舰，整编应当被拒")
	}

	state, order = compositionFixture("CV", "TRS", "TRS", "DD", "DD")
	if err := validateReorganization(state, "A", order); err != nil {
		t.Fatalf("两个护航名额配齐后应当被接受：%v", err)
	}
}

// 合并是另一条入口：两支各自没有护航的航母队不能靠合并变成合法编队。
func TestMergeFleetRequiresEscorts(t *testing.T) {
	state := State{Units: map[string]*Unit{
		"a-cv": {ID: "a-cv", Faction: "A", Coord: "0303", FleetID: "TF1", Type: "CV"},
		"b-cv": {ID: "b-cv", Faction: "A", Coord: "0303", FleetID: "TF2", Type: "CV"},
	}}
	order := Order{Type: "MERGE_FLEET", UnitID: "a-cv", TargetUnitID: "b-cv"}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("两艘航母合并后仍然没有护航舰，应当被拒")
	}

	state.Units["b-dd"] = &Unit{ID: "b-dd", Faction: "A", Coord: "0303", FleetID: "TF2", Type: "DD"}
	if err := ValidateOrder(state, "A", order); err == nil {
		t.Fatal("合并后两艘航母只配一艘护航舰，仍然应当被拒")
	}
	state.Units["b-dd2"] = &Unit{ID: "b-dd2", Faction: "A", Coord: "0303", FleetID: "TF2", Type: "DD"}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("目标舰队带两艘驱逐舰后应当可以合并：%v", err)
	}
}

// 两个剧本开局的每支舰队都必须自己满足护航要求，否则玩家一动就被校验顶回来。
func TestScenarioFleetsSatisfyEscortRule(t *testing.T) {
	for _, scenarioID := range []string{"_coral_real", "_marianas"} {
		scenario, gameMap, catalog, err := LoadScenario("../../content", scenarioID)
		if err != nil {
			t.Fatalf("%s 加载失败：%v", scenarioID, err)
		}
		state, err := NewState(scenario, gameMap, catalog, 42)
		if err != nil {
			t.Fatalf("%s 建局失败：%v", scenarioID, err)
		}
		compositions := map[string]*fleetComposition{}
		for _, unit := range state.Units {
			key := FleetKey(unit)
			if compositions[key] == nil {
				compositions[key] = &fleetComposition{}
			}
			compositions[key].add(unit.Type)
		}
		carriers, transports := 0, 0
		for fleet, composition := range compositions {
			carriers += composition.carriers
			transports += composition.transports
			if err := composition.validate(fleet); err != nil {
				t.Fatalf("%s 的 %s 不满足护航要求：%v", scenarioID, fleet, err)
			}
		}
		if carriers == 0 || transports == 0 {
			t.Fatalf("%s 的用例应当同时覆盖航母与运输船（航母 %d、运输船 %d）", scenarioID, carriers, transports)
		}
	}
}
