package qsim

import "testing"

func TestDequeueOrder(t *testing.T) {
	mk := func(p Policy) *Station {
		return newStation(0, StationConfig{Servers: 3, Capacity: -1, Policy: p})
	}

	// FCFS
	fcfs := mk(FCFS)
	for id := uint64(1); id <= 3; id++ {
		fcfs.enqueue(&Customer{ID: id, ArrivalTime: float64(id)})
	}
	for _, want := range []uint64{1, 2, 3} {
		if got := fcfs.dequeue().ID; got != want {
			t.Fatalf("FCFS got %d want %d", got, want)
		}
	}

	// LCFS
	lcfs := mk(LCFS)
	for id := uint64(1); id <= 3; id++ {
		lcfs.enqueue(&Customer{ID: id, ArrivalTime: float64(id)})
	}
	for _, want := range []uint64{3, 2, 1} {
		if got := lcfs.dequeue().ID; got != want {
			t.Fatalf("LCFS got %d want %d", got, want)
		}
	}

	// PriorityNP：高优先级先，同优先级按到达先后
	pp := mk(PriorityNP)
	pp.enqueue(&Customer{ID: 1, Priority: 5, ArrivalTime: 1})
	pp.enqueue(&Customer{ID: 2, Priority: 0, ArrivalTime: 2})
	pp.enqueue(&Customer{ID: 3, Priority: 0, ArrivalTime: 3})
	want := []uint64{2, 3, 1}
	for _, w := range want {
		if got := pp.dequeue().ID; got != w {
			t.Fatalf("PriorityNP got %d want %d", got, w)
		}
	}
}
