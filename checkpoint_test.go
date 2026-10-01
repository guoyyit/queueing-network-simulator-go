package qns

import (
	"bytes"
	"reflect"
	"testing"
)

func checkpointTestConfig() Config {
	return Config{
		Stations: []StationConfig{
			{Name: "A", Servers: 2, Capacity: 5, Policy: PriorityNonPreemptive,
				Service: Exponential{Rate: 4}},
			{Name: "B", Servers: 1, Capacity: 2, Policy: FCFS,
				Service: Exponential{Rate: 3}},
		},
		Blocking: Reject,
		Open: &OpenModel{
			Arrivals:    PoissonProcess{Rate: 5},
			Entry:       []Route{{To: 0, Prob: 0.7}, {To: 1, Prob: 0.3}},
			Transitions: [][]Route{{{To: 1, Prob: 0.6}}, {}},
		},
		Warmup:  20,
		Batches: 5,
	}
}

// TestCheckpointResumeEquality runs the same simulation once continuously and
// once checkpointed at several intermediate times, then compares every public
// statistic. The resumed run must be identical, RNG stream included.
func TestCheckpointResumeEquality(t *testing.T) {
	cfg := checkpointTestConfig()
	const total = 100.0
	checkpoints := []float64{13.5, 47.25, 71.0, 99.99}

	continuous := New(cfg, 999)
	if err := continuous.Run(total); err != nil {
		t.Fatal(err)
	}

	resumed := New(cfg, 999)
	var cp *Checkpoint
	for i, ct := range checkpoints {
		if err := resumed.Run(ct); err != nil {
			t.Fatal(err)
		}
		cp, _ = resumed.Checkpoint()
		var err error
		resumed, err = Resume(cfg, cp)
		if err != nil {
			t.Fatalf("resume at checkpoint %d: %v", i, err)
		}
	}
	if err := resumed.Run(total); err != nil {
		t.Fatal(err)
	}

	got := resumed.Results()
	want := continuous.Results()
	if !reflect.DeepEqual(got, want) {
		for i := range want.Stations {
			if !reflect.DeepEqual(got.Stations[i], want.Stations[i]) {
				t.Errorf("station %d differs:\n got=%#v\nwant=%#v",
					i, got.Stations[i], want.Stations[i])
			}
		}
		t.Errorf("totals differ: completed got %d want %d; rejected got %d want %d",
			got.Completed, want.Completed, got.Rejected, want.Rejected)
	}
}

// TestCheckpointSerialization round-trips a checkpoint through JSON and
// resumes from the decoded copy; results must still match a continuous run.
func TestCheckpointSerialization(t *testing.T) {
	cfg := checkpointTestConfig()
	const total = 80.0

	continuous := New(cfg, 4242)
	if err := continuous.Run(total); err != nil {
		t.Fatal(err)
	}

	sim := New(cfg, 4242)
	if err := sim.Run(37.7); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := sim.WriteCheckpoint(&buf); err != nil {
		t.Fatal(err)
	}
	resumed, err := ReadCheckpoint(cfg, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := resumed.Run(total); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(resumed.Results(), continuous.Results()) {
		t.Fatal("JSON round-trip resumed run does not match continuous run")
	}

	// Serializing the same logical state twice must give identical bytes.
	cp1, _ := sim.Checkpoint()
	cp2, _ := sim.Checkpoint()
	var b1, b2 bytes.Buffer
	cp1.Write(&b1)
	cp2.Write(&b2)
	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Fatal("checkpoint serialization is not deterministic")
	}
}

// TestCheckpointMidBlock resumes while a sender is blocked, exercising the
// blocked-server state in the snapshot.
func TestCheckpointMidBlock(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Name: "A", Servers: 1, Capacity: -1, Service: Constant{Value: 1}},
			{Name: "B", Servers: 1, Capacity: 0, Service: Constant{Value: 5}},
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
	continuous := New(cfg, 7)
	if err := continuous.Run(30); err != nil {
		t.Fatal(err)
	}

	resumed := New(cfg, 7)
	if err := resumed.Run(2.5); err != nil {
		t.Fatal(err)
	}
	if resumed.blockedAt(0) == 0 {
		t.Fatal("test setup expected a blocked server at checkpoint")
	}
	cp, _ := resumed.Checkpoint()
	r2, err := Resume(cfg, cp)
	if err != nil {
		t.Fatal(err)
	}
	if r2.blockedAt(0) == 0 {
		t.Fatal("blocked server state lost across checkpoint")
	}
	if err := r2.Run(30); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r2.Results(), continuous.Results()) {
		t.Fatal("results differ after resuming from mid-block checkpoint")
	}
}
