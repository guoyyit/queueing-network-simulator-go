package qns

import (
	"math"
	"testing"
)

// mm1Config builds a single M/M/1 station with arrival rate lambda and
// service rate mu; jobs leave after their single visit.
func mm1Config(lambda, mu float64, warmup float64) Config {
	return Config{
		Stations: []StationConfig{
			{Servers: 1, Capacity: -1, Policy: FCFS, Service: Exponential{Rate: mu}},
		},
		Blocking: Reject,
		Open: &OpenModel{
			Arrivals:    PoissonProcess{Rate: lambda},
			Entry:       []Route{{To: 0, Prob: 1}},
			Transitions: [][]Route{{}},
		},
		Warmup:  warmup,
		Batches: 10,
	}
}

// TestMM1Analytic checks the classical M/M/1 formulas:
//
//	rho = lambda/mu,  Wq = rho/(mu-lambda),  W = 1/(mu-lambda),
//	Lq = rho^2/(1-rho),  L = rho/(1-rho).
//
// A long single run with a long warm-up should land within ~5%.
func TestMM1Analytic(t *testing.T) {
	const lambda, mu = 8.0, 10.0
	rho := lambda / mu
	wantWait := rho / (mu - lambda)
	wantSojourn := 1 / (mu - lambda)
	wantLq := rho * rho / (1 - rho)
	wantL := rho / (1 - rho)

	cfg := mm1Config(lambda, mu, 500)
	sim := New(cfg, 7)
	if err := sim.Run(5000); err != nil {
		t.Fatalf("run: %v", err)
	}
	r := sim.Results().Stations[0]

	checks := []struct {
		name string
		got  float64
		want float64
		rel  float64
	}{
		{"utilization", r.Utilization, rho, 0.03},
		{"throughput", r.Throughput, lambda, 0.05},
		{"mean wait", r.MeanWait, wantWait, 0.10},
		{"mean sojourn", r.MeanSojourn, wantSojourn, 0.08},
		{"time avg queue length", r.TimeAvgQueueLength, wantLq, 0.12},
		{"time avg in system", r.TimeAvgInSystem, wantL, 0.10},
	}
	for _, c := range checks {
		if math.Abs(c.got-c.want) > c.rel*math.Abs(c.want) {
			t.Errorf("%s: got %.4f want %.4f (>%.0f%% rel)", c.name, c.got, c.want, c.rel*100)
		}
	}
	// Little's law: Lq ~ lambda * Wq, L ~ lambda * W.
	if math.Abs(r.TimeAvgQueueLength-lambda*r.MeanWait) > 0.15*math.Max(r.TimeAvgQueueLength, 0.01) {
		t.Errorf("Little's law Lq: %.4f vs %.4f", r.TimeAvgQueueLength, lambda*r.MeanWait)
	}
}

// TestMM1AcrossSeeds averages several independent long runs and tightens the
// tolerance, mirroring a replication-based capacity-planning workflow.
func TestMM1AcrossSeeds(t *testing.T) {
	const lambda, mu = 8.0, 10.0
	rho := lambda / mu
	var seeds []int64
	for i := int64(1); i <= 8; i++ {
		seeds = append(seeds, i*101)
	}
	_, summary, err := RunReplications(mm1Config(lambda, mu, 300), seeds, 2500)
	if err != nil {
		t.Fatal(err)
	}
	r := summary.Stations[0]
	if math.Abs(r.Utilization-rho) > 0.02 {
		t.Errorf("utilization: got %.4f want %.4f", r.Utilization, rho)
	}
	if math.Abs(r.MeanSojourn-1/(mu-lambda)) > 0.05 {
		t.Errorf("mean sojourn: got %.4f want %.4f", r.MeanSojourn, 1/(mu-lambda))
	}
}
