package qns

import "sort"

// Policy selects the queue scheduling discipline.
type Policy int

const (
	// FCFS is first-come-first-served.
	FCFS Policy = iota
	// LCFS is last-come-first-served (non-preemptive stack).
	LCFS
	// PriorityNonPreemptive serves the highest-priority waiting job; ties FCFS.
	PriorityNonPreemptive
	// PriorityPreemptive immediately preempts a lower-priority running job.
	PriorityPreemptive
)

// priorityRank translates a user priority into a "higher is better" rank.
// Larger Priority values mean more urgent.
func priorityRank(p int) int { return p }

// StationConfig describes one workstation.
type StationConfig struct {
	Name     string       // defaults to "station<index>" when empty
	Servers  int          // number of parallel servers (>= 1)
	Capacity int          // waiting-room capacity excluding jobs in service; -1 = infinite
	Policy   Policy       // queueing discipline
	Service  Distribution // service-time distribution
}

// station holds the runtime state of one workstation.
type station struct {
	cfg     StationConfig
	queue   []*Job // waiting jobs, held in the discipline-specific order
	service []*Job // one slot per server; nil when the server is idle
	// blocked[i] is non-nil when server i has finished job but the job cannot
	// leave yet (server blocking); the server stays unavailable. Exactly one of
	// service[i] and blocked[i] is non-nil while a server is occupied.
	blocked []*blockedJob
	stats   *stationStats
}

// blockedJob pairs a finished job with its chosen next destination.
type blockedJob struct {
	server int
	job    *Job
	to     int
}

func newStation(cfg StationConfig) *station {
	if cfg.Servers < 1 {
		cfg.Servers = 1
	}
	return &station{
		cfg:     cfg,
		queue:   make([]*Job, 0),
		service: make([]*Job, cfg.Servers),
		blocked: make([]*blockedJob, cfg.Servers),
		stats:   newStationStats(),
	}
}

// queueFull reports whether a new arrival must be rejected or block the
// sender. The waiting room only covers queued jobs; jobs in service and
// blocked-after-service do not occupy waiting slots.
func (s *station) queueFull() bool {
	return s.cfg.Capacity >= 0 && len(s.queue) >= s.cfg.Capacity
}

// freeServer returns the index of an idle, non-blocked server, or -1.
func (s *station) freeServer() int {
	for i, j := range s.service {
		if j == nil && s.blocked[i] == nil {
			return i
		}
	}
	return -1
}

// enqueue adds a waiting job according to the discipline. For LCFS the job is
// pushed on top of the stack; FCFS appends; priority disciplines keep waiting
// jobs sorted by (rank desc, arrival asc).
func (s *station) enqueue(j *Job, now float64) {
	switch s.cfg.Policy {
	case FCFS:
		s.queue = append(s.queue, j)
	case LCFS:
		s.queue = append(s.queue, j)
	default: // priority disciplines
		s.queue = append(s.queue, j)
		sort.SliceStable(s.queue, func(a, b int) bool {
			ra := priorityRank(s.queue[a].Priority)
			rb := priorityRank(s.queue[b].Priority)
			if ra != rb {
				return ra > rb
			}
			return s.queue[a].StationArrival < s.queue[b].StationArrival
		})
	}
}

// dequeue removes and returns the next job to serve.
func (s *station) dequeue() *Job {
	n := len(s.queue)
	if n == 0 {
		return nil
	}
	var j *Job
	if s.cfg.Policy == LCFS {
		j = s.queue[n-1]
		s.queue = s.queue[:n-1]
	} else {
		j = s.queue[0]
		s.queue = s.queue[1:]
	}
	return j
}

// lowestPriorityServer returns the index of the server whose running job has
// the lowest priority (ties: the server that started latest), or -1.
func (s *station) lowestPriorityServer() int {
	idx := -1
	for i, j := range s.service {
		if j == nil || s.blocked[i] != nil {
			continue
		}
		if idx == -1 ||
			priorityRank(j.Priority) < priorityRank(s.service[idx].Priority) ||
			(priorityRank(j.Priority) == priorityRank(s.service[idx].Priority) &&
				j.ServiceStart > s.service[idx].ServiceStart) {
			idx = i
		}
	}
	return idx
}

// waitingLen / inServiceLen / blockedLen count the jobs in each state.
func (s *station) waitingLen() int { return len(s.queue) }

func (s *station) inServiceLen() int {
	n := 0
	for _, j := range s.service {
		if j != nil {
			n++
		}
	}
	return n
}

func (s *station) blockedLen() int {
	n := 0
	for _, b := range s.blocked {
		if b != nil {
			n++
		}
	}
	return n
}

// inSystemLen is the total number of jobs at the station, including jobs that
// finished service but are blocking the server.
func (s *station) inSystemLen() int {
	return len(s.queue) + s.inServiceLen() + s.blockedLen()
}
