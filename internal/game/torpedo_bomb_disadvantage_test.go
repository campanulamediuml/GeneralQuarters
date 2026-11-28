package game

import "testing"

// 鱼雷机挂炸弹只能低空水平投弹，对舰取劣势；挂鱼雷本身不取劣势。
// 俯冲轰炸机挂炸弹也不吃这一条（它们本来就能俯冲）。
func TestTorpedoBomberBombDisadvantage(t *testing.T) {
	state := loadNightMarianas(t)

	var torpedoBomber *AirUnit
	var diveBomber *AirUnit
	for _, aircraft := range state.Aircraft {
		switch aircraft.Kind {
		case "torpedo_bomber":
			if torpedoBomber == nil {
				torpedoBomber = aircraft
			}
		case "dive_bomber":
			if diveBomber == nil {
				diveBomber = aircraft
			}
		}
	}
	if torpedoBomber == nil {
		t.Fatal("剧本里没有鱼雷机算子")
	}

	withBomb := *torpedoBomber
	withBomb.Loadout = LoadoutBomb
	if !bombAttackDisadvantage(&withBomb) {
		t.Fatalf("%s 挂炸弹没被判成取劣势", withBomb.Model)
	}
	withTorpedo := *torpedoBomber
	withTorpedo.Loadout = LoadoutTorpedo
	if bombAttackDisadvantage(&withTorpedo) {
		t.Fatalf("%s 挂鱼雷不该取劣势", withTorpedo.Model)
	}
	if diveBomber != nil {
		bombingDive := *diveBomber
		bombingDive.Loadout = LoadoutBomb
		if bombAttackDisadvantage(&bombingDive) {
			t.Fatalf("%s 是俯冲轰炸机，挂炸弹不该取劣势", bombingDive.Model)
		}
	}

	// 取劣势 = 掷两次取差，等价于命中率平方。命中率 50% 时有效命中率应接近 25%。
	const trials = 4000
	plain := 0
	disadvantage := 0
	for i := 0; i < trials; i++ {
		if strikeRollHits(&state, 50, 1) {
			plain++
		}
		if strikeRollHits(&state, 50, 2) {
			disadvantage++
		}
	}
	half := trials / 2
	quarter := trials / 4
	if plain < half-half/8 || plain > half+half/8 {
		t.Fatalf("普通判定命中 %d/%d，偏离 50%% 太多", plain, trials)
	}
	if disadvantage < quarter-quarter/3 || disadvantage > quarter+quarter/3 {
		t.Fatalf("取劣势命中 %d/%d，偏离 25%%（命中率平方）太多", disadvantage, trials)
	}
}

func TestNightFleetStrikeUsesThreeDiceDisadvantage(t *testing.T) {
	state := State{}
	dive := &AirUnit{Kind: "dive_bomber", Loadout: LoadoutBomb}
	torpedoBomb := &AirUnit{Kind: "torpedo_bomber", Loadout: LoadoutBomb}
	if got := strikeDisadvantageDice(state, dive); got != 1 {
		t.Fatalf("白天普通对舰投弹应为1骰，实际%d", got)
	}
	if got := strikeDisadvantageDice(state, torpedoBomb); got != 2 {
		t.Fatalf("白天鱼雷机挂炸弹应为2骰取劣，实际%d", got)
	}
	state.Night = true
	for _, attacker := range []*AirUnit{dive, torpedoBomb} {
		if got := strikeDisadvantageDice(state, attacker); got != 3 {
			t.Fatalf("夜间%s对舰投弹应为3骰取劣，实际%d", attacker.Kind, got)
		}
	}

	// 50% 单骰命中经过 3 骰取劣后为 12.5%。
	const trials = 4000
	state.RNGState = 42
	hits := 0
	for range trials {
		if strikeRollHits(&state, 50, 3) {
			hits++
		}
	}
	if hits < 350 || hits > 650 {
		t.Fatalf("夜间3骰取劣命中%d/%d，偏离12.5%%太多", hits, trials)
	}
}
