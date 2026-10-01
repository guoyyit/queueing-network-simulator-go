package qsim

import (
	"encoding/json"
	"fmt"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
	"github.com/guoyyit/queueing-network-simulator-go/rng"
	"github.com/guoyyit/queueing-network-simulator-go/stats"
)

// snapshotVersion 为检查点格式版本。
const snapshotVersion = 1

// configSnap 是 JSON 安全的配置快照（分布以带类型标签的 Envelope 表示）。
type configSnap struct {
	Blocking       int
	Seed           uint64
	Warmup         float64
	EndTime        float64
	BatchCount     int
	Confidence     float64
	Model          int
	Stations       []stationConfigSnap
	Routing        [][]float64
	ExternalArrive []float64
	Population     int
}

type stationConfigSnap struct {
	Name     string
	Servers  int
	Capacity int
	Policy   int
	Service  dist.Envelope
}

func encodeConfig(cfg *Config) configSnap {
	c := configSnap{
		Blocking: int(cfg.Blocking), Seed: cfg.Seed, Warmup: cfg.Warmup,
		EndTime: cfg.EndTime, BatchCount: cfg.BatchCount, Confidence: cfg.Confidence,
		Model: int(cfg.Network.Model), Routing: cfg.Network.Routing,
		ExternalArrive: cfg.Network.ExternalArrive, Population: cfg.Network.Population,
	}
	for _, st := range cfg.Network.Stations {
		c.Stations = append(c.Stations, stationConfigSnap{
			Name: st.Name, Servers: st.Servers, Capacity: st.Capacity,
			Policy: int(st.Policy), Service: dist.Wrap(st.Service),
		})
	}
	return c
}

func decodeConfig(c configSnap) *Config {
	net := &NetworkConfig{
		Model: FlowModel(c.Model), Routing: c.Routing,
		ExternalArrive: c.ExternalArrive, Population: c.Population,
	}
	for _, st := range c.Stations {
		net.Stations = append(net.Stations, StationConfig{
			Name: st.Name, Servers: st.Servers, Capacity: st.Capacity,
			Policy: Policy(st.Policy), Service: st.Service.Unwrap(),
		})
	}
	return &Config{
		Network: net, Blocking: BlockingMode(c.Blocking), Seed: c.Seed,
		Warmup: c.Warmup, EndTime: c.EndTime, BatchCount: c.BatchCount,
		Confidence: c.Confidence,
	}
}

// CustomerSnap 是顾客的完整可序列化状态。
type CustomerSnap struct {
	ID          uint64
	Class       int
	Priority    int
	BirthTime   float64
	ArrivalTime float64
	Remaining   float64
	Station     int
	Preempted   bool
}

// Snapshot 是仿真器完整可序列化状态。
type Snapshot struct {
	Version    int
	Config     configSnap
	Now        float64
	NextID     uint64
	CalSeq     uint64
	RNG        rng.State
	ArriveRNG  rng.State
	ServiceRNG []rng.State
	Events     []EventSnap
	Stations   []StationSnap
	Customers  map[uint64]CustomerSnap
	Created    int64
	Rejected   int64
	Done       int64
}

// EventSnap 是事件的序列化形式（顾客用 ID 引用）。
type EventSnap struct {
	Type        int
	Time        float64
	Station     int
	Server      int
	Seq         uint64
	CustomerID  uint64
	HasCustomer bool
}

// StationSnap 是工作站完整运行时状态。
type StationSnap struct {
	Waiting     []uint64
	Servers     []ServerSnap
	Blocked     []BlockedSnap
	QLenTA      stats.State
	PopTA       stats.State
	BusyTA      stats.State
	WaitAcc     []stats.TimedSample
	SojAcc      []stats.TimedSample
	DeptQ       []float64
	Rejects     int64
	Complete    int64
	Completions []float64
}

// ServerSnap 是单个服务台的序列化状态。
type ServerSnap struct {
	CustomerID uint64
	Occupied   bool
	Start      float64
	Remaining  float64
	IsBlocked  bool
}

// BlockedSnap 是阻塞记录的序列化形式。
type BlockedSnap struct {
	CustomerID uint64
	From       int
	To         int
	Server     int
	BlockedAt  float64
	SlotStart  float64
	HasSlot    bool
}

func encodeCustomer(c *Customer) CustomerSnap {
	return CustomerSnap{
		ID: c.ID, Class: c.Class, Priority: c.Priority,
		BirthTime: c.BirthTime, ArrivalTime: c.ArrivalTime,
		Remaining: c.Remaining, Station: c.Station, Preempted: c.Preempted,
	}
}

func decodeCustomer(sn CustomerSnap) *Customer {
	return &Customer{
		ID: sn.ID, Class: sn.Class, Priority: sn.Priority,
		BirthTime: sn.BirthTime, ArrivalTime: sn.ArrivalTime,
		Remaining: sn.Remaining, Station: sn.Station, Preempted: sn.Preempted,
	}
}

// Save 导出仿真器完整状态。
func (s *Simulator) Save() (*Snapshot, error) {
	customers := map[uint64]CustomerSnap{}
	register := func(c *Customer) {
		if c != nil {
			customers[c.ID] = encodeCustomer(c)
		}
	}

	snap := &Snapshot{
		Version: snapshotVersion, Config: encodeConfig(s.cfg),
		Now: s.now, NextID: s.nextID, CalSeq: s.cal.Seq(),
		RNG:        s.rng.StateOf(),
		ArriveRNG:  s.arrive.StateOf(),
		ServiceRNG: make([]rng.State, len(s.svc)),
		Created:    s.created, Rejected: s.rejected, Done: s.completed,
		Customers: customers,
	}
	for i, rr := range s.svc {
		snap.ServiceRNG[i] = rr.StateOf()
	}
	for _, e := range s.cal.Items() {
		es := EventSnap{
			Type: int(e.Type), Time: e.Time, Station: e.Station,
			Server: e.Server, Seq: e.Seq,
		}
		if e.Customer != nil {
			es.HasCustomer = true
			es.CustomerID = e.Customer.ID
			register(e.Customer)
		}
		snap.Events = append(snap.Events, es)
	}
	snap.Stations = make([]StationSnap, len(s.st))
	for i, st := range s.st {
		ss := StationSnap{
			QLenTA:      st.qlenTA.State(),
			PopTA:       st.popTA.State(),
			BusyTA:      st.busyTA.State(),
			WaitAcc:     st.waitAcc.Samples(),
			SojAcc:      st.sojAcc.Samples(),
			DeptQ:       st.deptQ.Values(),
			Rejects:     st.rejects,
			Complete:    st.complete,
			Completions: append([]float64(nil), st.completions...),
		}
		for _, c := range st.waiting {
			ss.Waiting = append(ss.Waiting, c.ID)
			register(c)
		}
		ss.Servers = make([]ServerSnap, len(st.busy))
		for k, slot := range st.busy {
			if slot == nil {
				continue
			}
			ss.Servers[k] = ServerSnap{Occupied: true, Start: slot.Start, Remaining: slot.Remaining}
			if slot.Customer != nil {
				ss.Servers[k].CustomerID = slot.Customer.ID
				register(slot.Customer)
			} else {
				ss.Servers[k].IsBlocked = true
			}
		}
		for _, b := range st.blocked {
			ss.Blocked = append(ss.Blocked, BlockedSnap{
				CustomerID: b.Customer.ID, From: b.From, To: b.To,
				Server: b.Server, BlockedAt: b.BlockedAt,
				HasSlot: b.Slot != nil,
			})
			if b.Slot != nil {
				last := len(ss.Blocked) - 1
				ss.Blocked[last].SlotStart = b.Slot.Start
			}
			register(b.Customer)
		}
		snap.Stations[i] = ss
	}
	return snap, nil
}

// RestoreSnapshot 从快照重建仿真器。
func RestoreSnapshot(snap *Snapshot) (*Simulator, error) {
	if snap.Version != snapshotVersion {
		return nil, fmt.Errorf("qsim: unsupported snapshot version %d", snap.Version)
	}
	cfg := decodeConfig(snap.Config)
	s := &Simulator{
		cfg: cfg, net: cfg.Network,
		rng:    rng.Restore(snap.RNG),
		arrive: rng.Restore(snap.ArriveRNG),
		cal:    NewCalendar(), now: snap.Now, nextID: snap.NextID,
		created: snap.Created, rejected: snap.Rejected, completed: snap.Done,
	}
	s.svc = make([]*rng.Stream, len(snap.ServiceRNG))
	for i, rs := range snap.ServiceRNG {
		s.svc[i] = rng.Restore(rs)
	}
	s.st = make([]*Station, len(snap.Stations))
	customers := map[uint64]*Customer{}
	get := func(id uint64) *Customer {
		if c, ok := customers[id]; ok {
			return c
		}
		sn, ok := snap.Customers[id]
		if !ok {
			sn = CustomerSnap{ID: id}
		}
		c := decodeCustomer(sn)
		customers[id] = c
		return c
	}

	for i, ss := range snap.Stations {
		sc := cfg.Network.Stations[i]
		st := newStation(i, sc)
		st.qlenTA.RestoreState(ss.QLenTA)
		st.popTA.RestoreState(ss.PopTA)
		st.busyTA.RestoreState(ss.BusyTA)
		st.waitAcc = stats.NewTimedAccumulator()
		for _, ts := range ss.WaitAcc {
			st.waitAcc.Observe(ts.Time, ts.Value)
		}
		st.sojAcc = stats.NewTimedAccumulator()
		for _, ts := range ss.SojAcc {
			st.sojAcc.Observe(ts.Time, ts.Value)
		}
		st.deptQ.RestoreValues(ss.DeptQ)
		st.rejects = ss.Rejects
		st.complete = ss.Complete
		st.completions = append([]float64(nil), ss.Completions...)

		st.waiting = make([]*Customer, 0, len(ss.Waiting))
		for _, id := range ss.Waiting {
			st.waiting = append(st.waiting, get(id))
		}
		st.busy = make([]*serverSlot, len(ss.Servers))
		for k, srv := range ss.Servers {
			if !srv.Occupied {
				continue
			}
			slot := &serverSlot{Start: srv.Start, Remaining: srv.Remaining}
			if !srv.IsBlocked && srv.CustomerID != 0 {
				slot.Customer = get(srv.CustomerID)
			}
			st.busy[k] = slot
		}
		st.blocked = make([]*blockedJob, 0, len(ss.Blocked))
		for _, bs := range ss.Blocked {
			var slot *serverSlot
			if bs.HasSlot {
				c := get(bs.CustomerID)
				slot = &serverSlot{Customer: c, Start: bs.SlotStart}
			}
			st.blocked = append(st.blocked, &blockedJob{
				Customer: get(bs.CustomerID), From: bs.From, To: bs.To,
				Server: bs.Server, BlockedAt: bs.BlockedAt, Slot: slot,
			})
		}
		s.st[i] = st
	}

	for _, es := range snap.Events {
		e := &Event{
			Type: EventType(es.Type), Time: es.Time, Station: es.Station,
			Server: es.Server, Seq: es.Seq,
		}
		if es.HasCustomer {
			e.Customer = get(es.CustomerID)
		}
		s.cal.PushRaw(e)
	}
	s.cal.RestoreSeq(snap.CalSeq)
	return s, nil
}

// Marshal 把快照编码为 JSON（供落盘）。
func (snap *Snapshot) Marshal() ([]byte, error) {
	return json.Marshal(snap)
}

// UnmarshalSnapshot 从 JSON 解码快照。
func UnmarshalSnapshot(data []byte) (*Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
