package qns

import "fmt"

// Simulator runs a discrete-event queueing-network simulation.
//
// Time advances by the next-event mechanism: the future event list always
// yields the event with the smallest key under event.less, so simultaneous
// events are processed in one fully documented, deterministic order. Given
// the same Config and seed, a run is bit-for-bit reproducible.
type Simulator struct {
	cfg   Config
	rng   *RNG
	st    []*station
	fel   *eventList
	now   float64
	until float64

	nextJobID int64
	nextSeq   int64

	completed int64
	rejected  int64

	initialized  bool
	levelsSeeded bool

	// completionOrder, when non-nil, records job IDs in the exact order their
	// completion events are processed (used by tests of simultaneous events).
	completionOrder []int64
}

// New constructs a simulator from cfg and the given deterministic seed.
func New(cfg Config, seed int64) *Simulator {
	s := &Simulator{cfg: cfg, rng: NewRNG(seed)}
	return s
}

// initState builds station runtime state and seeds the first events.
func (s *Simulator) initState() error {
	if err := s.validate(); err != nil {
		return err
	}
	s.st = make([]*station, len(s.cfg.Stations))
	for i, c := range s.cfg.Stations {
		if c.Name == "" {
			c.Name = fmt.Sprintf("station%d", i)
		}
		s.st[i] = newStation(c)
	}
	s.fel = newEventList()

	if s.cfg.Closed != nil {
		for k := 0; k < s.cfg.Closed.Population; k++ {
			j := s.newJob(s.now)
			s.admit(j, s.cfg.Closed.StartStation)
		}
	}
	if s.cfg.Open != nil {
		dt := s.cfg.Open.Arrivals.NextInterarrival(s.rng)
		s.schedule(evArrival, s.now+dt, 0, -1)
	}
	s.initialized = true
	return nil
}

// validate checks configuration consistency.
func (s *Simulator) validate() error {
	if len(s.cfg.Stations) == 0 {
		return fmt.Errorf("qns: at least one station is required")
	}
	for i, c := range s.cfg.Stations {
		if c.Servers < 0 {
			return fmt.Errorf("qns: station %d has negative server count", i)
		}
		if c.Service == nil {
			return fmt.Errorf("qns: station %d has no service distribution", i)
		}
	}
	switch {
	case s.cfg.Open != nil && s.cfg.Closed != nil:
		return fmt.Errorf("qns: open and closed models are mutually exclusive")
	case s.cfg.Open != nil:
		if s.cfg.Open.Arrivals == nil {
			return fmt.Errorf("qns: open model needs an arrival process")
		}
	case s.cfg.Closed != nil:
		if s.cfg.Closed.Population <= 0 {
			return fmt.Errorf("qns: closed model needs a positive population")
		}
		if s.cfg.Closed.StartStation < 0 || s.cfg.Closed.StartStation >= len(s.cfg.Stations) {
			return fmt.Errorf("qns: closed model start station out of range")
		}
	}
	if err := validateRoutes("transitions", s.transitions(), len(s.cfg.Stations)); err != nil {
		return err
	}
	if s.cfg.Closed != nil {
		for i, row := range s.cfg.Closed.Transitions {
			var sum float64
			for _, r := range row {
				sum += r.Prob
			}
			if d := sum - 1; d > 1e-9 || d < -1e-9 {
				return fmt.Errorf("qns: closed model transitions[%d] sum to %g, must be 1", i, sum)
			}
		}
	}
	return nil
}

func (s *Simulator) transitions() [][]Route {
	if s.cfg.Open != nil {
		return s.cfg.Open.Transitions
	}
	return s.cfg.Closed.Transitions
}

func validateRoutes(name string, routes [][]Route, n int) error {
	if routes == nil {
		return nil
	}
	if len(routes) > n {
		return fmt.Errorf("qns: %s has rows for unknown stations", name)
	}
	for i, row := range routes {
		var sum float64
		for _, r := range row {
			if r.To < 0 || r.To >= n {
				return fmt.Errorf("qns: %s[%d] targets station %d out of range", name, i, r.To)
			}
			if r.Prob < 0 {
				return fmt.Errorf("qns: %s[%d] has negative probability", name, i)
			}
			sum += r.Prob
		}
		if sum > 1+1e-9 {
			return fmt.Errorf("qns: %s[%d] probabilities sum to %g > 1", name, i, sum)
		}
	}
	return nil
}

func (s *Simulator) newJob(now float64) *Job {
	j := &Job{
		ID:        s.nextJobID,
		BirthTime: now,
	}
	s.nextJobID++
	return j
}

func (s *Simulator) schedule(typ eventType, t float64, station int, jobID int64) {
	s.fel.push(event{time: t, typ: typ, station: station, jobID: jobID, seq: s.nextSeq})
	s.nextSeq++
}

// Run advances the simulation until simulated time until. Run may be called
// repeatedly; statistics always cover [warmup, until) of simulated time.
func (s *Simulator) Run(until float64) error {
	if until <= s.now {
		return fmt.Errorf("qns: run end %g must exceed current time %g", until, s.now)
	}
	if !s.initialized {
		if err := s.initState(); err != nil {
			return err
		}
	}
	s.until = until
	for {
		e, ok := s.fel.pop()
		if !ok || e.time > until {
			if ok {
				s.fel.push(e)
			}
			break
		}
		s.maybeSeedLevels(e.time)
		s.now = e.time
		s.dispatch(e)
	}
	s.now = until
	return nil
}

func (s *Simulator) dispatch(e event) {
	switch e.typ {
	case evArrival:
		s.handleExternalArrival(e)
	case evCompletion:
		s.handleCompletion(e)
	}
}

// handleExternalArrival generates one new job at a randomly chosen entry
// station and immediately schedules the next external arrival (for open
// models). Admission statistics are handled inside admit.
func (s *Simulator) handleExternalArrival(e event) {
	if s.cfg.Open == nil {
		return
	}
	j := s.newJob(s.now)
	to := s.pickRoute(s.cfg.Open.Entry)
	if to >= 0 {
		s.admit(j, to)
	} else {
		// An entry distribution with residual mass means "arrive and leave
		// immediately", which is counted as a completion with zero station time.
		s.completed++
	}
	dt := s.cfg.Open.Arrivals.NextInterarrival(s.rng)
	s.schedule(evArrival, s.now+dt, 0, -1)
}

// admit tries to place a job arriving at station idx at time s.now. Rejected
// jobs are counted per station and globally and carry no further state.
type admitOutcome int

const (
	admitOK       admitOutcome = iota // job is now in service or in the queue
	admitRejected                     // full queue, job rejected and counted
	admitBlocked                      // full station in Block mode; sender parks
)

// admit tries to place a job arriving at station idx at time s.now. A routed
// job (routedFrom >= 0) whose target is full in Block mode returns
// admitBlocked so the caller parks the sender server; external arrivals have
// no sender and are rejected instead.
func (s *Simulator) admit(j *Job, idx int) admitOutcome {
	return s.admitAs(j, idx, j.routedFrom)
}

// admitAs is admit with an explicit sender server; used when resuming blocked
// jobs so the job keeps its parked-sender identity across repeated attempts.
func (s *Simulator) admitAs(j *Job, idx int, routedFrom int) admitOutcome {
	j.Station = idx
	j.StationArrival = s.now
	j.ServiceStart = 0
	j.ServiceTime = 0
	j.Remaining = 0
	j.firstService = -1
	j.waitExtra = 0
	j.enqueuedAt = 0
	j.everQueued = false
	j.routedFrom = routedFrom
	st := s.st[idx]

	// Record the queue length seen by this arrival (post-warmup arrivals only).
	s.recordArrivalQL(idx, st.waitingLen())

	if srv := st.freeServer(); srv >= 0 {
		s.startService(idx, srv, j)
		s.recordIS(idx)
		return admitOK
	}
	if st.cfg.Policy == PriorityPreemptive {
		victimSrv := st.lowestPriorityServer()
		if victimSrv >= 0 && priorityRank(j.Priority) > priorityRank(st.service[victimSrv].Priority) {
			victim := st.service[victimSrv]
			// Preemption semantics: the victim keeps its unconsumed remaining
			// service time (Remaining -= elapsed), returns to the waiting room
			// ordered ahead of ordinary waiters and resumes without a new
			// service-time sample. A preempted victim is always accepted even
			// if the nominal waiting capacity is momentarily full.
			victim.Remaining -= s.now - victim.ServiceStart
			victim.ServiceStart = 0
			st.service[victimSrv] = nil
			s.cancelCompletion(idx, victim.ID)
			victim.enqueuedAt = s.now
			victim.everQueued = true
			st.enqueue(victim, s.now)
			s.startService(idx, victimSrv, j)
			s.recordQL(idx)
			s.recordIS(idx)
			return admitOK
		}
	}

	if st.queueFull() {
		if s.cfg.Blocking == Block && j.routedFrom >= 0 {
			return admitBlocked
		}
		st.stats.rejected++
		s.rejected++
		return admitRejected
	}
	st.enqueue(j, s.now)
	j.enqueuedAt = s.now
	j.everQueued = true
	s.recordQL(idx)
	s.recordIS(idx)
	return admitOK
}

// startService begins a fresh service segment on server srv.
func (s *Simulator) startService(idx, srv int, j *Job) {
	st := s.st[idx]
	svc := st.cfg.Service.Sample(s.rng)
	if svc < 0 {
		svc = 0
	}
	j.ServiceStart = s.now
	if j.firstService < 0 {
		j.firstService = s.now
	}
	j.ServiceTime = svc
	j.Remaining = svc
	st.service[srv] = j
	s.schedule(evCompletion, s.now+svc, idx, j.ID)
}

// handleCompletion processes a service completion: preempted jobs never have
// a completion scheduled, so every completion event refers to a running job.
func (s *Simulator) handleCompletion(e event) {
	idx := e.station
	st := s.st[idx]
	srv := -1
	var j *Job
	for i, running := range st.service {
		if running != nil && running.ID == e.jobID {
			srv, j = i, running
			break
		}
	}
	if j == nil {
		return // stale event (cannot currently occur; kept defensively)
	}
	if s.completionOrder != nil {
		s.completionOrder = append(s.completionOrder, j.ID)
	}
	st.service[srv] = nil

	// Statistics for this completed visit: queueing time and full sojourn.
	wait := j.firstService - j.StationArrival + j.waitExtra
	sojourn := s.now - j.StationArrival
	s.recordCompletion(idx, wait, sojourn, j.ServiceTime)

	to := s.nextStation(idx)
	if to < 0 {
		// Job leaves the system.
		s.completed++
		s.tryUnblockWaiters(idx)
		s.pullNext(idx)
		s.recordQL(idx)
		s.recordIS(idx)
		return
	}
	j.routedFrom = srv
	outcome := s.admit(j, to)
	if outcome == admitBlocked {
		// Server blocking: the server stays occupied by the finished, parked
		// job until the target makes room; no new job can start on it.
		s.st[idx].blocked[srv] = &blockedJob{server: srv, job: j, to: to}
		s.recordIS(idx)
		return
	}
	// The just-finished job has moved on, so the server slot is truly free:
	// blocked upstream senders targeting this station can now deliver.
	s.tryUnblockWaiters(idx)
	s.pullNext(idx)
	s.recordQL(idx)
	s.recordIS(idx)
}

// pullNext starts the next queued job on a free server, honoring priority
// preemption at the same instant: a newly admitted higher-priority job is
// handled in admitPreempt; here we only fill idle servers.
func (s *Simulator) pullNext(idx int) {
	st := s.st[idx]
	for {
		srv := st.freeServer()
		if srv < 0 {
			return
		}
		j := st.dequeue()
		if j == nil {
			return
		}
		// The first queueing spell is covered by (firstService - arrival);
		// only later spells, caused by preemption, are added separately.
		if j.waitExtra < 0 {
			j.waitExtra = 0
		}
		if j.ServiceTime > 0 && j.Remaining > 0 && j.Remaining < j.ServiceTime {
			j.waitExtra += s.now - j.enqueuedAt
		}
		s.resumeOrStart(idx, srv, j)
	}
}

// resumeOrStart starts service; a preempted job (Remaining < ServiceTime)
// resumes for exactly its remaining time on the same work, while a fresh job
// is sampled normally.
func (s *Simulator) resumeOrStart(idx, srv int, j *Job) {
	st := s.st[idx]
	if j.Remaining > 0 && j.Remaining < j.ServiceTime {
		j.ServiceStart = s.now
		st.service[srv] = j
		s.schedule(evCompletion, s.now+j.Remaining, idx, j.ID)
		return
	}
	s.startService(idx, srv, j)
}

// tryUnblockWaiters is invoked when a slot appears at station idx: every
// blocked sender targeting idx that can now deliver does so, in deterministic
// (station index, server index, job id) order. A delivered job may free a
// server that then pulls its own queue, cascading in a fixed order.
func (s *Simulator) tryUnblockWaiters(idx int) {
	target := s.st[idx]
	type waiter struct {
		from, srv int
		b         *blockedJob
	}
	var pending []waiter
	for fi, fs := range s.st {
		for srv, b := range fs.blocked {
			if b != nil && b.to == idx {
				pending = append(pending, waiter{fi, srv, b})
			}
		}
	}
	// Deterministic order even though slot may only fit one: station asc.
	for _, w := range pending {
		if target.queueFull() && target.freeServer() < 0 {
			return
		}
		fs := s.st[w.from]
		fs.blocked[w.srv] = nil
		j := w.b.job
		outcome := s.admitAs(j, idx, w.srv)
		if outcome == admitBlocked {
			// Target filled again earlier in this cascade: keep parked.
			fs.blocked[w.srv] = w.b
			return
		}
		s.pullNext(w.from)
		s.recordQL(w.from)
		s.recordIS(w.from)
	}
}

// nextStation samples the routing table of station idx. A negative result
// means the job leaves the system.
func (s *Simulator) nextStation(idx int) int {
	rows := s.transitions()
	if idx >= len(rows) {
		return -1
	}
	return s.pickRoute(rows[idx])
}

// pickRoute selects a target from probabilistic routes. Residual mass
// (1 - sum of probabilities) selects departure (open model) or, for closed
// models, must be zero.
func (s *Simulator) pickRoute(routes []Route) int {
	u := s.rng.Float64()
	var acc float64
	for _, r := range routes {
		acc += r.Prob
		if u < acc {
			return r.To
		}
	}
	return -1
}

func (s *Simulator) cancelCompletion(idx int, jobID int64) {
	s.fel.removeCompletions(idx, jobID)
}

// afterWarmup returns t relative to the warm-up cut-over (negative means the
// observation has not started).
func (s *Simulator) afterWarmup(t float64) float64 { return t - s.cfg.Warmup }

// maybeSeedLevels records level samples exactly once at the warm-up boundary
// for every station, capturing queues that built up during warm-up.
func (s *Simulator) maybeSeedLevels(eventTime float64) {
	if s.levelsSeeded || s.now >= s.cfg.Warmup || eventTime < s.cfg.Warmup {
		return
	}
	s.levelsSeeded = true
	for idx, st := range s.st {
		st.stats.qlLevel = append(st.stats.qlLevel, levelPoint{t: 0, level: s.st[idx].waitingLen()})
		st.stats.isLevel = append(st.stats.isLevel, levelPoint{t: 0, level: s.st[idx].inSystemLen()})
	}
}

// recordCompletion stores per-job metrics for post-warmup completions and
// busy time split at the warm-up boundary.
func (s *Simulator) recordCompletion(idx int, wait, sojourn, service float64) {
	if s.now <= s.cfg.Warmup {
		return
	}
	st := s.st[idx].stats
	busy := service
	if start := s.now - service; start < s.cfg.Warmup {
		busy = s.now - s.cfg.Warmup
	}
	st.recordCompletion(s.afterWarmup(s.now), wait, sojourn, busy)
}

// recordArrivalQL samples the waiting-room length at an arrival instant.
func (s *Simulator) recordArrivalQL(idx, ql int) {
	if s.now < s.cfg.Warmup {
		return
	}
	st := s.st[idx].stats
	st.arrivalQLSum += float64(ql)
	st.arrivalQLCounts++
}

// recordQL appends a post-warmup level point of the waiting queue.
func (s *Simulator) recordQL(idx int) {
	if s.now < s.cfg.Warmup {
		return
	}
	st := s.st[idx].stats
	st.qlLevel = append(st.qlLevel, levelPoint{t: s.afterWarmup(s.now), level: s.st[idx].waitingLen()})
}

// recordIS appends a post-warmup level point of the station population.
func (s *Simulator) recordIS(idx int) {
	if s.now < s.cfg.Warmup {
		return
	}
	st := s.st[idx].stats
	st.isLevel = append(st.isLevel, levelPoint{t: s.afterWarmup(s.now), level: s.st[idx].inSystemLen()})
}

// Now returns the current simulated time.
func (s *Simulator) Now() float64 { return s.now }

// runningAt returns the jobs currently in service at station idx (nil slots
// for idle/blocked servers). Intended for tests and diagnostics.
func (s *Simulator) runningAt(idx int) []*Job {
	out := make([]*Job, len(s.st[idx].service))
	copy(out, s.st[idx].service)
	return out
}

// queueAt returns the waiting jobs at station idx in dequeue order.
func (s *Simulator) queueAt(idx int) []*Job {
	return append([]*Job(nil), s.st[idx].queue...)
}

// blockedAt returns the number of servers at station idx that are held by
// finished jobs waiting for a full downstream queue.
func (s *Simulator) blockedAt(idx int) int { return s.st[idx].blockedLen() }

// inSystemAt returns the current number of jobs at station idx.
func (s *Simulator) inSystemAt(idx int) int { return s.st[idx].inSystemLen() }

// Results returns the statistics accumulated over [warmup, now).
func (s *Simulator) Results() Results {
	statsTime := s.now - s.cfg.Warmup
	if statsTime < 0 {
		statsTime = 0
	}
	batches := s.cfg.Batches
	if batches < 1 {
		batches = 1
	}
	reports := make([]StationReport, 0, len(s.st))
	for _, st := range s.st {
		reports = append(reports, st.stats.report(st.cfg.Name, st.cfg.Servers, statsTime, batches))
	}
	return Results{
		Time:      s.now,
		StatsTime: statsTime,
		Warmup:    s.cfg.Warmup,
		Stations:  reports,
		Rejected:  s.rejected,
		Completed: s.completed,
	}
}

// exportState serializes the complete runtime state of the simulation,
// including the RNG position, future event list, every job's service progress
// and every statistics accumulator. Config is deliberately not embedded:
// Resume takes the original Config, and checkpoints are validated against it.
func (s *Simulator) exportState() *simState {
	st := &simState{
		Version:      1,
		Now:          s.now,
		Until:        s.until,
		RNGState:     s.rng.snapshotState(),
		Seed:         s.rng.seed,
		NextJobID:    s.nextJobID,
		NextSeq:      s.nextSeq,
		Completed:    s.completed,
		Rejected:     s.rejected,
		Initialized:  s.initialized,
		LevelsSeeded: s.levelsSeeded,
		Events:       eventsToState(s.fel.sorted()),
	}
	for _, stn := range s.st {
		ss := stationState{
			Waiting: jobsToState(stn.queue),
			Service: jobsToState(stn.service),
			Blocked: make([]*blockedState, len(stn.blocked)),
			Stats:   stn.stats.export(),
		}
		for i, b := range stn.blocked {
			if b != nil {
				jb := jobToState(b.job)
				ss.Blocked[i] = &blockedState{Server: b.server, Job: jb, To: b.to}
			}
		}
		st.Stations = append(st.Stations, ss)
	}
	return st
}

// importState rebuilds runtime state from a checkpoint.
func (s *Simulator) importState(st *simState) error {
	if st.Version != 1 {
		return fmt.Errorf("qns: unsupported checkpoint version %d", st.Version)
	}
	if err := s.validate(); err != nil {
		return err
	}
	if len(st.Stations) != len(s.cfg.Stations) {
		return fmt.Errorf("qns: checkpoint has %d stations, config has %d",
			len(st.Stations), len(s.cfg.Stations))
	}
	s.now = st.Now
	s.until = st.Until
	s.nextJobID = st.NextJobID
	s.nextSeq = st.NextSeq
	s.completed = st.Completed
	s.rejected = st.Rejected
	s.initialized = st.Initialized
	s.levelsSeeded = st.LevelsSeeded

	s.rng = NewRNG(st.Seed)
	s.rng.restoreState(st.RNGState)

	s.fel = newEventList()
	for _, e := range st.Events {
		s.fel.push(event{time: e.Time, typ: eventType(e.Type), station: e.Station, jobID: e.JobID, seq: e.Seq})
	}

	s.st = make([]*station, len(s.cfg.Stations))
	for i, cfg := range s.cfg.Stations {
		if cfg.Name == "" {
			cfg.Name = fmt.Sprintf("station%d", i)
		}
		stn := newStation(cfg)
		ss := st.Stations[i]
		stn.queue = jobsFromState(ss.Waiting)
		for j, js := range ss.Service {
			if js != nil {
				stn.service[j] = jobFromState(js)
			}
		}
		for j, b := range ss.Blocked {
			if b != nil {
				stn.blocked[j] = &blockedJob{
					server: b.Server,
					job:    jobFromState(b.Job),
					to:     b.To,
				}
			}
		}
		stn.stats = importStationStats(ss.Stats)
		s.st[i] = stn
	}
	return nil
}

func jobsToState(js []*Job) []*jobState {
	out := make([]*jobState, len(js))
	for i, j := range js {
		if j != nil {
			out[i] = jobToState(j)
		}
	}
	return out
}

func eventsToState(es []event) []eventState {
	out := make([]eventState, len(es))
	for i, e := range es {
		out[i] = eventState{
			Time: e.time, Type: int(e.typ), Station: e.station, JobID: e.jobID, Seq: e.seq,
		}
	}
	return out
}

func jobsFromState(js []*jobState) []*Job {
	out := make([]*Job, len(js))
	for i, j := range js {
		if j != nil {
			out[i] = jobFromState(j)
		}
	}
	return out
}

func jobToState(j *Job) *jobState {
	return &jobState{
		ID:             j.ID,
		Priority:       j.Priority,
		BirthTime:      j.BirthTime,
		StationArrival: j.StationArrival,
		ServiceStart:   j.ServiceStart,
		ServiceTime:    j.ServiceTime,
		Remaining:      j.Remaining,
		Station:        j.Station,
		RoutedFrom:     j.routedFrom,
		FirstService:   j.firstService,
		WaitExtra:      j.waitExtra,
		EnqueuedAt:     j.enqueuedAt,
		EverQueued:     j.everQueued,
	}
}

func jobFromState(j *jobState) *Job {
	return &Job{
		ID:             j.ID,
		Priority:       j.Priority,
		BirthTime:      j.BirthTime,
		StationArrival: j.StationArrival,
		ServiceStart:   j.ServiceStart,
		ServiceTime:    j.ServiceTime,
		Remaining:      j.Remaining,
		Station:        j.Station,
		routedFrom:     j.RoutedFrom,
		firstService:   j.FirstService,
		waitExtra:      j.WaitExtra,
		enqueuedAt:     j.EnqueuedAt,
		everQueued:     j.EverQueued,
	}
}
