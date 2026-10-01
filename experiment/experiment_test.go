package experiment

import (
	"math"
	"testing"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
	"github.com/guoyyit/queueing-network-simulator-go/qsim"
)

func testConfig() *qsim.Config {
	return &qsim.Config{
		Blocking: qsim.Reject, Seed: 99, Warmup: 200, EndTime: 4000,
		BatchCount: 10, Confidence: 0.95,
		Network: &qsim.NetworkConfig{
			Model:          qsim.Open,
			ExternalArrive: []float64{4},
			Routing:        [][]float64{{0}},
			Stations: []qsim.StationConfig{{
				Name: "s", Servers: 1, Capacity: -1,
				Policy: qsim.FCFS, Service: dist.Exponential{Rate: 5},
			}},
		},
	}
}

func TestRunSeedsDistinctAndDeterministic(t *testing.T) {
	r1, err := Run(testConfig(), 8)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[uint64]bool{}
	for _, r := range r1 {
		if seen[r.Seed] {
			t.Fatalf("duplicate seed %d", r.Seed)
		}
		seen[r.Seed] = true
		if r.Report == nil {
			t.Fatal("nil report")
		}
	}

	r2, _ := Run(testConfig(), 8)
	for i := range r1 {
		if r1[i].Seed != r2[i].Seed {
			t.Fatalf("replication %d seed not deterministic", i)
		}
		a := r1[i].Report.Stations[0]
		b := r2[i].Report.Stations[0]
		if a.MeanWait != b.MeanWait || a.Completed != b.Completed {
			t.Fatalf("replication %d not deterministic", i)
		}
	}
}

func TestConfigNotShared(t *testing.T) {
	cfg := testConfig()
	_, _ = Run(cfg, 4)
	// 并发运行不得改写调用方配置
	if cfg.Seed != 99 {
		t.Fatalf("caller seed mutated to %d", cfg.Seed)
	}
	if cfg.Network.Stations[0].Name != "s" {
		t.Fatal("caller network config mutated")
	}
}

func TestAggregate(t *testing.T) {
	results, err := Run(testConfig(), 16)
	if err != nil {
		t.Fatal(err)
	}
	sum := Aggregate(results)
	if sum.Replications != 16 {
		t.Fatalf("replications = %d", sum.Replications)
	}
	s := sum.Stations[0]
	if math.Abs(s.MeanUtilization-0.8) > 0.05 {
		t.Errorf("aggregated utilization = %.3f, want ~0.8", s.MeanUtilization)
	}
	// 均值应等于逐复制均值的算术平均
	var meanWait float64
	for _, r := range results {
		meanWait += r.Report.Stations[0].MeanWait
	}
	meanWait /= float64(len(results))
	if math.Abs(meanWait-s.MeanWait) > 1e-12 {
		t.Errorf("aggregate mean mismatch: %.6f vs %.6f", meanWait, s.MeanWait)
	}
	if s.StdWait < 0 {
		t.Error("negative std")
	}
}

func TestRejectsBadInput(t *testing.T) {
	if _, err := Run(testConfig(), 0); err == nil {
		t.Fatal("expected error for zero replications")
	}
}
