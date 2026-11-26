package game

import "testing"

// 岸基航空兵没有升降机：机场的整备流程是 机库整备 → 滑行到跑道 → 起飞，
// 全程不会出现 elevator / lifting 这类航母阶段，所以同样一批飞机比航母早出动一回合。
func TestAirfieldPrepNeedsNoElevator(t *testing.T) {
	state := coralCombatState(t)
	state.Timeline = nil
	plane := state.GroundAir["af-a-moresby-p40-1"]
	if plane == nil {
		t.Fatal("找不到莫尔兹比 P-40")
	}
	order := Order{Type: "LAUNCH_CAP", AircraftID: plane.ID}
	if err := ValidateOrder(state, "A", order); err != nil {
		t.Fatalf("机场 CAP 命令应合法：%v", err)
	}
	ApplyGroundAirOrder(&state, order)

	phases := []string{}
	phaseOf := func() string {
		for _, task := range GroundAviationView(state, "A").Tasks {
			if task.ID == plane.TaskID {
				return task.Phase
			}
		}
		return ""
	}
	if got := phaseOf(); got != "servicing" {
		t.Fatalf("分配任务后应是机库整备阶段，实际 %q", got)
	}
	for turn := 0; turn < 3; turn++ {
		phases = append(phases, phaseOf())
		AdvanceAirfields(&state)
		AdvanceGroundAir(&state)
		if plane.Status == "elevator" {
			t.Fatalf("岸基算子不该出现在升降机阶段（第 %d 回合）", turn+1)
		}
	}
	// 滑行到跑道是独立的一回合，机库整备不占升降机，所以第 2 回合就该升空。
	if phases[1] != "taxiing" {
		t.Fatalf("第 2 回合应是滑行至跑道，实际 %q", phases[1])
	}
	if !airfieldAirborne(plane) {
		t.Fatalf("岸基算子应在 2 回合内升空，实际状态 %s 阶段 %q", plane.Status, phaseOf())
	}
	for _, phase := range phases {
		if phase == "lifting" {
			t.Fatalf("机场任务不该出现升降机提升阶段：%v", phases)
		}
	}
}
