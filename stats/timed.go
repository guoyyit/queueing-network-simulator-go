package stats

import "sort"

// TimedSample 是带观测时刻的样本。
type TimedSample struct {
	Time  float64
	Value float64
}

// TimedAccumulator 记录带时间戳的样本，支持整体均值/分位数，
// 以及按时间区间（批）归集的批均值。
type TimedAccumulator struct {
	samples []TimedSample
	sum     float64
}

func NewTimedAccumulator() *TimedAccumulator { return &TimedAccumulator{} }

// Observe 在时刻 t 记录样本 v。
func (a *TimedAccumulator) Observe(t, v float64) {
	a.samples = append(a.samples, TimedSample{Time: t, Value: v})
	a.sum += v
}

// Count 返回样本数。
func (a *TimedAccumulator) Count() int { return len(a.samples) }

// Mean 返回全部样本均值。
func (a *TimedAccumulator) Mean() float64 {
	if len(a.samples) == 0 {
		return 0
	}
	return a.sum / float64(len(a.samples))
}

// Quantile 返回样本值的 q 分位数（nearest-rank 定义，与 Accumulator 一致）。
func (a *TimedAccumulator) Quantile(q float64) float64 {
	n := len(a.samples)
	if n == 0 {
		return 0
	}
	vals := make([]float64, n)
	for i, s := range a.samples {
		vals[i] = s.Value
	}
	sort.Float64s(vals)
	if q <= 0 {
		return vals[0]
	}
	if q >= 1 {
		return vals[n-1]
	}
	idx := int(float64(n)*q+0.999999999) - 1 // ceil(nq)-1，避免浮点误差
	if idx < 0 {
		idx = 0
	}
	return vals[idx]
}

// Samples 返回全部样本副本。
func (a *TimedAccumulator) Samples() []TimedSample {
	return append([]TimedSample(nil), a.samples...)
}

// BatchMeans 把 [from, to) 等分为 b 个时间批，按样本时刻归集，
// 返回每批的样本均值。样本按时间有序（仿真顺序产生）。
func (a *TimedAccumulator) BatchMeans(from, to float64, b int) []float64 {
	means := make([]float64, b)
	counts := make([]int, b)
	width := (to - from) / float64(b)
	for _, s := range a.samples {
		if s.Time < from || s.Time >= to {
			continue
		}
		idx := int((s.Time - from) / width)
		if idx >= b {
			idx = b - 1
		}
		means[idx] += s.Value
		counts[idx]++
	}
	for i := 0; i < b; i++ {
		if counts[i] > 0 {
			means[i] /= float64(counts[i])
		}
	}
	return means
}

// Reset 清空样本。
func (a *TimedAccumulator) Reset() {
	a.samples = nil
	a.sum = 0
}
