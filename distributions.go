package qns

import "math"

// Distribution produces independent service-time samples.
type Distribution interface {
	Sample(rng *RNG) float64
}

// Exponential distribution with rate 1/mean.
type Exponential struct {
	Rate float64
}

func (d Exponential) Sample(rng *RNG) float64 { return rng.Exp(d.Rate) }

// Uniform distribution on [Min, Max).
type Uniform struct {
	Min float64
	Max float64
}

func (d Uniform) Sample(rng *RNG) float64 {
	return d.Min + rng.Float64()*(d.Max-d.Min)
}

// Constant always returns Value.
type Constant struct {
	Value float64
}

func (d Constant) Sample(rng *RNG) float64 { return d.Value }

// Normal distribution with the given mean and standard deviation.
type Normal struct {
	Mean   float64
	StdDev float64
}

func (d Normal) Sample(rng *RNG) float64 {
	// Box-Muller; negative draws are kept (the caller chooses the distribution).
	u1 := 1 - rng.Float64()
	u2 := 1 - rng.Float64()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return d.Mean + z*d.StdDev
}

// ArrivalProcess produces inter-arrival times for open models.
type ArrivalProcess interface {
	NextInterarrival(rng *RNG) float64
}

// PoissonProcess is a renewal arrival process with exponential inter-arrivals.
type PoissonProcess struct {
	Rate float64
}

func (p PoissonProcess) NextInterarrival(rng *RNG) float64 {
	return rng.Exp(p.Rate)
}

// DeterministicProcess produces arrivals every Interval time units.
type DeterministicProcess struct {
	Interval float64
}

func (p DeterministicProcess) NextInterarrival(rng *RNG) float64 {
	return p.Interval
}
