package qsim

import (
	"fmt"
	"math"

	"github.com/guoyyit/queueing-network-simulator-go/rng"
	"github.com/guoyyit/queueing-network-simulator-go/stats"
)

// BlockingMode 定义目标队列已满时的处理语义。
type BlockingMode int

const (
	// Reject 拒绝到达：任务被拒绝并计入独立统计。
	Reject BlockingMode = iota
	// Block 发送方阻塞：服务完成的任务留在服务台，直到目标出现空位。
	Block
)

// Config 是一次仿真运行的配置。
type Config struct {
	Network    *NetworkConfig
	Blocking   BlockingMode
	Seed       uint64
	Warmup     float64 // 预热期长度（[0, Warmup) 的样本不进入稳态统计）
	EndTime    float64 // 仿真终止时刻
	BatchCount int     // 批均值法的批数（预热后区间 [Warmup, EndTime) 等分）
	Confidence float64 // 置信水平，如 0.95
}

// Simulator 是离散事件仿真器。
type Simulator struct {
	cfg    *Config
	net    *NetworkConfig
	rng    *rng.Stream   // 保留主种子流用于路由决策
	arrive *rng.Stream   // 外部到达间隔子流
	svc    []*rng.Stream // 各工作站服务时间子流
	cal    *Calendar
	st     []*Station
	now    float64
	nextID uint64

	created   int64
	rejected  int64
	completed int64
}

// New 根据配置构造仿真器并完成初始调度。
func New(cfg *Config) *Simulator {
	if cfg.Network == nil || len(cfg.Network.Stations) == 0 {
		panic("qsim: config requires at least one station")
	}
	net := cfg.Network
	s := &Simulator{
		cfg:    cfg,
		net:    net,
		rng:    rng.New(rng.SubSeed(cfg.Seed, 2)),
		arrive: rng.New(rng.SubSeed(cfg.Seed, rng.KindArrival)),
		cal:    NewCalendar(),
	}
	s.svc = make([]*rng.Stream, len(net.Stations))
	for i := range net.Stations {
		s.svc[i] = rng.New(rng.SubSeed(cfg.Seed, rng.Kind(rng.KindServiceBase)+rng.Kind(i)))
	}
	s.st = make([]*Station, len(net.Stations))
	for i, sc := range net.Stations {
		if sc.Servers <= 0 {
			sc.Servers = 1
		}
		if sc.Name == "" {
			sc.Name = fmt.Sprintf("station%d", i)
		}
		station := newStation(i, sc)
		station.qlenTA.Update(0, 0)
		station.popTA.Update(0, 0)
		station.busyTA.Update(0, 0)
		s.st[i] = station
	}

	switch net.Model {
	case Closed:
		if net.Population <= 0 {
			panic("qsim: closed model requires positive Population")
		}
		s.initClosed()
	default:
		s.scheduleExternalArrivals(0)
	}
	return s
}

// initClosed 在时刻 0 于来源工作站（默认工作站 0）注入全部常驻任务。
// 任务按编号顺序创建、依次走到达流程；同刻顺序由事件规则与顾客 ID 确定。
func (s *Simulator) initClosed() {
	for i := 0; i < s.net.Population; i++ {
		c := s.newCustomer(0, 0)
		s.created++
		s.processArrival(c, 0)
	}
}

// scheduleExternalArrivals 为开放模型从时刻 from 起调度各工作站的外部到达。
// 外部到达按独立泊松过程（指数间隔，速率 ExternalArrive[i]）建模。
func (s *Simulator) scheduleExternalArrivals(from float64) {
	for i, rate := range s.net.ExternalArrive {
		if rate <= 0 {
			continue
		}
		t := from + s.exponential(rate)
		s.cal.Push(&Event{Type: EvArrival, Time: t, Station: i})
	}
}

func (s *Simulator) exponential(rate float64) float64 {
	u := 1.0 - s.arrive.Float64()
	return -math.Log(u) / rate
}

// newCustomer 创建一个在当前时刻到达工作站 station 的任务。
func (s *Simulator) newCustomer(priority, station int) *Customer {
	s.nextID++
	return &Customer{
		ID:          s.nextID,
		Priority:    priority,
		BirthTime:   s.now,
		ArrivalTime: s.now,
		Station:     station,
	}
}

// Now 返回当前仿真时刻。
func (s *Simulator) Now() float64 { return s.now }

// syncTimeSeries 在工作站状态变化后同步三类时间持久型变量。
func (s *Simulator) syncTimeSeries(st *Station) {
	st.qlenTA.Update(s.now, float64(st.QueueLen()))
	st.popTA.Update(s.now, float64(st.Population()))
	st.busyTA.Update(s.now, float64(st.occupiedServers()))
}

// Step 处理下一个时刻严格早于 EndTime 的事件；返回 false 表示无事件可处理。
// 外部到达在事件被取出且 time < EndTime 时才创建任务与续调下一到达，
// 因此不消耗多余随机数（保证检查点续跑与连续运行逐位一致）。
func (s *Simulator) Step() bool {
	e := s.cal.Peek()
	if e == nil || e.Time >= s.cfg.EndTime {
		return false
	}
	s.cal.Pop()
	s.now = e.Time

	if e.Type == EvArrival && e.Customer == nil {
		// 外部到达：先续调同工作站下一到达，再创建并处理本任务。
		rate := 0.0
		if e.Station < len(s.net.ExternalArrive) {
			rate = s.net.ExternalArrive[e.Station]
		}
		if rate > 0 {
			nextTime := e.Time + s.exponential(rate)
			s.cal.Push(&Event{Type: EvArrival, Time: nextTime, Station: e.Station})
		}
		c := s.newCustomer(0, e.Station)
		s.created++
		e.Customer = c
		s.processArrival(c, e.Station)
		return true
	}

	s.processCompletion(e.Station, e.Server, e.Customer)
	return true
}

// Run 处理到 EndTime 前的全部事件，然后封口时间序列。
func (s *Simulator) Run(endTime float64) {
	if endTime > 0 {
		s.cfg.EndTime = endTime
	}
	for s.Step() {
	}
	for _, st := range s.st {
		st.qlenTA.Close(s.cfg.EndTime)
		st.popTA.Close(s.cfg.EndTime)
		st.busyTA.Close(s.cfg.EndTime)
	}
	s.now = s.cfg.EndTime
}

// StepUntil 反复处理严格早于 t 的事件（用于在运行中某时刻保存检查点）。
// 不封口时间序列、不改变 EndTime。
func (s *Simulator) StepUntil(t float64) {
	for {
		e := s.cal.Peek()
		if e == nil || e.Time >= t {
			break
		}
		s.Step()
	}
	s.now = t
}

// processArrival 处理任务到达工作站 i（外部注入、封闭注入或路由转入）。
func (s *Simulator) processArrival(c *Customer, i int) {
	st := s.st[i]
	c.Station = i
	c.ArrivalTime = s.now

	if st.queueFull() {
		// 队列满：Reject 拒收并独立计数；Block 语义只对“有发送方”的
		// 路由到达有效（在 processCompletion 中处理），外部/初始到达一律拒收。
		s.reject(c, i)
		return
	}

	// 优先级抢占：无空闲服务台但到达者严格高于在服最低优先级者时抢占。
	if st.cfg.Policy == PriorityPP && st.freeServerUnblocked() < 0 {
		if victim := st.lowestPriorityInService(); victim >= 0 {
			v := st.busy[victim].Customer
			if c.Priority < v.Priority {
				s.preempt(st, victim, c)
				return
			}
		}
	}

	if server := st.freeServerUnblocked(); server >= 0 {
		s.beginService(st, server, c)
		return
	}

	st.enqueue(c)
	s.syncTimeSeries(st)
}

// beginService 让任务 c 在 server 号服务台开始/恢复服务并调度完成事件。
func (s *Simulator) beginService(st *Station, server int, c *Customer) {
	if s.now >= s.cfg.Warmup && !c.Preempted {
		st.waitAcc.Observe(s.now, s.now-c.ArrivalTime)
	}
	dur := c.Remaining
	if c.Preempted || dur > 0 {
		c.Remaining = 0
	} else {
		dur = st.cfg.Service.Sample(s.svc[st.index])
	}
	c.Preempted = false
	st.busy[server] = &serverSlot{Customer: c, Start: s.now, Remaining: dur}
	s.syncTimeSeries(st)
	s.cal.Push(&Event{
		Type:     evCompletion,
		Time:     s.now + dur,
		Station:  st.index,
		Server:   server,
		Customer: c,
	})
}

// preempt 执行优先级抢占：victim 服务台的在服任务被 c 立即取代。
// 被抢占任务保留剩余服务时间 Remaining（原完成时刻 - 当前时刻），
// 回到等待队列头部；恢复时继续使用该剩余时间，不重新采样，
// 且恢复时刻不再重复记录排队时长。
func (s *Simulator) preempt(st *Station, victim int, c *Customer) {
	slot := st.busy[victim]
	var oldEnd float64
	s.cal.findAndRemove(func(e *Event) bool {
		if e.Type == evCompletion && e.Station == st.index && e.Server == victim {
			oldEnd = e.Time
			return true
		}
		return false
	})
	preempted := slot.Customer
	preempted.Remaining = oldEnd - s.now
	preempted.Preempted = true

	// 被抢占者置于等待队列最前；调度选取优先级策略时，高优先级新到达者
	// 已直接进入服务，被抢占者恢复仍优先于普通等待者。
	st.waiting = append([]*Customer{preempted}, st.waiting...)
	s.syncTimeSeries(st)

	s.beginService(st, victim, c)
}

// reject 将任务计为被拒绝（网络级与工作站级分别计数，不静默丢失）。
func (s *Simulator) reject(c *Customer, i int) {
	s.rejected++
	s.st[i].rejects++
}

// processCompletion 处理工作站 i、服务台 server 上任务 c 的服务完成。
func (s *Simulator) processCompletion(i, server int, c *Customer) {
	st := s.st[i]
	st.busy[server] = nil
	st.complete++
	st.completions = append(st.completions, s.now)
	s.syncTimeSeries(st)
	if s.now >= s.cfg.Warmup {
		st.sojAcc.Observe(s.now, s.now-c.ArrivalTime)
		st.deptQ.Observe(float64(st.QueueLen()))
	}

	next, leave := s.route(i)
	if leave {
		s.completed++
		s.afterServerFreed(st)
		return
	}

	target := s.st[next]
	if s.cfg.Blocking == Block && target.queueFull() {
		// 发送方阻塞：任务连同服务台槽位一起挂入 blocked（busy 槽位置空，
		// 占用在 occupiedServers 中由 blocked 唯一计数），直到目标有空位。
		st.blocked = append(st.blocked, &blockedJob{
			Customer: c, From: i, To: next, Server: server, BlockedAt: s.now,
			Slot: &serverSlot{Customer: c, Start: s.now},
		})
		s.syncTimeSeries(st)
		return
	}

	if target.queueFull() {
		s.reject(c, next)
	} else {
		s.processArrival(c, next)
	}
	s.afterServerFreed(st)
}

// afterServerFreed 在服务台空出后：先解除上游指向本站的阻塞，再启动等待任务。
func (s *Simulator) afterServerFreed(st *Station) {
	s.tryUnblockInto(st.index)
	if server := st.freeServerUnblocked(); server >= 0 {
		if c := st.dequeue(); c != nil {
			s.syncTimeSeries(st)
			s.beginService(st, server, c)
		}
	}
}

// tryUnblockInto 在工作站 i 出现空位时，解除因 i 满而阻塞的上游任务。
// 解除顺序确定：按上游工作站编号升序；同一上游按阻塞先后（FIFO）。
func (s *Simulator) tryUnblockInto(i int) {
	target := s.st[i]
	for from := 0; from < len(s.st) && !target.queueFull(); from++ {
		up := s.st[from]
		for k := 0; k < len(up.blocked); {
			b := up.blocked[k]
			if b.To != i {
				k++
				continue
			}
			if target.queueFull() {
				break
			}
			up.blocked = append(up.blocked[:k], up.blocked[k+1:]...)
			s.syncTimeSeries(up)
			s.processArrival(b.Customer, i)
			// 发送方服务台已真正空出：解除其可能存在的上游阻塞并启动等待任务。
			s.afterServerFreed(up)
		}
	}
}

// route 按 Routing 转移概率选择下一工作站；累积概率不足 1 的部分视为离开。
func (s *Simulator) route(from int) (next int, leave bool) {
	r := s.rng.Float64() // 路由使用独立主种子流
	var cum float64
	for j, p := range s.net.Routing[from] {
		cum += p
		if r < cum {
			return j, false
		}
	}
	return -1, true
}

// Rejected 返回累计被拒绝任务数。
func (s *Simulator) Rejected() int64 { return s.rejected }

// Completed 返回累计离开系统的任务数。
func (s *Simulator) Completed() int64 { return s.completed }

// SimulationReport 是一次运行的完整统计输出。
// 稳态口径：时间平均在 [Warmup, EndTime) 上计算；时长样本只统计
// 在 Warmup 时刻（含）之后开始服务/完成的任务。
type SimulationReport struct {
	EndTime    float64
	Warmup     float64
	Stations   []StationReport
	Rejected   int64
	Completed  int64
	Confidence float64
}

// StationReport 是单个工作站的统计指标。
type StationReport struct {
	Name               string
	Throughput         float64   // 完成次数 / 稳态时长
	Utilization        float64   // 时间平均占用服务台数 / 服务台数
	MeanWait           float64   // 平均排队时长
	WaitQuantiles      []float64 // 排队时长分位数（与请求的分位对应）
	MeanSojourn        float64   // 平均逗留时长（入站到服务完成）
	SojournQuantiles   []float64
	MeanQueueLength    float64 // 时间平均等待队长（稳态区间）
	AvgQueueLength     float64 // 样本意义“平均队长”：完成时刻点均值（稳态区间等权）
	MeanNumberInSystem float64 // 时间平均站内人数（等待+服务+阻塞）
	BusyServers        float64 // 时间平均占用服务台数

	WaitCI    stats.CI
	SojournCI stats.CI
	QueueCI   stats.CI
	UtilCI    stats.CI
	NumberCI  stats.CI

	Completed int64
	Rejected  int64
}

// DefaultQuantiles 是报告默认输出的分位点。
var DefaultQuantiles = []float64{0.5, 0.9, 0.95, 0.99}

// Report 汇总稳态统计。BatchCount>0 时用批均值给出各均值的置信区间：
// [Warmup, EndTime) 被等分为 BatchCount 批，时长样本按所属时间批归集，
// 时间平均指标在每批区间上独立积分。
func (s *Simulator) Report() *SimulationReport {
	end := s.cfg.EndTime
	warmup := s.cfg.Warmup
	steady := end - warmup
	level := s.cfg.Confidence
	if level <= 0 {
		level = 0.95
	}
	batches := s.cfg.BatchCount

	rep := &SimulationReport{
		EndTime: end, Warmup: warmup,
		Rejected: s.rejected, Completed: s.completed, Confidence: level,
		Stations: make([]StationReport, len(s.st)),
	}
	for i, st := range s.st {
		r := StationReport{Name: st.cfg.Name, Completed: st.complete, Rejected: st.rejects}
		if steady > 0 {
			r.Throughput = float64(st.completeAfter(warmup)) / steady
		}
		busyMean := st.busyTA.IntervalMean(warmup, end)
		r.BusyServers = busyMean
		if st.cfg.Servers > 0 {
			r.Utilization = busyMean / float64(st.cfg.Servers)
		}
		r.MeanWait = st.waitAcc.Mean()
		r.MeanSojourn = st.sojAcc.Mean()
		r.MeanQueueLength = st.qlenTA.IntervalMean(warmup, end)
		r.AvgQueueLength = st.deptQ.Mean()
		r.MeanNumberInSystem = st.popTA.IntervalMean(warmup, end)
		r.WaitQuantiles = make([]float64, len(DefaultQuantiles))
		r.SojournQuantiles = make([]float64, len(DefaultQuantiles))
		for k, q := range DefaultQuantiles {
			r.WaitQuantiles[k] = st.waitAcc.Quantile(q)
			r.SojournQuantiles[k] = st.sojAcc.Quantile(q)
		}

		if batches > 0 && steady > 0 {
			r.WaitCI = stats.MeanCI(st.waitAcc.BatchMeans(warmup, end, batches), level)
			r.SojournCI = stats.MeanCI(st.sojAcc.BatchMeans(warmup, end, batches), level)
			r.QueueCI = timeSeriesBatchCI(st.qlenTA, warmup, end, batches, level)
			r.NumberCI = timeSeriesBatchCI(st.popTA, warmup, end, batches, level)
			utilMeans := make([]float64, batches)
			for b := 0; b < batches; b++ {
				a := warmup + steady*float64(b)/float64(batches)
				z := warmup + steady*float64(b+1)/float64(batches)
				utilMeans[b] = st.busyTA.IntervalMean(a, z) / float64(st.cfg.Servers)
			}
			r.UtilCI = stats.MeanCI(utilMeans, level)
		}
		rep.Stations[i] = r
	}
	return rep
}

// timeSeriesBatchCI 在等分时间批上计算时间平均变量的批均值置信区间。
func timeSeriesBatchCI(ta *stats.TimeAverage, warmup, end float64, b int, level float64) stats.CI {
	steady := end - warmup
	means := make([]float64, b)
	for k := 0; k < b; k++ {
		a := warmup + steady*float64(k)/float64(b)
		z := warmup + steady*float64(k+1)/float64(b)
		means[k] = ta.IntervalMean(a, z)
	}
	return stats.MeanCI(means, level)
}
