package qns

import "testing"

// TestLCFSOrder fills a single server with three known jobs and checks that
// after it frees, the most recently queued job is served first.
func TestLCFSOrder(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Name: "s", Servers: 1, Capacity: -1, Policy: LCFS,
				Service: Constant{Value: 10}},
		},
		Blocking: Reject,
		Closed: &ClosedModel{
			Population:   1,
			StartStation: 0,
			Transitions:  [][]Route{{{To: 0, Prob: 1}}},
		},
	}
	sim := New(cfg, 9)
	if err := sim.initState(); err != nil {
		t.Fatal(err)
	}
	busy := sim.runningAt(0)[0]

	j1 := sim.newJob(sim.Now())
	j2 := sim.newJob(sim.Now())
	j3 := sim.newJob(sim.Now())
	for _, j := range []*Job{j1, j2, j3} {
		if sim.admit(j, 0) != admitOK {
			t.Fatalf("admit %d failed", j.ID)
		}
	}
	// Queue in stack order; the busy job is the original closed job.
	q := sim.queueAt(0)
	if len(q) != 3 || q[2].ID != j3.ID {
		t.Fatalf("LCFS stack top wrong: %v", q)
	}

	// Free the server by completing the busy job (route it away: switch to
	// open semantics with zero routing like the preemption tests).
	sim.cfg.Closed = nil
	sim.cfg.Open = &OpenModel{
		Arrivals:    DeterministicProcess{Interval: 1e12},
		Entry:       []Route{{To: 0, Prob: 1}},
		Transitions: [][]Route{{}},
	}
	_ = busy
	if err := sim.Run(10); err != nil {
		t.Fatal(err)
	}
	next := sim.runningAt(0)[0]
	if next == nil || next.ID != j3.ID {
		id := int64(-1)
		if next != nil {
			id = next.ID
		}
		t.Fatalf("LCFS should serve j3 (id %d) first, got id %d", j3.ID, id)
	}
}
