package game

import (
	"strings"
	"testing"
)

// 突袭规则（取代"每回合重投"）：
//   - 只在接战的第一轮判定：这一回合有人开进同一个 hex 才算新接战；
//   - 突袭成功后突袭方进入 6 回合冷却，快船可以反复机动，但不能把雷击刷成每两回合一次。

func surpriseState(seed uint64, moved, lock bool) *State {
	units := map[string]*Unit{
		"a1": {ID: "a1", FleetID: "af", Name: "美军重巡", Type: "CA", Faction: "A", Coord: "0101",
			Strength: 6, MaxStrength: 6,
			Card: ShipCard{Gunnery: 8, DamageControl: 6, SpeedClass: "normal", MoveCycle: []int{3}}},
		"b1": {ID: "b1", FleetID: "bf", Name: "日军驱逐", Type: "DD", Faction: "B", Coord: "0101",
			Strength: 2, MaxStrength: 2, Moved: moved,
			Card: ShipCard{Gunnery: 3, DamageControl: 3, SpeedClass: "fast", MoveCycle: []int{2}, TorpedoSalvos: 3}},
	}
	state := &State{
		RNGState:      seed,
		Map:           MapDefinition{Rows: 2, Cols: 2},
		Contacts:      map[string]map[string]*FleetContact{},
		Doctrines:     map[string]string{"A": "us", "B": "japan"},
		FleetSessions: map[string]int{},
		Year:          1942,
		Night:         true,
		Units:         units,
	}
	if lock {
		state.SurpriseLock = map[string]int{"bf": surpriseLockTurns}
	}
	return state
}

// 双方贴着打第二回合、谁也没动：不投突袭，也不会冒出击鱼雷。
func TestSurpriseOnlyOnEngagementRound(t *testing.T) {
	opening := resolveContacts(surpriseState(11, true, false))
	if !hasEvent(opening, "surprise_rolls") {
		t.Fatal("接战首轮应该投突袭")
	}

	// 已经是第二回合：没人开进这个 hex，直接进炮战。
	state := surpriseState(11, false, false)
	events := resolveContacts(state)
	if hasEvent(events, "surprise_rolls") {
		t.Fatal("贴在一起打第二回合不该再投突袭")
	}
	if hasEvent(events, "torpedo_volley") {
		t.Fatal("没有突袭就没有鱼雷齐射：后续回合只打炮战")
	}
	if !hasEvent(events, "gunnery_volley") {
		t.Fatal("后续回合仍然要打炮战")
	}
}

// 突袭成功后挂 6 回合冷却；冷却期间同样有人开进 hex，也拿不到突袭。
// 冷却只挂在打过硬仗的那一支编队上，对面该有的先手不受影响。
func TestSurpriseLockBlocksRepeatedAmbush(t *testing.T) {
	count := func(lockB bool) (total, byB int) {
		for seed := uint64(1); seed <= 200; seed++ {
			state := surpriseState(seed, true, false)
			if lockB {
				state.SurpriseLock = map[string]int{"bf": surpriseLockTurns}
			}
			for _, event := range resolveContacts(state) {
				if event.Type != "surprise" {
					continue
				}
				total++
				if strings.HasPrefix(event.Text, "B") {
					byB++
				}
			}
		}
		return total, byB
	}
	_, baseByB := count(false)
	if baseByB == 0 {
		t.Fatal("1942 年夜间雷击队这批种子下应该判得出突袭")
	}
	total, lockedByB := count(true)
	if lockedByB != 0 {
		t.Fatalf("冷却中的舰队不该再取得突袭：%d 次", lockedByB)
	}
	if total == 0 {
		t.Fatal("冷却只该锁住那一支编队，对面照样可能抢先手")
	}
}

// 冷却按回合递减，到期后重新可以突袭。
func TestSurpriseLockTicksDown(t *testing.T) {
	state := surpriseState(1, false, true)
	for turn := 1; turn <= surpriseLockTurns; turn++ {
		AdvanceMovementCooldown(state)
	}
	if state.SurpriseLock["bf"] != 0 {
		t.Fatalf("%d 回合后冷却应该清空：%d", surpriseLockTurns, state.SurpriseLock["bf"])
	}
}

// 突袭成功后引擎自己挂上冷却，不用外部再补。
func TestSurpriseSuccessLocksFleet(t *testing.T) {
	for seed := uint64(1); seed <= 200; seed++ {
		state := surpriseState(seed, true, false)
		if !hasEvent(resolveContacts(state), "surprise") {
			continue
		}
		if state.SurpriseLock["bf"] != surpriseLockTurns {
			t.Fatalf("突袭成功后该挂 %d 回合冷却，实际 %d", surpriseLockTurns, state.SurpriseLock["bf"])
		}
		return
	}
	t.Fatal("200 个种子里没有一个判出突袭")
}
