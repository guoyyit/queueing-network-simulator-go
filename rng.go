package qns

import "math"

// RNG is a deterministic pseudo-random number generator whose full internal
// state can be snapshotted and restored, so checkpoints resume the exact
// random stream.
type RNG struct {
	state uint64
	seed  int64
}

// NewRNG creates a generator seeded with seed.
func NewRNG(seed int64) *RNG {
	// SplitMix64 avalanche so nearby seeds produce unrelated streams and the
	// xorshift forbidden all-zero state never occurs.
	z := uint64(seed) + uint64(0x9E3779B97F4A7C15)
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	z = z ^ (z >> 31)
	if z == 0 {
		z = 1
	}
	return &RNG{state: z, seed: seed}
}

// Uint64 returns the next pseudo-random uint64.
func (r *RNG) Uint64() uint64 {
	x := r.state
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	return x * 0x2545F4914F6CDD1D
}

// Float64 returns a pseudo-random value in [0, 1).
func (r *RNG) Float64() float64 {
	return float64(r.Uint64()>>11) / float64(uint64(1)<<53)
}

// Exp returns an exponentially distributed sample with the given rate.
func (r *RNG) Exp(rate float64) float64 {
	// Draw (0,1] so the logarithm is always finite.
	u := 1 - r.Float64()
	return -math.Log(u) / rate
}

func (r *RNG) snapshotState() uint64     { return r.state }
func (r *RNG) restoreState(state uint64) { r.state = state }
