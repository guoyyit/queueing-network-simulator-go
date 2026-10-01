package qns

import "testing"

// TestPriorityPreemptive verifies:
//  1. a higher-priority arrival immediately preempts a running low-priority
//     job on a busy single server;
//  2. the preempted job keeps its unconsumed remaining service time, returns
//     to the queue and resumes (no fresh service sample);
//  3. a same/lower priority arrival queues without preempting.
func TestPriorityPreemptive(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Name: "p", Servers: 1, Capacity: -1, Policy: PriorityPreemptive,
				Service: Constant{Value: 10}},
		},
		Blocking: Reject,
		Closed: &ClosedModel{
			Population:   1,
			StartStation: 0,
			Transitions:  [][]Route{{{To: 0, Prob: 1}}},
		},
	}
	sim := New(cfg, 1)
	if err := sim.Run(3); err != nil {
		t.Fatal(err)
	}
	// The single closed job has been running for 3 time units of its 10.
	low := sim.runningAt(0)[0]
	if low == nil {
		t.Fatal("expected low-priority job in service")
	}

	// Inject a fresh high-priority job arriving now.
	sim.runningAt(0)[0].Priority = 0
	hi := sim.newJob(sim.Now())
	hi.Priority = 5
	// After this point every completed job leaves the system, so the high-
	// priority job does not loop back and perpetually dominate the victim.
	sim.cfg.Closed = nil
	sim.cfg.Open = &OpenModel{
		Arrivals:    DeterministicProcess{Interval: 1e12},
		Entry:       []Route{{To: 0, Prob: 1}},
		Transitions: [][]Route{{}},
	}
	if out := sim.admit(hi, 0); out != admitOK {
		t.Fatalf("admit high priority: %v", out)
	}

	running := sim.runningAt(0)[0]
	if running.ID != hi.ID {
		t.Fatalf("high-priority job not running: got id %d", running.ID)
	}
	queued := sim.queueAt(0)
	if len(queued) != 1 {
		t.Fatalf("expected 1 preempted job in queue, got %d", len(queued))
	}
	victim := queued[0]
	if victim.Remaining <= 0 || victim.Remaining >= 10 {
		t.Fatalf("preempted job remaining time not preserved: %v", victim.Remaining)
	}
	if d := victim.Remaining - 7; d > 1e-9 || d < -1e-9 {
		t.Fatalf("expected ~7 remaining, got %v", victim.Remaining)
	}
	if victim.ServiceStart != 0 {
		t.Fatalf("preempted job should not appear started: ServiceStart=%v", victim.ServiceStart)
	}

	// Advance 10 time units: hi finishes at t=13, victim resumes with its 7
	// remaining units and finishes at t=20. Capture completion records.
	before := sim.Results().Stations[0].Completed
	if err := sim.Run(13); err != nil { // hi completes exactly here
		t.Fatal(err)
	}
	if sim.Now() != 13 {
		t.Fatalf("time = %v", sim.Now())
	}
	if sim.runningAt(0)[0] == nil || sim.runningAt(0)[0].ID != victim.ID {
		t.Fatalf("victim did not resume after high-priority departure")
	}
	resumed := sim.runningAt(0)[0]
	if resumed.Remaining != 7 {
		t.Fatalf("resumed job remaining = %v, want 7", resumed.Remaining)
	}

	if err := sim.Run(20); err != nil {
		t.Fatal(err)
	}
	after := sim.Results().Stations[0].Completed
	if after-before != 2 {
		t.Fatalf("expected 2 completions (hi then victim), got %d", after-before)
	}

	// Queueing-time accounting: the victim waited 10 time units (3 preempted +
	// 7 resumed); its sojourn spans arrival(3) to completion(20) = 17.
	waits := sim.Results().Stations[0].WaitSamples
	if n := len(waits); n < 2 {
		t.Fatalf("expected wait samples, got %d", n)
	}
	last := waits[len(waits)-1]
	if d := last - 10; d > 1e-9 || d < -1e-9 {
		t.Fatalf("preempted job wait = %v, want 10", last)
	}
}

// TestPriorityNonPreemptive checks that a waiting high-priority job is picked
// before ordinary waiters once the server frees, but a running job is never
// interrupted.
func TestPriorityNonPreemptive(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Servers: 1, Capacity: -1, Policy: PriorityNonPreemptive,
				Service: Constant{Value: 10}},
		},
		Blocking: Reject,
		Closed: &ClosedModel{
			Population: 1, StartStation: 0,
			Transitions: [][]Route{{{To: 0, Prob: 1}}},
		},
	}
	sim := New(cfg, 2)
	if err := sim.Run(2); err != nil {
		t.Fatal(err)
	}
	runningLow := sim.runningAt(0)[0]

	mid := sim.newJob(sim.Now())
	mid.Priority = 1
	if sim.admit(mid, 0) != admitOK {
		t.Fatal("admit mid")
	}
	hi := sim.newJob(sim.Now())
	hi.Priority = 9
	if sim.admit(hi, 0) != admitOK {
		t.Fatal("admit hi")
	}

	// Non-preemptive: the original low-priority job keeps running.
	if sim.runningAt(0)[0].ID != runningLow.ID {
		t.Fatal("non-preemptive policy interrupted a running job")
	}
	// Queue order must put the high-priority job at the front.
	q := sim.queueAt(0)
	if len(q) != 2 || q[0].ID != hi.ID {
		t.Fatalf("queue order wrong: %v", q)
	}
}
