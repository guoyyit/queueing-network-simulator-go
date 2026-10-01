package qns

import (
	"math"
	"sort"
)

// ConfidenceInterval is a point estimate with a half-width at a confidence level.
type ConfidenceInterval struct {
	Mean       float64
	HalfWidth  float64
	Confidence float64
}

// StationReport is the statistical output for one workstation.
type StationReport struct {
	Name               string
	Completed          int64
	Rejected           int64
	Throughput         float64
	Utilization        float64
	MeanWait           float64
	WaitQuantiles      map[float64]float64
	WaitCI             ConfidenceInterval
	MeanSojourn        float64
	SojournQuantiles   map[float64]float64
	SojournCI          ConfidenceInterval
	MeanQueueLength    float64 // arithmetic mean queue length at arrival instants
	TimeAvgQueueLength float64 // time-weighted average waiting-only queue length
	TimeAvgInSystem    float64 // time-weighted average number of jobs at station
	QueueLengthCI      ConfidenceInterval
	WaitSamples        []float64 // raw post-warmup queueing-time samples
	SojournSamples     []float64 // raw post-warmup sojourn-time samples
}

// Results is the full per-run statistics output.
type Results struct {
	Time      float64
	StatsTime float64 // time minus warm-up
	Warmup    float64
	Stations  []StationReport
	Rejected  int64
	Completed int64
}

// stationStats accumulates raw statistics for one station. All steady-state
// averages and variance estimates only count jobs whose completion time is
// strictly after the warm-up cut-over, and time areas are split at that
// boundary.
type stationStats struct {
	completed int64
	rejected  int64

	serviceSum float64 // post-warmup busy time (utilization numerator)

	waitSamples    []float64 // post-warmup queueing times
	sojournSamples []float64 // post-warmup times spent at the station

	arrivalQLSum    float64 // sum of waiting lengths seen by post-warmup arrivals
	arrivalQLCounts int64

	// waitJobs/sojournJobs record each post-warmup completion as (time after
	// warm-up, value); batch means are partitioned at report time.
	waitJobs    []timedSample
	sojournJobs []timedSample

	// qlLevel / isLevel record post-warmup level samples (t, level) of the
	// waiting-only queue and the total population at the station.
	qlLevel []levelPoint
	isLevel []levelPoint
}

// timedSample is a (time after warm-up, metric value) job-level observation.
type timedSample struct {
	t float64
	v float64
}

// levelPoint is a (time after warm-up, level) population observation.
type levelPoint struct {
	t     float64
	level int
}

func newStationStats() *stationStats { return &stationStats{} }

// recordCompletion folds one post-warmup completed job into the accumulators.
func (st *stationStats) recordCompletion(tAfterWarmup, wait, sojourn, service float64) {
	st.completed++
	st.serviceSum += service
	st.waitSamples = append(st.waitSamples, wait)
	st.sojournSamples = append(st.sojournSamples, sojourn)
	st.waitJobs = append(st.waitJobs, timedSample{tAfterWarmup, wait})
	st.sojournJobs = append(st.sojournJobs, timedSample{tAfterWarmup, sojourn})
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// quantile returns the empirical p-quantile (0 < p <= 1) via type-7 linear
// interpolation (R-7 / numpy "linear").
func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return sorted[0]
	}
	h := (float64(n)-1)*p + 1
	lo := int(math.Floor(h)) - 1
	if lo < 0 {
		lo = 0
	}
	if lo >= n {
		return sorted[n-1]
	}
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}
	frac := h - math.Floor(h)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// jobBatchMeans partitions job observations into batches of equal post-warmup
// time and returns one mean value per non-empty batch.
func jobBatchMeans(samples []timedSample, statsTime float64, batches int) []float64 {
	if batches < 1 || statsTime <= 0 {
		batches = 1
	}
	sums := make([]float64, batches)
	counts := make([]int64, batches)
	for _, s := range samples {
		idx := int(s.t / statsTime * float64(batches))
		if idx >= batches {
			idx = batches - 1
		}
		sums[idx] += s.v
		counts[idx]++
	}
	out := make([]float64, 0, batches)
	for i := range sums {
		if counts[i] > 0 {
			out = append(out, sums[i]/float64(counts[i]))
		}
	}
	return out
}

// levelAreas returns per-batch areas of a piecewise-constant level over equal
// post-warmup time intervals, followed by the total area and total time.
func levelAreas(points []levelPoint, statsTime float64, batches int) (areas []float64, durations []float64, totalArea float64) {
	if batches < 1 || statsTime <= 0 {
		batches = 1
	}
	areas = make([]float64, batches)
	durations = make([]float64, batches)
	// Insert a trailing point at statsTime so the final interval is closed.
	prevT := 0.0
	prevLevel := 0
	for _, p := range points {
		levelAddSegment(areas, durations, prevT, p.t, float64(prevLevel), statsTime, batches)
		totalArea += (p.t - prevT) * float64(prevLevel)
		prevT, prevLevel = p.t, p.level
	}
	levelAddSegment(areas, durations, prevT, statsTime, float64(prevLevel), statsTime, batches)
	totalArea += (statsTime - prevT) * float64(prevLevel)
	return
}

// levelAddSegment adds the area of a constant level over [from, to) to the
// batches straddled by the interval.
func levelAddSegment(areas, durations []float64, from, to, level, statsTime float64, batches int) {
	if to <= from {
		return
	}
	width := statsTime / float64(batches)
	lo := int(from / width)
	hi := int((to - 1e-15) / width)
	if lo >= batches {
		return
	}
	if hi >= batches {
		hi = batches - 1
	}
	for b := lo; b <= hi; b++ {
		segLo := maxF(float64(b)*width, from)
		segHi := minF(float64(b+1)*width, to)
		dt := segHi - segLo
		areas[b] += dt * level
		durations[b] += dt
	}
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxF(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func ciFromMeans(means []float64, point float64) ConfidenceInterval {
	ci := ConfidenceInterval{Mean: point, Confidence: 0.95}
	k := len(means)
	if k < 2 {
		return ci
	}
	m := mean(means)
	ss := 0.0
	for _, x := range means {
		ss += (x - m) * (x - m)
	}
	se := math.Sqrt((ss / float64(k-1)) / float64(k))
	ci.HalfWidth = tCritical95(k-1) * se
	return ci
}

// tCritical95 returns the two-sided 95% Student-t critical value for df
// degrees of freedom, using a compact table and a normal approximation beyond
// 30 degrees of freedom.
func tCritical95(df int) float64 {
	table := []float64{
		12.706, 4.303, 3.182, 2.776, 2.571,
		2.447, 2.365, 2.306, 2.262, 2.228,
		2.201, 2.179, 2.160, 2.145, 2.131,
		2.120, 2.110, 2.101, 2.093, 2.086,
		2.080, 2.074, 2.069, 2.064, 2.060,
		2.056, 2.052, 2.048, 2.045, 2.042,
	}
	if df <= 0 {
		return 0
	}
	if df <= len(table) {
		return table[df-1]
	}
	return 1.96
}

// report renders the accumulated station statistics into the public report.
func (st *stationStats) report(name string, servers int, statsTime float64, batches int) StationReport {
	r := StationReport{
		Name:      name,
		Completed: st.completed,
		Rejected:  st.rejected,
	}
	if statsTime > 0 {
		r.Throughput = float64(st.completed) / statsTime
		r.Utilization = st.serviceSum / (float64(servers) * statsTime)
	}

	waits := append([]float64(nil), st.waitSamples...)
	sort.Float64s(waits)
	sojs := append([]float64(nil), st.sojournSamples...)
	sort.Float64s(sojs)

	r.WaitSamples = waits
	r.SojournSamples = sojs
	r.MeanWait = mean(waits)
	r.MeanSojourn = mean(sojs)
	r.WaitQuantiles = map[float64]float64{
		0.5:  quantile(waits, 0.5),
		0.9:  quantile(waits, 0.9),
		0.95: quantile(waits, 0.95),
		0.99: quantile(waits, 0.99),
	}
	r.SojournQuantiles = map[float64]float64{
		0.5:  quantile(sojs, 0.5),
		0.9:  quantile(sojs, 0.9),
		0.95: quantile(sojs, 0.95),
		0.99: quantile(sojs, 0.99),
	}

	waitBatchMeans := jobBatchMeans(st.waitJobs, statsTime, batches)
	r.WaitCI = ciFromMeans(waitBatchMeans, r.MeanWait)
	sojBatchMeans := jobBatchMeans(st.sojournJobs, statsTime, batches)
	r.SojournCI = ciFromMeans(sojBatchMeans, r.MeanSojourn)
	areas, durations, totalQLArea := levelAreas(st.qlLevel, statsTime, batches)
	_, _, totalISArea := levelAreas(st.isLevel, statsTime, batches)
	qlMeans := make([]float64, 0, len(areas))
	for i := range areas {
		if durations[i] > 0 {
			qlMeans = append(qlMeans, areas[i]/durations[i])
		}
	}
	r.QueueLengthCI = ciFromMeans(qlMeans, totalQLArea/maxF(statsTime, 1e-300))

	if st.arrivalQLCounts > 0 {
		r.MeanQueueLength = st.arrivalQLSum / float64(st.arrivalQLCounts)
	}
	if statsTime > 0 {
		r.TimeAvgQueueLength = totalQLArea / statsTime
		r.TimeAvgInSystem = totalISArea / statsTime
	}
	return r
}

// statsState is the serializable form of a stationStats accumulator.
type statsState struct {
	Completed       int64
	Rejected        int64
	ServiceSum      float64
	WaitSamples     []float64
	SojournSamples  []float64
	WaitJobs        []timedSampleState
	SojournJobs     []timedSampleState
	ArrivalQLSum    float64
	ArrivalQLCounts int64
	QLLevel         []levelPointState
	ISLevel         []levelPointState
}

// timedSampleState / levelPointState are exported as value-only structs so the
// JSON encoding is stable.
type timedSampleState struct {
	T float64
	V float64
}

type levelPointState struct {
	T     float64
	Level int
}

func (st *stationStats) export() statsState {
	return statsState{
		Completed:       st.completed,
		Rejected:        st.rejected,
		ServiceSum:      st.serviceSum,
		WaitSamples:     append([]float64(nil), st.waitSamples...),
		SojournSamples:  append([]float64(nil), st.sojournSamples...),
		WaitJobs:        timedSamplesToState(st.waitJobs),
		SojournJobs:     timedSamplesToState(st.sojournJobs),
		ArrivalQLSum:    st.arrivalQLSum,
		ArrivalQLCounts: st.arrivalQLCounts,
		QLLevel:         levelsToState(st.qlLevel),
		ISLevel:         levelsToState(st.isLevel),
	}
}

func importStationStats(x statsState) *stationStats {
	return &stationStats{
		completed:       x.Completed,
		rejected:        x.Rejected,
		serviceSum:      x.ServiceSum,
		waitSamples:     append([]float64(nil), x.WaitSamples...),
		sojournSamples:  append([]float64(nil), x.SojournSamples...),
		waitJobs:        timedSamplesFromState(x.WaitJobs),
		sojournJobs:     timedSamplesFromState(x.SojournJobs),
		arrivalQLSum:    x.ArrivalQLSum,
		arrivalQLCounts: x.ArrivalQLCounts,
		qlLevel:         levelsFromState(x.QLLevel),
		isLevel:         levelsFromState(x.ISLevel),
	}
}

func timedSamplesToState(xs []timedSample) []timedSampleState {
	out := make([]timedSampleState, len(xs))
	for i, x := range xs {
		out[i] = timedSampleState{T: x.t, V: x.v}
	}
	return out
}

func timedSamplesFromState(xs []timedSampleState) []timedSample {
	out := make([]timedSample, len(xs))
	for i, x := range xs {
		out[i] = timedSample{t: x.T, v: x.V}
	}
	return out
}

func levelsToState(xs []levelPoint) []levelPointState {
	out := make([]levelPointState, len(xs))
	for i, x := range xs {
		out[i] = levelPointState{T: x.t, Level: x.level}
	}
	return out
}

func levelsFromState(xs []levelPointState) []levelPoint {
	out := make([]levelPoint, len(xs))
	for i, x := range xs {
		out[i] = levelPoint{t: x.T, level: x.Level}
	}
	return out
}
