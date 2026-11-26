package game

// primeService 把机库整备推进到"可提升"状态（规则书 240–244 行：整备 1 回合就够）。
// 老用例是按"分配任务当回合就上电梯"写的，引入整备计数后需要显式把这 1 回合补上，
// 但升降机/甲板容量本身的行为没有变。
func primeService(state *State) {
	for _, aircraft := range state.Aircraft {
		if aircraft.Status == "service" && aircraft.ServiceTurns < 1 {
			aircraft.ServiceTurns = 1
		}
	}
}
