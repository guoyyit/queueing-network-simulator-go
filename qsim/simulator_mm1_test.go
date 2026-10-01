package qsim

import (
	"math"
	"testing"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
)

func mm1Config(seed uint64, lambda, mu float64, end, warmup float64) *Config {
	return &Config{
		Blocking: Reject,
		Seed:     seed,
		Warmup:   warmup,
		EndTime:  end,
		Network: &NetworkConfig{
			Model:          Open,
			ExternalArrive: []float64{lambda},
			Routing:        [][]float64{{0.0}},
			Stations: []StationConfig{{
				Name: "mm1", Servers: 1, Capacity: -1,
				Policy: FCFS, Service: dist.Exponential{Rate: mu},
			}},
		},
	}
}

func TestMM1Analytical(t *testing.T) {
	lambda, mu := 4.0, 5.0
	cfg := mm1Config(12345, lambda, mu, 60000, 6000)
	cfg.BatchCount = 20
	cfg.Confidence = 0.95
	sim := New(cfg)
	sim.Run(0)
	rep := sim.Report()
	r := rep.Stations[0]

	rho := lambda / mu
	// M/M/1: L = rho/(1-rho), Wq = rho/(mu-lambda), rho 利用率。
	wantL := rho / (1 - rho)
	wantWq := rho / (mu - lambda)

	if math.Abs(r.Utilization-rho) > 0.02 {
		t.Errorf("utilization = %.3f, want ~%.3f", r.Utilization, rho)
	}
	if math.Abs(r.MeanNumberInSystem-wantL) > 0.15 {
		t.Errorf("mean number in system = %.3f, want ~%.3f", r.MeanNumberInSystem, wantL)
	}
	if math.Abs(r.MeanWait-wantWq) > 0.03 {
		t.Errorf("mean wait = %.4f, want ~%.4f", r.MeanWait, wantWq)
	}
	if math.Abs(r.Throughput-lambda) > 0.08 {
		t.Errorf("throughput = %.3f, want ~%.3f", r.Throughput, lambda)
	}
	if math.Abs(r.MeanSojourn-1/(mu-lambda)) > 0.03 {
		t.Errorf("mean sojourn = %.4f, want ~%.4f", r.MeanSojourn, 1/(mu-lambda))
	}
	if r.WaitCI.HalfWidth <= 0 || r.WaitCI.HalfWidth > 0.1 {
		t.Errorf("wait CI half-width unexpected: %+v", r.WaitCI)
	}
	if r.Rejected != 0 {
		t.Errorf("infinite queue should reject none, got %d", r.Rejected)
	}
}

func TestReproducibleRuns(t *testing.T) {
	c1 := mm1Config(777, 4, 5, 500, 50)
	s1 := New(c1)
	s1.Run(0)
	r1 := s1.Report()

	c2 := mm1Config(777, 4, 5, 500, 50)
	s2 := New(c2)
	s2.Run(0)
	r2 := s2.Report()

	if !reportsEqual(r1, r2) {
		t.Fatal("identical seed/config produced different reports")
	}
}

func reportsEqual(a, b *SimulationReport) bool {
	if a.Rejected != b.Rejected || a.Completed != b.Completed ||
		len(a.Stations) != len(b.Stations) {
		return false
	}
	for i := range a.Stations {
		x, y := a.Stations[i], b.Stations[i]
		if x.Completed != y.Completed || x.Rejected != y.Rejected ||
			x.MeanWait != y.MeanWait || x.MeanSojourn != y.MeanSojourn ||
			x.Throughput != y.Throughput || x.Utilization != y.Utilization ||
			x.MeanQueueLength != y.MeanQueueLength {
			return false
		}
	}
	return true
}
