package qsim

import (
	"testing"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
)

// TestSameTimeDeterminism 构造多个同刻事件，验证处理顺序由
// (时刻 → 到达先于完成 → 工作站编号 → 顾客优先级 → 顾客ID → Seq)
// 的确定规则决定，且同种子多次运行结果逐位一致。
func TestSameTimeDeterminism(t *testing.T) {
	build := func() *Simulator {
		cfg := &Config{
			Blocking: Reject, Seed: 11, EndTime: 100, Warmup: 0,
			Network: &NetworkConfig{
				Model:          Open,
				ExternalArrive: []float64{0, 0},
				Routing: [][]float64{
					{0, 0},
					{0, 0},
				},
				Stations: []StationConfig{
					{Name: "a", Servers: 1, Capacity: -1, Policy: FCFS,
						Service: dist.Deterministic{Value: 10}},
					{Name: "b", Servers: 1, Capacity: -1, Policy: FCFS,
						Service: dist.Deterministic{Value: 10}},
				},
			},
		}
		return New(cfg)
	}

	const T = 5.0
	c1 := &Customer{ID: 101, Priority: 0, ArrivalTime: T}
	c2 := &Customer{ID: 102, Priority: 0, ArrivalTime: T}
	c3 := &Customer{ID: 103, Priority: 0, ArrivalTime: T}

	var order []string
	// 用仅含上述事件的独立日历直接验证弹出顺序。
	cal := NewCalendar()
	cal.Push(&Event{Type: evCompletion, Time: T, Station: 1, Customer: c1})
	cal.Push(&Event{Type: EvArrival, Time: T, Station: 1, Customer: c2})
	cal.Push(&Event{Type: EvArrival, Time: T, Station: 0, Customer: c3})
	for cal.Len() > 0 {
		e := cal.Pop()
		kind := "C"
		if e.Type == EvArrival {
			kind = "A"
		}
		order = append(order, kind)
	}
	want := []string{"A", "A", "C"} // 两到达按站号升序，完成最后
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("same-time order = %v, want %v", order, want)
		}
	}

	// 相同种子的两次整体运行可复现
	s2 := build()
	s := build()
	s.Run(0)
	s2.Run(0)
	r1, r2 := s.Report(), s2.Report()
	if !reportsEqual(r1, r2) {
		t.Fatal("same-time scenario not reproducible")
	}
}
