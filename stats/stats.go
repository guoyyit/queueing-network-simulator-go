package stats

import (
	"math"
	"sort"
)

// CI 是稳态均值的置信区间（均值 ± HalfWidth）。
type CI struct {
	Mean      float64
	HalfWidth float64
	Level     float64
}

// Accumulator 收集独立观测样本（排队时长、逗留时长等）（骨架）。
type Accumulator struct {
	samples []float64
	sum     float64
}

func NewAccumulator() *Accumulator { return &Accumulator{} }

// Observe 记录一个样本。
func (a *Accumulator) Observe(v float64) {
	a.samples = append(a.samples, v)
	a.sum += v
}

// Mean 返回样本均值。
func (a *Accumulator) Mean() float64 {
	if len(a.samples) == 0 {
		return 0
	}
	return a.sum / float64(len(a.samples))
}

// Quantile 返回样本的 q 分位数（0 <= q <= 1）。
// 采用 nearest-rank（取秩上取整）定义：对排好序的样本取
// index = ceil(q*n) - 1。该定义确定且无插值，便于复现。
func (a *Accumulator) Quantile(q float64) float64 {
	n := len(a.samples)
	if n == 0 {
		return 0
	}
	sorted := append([]float64(nil), a.samples...)
	sort.Float64s(sorted)
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[n-1]
	}
	rank := int(math.Ceil(q*float64(n))) - 1
	if rank < 0 {
		rank = 0
	}
	return sorted[rank]
}

// Count 返回样本数。
func (a *Accumulator) Count() int { return len(a.samples) }

// Reset 清空累加器。
func (a *Accumulator) Reset() {
	a.samples = nil
	a.sum = 0
}

// Samples 返回样本副本。
func (a *Accumulator) Samples() []float64 {
	return append([]float64(nil), a.samples...)
}

// Values 返回全部样本值（与 Samples 等价，语义化别名）。
func (a *Accumulator) Values() []float64 {
	return append([]float64(nil), a.samples...)
}

// RestoreValues 用样本值重建累加器（检查点恢复用）。
func (a *Accumulator) RestoreValues(values []float64) {
	a.Reset()
	for _, v := range values {
		a.Observe(v)
	}
}

// BatchMeans 将样本等分为 b 批并返回各批均值。
// 样本数不能被 b 整除时，前面 (n%b) 批各多分配一个样本，
// 保证所有样本恰好被使用一次。
func BatchMeans(values []float64, b int) []float64 {
	if b <= 0 || len(values) == 0 {
		return nil
	}
	means := make([]float64, b)
	n := len(values)
	base := n / b
	extra := n % b
	idx := 0
	for k := 0; k < b; k++ {
		size := base
		if k < extra {
			size++
		}
		sum := 0.0
		for j := 0; j < size; j++ {
			sum += values[idx]
			idx++
		}
		if size > 0 {
			means[k] = sum / float64(size)
		}
	}
	return means
}

// MeanCI 根据批均值给出 t 分布置信区间。
func MeanCI(batchMeans []float64, level float64) CI {
	n := len(batchMeans)
	if n == 0 {
		return CI{Level: level}
	}
	mean := 0.0
	for _, v := range batchMeans {
		mean += v
	}
	mean /= float64(n)
	if n == 1 {
		return CI{Mean: mean, Level: level}
	}
	var ss float64
	for _, v := range batchMeans {
		d := v - mean
		ss += d * d
	}
	se := math.Sqrt(ss/float64(n-1)) / math.Sqrt(float64(n))
	alpha := 1 - level
	tcrit := tCritical(1-alpha/2, float64(n-1))
	return CI{Mean: mean, HalfWidth: tcrit * se, Level: level}
}

// tCritical 返回自由度 df 的 Student-t 分布 p 分位数。
// df=1、2 使用解析公式，df>=3 使用 Hill (1970) Cornish-Fisher
// 展开（取至 1/df^3 项），常规仿真精度足够。
func tCritical(p, df float64) float64 {
	z := normalQuantile(p)
	sign := 1.0
	if p < 0.5 {
		sign = -1
		p = 1 - p
	}
	alpha := 1 - p // 单侧尾概率（p >= 0.5）
	switch {
	case df == 1:
		return sign * math.Tan(math.Pi*(0.5-alpha))
	case df == 2:
		q := 0.5 - alpha
		t2 := 8 * q * q / (1 - 4*q*q)
		return sign * math.Sqrt(t2)
	default:
		z2 := z * z
		z3 := z2 * z
		z5 := z3 * z2
		z7 := z5 * z2
		t := z + (z3+z)/(4*df)
		t += (5*z5 + 16*z3 + 3*z) / (96 * df * df)
		t += (3*z7 + 19*z5 + 17*z3 - 15*z) / (384 * df * df * df)
		return sign * t
	}
}

// normalQuantile 返回标准正态分布的 p 分位数（Acklam 近似）。
func normalQuantile(p float64) float64 {
	if p <= 0 {
		return math.Inf(-1)
	}
	if p >= 1 {
		return math.Inf(1)
	}
	// Beasley-Springer-Moro 风格有理逼近
	const a1 = -39.69683028665376
	const a2 = 220.9460984245205
	const a3 = -275.9285104469687
	const a4 = 138.3577518672690
	const a5 = -30.66479806614716
	const a6 = 2.506628277459239
	const b1 = -54.47609879822406
	const b2 = 161.5858368580409
	const b3 = -155.6989798598866
	const b4 = 66.80131188771972
	const b5 = -13.28068155288572
	const c1 = -0.007784894002430293
	const c2 = -0.3223964580411365
	const c3 = -2.400758277161838
	const c4 = -2.549732539343734
	const c5 = 4.374664141464968
	const c6 = 2.938163982698783
	const d1 = 0.007784695709041462
	const d2 = 0.3224671290700398
	const d3 = 2.445134137142996
	const d4 = 3.754408661907416

	plow, phigh := 0.02425, 0.97575
	switch {
	case p < plow:
		q := math.Sqrt(-2 * math.Log(p))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	case p <= phigh:
		q := p - 0.5
		r := q * q
		return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
	default:
		q := math.Sqrt(-2 * math.Log(1-p))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	}
}
