package game

import "testing"

// TestJoinReconGroupInheritsParams 回归：向已有侦察组加飞机时只提交 task_id，
// 不重复抄 heading/sector_width，服务端必须继承原组参数（API.md 的约定）。
func TestJoinReconGroupInheritsParams(t *testing.T) {
	state := sandboxState(t)
	first := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-dive_bomber-x1", Heading: "W", SectorWidth: 60}
	if err := ValidateAirOrder(state, "B", first); err != nil {
		t.Fatalf("首架侦察机校验失败：%v", err)
	}
	ApplyAirOrder(&state, first)
	leader := state.Aircraft["b-cv-01-dive_bomber-x1"]
	join := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-torpedo_bomber-x1", TaskID: leader.TaskID}
	if err := ValidateAirOrder(state, "B", join); err != nil {
		t.Fatalf("只带 task_id 加入侦察组应被接受，实际被拒：%v", err)
	}
	ApplyAirOrder(&state, join)
	member := state.Aircraft["b-cv-01-torpedo_bomber-x1"]
	if member.Heading != leader.Heading || member.SectorWidth != leader.SectorWidth || member.TaskID != leader.TaskID {
		t.Fatalf("加入的算子应继承原组航向/扇面：%+v vs %+v", member, leader)
	}
	// 半径不继承：它是各机型自己的最大搜索半径（九七舰攻和九九舰爆基础航程不同）。
	if member.ReconRadius != ReconRadiusLimit(*member) {
		t.Fatalf("半径应等于该机型上限 %d，实际 %d", ReconRadiusLimit(*member), member.ReconRadius)
	}
}

// TestNewReconGroupStillRequiresHeading 新建侦察组仍然必须给航向和扇面。
func TestNewReconGroupStillRequiresHeading(t *testing.T) {
	state := sandboxState(t)
	order := Order{Type: "LAUNCH_RECON", AircraftID: "b-cv-01-dive_bomber-x1"}
	if err := ValidateAirOrder(state, "B", order); err == nil {
		t.Fatal("新建侦察组不带航向应被拒绝")
	}
}
