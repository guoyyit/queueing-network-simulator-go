package qsim

import (
	"testing"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
)

// TestPreemptionRemaining 验证：高优先级抢占低优先级时，被抢占任务
// 保留剩余服务时间并在之后完成；总服务工作量守恒。
func TestPreemptionRemaining(t *testing.T) {
	cfg := &Config{
		Blocking: Reject, Seed: 5, EndTime: 30, Warmup: 0,
		Network: &NetworkConfig{
			Model:          Open,
			ExternalArrive: []float64{0},
			Routing:        [][]float64{{0}},
			Stations: []StationConfig{{
				Name: "p", Servers: 1, Capacity: -1,
				Policy: PriorityPP, Service: dist.Deterministic{Value: 10},
			}},
		},
	}
	s := New(cfg)

	// 时刻 0 注入低优先级任务 A（服务 10）。
	a := &Customer{ID: 1, Priority: 5, ArrivalTime: 0, BirthTime: 0}
	s.nextID = 1
	s.processArrival(a, 0)

	// 时刻 3 注入高优先级任务 B（抢占）。
	s.now = 3
	b := &Customer{ID: 2, Priority: 0, ArrivalTime: 3, BirthTime: 3}
	s.nextID = 2
	s.processArrival(b, 0)

	st := s.st[0]
	if st.busy[0].Customer != b {
		t.Fatal("high priority B should now occupy server")
	}
	if len(st.waiting) != 1 || st.waiting[0] != a {
		t.Fatal("preempted A should return to waiting queue")
	}
	// A 的剩余服务时间应为 10-3=7。
	if a.Remaining < 6.999 || a.Remaining > 7.001 {
		t.Fatalf("A remaining = %v, want 7", a.Remaining)
	}

	// B 在 [3,13) 服务；13 完成后 A 恢复服务 7，到 20 完成。
	for s.Step() {
	}
	if st.complete != 2 {
		t.Fatalf("completions = %d, want 2", st.complete)
	}
}

// TestRejectCounted 验证容量满时拒绝被独立计数且不静默丢失。
func TestRejectCounted(t *testing.T) {
	cfg := &Config{
		Blocking: Reject, Seed: 1, EndTime: 100, Warmup: 0,
		Network: &NetworkConfig{
			Model:          Open,
			ExternalArrive: []float64{50}, // 到达远快于服务
			Routing:        [][]float64{{0}},
			Stations: []StationConfig{{
				Name: "finite", Servers: 1, Capacity: 2,
				Policy: FCFS, Service: dist.Deterministic{Value: 5},
			}},
		},
	}
	s := New(cfg)
	s.Run(0)
	r := s.Report()
	if r.Rejected == 0 {
		t.Fatal("expected rejections under overload with finite capacity")
	}
	if r.Stations[0].Rejected != r.Rejected {
		t.Fatalf("station rejects %d != network rejects %d", r.Stations[0].Rejected, r.Rejected)
	}
	// 守恒：created = completed + rejected + inSystem
	inSystem := int64(0)
	for _, st := range s.st {
		inSystem += int64(st.Population())
	}
	if s.created != s.completed+s.rejected+inSystem {
		t.Fatalf("mass not conserved: %d != %d+%d+%d",
			s.created, s.completed, s.rejected, inSystem)
	}
}

// TestBlockingSender 验证 Block 语义：下游满时发送方阻塞、不拒绝；
// 下游有空位后任务转入，系统无丢失。
func TestBlockingSender(t *testing.T) {
	cfg := &Config{
		Blocking: Block, Seed: 3, EndTime: 80, Warmup: 0,
		Network: &NetworkConfig{
			Model:          Open,
			ExternalArrive: []float64{0.8, 0},
			Routing: [][]float64{
				{0, 1}, // station0 完成后必去 station1
				{0, 0}, // station1 完成后离开
			},
			Stations: []StationConfig{
				{Name: "src", Servers: 1, Capacity: -1, Policy: FCFS,
					Service: dist.Deterministic{Value: 1}},
				{Name: "sink", Servers: 1, Capacity: 1, Policy: FCFS,
					Service: dist.Deterministic{Value: 4}},
			},
		},
	}
	s := New(cfg)
	s.Run(0)
	r := s.Report()
	if r.Rejected != 0 {
		t.Fatalf("blocking mode must not reject, got %d", r.Rejected)
	}
	// 守恒
	inSystem := int64(0)
	for _, st := range s.st {
		inSystem += int64(st.Population())
	}
	if s.created != s.completed+s.rejected+inSystem {
		t.Fatalf("mass not conserved under blocking: %d != %d+%d+%d",
			s.created, s.completed, s.rejected, inSystem)
	}
}

// TestClosedModel 验证封闭模型：任务总数恒为 Population，无外部到达、无拒收。
func TestClosedModel(t *testing.T) {
	cfg := &Config{
		Blocking: Reject, Seed: 9, EndTime: 200, Warmup: 20,
		Network: &NetworkConfig{
			Model: Closed, Population: 5,
			Routing: [][]float64{
				{0.2, 0.8},
				{0.5, 0.5},
			},
			Stations: []StationConfig{
				{Name: "a", Servers: 2, Capacity: -1, Policy: LCFS,
					Service: dist.Exponential{Rate: 3}},
				{Name: "b", Servers: 2, Capacity: -1, Policy: FCFS,
					Service: dist.Exponential{Rate: 2}},
			},
		},
	}
	s := New(cfg)
	for s.Step() {
		total := 0
		for _, st := range s.st {
			total += st.Population()
		}
		if total != 5 {
			t.Fatalf("closed population = %d, want 5", total)
		}
	}
	if s.Rejected() != 0 {
		t.Fatalf("closed model rejected %d", s.Rejected())
	}
}
