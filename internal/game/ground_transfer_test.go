package game

import "testing"

// 陆基转场：把机场算子挪到另一个己方机场。
// 以前 ValidateGroundAirOrder 里没有 LAUNCH_TRANSFER 分支，机场飞机只能
// CAP/侦察/打击，后方基地（凯恩斯、莫尔兹比）的飞机永远上不了前线。
func TestGroundAircraftTransfersToAnotherAirfield(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 17 // 05:20，起飞窗已开
	UpdateDayNight(&state)

	ids := []string{"af-a-moresby-p39-1", "af-a-moresby-p39-2"}
	order := Order{Type: "LAUNCH_TRANSFER", AircraftIDs: ids, Coord: "3707"}
	if err := ValidateGroundAirOrder(state, order); err != nil {
		t.Fatalf("莫尔兹比 -> 凯恩斯 应该允许转场：%v", err)
	}
	ApplyGroundAirOrder(&state, order)

	sawAirborne := false
	for turn := 0; turn < 60; turn++ {
		ResolveOrders(&state, nil)
		done := true
		for _, id := range ids {
			aircraft := state.GroundAir[id]
			if aircraft == nil {
				t.Fatalf("第%d回合：%s 在转场途中消失了", turn, id)
			}
			if airfieldAirborne(aircraft) {
				sawAirborne = true
			}
			if aircraft.AirfieldID != "af-a-cairns" || aircraft.Coord != "3707" {
				done = false
			}
		}
		if done {
			if !sawAirborne {
				t.Fatal("转场应当经过空中阶段，而不是瞬移")
			}
			return
		}
	}
	t.Fatalf("60 回合内没有转场到凯恩斯（airborne=%v）", sawAirborne)
}

// 转场目标可以用"编队牌 tag"提交：以前校验阶段按 TaskID 回退解析，
// 执行阶段却只查 state.GroundAir[tag]，结果是校验通过、实际一架都没改目标——
// 玩家点目的地看到的就是"点了没反应"。
func TestGroundTransferTargetAcceptsTaskTag(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 17
	UpdateDayNight(&state)
	ids := []string{"af-a-townsville-p40-1", "af-a-townsville-p40-2"}
	launch := Order{Type: "LAUNCH_TRANSFER", AircraftIDs: ids}
	if err := ValidateGroundAirOrder(state, launch); err != nil {
		t.Fatalf("先建转场编队应该成功：%v", err)
	}
	ApplyGroundAirOrder(&state, launch)
	taskID := state.GroundAir[ids[0]].TaskID
	if taskID == "" {
		t.Fatal("转场编队应该有编队牌 tag")
	}
	target := Order{Type: "SET_STRIKE_TARGET", AircraftID: taskID, TaskID: taskID, Coord: "1611"}
	if err := ValidateGroundAirOrder(state, target); err != nil {
		t.Fatalf("用编队牌 tag 设转场目标应该通过校验：%v", err)
	}
	ApplyGroundAirOrder(&state, target)
	for _, id := range ids {
		if got := state.GroundAir[id].TargetCoord; got != "1611" {
			t.Fatalf("%s 的转场目标没有落到状态里：%q", id, got)
		}
	}
}

// 航程不够就拒绝：P-39 转场航程 11×2.21≈24 hex，汤斯维尔到莫尔兹比 28 hex；
// P-40 转场航程 17×2.21≈37 hex，同一段能飞。两架都先挪到汤斯维尔再测。
func TestGroundTransferRangeIsEnforced(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 17
	UpdateDayNight(&state)
	for _, id := range []string{"af-a-moresby-p39-1", "af-a-moresby-p40-1"} {
		state.GroundAir[id].AirfieldID = "af-a-townsville"
	}
	order := Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"af-a-moresby-p39-1"}, Coord: "1611"}
	if err := ValidateGroundAirOrder(state, order); err == nil {
		t.Fatal("P-39 航程不足以从汤斯维尔飞到莫尔兹比，应该报错")
	}
	ok := Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"af-a-moresby-p40-1"}, Coord: "1611"}
	if err := ValidateGroundAirOrder(state, ok); err != nil {
		t.Fatalf("P-40 应该能飞汤斯维尔 -> 莫尔兹比：%v", err)
	}
}

// 水上飞机基地只能转场到另一个水上飞机基地；目的地必须是己方机场，且不能是自己。
func TestGroundTransferDestinationRules(t *testing.T) {
	state := coralCombatState(t)
	state.Turn = 17
	UpdateDayNight(&state)
	cases := []struct {
		name  string
		order Order
	}{
		{"水上机转陆地机场", Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"af-a-vila-pby-1"}, Coord: "3707"}},
		{"转场到敌方机场", Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"af-a-townsville-p40-1"}, Coord: "0831"}},
		{"转场到没有机场的hex", Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"af-a-townsville-p40-1"}, Coord: "3100"}},
		{"转场到同一个机场", Order{Type: "LAUNCH_TRANSFER", AircraftIDs: []string{"af-a-townsville-p40-1"}, Coord: "4410"}},
	}
	for _, test := range cases {
		if err := ValidateGroundAirOrder(state, test.order); err == nil {
			t.Fatalf("%s 应该被拒绝", test.name)
		}
	}
}
