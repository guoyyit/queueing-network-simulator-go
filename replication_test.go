package qns

import (
	"reflect"
	"testing"
)

// TestReproducible checks that two runs with identical config and seed return
// bit-for-bit identical statistics, while a different seed produces a
// different (but still deterministic) trajectory.
func TestReproducible(t *testing.T) {
	cfg := checkpointTestConfig()
	run := func(seed int64) Results {
		sim := New(cfg, seed)
		if err := sim.Run(120); err != nil {
			t.Fatal(err)
		}
		return sim.Results()
	}
	a1 := run(12345)
	a2 := run(12345)
	b := run(54321)

	if !reflect.DeepEqual(a1, a2) {
		t.Fatal("identical seeds did not reproduce identical results")
	}
	if reflect.DeepEqual(a1, b) {
		t.Fatal("different seeds unexpectedly produced identical results")
	}
}

// TestReplicationIsolation runs several seeds concurrently and confirms each
// parallel result equals an independent serial run of the same seed (no
// shared mutable state or RNG stream), and that the merged summary aggregates
// all replications.
func TestReplicationIsolation(t *testing.T) {
	cfg := mm1Config(6, 10, 50)
	seeds := []int64{11, 22, 33, 44, 55, 66}

	serial := make([]Results, len(seeds))
	for i, seed := range seeds {
		sim := New(cfg, seed)
		if err := sim.Run(600); err != nil {
			t.Fatal(err)
		}
		serial[i] = sim.Results()
	}

	parallel, summary, err := RunReplications(cfg, seeds, 600)
	if err != nil {
		t.Fatal(err)
	}
	if len(parallel) != len(seeds) {
		t.Fatalf("got %d replications, want %d", len(parallel), len(seeds))
	}
	for i := range seeds {
		if !reflect.DeepEqual(parallel[i], serial[i]) {
			t.Fatalf("replication %d diverged from its serial run", i)
		}
	}
	if summary.Replications != len(seeds) {
		t.Fatalf("summary replication count %d", summary.Replications)
	}
	var completed int64
	for _, r := range serial {
		completed += r.Completed
	}
	if summary.Completed != completed {
		t.Fatalf("merged completed %d, want %d", summary.Completed, completed)
	}
	if n := len(summary.Stations[0].WaitSamples); n == 0 {
		t.Fatal("merged summary should pool wait samples")
	}
}
