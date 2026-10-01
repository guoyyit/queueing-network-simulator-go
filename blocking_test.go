package qns

import "testing"

// TestBlockingServerHeld verifies server blocking with a closed population of
// four jobs cycling A(1 time unit, infinite queue) -> B(5 time units, zero
// waiting room) -> A:
//
//	t=1: job1 finishes A and enters B (finishes at t=6); job2 starts A.
//	t=2: job2 finishes A but B is busy with no slot -> A blocks, holding
//	     job2 on its server; job3 and job4 remain queued at A.
//	t=6: job1 finishes B and queues back at A; the unblock cascade moves
//	     job2 into B and A resumes serving its queue.
func TestBlockingServerHeld(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Name: "A", Servers: 1, Capacity: -1, Policy: FCFS, Service: Constant{Value: 1}},
			{Name: "B", Servers: 1, Capacity: 0, Policy: FCFS, Service: Constant{Value: 5}},
		},
		Blocking: Block,
		Closed: &ClosedModel{
			Population: 4, StartStation: 0,
			Transitions: [][]Route{
				{{To: 1, Prob: 1}},
				{{To: 0, Prob: 1}},
			},
		},
	}
	sim := New(cfg, 1)
	if err := sim.Run(2.5); err != nil {
		t.Fatal(err)
	}
	if sim.blockedAt(0) != 1 {
		t.Fatalf("expected A blocked with 1 held job, got %d", sim.blockedAt(0))
	}
	if n := sim.queueAt(0); len(n) != 2 {
		t.Fatalf("expected 2 jobs queued behind blocked A, got %d", len(n))
	}
	if running := sim.runningAt(0)[0]; running != nil {
		t.Fatalf("blocked A should have no actively running job")
	}

	// Right after B finishes (t=6) the cascade delivers the parked job and
	// resumes A.
	if err := sim.Run(6.5); err != nil {
		t.Fatal(err)
	}
	if sim.blockedAt(0) != 0 {
		t.Fatalf("A should have unblocked, still %d held", sim.blockedAt(0))
	}
	if sim.runningAt(1)[0] == nil {
		t.Fatal("B should be serving the unblocked job")
	}
	if sim.runningAt(0)[0] == nil {
		t.Fatal("A should have resumed serving after unblock")
	}

	// Population conservation over a long horizon: no job is ever lost.
	if err := sim.Run(50); err != nil {
		t.Fatal(err)
	}
	total := sim.inSystemAt(0) + sim.inSystemAt(1)
	if total != 4 {
		t.Fatalf("population not conserved: %d jobs in system (want 4)", total)
	}
	rejected := sim.Results().Stations[0].Rejected + sim.Results().Stations[1].Rejected
	if rejected != 0 {
		t.Fatalf("block mode must never reject routed jobs, got %d", rejected)
	}
}

// TestRejectCounts verifies finite-capacity rejection: overflow arrivals are
// refused, counted per station and globally, and not silently dropped.
func TestRejectCounts(t *testing.T) {
	// Very fast arrivals, slow single server, capacity 0 at the only station.
	cfg := Config{
		Stations: []StationConfig{
			{Servers: 1, Capacity: 0, Policy: FCFS, Service: Constant{Value: 1000}},
		},
		Blocking: Reject,
		Open: &OpenModel{
			Arrivals:    PoissonProcess{Rate: 100},
			Entry:       []Route{{To: 0, Prob: 1}},
			Transitions: [][]Route{{}},
		},
	}
	sim := New(cfg, 3)
	if err := sim.Run(10); err != nil {
		t.Fatal(err)
	}
	res := sim.Results()
	if res.Rejected == 0 {
		t.Fatal("expected rejections under overload with zero capacity")
	}
	if res.Stations[0].Rejected != res.Rejected {
		t.Fatalf("station/global rejection counts disagree: %d vs %d",
			res.Stations[0].Rejected, res.Rejected)
	}
	// Exactly one job should be in service and never completed within the run.
	if res.Stations[0].Completed != 0 {
		t.Fatalf("no job should finish a 1000-unit service in 10 time units")
	}
	// Arrival accounting: rejected jobs are counted separately, the accepted
	// job remains in the system and is neither lost nor double counted.
	if sim.inSystemAt(0) != 1 {
		t.Fatalf("expected exactly 1 accepted job in system, got %d", sim.inSystemAt(0))
	}
	if res.Completed+res.Rejected+int64(sim.inSystemAt(0)) <= res.Rejected {
		t.Fatal("arrival accounting lost accepted jobs")
	}
}
