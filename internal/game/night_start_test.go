package game

import "testing"

// 回归：开了时间线的剧本，开局天色必须按时钟算。
// 以前 State.Night 取剧本里那个静态字段（默认 false），而珊瑚海开局是 00:00，
// 于是第一回合结算被当成"刚入夜"：播一条假的"夜幕降临"，并且
// parkAircraftForNight 把玩家那一刻刚下的 CAP／侦察任务当成夜间待发任务收进机库，
// 任务直接消失，05:20 也不会起飞。
func TestNewStateStartsInCorrectNightPhase(t *testing.T) {
	scenario, gameMap, catalog, err := LoadScenario("../../content", "_coral_real")
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewState(scenario, gameMap, catalog, 7)
	if err != nil {
		t.Fatal(err)
	}
	if state.TurnsPerDay <= 0 {
		t.Skip("剧本没有配置时间线")
	}
	want := state.nightAt(clockAt(state))
	if state.Night != want {
		t.Fatalf("开局 Night 应按时钟算：期望 %v，实际 %v（%s）", want, state.Night, formatClock(clockAt(state)))
	}
	if !want {
		t.Skip("这个剧本不是从夜间开局")
	}
	// 从夜间开局时，第一回合结束不该产生"入夜"切换。
	for _, event := range settleDayNightAtTurnEnd(&state) {
		if event.Type == "day_night" || event.Type == "air_night_parked" {
			t.Fatalf("开局那一回合不该再播入夜事件：%+v", event)
		}
	}
}
