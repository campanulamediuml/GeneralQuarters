package game

import "testing"

// 每部可用升降机 4 算子，损坏的整部不计入：这就是"坏一部 -4"的实现口径。
func TestElevatorBudgetDropsFourPerDamagedElevator(t *testing.T) {
	carrier := &Unit{ID: "cv", Type: "CV", Deck: &FlightDeck{Elevators: []*AircraftElevator{
		{ID: "e1", Capacity: 4, Status: "idle"},
		{ID: "e2", Capacity: 4, Status: "idle"},
		{ID: "e3", Capacity: 4, Status: "idle"},
	}}}
	if got := elevatorBudget(carrier); got != 12 {
		t.Fatalf("三部可用升降机应有 12 个算子名额，实际 %d", got)
	}
	carrier.Deck.Elevators[0].Status = "damaged"
	if got := elevatorBudget(carrier); got != 8 {
		t.Fatalf("坏一部应少 4，实际 %d", got)
	}
	carrier.Deck.Elevators[2].Status = "damaged"
	if got := elevatorBudget(carrier); got != 4 {
		t.Fatalf("坏两部应剩 4，实际 %d", got)
	}
	carrier.Deck.Elevators[1].Status = "damaged"
	if got := elevatorBudget(carrier); got != 0 {
		t.Fatalf("全坏应为 0，实际 %d", got)
	}
	if AviationAvailable(carrier) {
		t.Fatal("升降机全坏时不应还能做航空作业")
	}
}
