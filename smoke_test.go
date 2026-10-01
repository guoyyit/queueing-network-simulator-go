package qns

import "testing"

func TestSmokeMM1(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Servers: 1, Capacity: -1, Policy: FCFS, Service: Exponential{Rate: 2}},
		},
		Blocking: Reject,
		Open: &OpenModel{
			Arrivals:    PoissonProcess{Rate: 1},
			Entry:       []Route{{To: 0, Prob: 1}},
			Transitions: [][]Route{{}}, // everyone leaves after service
		},
		Warmup:  10,
		Batches: 5,
	}
	sim := New(cfg, 42)
	if err := sim.Run(200); err != nil {
		t.Fatalf("run: %v", err)
	}
	r := sim.Results()
	if r.Completed == 0 {
		t.Fatal("no jobs completed")
	}
	rep := r.Stations[0]
	t.Logf("completed=%d throughput=%.3f util=%.3f meanWait=%.3f meanSojourn=%.3f timeAvgQL=%.3f timeAvgIS=%.3f rejected=%d",
		rep.Completed, rep.Throughput, rep.Utilization, rep.MeanWait, rep.MeanSojourn,
		rep.TimeAvgQueueLength, rep.TimeAvgInSystem, rep.Rejected)
	if rep.Utilization <= 0 || rep.Utilization >= 1 {
		t.Errorf("utilization out of range: %v", rep.Utilization)
	}
	if rep.MeanSojourn <= 0 {
		t.Errorf("nonpositive sojourn: %v", rep.MeanSojourn)
	}
}
