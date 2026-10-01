package stats

import "math"

// Segment 表示变量在 [Start, End) 内保持值 Value 的一段常数区间。
type Segment struct {
	Start float64
	End   float64
	Value float64
}

// TimeAverage 记录分段常数变量（如队长、占用服务台数）的完整变化史，
// 以时间加权方式计算时间平均值，并支持在任意时间区间上积分
// （用于预热期剔除与批均值法）。
type TimeAverage struct {
	segments  []Segment
	lastTime  float64
	lastValue float64
}

func NewTimeAverage() *TimeAverage { return &TimeAverage{} }

// Update 在时刻 now 将变量值设置为 v。
// 同刻重复调用以最后一次为准；同值调用不产生新区间。
func (t *TimeAverage) Update(now, v float64) {
	if now < t.lastTime {
		return
	}
	if now == t.lastTime {
		// 同一时刻多次变化（零时长的瞬时过渡态）：只更新“当前开放值”，
		// 不回改已在 [.., lastTime) 关闭的前一段，避免把上一状态段的
		// 历史值覆盖成中间值。
		t.lastValue = v
		return
	}
	t.segments = append(t.segments, Segment{Start: t.lastTime, End: now, Value: t.lastValue})
	t.lastTime = now
	t.lastValue = v
}

// Close 以时刻 now 封口最后一段（值保持到 now）。
func (t *TimeAverage) Close(now float64) {
	if now <= t.lastTime {
		return
	}
	t.segments = append(t.segments, Segment{Start: t.lastTime, End: now, Value: t.lastValue})
	t.lastTime = now
}

// Mean 返回 [0, now] 上的时间平均值。
func (t *TimeAverage) Mean() float64 {
	return t.IntervalMean(0, t.lastTime)
}

// IntervalMean 返回 [from, to] 上的时间加权平均值；空区间返回 0。
func (t *TimeAverage) IntervalMean(from, to float64) float64 {
	if to <= from {
		return 0
	}
	area, width := 0.0, 0.0
	for _, seg := range t.segments {
		a := math.Max(from, seg.Start)
		b := math.Min(to, seg.End)
		if b > a {
			area += seg.Value * (b - a)
			width += b - a
		}
	}
	// 封口后末段之后（或未封口时 lastTime 之前的当前值段）。
	if width < to-from-1e-12 && t.lastTime < to {
		a := math.Max(from, t.lastTime)
		b := to
		if b > a {
			area += t.lastValue * (b - a)
			width += b - a
		}
	}
	if width == 0 {
		return 0
	}
	return area / width
}

// Current 返回最近一次设置的变量值。
func (t *TimeAverage) Current() float64 { return t.lastValue }

// Segments 返回内部段历史的副本（供检查点序列化）。
func (t *TimeAverage) Segments() []Segment {
	return append([]Segment(nil), t.segments...)
}

// State 是 TimeAverage 的完整可序列化状态（含尚未封口的当前值）。
type State struct {
	Segments  []Segment
	LastTime  float64
	LastValue float64
}

// State 导出完整状态（供检查点保存）。
func (t *TimeAverage) State() State {
	return State{Segments: t.Segments(), LastTime: t.lastTime, LastValue: t.lastValue}
}

// RestoreState 用完整状态重建时间平均器（供检查点恢复）。
func (t *TimeAverage) RestoreState(state State) {
	t.segments = append([]Segment(nil), state.Segments...)
	t.lastTime = state.LastTime
	t.lastValue = state.LastValue
}

// Reset 清空累计状态。
func (t *TimeAverage) Reset(now float64) {
	t.segments = nil
	t.lastTime = now
	t.lastValue = 0
}
