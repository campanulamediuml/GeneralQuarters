package game

import (
	"strings"
	"testing"
)

// TestBurningBlocksLaunchButNotHangarWork 起火燃烧只禁起降，不禁整备：
// 规则书 219 行禁止的是"在起火燃烧的航母上起降"，机库加油和升降机调度照常。
func TestBurningBlocksLaunchButNotHangarWork(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	EnsureDamage(carrier).Fire = "burning"
	if !AviationAvailable(carrier) {
		t.Fatal("起火燃烧的航母仍应能整备和调度升降机")
	}
	if reason := launchBlockReason(state, carrier); !strings.Contains(reason, "起火") {
		t.Fatalf("起飞阻塞原因应明确指出起火，实际：%q", reason)
	}
	if reason := landingBlockReason(state, carrier); !strings.Contains(reason, "起火") {
		t.Fatalf("降落阻塞原因应明确指出起火，实际：%q", reason)
	}
	// 甲板放行容量必须归零，否则飞机照样会起飞。
	if got := launchCapacityFor(state, carrier.ID); got != 0 {
		t.Fatalf("起火的航母本回合放行容量应为 0，实际 %d", got)
	}
	// 灭火后应恢复可用。
	EnsureDamage(carrier).Fire = ""
	if reason := launchBlockReason(state, carrier); reason != "" {
		t.Fatalf("灭火后不应再阻塞起飞：%q", reason)
	}
}

// TestSmallFireStopsAllAirOperations 小型火灾按规则书 432 行"本回合无法进行航空作业"，
// 整备也要停——这和"起火燃烧只禁起降"（219 行）是两条不同的规则。
func TestSmallFireStopsAllAirOperations(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	EnsureDamage(carrier).Fire = "small"
	if AviationAvailable(carrier) {
		t.Fatal("小型火灾期间不应进行航空作业（含整备）")
	}
	EnsureDamage(carrier).Fire = ""
	if !AviationAvailable(carrier) {
		t.Fatal("小火扑灭后应恢复航空作业")
	}
}

// TestReconFormationCanBeRecalledByRadio 侦察组必须能无线电召回：
// 否则名额一直被占，想换搜索方向只能等它自己油尽返航。
func TestReconFormationCanBeRecalledByRadio(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	aircraft.Status = "recon"
	aircraft.Launched = true
	aircraft.TaskID = "recon-recall"
	aircraft.TaskKind = "recon"
	aircraft.Heading = "W"
	aircraft.SectorWidth = 60
	aircraft.Coord = state.Units[aircraft.CarrierID].Coord
	order := Order{Type: "RECALL_AIR", TaskID: "recon-recall"}
	if err := validateRadioOrder(state, "B", order); err != nil {
		t.Fatalf("侦察编队应允许无线电召回：%v", err)
	}
	retask := Order{Type: "RETASK_AIR", TaskID: "recon-recall", TaskKind: "cap"}
	if err := validateRadioOrder(state, "B", retask); err == nil {
		t.Fatal("侦察编队不应允许空中改任务")
	}
}

// TestContactDecayWarnsBeforeLoss 各等级的存续回合数固定（4 级 6、其余 3），
// 每级到期前一回合发 contact_stale 预警，别让玩家以为坐标还能用。
func TestContactDecayWarnsBeforeLoss(t *testing.T) {
	if got := contactDecayLimit(intelMaxLevel); got != 6 {
		t.Fatalf("4 级情报应存续 6 回合，实际 %d", got)
	}
	for level := 1; level < intelMaxLevel; level++ {
		if got := contactDecayLimit(level); got != 3 {
			t.Fatalf("%d 级情报应每 3 回合降一级，实际 %d", level, got)
		}
	}

	state := sandboxState(t)
	state.Contacts = map[string]map[string]*FleetContact{
		"B": {"TG-A": {FleetID: "TG-A", Level: 1, Coord: "0303", DisplayCoord: "0303"}},
	}
	stale := false
	for turn := 0; turn < 3; turn++ {
		state.IntelFresh = map[string]bool{}
		for _, event := range decayContactIntel(&state) {
			if event.Type == "contact_stale" {
				stale = true
			}
		}
		if turn < 2 {
			if contact := state.Contacts["B"]["TG-A"]; contact == nil || contact.Level != 1 {
				t.Fatalf("第 %d 回合情报不该消失或降级", turn+1)
			}
		}
	}
	if !stale {
		t.Fatal("失去接触前一回合应有 contact_stale 预警")
	}
	if state.Contacts["B"]["TG-A"] != nil {
		t.Fatal("1 级情报脱离接触 3 回合后应归零并移除")
	}

	// 夜间不逐级衰减：满 4 级也就撑 4 回合，然后直接归零。
	night := sandboxState(t)
	night.Night = true
	night.Contacts["B"] = map[string]*FleetContact{
		"TG-A": {FleetID: "TG-A", Level: 4, Coord: "0303", DisplayCoord: "0303"},
	}
	for turn := 1; turn < contactNightDecayTurns; turn++ {
		night.IntelFresh = map[string]bool{}
		decayContactIntel(&night)
		if contact := night.Contacts["B"]["TG-A"]; contact == nil || contact.Level != 4 {
			t.Fatalf("夜间第 %d 回合情报不该降级或消失", turn)
		}
	}
	night.IntelFresh = map[string]bool{}
	decayContactIntel(&night)
	if night.Contacts["B"]["TG-A"] != nil {
		t.Fatal("夜间脱离接触 4 回合后情报应直接归零")
	}
}

// TestStrikeSearchGivesUpWhenFuelLow 搜索接敌失败时，剩余航程只够回家就必须返航并说明，
// 而不是对着空海一直兜圈把油烧到 1。
func TestStrikeSearchGivesUpWhenFuelLow(t *testing.T) {
	state := sandboxState(t)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	aircraft.Status = "attack"
	aircraft.Launched = true
	aircraft.TaskID = "strike-search"
	aircraft.TaskKind = "attack"
	aircraft.TargetCoord = carrier.Coord
	away := Neighbors(carrier.Coord)[0]
	aircraft.Coord = away
	aircraft.Fuel = Fuel{Tank: 6, Internal: 6, Reserve: 4}
	aircraft.MaxRange = 18
	aircraft.SetRange(3)
	events := searchAfterFailedEngage(&state, aircraft, 2)
	found := false
	for _, event := range events {
		if event.Type == "attack_rtb" {
			found = true
		}
	}
	if !aircraft.Returning {
		t.Fatal("剩余航程只够返航时应停止搜索并返航")
	}
	if !found {
		t.Fatalf("返航时应给出战报：%+v", events)
	}
}

// TestStrikeSearchSpreadsOutward 接敌失败后编队必须持续朝前搜索、逐渐外扩，
// 不能一到目标点就往旧坐标折返——那样 26 架飞机会在两格见方里原地打转，
// 明明敌舰队就在两三格之外也永远撞不上。
func TestStrikeSearchSpreadsOutward(t *testing.T) {
	state := sandboxState(t)
	freezeFleets(&state)
	aircraft := state.Aircraft["b-cv-04-dive_bomber-x1"]
	carrier := state.Units[aircraft.CarrierID]
	carrier.Coord = "0402"
	aircraft.Status = "attack"
	aircraft.Launched = true
	aircraft.TaskID = "strike-spread"
	aircraft.TaskKind = "attack"
	aircraft.TargetCoord = carrier.Coord
	aircraft.Coord = carrier.Coord
	aircraft.Heading = "E"
	aircraft.Fuel = Fuel{Tank: 18, Internal: 18, Reserve: 4}
	aircraft.MaxRange = 18
	aircraft.SetRange(18)

	origin := aircraft.Coord
	previous := Distance(origin, aircraft.Coord)
	visited := map[string]bool{origin: true}
	for turn := 0; turn < 5 && !aircraft.Returning; turn++ {
		searchAfterFailedEngage(&state, aircraft, 2)
		current := Distance(origin, aircraft.Coord)
		if current < previous {
			t.Fatalf("第 %d 回合搜索折回了原点方向：%d → %d", turn, previous, current)
		}
		if visited[aircraft.Coord] {
			t.Fatalf("第 %d 回合又飞回了刚搜过的 %s，搜索在原地打转", turn, aircraft.Coord)
		}
		visited[aircraft.Coord] = true
		previous = current
	}
	if previous < 3 {
		t.Fatalf("搜索 5 回合后只离开起点 %d 格，搜索没有扩散出去", previous)
	}
}
