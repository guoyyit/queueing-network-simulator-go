package dist

import (
	"math"
	"testing"

	"github.com/guoyyit/queueing-network-simulator-go/rng"
)

func TestExponentialMean(t *testing.T) {
	r := rng.New(2024)
	d := Exponential{Rate: 4}
	const n = 200000
	sum := 0.0
	for i := 0; i < n; i++ {
		v := d.Sample(r)
		if v <= 0 {
			t.Fatalf("non-positive exponential draw %v", v)
		}
		sum += v
	}
	mean := sum / n
	if math.Abs(mean-0.25) > 0.005 {
		t.Fatalf("exponential mean = %.4f, want ~0.25", mean)
	}
}

func TestDeterministic(t *testing.T) {
	if got := (Deterministic{Value: 3.5}).Sample(rng.New(1)); got != 3.5 {
		t.Fatalf("deterministic = %v, want 3.5", got)
	}
}

func TestUniformRange(t *testing.T) {
	r := rng.New(7)
	u := Uniform{Min: 2, Max: 5}
	for i := 0; i < 1000; i++ {
		v := u.Sample(r)
		if v < 2 || v >= 5 {
			t.Fatalf("uniform draw out of range: %v", v)
		}
	}
}

func TestErlangMean(t *testing.T) {
	r := rng.New(9)
	e := Erlang{K: 3, Rate: 6}
	const n = 100000
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += e.Sample(r)
	}
	mean := sum / n
	if math.Abs(mean-0.5) > 0.02 {
		t.Fatalf("erlang mean = %.4f, want ~0.5", mean)
	}
}
