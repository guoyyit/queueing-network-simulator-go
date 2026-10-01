package rng

import "testing"

func TestSubstreamsUncorrelated(t *testing.T) {
	a := New(SubSeed(12345, KindArrival))
	b := New(SubSeed(12345, Kind(KindServiceBase)))
	const n = 100000
	var sa, sb, sab float64
	for i := 0; i < n; i++ {
		x, y := a.Float64(), b.Float64()
		sa += x
		sb += y
		sab += x * y
	}
	ma, mb := sa/n, sb/n
	cov := sab/n - ma*mb
	if cov > 0.002 || cov < -0.002 {
		t.Fatalf("substream covariance = %.5f, want ~0", cov)
	}
}
