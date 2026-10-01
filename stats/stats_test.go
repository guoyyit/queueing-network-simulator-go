package stats

import (
	"math"
	"testing"
)

func TestAccumulatorMeanQuantile(t *testing.T) {
	a := NewAccumulator()
	for i := 1; i <= 10; i++ {
		a.Observe(float64(i))
	}
	if a.Count() != 10 {
		t.Fatalf("count = %d", a.Count())
	}
	if math.Abs(a.Mean()-5.5) > 1e-12 {
		t.Fatalf("mean = %v", a.Mean())
	}
	if got := a.Quantile(0.5); got != 5 {
		t.Fatalf("median = %v, want 5", got)
	}
	if got := a.Quantile(1); got != 10 {
		t.Fatalf("max = %v, want 10", got)
	}
	if got := a.Quantile(0); got != 1 {
		t.Fatalf("min = %v, want 1", got)
	}
}

func TestBatchMeansCoversAll(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	m := BatchMeans(vals, 3)
	if len(m) != 3 {
		t.Fatalf("got %d batches", len(m))
	}
	// 10 = 4 + 3 + 3
	if math.Abs(m[0]-2.5) > 1e-12 || math.Abs(m[1]-6) > 1e-12 || math.Abs(m[2]-9) > 1e-12 {
		t.Fatalf("batch means = %v", m)
	}
}

func TestTCriticalKnownValues(t *testing.T) {
	cases := []struct {
		df    float64
		p     float64
		table float64
	}{
		{1, 0.975, 12.706},
		{2, 0.975, 4.303},
		{4, 0.975, 2.776},
		{9, 0.975, 2.262},
		{29, 0.975, 2.045},
	}
	for _, c := range cases {
		got := tCritical(c.p, c.df)
		if math.Abs(got-c.table) > 0.01 {
			t.Errorf("tCritical(p=%.3f, df=%.0f) = %.3f, want ~%.3f", c.p, c.df, got, c.table)
		}
	}
}

func TestNormalQuantile(t *testing.T) {
	cases := []struct {
		p, want float64
	}{
		{0.975, 1.959964},
		{0.5, 0},
		{0.025, -1.959964},
		{0.95, 1.644854},
	}
	for _, c := range cases {
		got := normalQuantile(c.p)
		if math.Abs(got-c.want) > 1e-4 {
			t.Errorf("normalQuantile(%v) = %.5f, want %.5f", c.p, got, c.want)
		}
	}
}

func TestMeanCIWidth(t *testing.T) {
	m := []float64{2.1, 2.0, 2.2, 1.9, 2.0, 2.1, 2.05, 1.95, 2.15, 2.0}
	ci := MeanCI(m, 0.95)
	if math.Abs(ci.Mean-2.045) > 1e-9 {
		t.Fatalf("ci mean = %v", ci.Mean)
	}
	if ci.HalfWidth <= 0 || ci.HalfWidth > 0.5 {
		t.Fatalf("unexpected half width %v", ci.HalfWidth)
	}
}
