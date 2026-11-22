package game

import (
	"strings"
	"testing"
)

// TestCarrierBlockedReasonProjected 航空舰不能起降时要把原因投影给客户端。
func TestCarrierBlockedReasonProjected(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	EnsureDamage(carrier).Tilt = true
	view := VisibleAviation(state, "B")
	for _, item := range view.Carriers {
		if item.UnitID != "b-cv-01" {
			continue
		}
		if item.BlockedReason == "" {
			t.Fatal("倾斜的航母应投影 blocked_reason")
		}
		if !containsText(item.BlockedReason, "倾斜") {
			t.Fatalf("原因应说明倾斜：%s", item.BlockedReason)
		}
		return
	}
	t.Fatal("投影里找不到 b-cv-01")
}

// TestCarrierPreventedByReportsIdleStrike 甲板被无目标攻击编队占满时，
// prevented_by 要指出挡路的是谁，玩家才知道该取消哪些任务。
func TestCarrierPreventedByReportsIdleStrike(t *testing.T) {
	state := sandboxState(t)
	carrier := state.Units["b-cv-01"]
	for id, aircraft := range state.Aircraft {
		if aircraft.CarrierID != carrier.ID {
			continue
		}
		aircraft.Status = "deck"
		aircraft.TaskID = "strike-idle"
		aircraft.TaskKind = "attack"
		aircraft.TargetCoord = ""
		aircraft.TargetFleetID = ""
		_ = id
	}
	view := VisibleAviation(state, "B")
	for _, item := range view.Carriers {
		if item.UnitID != carrier.ID {
			continue
		}
		if item.PreventedBy == "" {
			t.Fatalf("甲板被占满时应提示挡路来源：deck_used=%d/%d blocked=%q", item.DeckUsed, item.DeckLength, item.BlockedReason)
		}
		return
	}
	t.Fatal("投影里找不到 b-cv-01")
}

func containsText(haystack, needle string) bool {
	return needle == "" || strings.Contains(haystack, needle)
}
