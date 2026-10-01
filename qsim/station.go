package qsim

import (
	"github.com/guoyyit/queueing-network-simulator-go/dist"
	"github.com/guoyyit/queueing-network-simulator-go/stats"
)

// StationConfig 描述单个工作站的静态配置。
type StationConfig struct {
	Name     string
	Servers  int // 并联服务台数量
	Capacity int // 排队队列容量；-1 表示无限
	Policy   Policy
	Service  dist.Distribution
}

// Station 是工作站的运行时状态（骨架，后续填充）。
type Station struct {
	cfg         StationConfig
	index       int
	waiting     []*Customer   // 等待队列，顺序与调度策略相关
	busy        []*serverSlot // 每个服务台一个槽位；nil 表示空闲
	blocked     []*blockedJob // 服务完成但目标已满、占用原服务台的任务
	qlenTA      *stats.TimeAverage
	popTA       *stats.TimeAverage // 站内总人数（等待+服务+阻塞）
	busyTA      *stats.TimeAverage
	waitAcc     *stats.TimedAccumulator // 排队时长样本（入队到开始服务）
	sojAcc      *stats.TimedAccumulator // 逗留时长样本（入站到完成出站）
	deptQ       *stats.Accumulator      // 稳态内完成时刻点的等待队长样本
	rejects     int64                   // 被拒绝任务数（独立统计，不静默丢失）
	complete    int64                   // 本站完成服务总次数
	completions []float64               // 每次完成的时刻
}

// serverSlot 是单个服务台的运行时状态。
type serverSlot struct {
	Customer  *Customer
	Start     float64 // 本次服务（或抢占后续作）开始时刻
	Remaining float64 // 剩余服务时间
}

// blockedJob 记录“服务完成、等待进入下游”而被阻塞的任务。
type blockedJob struct {
	Customer  *Customer
	From      int         // 阻塞它的发送方工作站（即本站）
	To        int         // 目标工作站
	Server    int         // 被占用的发送方服务台编号
	BlockedAt float64     // 阻塞开始时刻
	Slot      *serverSlot // 非 nil 表示该发送方服务台被此阻塞任务占用
}

// newStation 按配置创建工作站，并在时刻 0 初始化时间序列。
func newStation(index int, cfg StationConfig) *Station {
	return &Station{
		cfg:     cfg,
		index:   index,
		waiting: nil,
		busy:    make([]*serverSlot, cfg.Servers),
		blocked: nil,
		qlenTA:  stats.NewTimeAverage(),
		popTA:   stats.NewTimeAverage(),
		busyTA:  stats.NewTimeAverage(),
		waitAcc: stats.NewTimedAccumulator(),
		sojAcc:  stats.NewTimedAccumulator(),
		deptQ:   stats.NewAccumulator(),
	}
}

// Name 返回工作站名称。
func (s *Station) Name() string { return s.cfg.Name }

// QueueLen 返回等待中的任务数（不含正在服务与被阻塞的任务）。
func (s *Station) QueueLen() int { return len(s.waiting) }

// InService 返回正在接受服务的任务数。
func (s *Station) InService() int {
	n := 0
	for _, slot := range s.busy {
		if slot != nil {
			n++
		}
	}
	return n
}

// BlockedCount 返回因下游满而占用本站服务台的阻塞任务数。
func (s *Station) BlockedCount() int { return len(s.blocked) }

// Population 返回站内总任务数 = 等待 + 服务中 + 阻塞中。
func (s *Station) Population() int {
	// busy 槽位与 blocked 互斥（阻塞任务挂在 blocked 上、槽位置空），
	// 故不重复计数。
	return len(s.waiting) + s.InService() + len(s.blocked)
}

// queueFull 判断等待队列是否已满。容量检查只计算等待中的任务。
func (s *Station) queueFull() bool {
	return s.cfg.Capacity >= 0 && len(s.waiting) >= s.cfg.Capacity
}

// enqueue 将任务放入等待队列的队尾。
func (s *Station) enqueue(c *Customer) {
	s.waiting = append(s.waiting, c)
}

// dequeue 按调度策略从等待队列选取一个任务并移除。
func (s *Station) dequeue() *Customer {
	n := len(s.waiting)
	if n == 0 {
		return nil
	}
	idx := s.pickIndex()
	c := s.waiting[idx]
	s.waiting = append(s.waiting[:idx], s.waiting[idx+1:]...)
	return c
}

// pickIndex 返回按策略应服务的等待任务下标。
// FCFS 取最早到达（队首）；LCFS 取队尾；优先级策略取优先级最高者，
// 同优先级按到达时间先后（仍取最早），再同按顾客 ID 兜底，确保确定。
func (s *Station) pickIndex() int {
	switch s.cfg.Policy {
	case FCFS:
		return 0
	case LCFS:
		return len(s.waiting) - 1
	case PriorityNP, PriorityPP:
		best := 0
		for i := 1; i < len(s.waiting); i++ {
			if customerBefore(s.waiting[i], s.waiting[best]) {
				best = i
			}
		}
		return best
	default:
		return 0
	}
}

// customerBefore 报告优先级排队下 a 是否应排在 b 之前。
func customerBefore(a, b *Customer) bool {
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	if a.ArrivalTime != b.ArrivalTime {
		return a.ArrivalTime < b.ArrivalTime
	}
	return a.ID < b.ID
}

// lowestPriorityInService 返回正在服务任务中优先级最低（数值最大）者的
// 服务台编号；用于优先级抢占到达时挑选被抢占者。同优先级选 ID 最大者。
func (s *Station) lowestPriorityInService() int {
	worst := -1
	for i, slot := range s.busy {
		if slot == nil {
			continue
		}
		if worst == -1 {
			worst = i
			continue
		}
		cur, best := slot.Customer, s.busy[worst].Customer
		if cur.Priority > best.Priority ||
			(cur.Priority == best.Priority && cur.ID > best.ID) {
			worst = i
		}
	}
	return worst
}

// occupiedServers 返回被占用的服务台数（含发送方阻塞占位）。
func (s *Station) occupiedServers() int {
	n := 0
	for _, slot := range s.busy {
		if slot != nil {
			n++
		}
	}
	for _, b := range s.blocked {
		if b.Slot != nil {
			n++
		}
	}
	return n
}

// serverBlocked 返回占用指定服务台的阻塞任务（无则 nil）。
func (s *Station) serverBlocked(server int) *blockedJob {
	for _, b := range s.blocked {
		if b.Server == server && b.Slot != nil {
			return b
		}
	}
	return nil
}

// freeServer 返回真正空闲（无服务、无阻塞占位）的首个服务台编号。
func (s *Station) freeServerUnblocked() int {
	for i, slot := range s.busy {
		if slot == nil && s.serverBlocked(i) == nil {
			return i
		}
	}
	return -1
}

// completeAfter 返回时刻 t（含）之后的完成次数，用于稳态吞吐。
func (s *Station) completeAfter(t float64) int {
	n := 0
	for _, ct := range s.completions {
		if ct >= t {
			n++
		}
	}
	return n
}
