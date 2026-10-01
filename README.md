# Queueing Network Simulator (Go)

`qns` is a deterministic, locally-runnable discrete-event simulator for
networks of workstations. It targets capacity planning and latency
evaluation: configure stations, traffic and routing, run one or many
replications, and obtain throughput, utilization, queueing/sojourn times,
queue lengths, steady-state confidence intervals, checkpoints and exact
reproducibility.

## Features

- Multi-station networks with per-station:
  - number of parallel servers (`Servers`),
  - finite or infinite waiting room (`Capacity`; number of waiting slots
    **excluding** jobs in service; `-1` is infinite, `0` means no waiting
    room),
  - service-time distribution (`Exponential`, `Uniform`, `Constant`,
    `Normal`, or any implementation of `Distribution`),
  - scheduling policy: `FCFS`, `LCFS`, `PriorityNonPreemptive`,
    `PriorityPreemptive`.
- **Open model**: jobs arrive from a configurable arrival process
  (`PoissonProcess`, `DeterministicProcess`, or any `ArrivalProcess`),
  enter at stations according to a probability distribution, and route
  probabilistically after each visit; residual routing mass means "leave the
  system".
- **Closed model**: a fixed population of jobs cycles forever (routing rows
  must sum to 1), with no external arrivals.
- Deterministic simultaneous-event ordering (see below) and bit-for-bit
  reproducibility for a given config and seed.
- Finite capacity with two blocking semantics: `Reject` (counted rejections)
  or `Block` (server blocking / *block-after-service*).
- Per-station throughput, utilization, mean and 50/90/95/99-percentile
  queueing and sojourn times, both arithmetic and time-averaged queue length,
  plus time-averaged population.
- Warm-up period and batch-means steady-state confidence intervals
  (two-sided Student-t, 95%).
- Full-state checkpoints (in-memory or JSON) with RNG-position-preserving
  resume that is provably identical to a continuous run.
- Concurrent independent replications with merged summaries.

## Time-advance mechanism

The simulator uses the **next-event** (event-scheduling) approach:

1. Every future activity is an `event` on a min-heap future event list.
2. Each iteration pops the smallest event, advances the simulation clock to
   its time, and executes it. There is no fixed time step; the clock jumps
   from event time to event time.
3. The two activity types are external arrivals (`evArrival`) and service
   completions (`evCompletion`). Routing, blocking/unblocking, preemption and
   queue pulls all happen synchronously within the completion processing at
   that instant; no additional event time is required.

### Ordering of simultaneous events

Events at the exact same simulated time are processed in this total order:

1. earlier time;
2. event type: arrival before completion before block retry;
3. lower station index;
4. lower job id;
5. lower insertion sequence (FIFO) for events identical in all the above.

Because the ordering is a total order, a run has no unspecified nondeterminism
at any instant. Combined with the seeded stream, the same config and seed
produce the same event sequence and the same statistics on every run,
concurrent replications included.

### Random-number stream

A single xorshift64* stream seeded through SplitMix64 drives every random
draw (inter-arrivals, service samples, routing choices). Its 64-bit internal
state is part of every checkpoint, so resume continues the stream at the
exact next draw.

## Scheduling policies

- `FCFS`: one waiting queue in arrival order.
- `LCFS`: a non-preemptive stack; the latest arrival is served next.
- `PriorityNonPreemptive`: waiting jobs are ordered by descending priority
  (larger `Priority` value is more urgent), ties broken by earlier arrival; a
  running job is never interrupted.
- `PriorityPreemptive`: if a newly arriving job strictly outranks the
  lowest-priority running job on a busy station, that running job is
  preempted immediately.

**Preemption semantics (explicit).** When a job is preempted:

- its consumed service is `now - ServiceStart`; `Remaining` is decremented by
  exactly that amount,
- it returns to the waiting room ordered by the same priority rule (a
  preempted high-priority job naturally precedes lower-priority waiters),
- when it reaches a server again it **resumes for `Remaining` time units**;
  no new service-time sample is drawn,
- its full-visit sojourn and its total queueing time (the preempted interval
  plus the final queueing spell) are both counted once.

## Finite capacity and blocking

`Capacity` counts waiting slots only; jobs in service do not occupy them.

- `Reject`: a job that finds a full waiting room and no server is refused.
  External arrivals and routed jobs are both rejected in this mode. Every
  rejection increments a per-station and a global counter in the results;
  rejected jobs are never silently lost.
- `Block` (block-after-service / server blocking): a routed job whose target
  is full **parks on the sender server**, which stays unavailable until the
  target frees a slot (server or waiting position). The parked job is
  delivered the instant space appears, in a deterministic unblock cascade
  ordered by sender station index and server index. An external arrival with
  no sender server is still rejected even in `Block` mode.

The state is always conservational: under a closed model the population is
  constant for the entire run (covered by a test).

## Statistics

Observation begins strictly after the configurable `Warmup` period.

- **Throughput**: completed post-warmup visits / observed time.
- **Utilization**: post-warmup busy time / (servers × observed time). Busy
  time is split at the warm-up boundary for a service straddling it.
- **Mean queueing time** and **mean sojourn time**: averages of per-job
  samples. Queueing time excludes time in service; sojourn time is
  `departure - station arrival`, i.e. waiting plus all service spells.
- **Quantiles**: empirical, R-7 / numpy "linear" interpolation, reported at
  p = 0.5, 0.9, 0.95, 0.99.
- **Mean queue length**:
  - `MeanQueueLength` is the arithmetic mean of the waiting-room length
    observed by arrivals (a job-event average),
  - `TimeAvgQueueLength` is the time-weighted average of the waiting-only
    queue (the `Lq` in Little's law),
  - `TimeAvgInSystem` is the time-weighted average total population at the
    station (waiting + in service + blocked-after-service).
- **Confidence intervals**: batch means. The post-warmup interval is split
  into `Batches` equal time batches; per-batch means of per-job metrics (or
  per-batch time averages for queue length) feed a Student-t 95% half-width.
  With fewer than two non-empty batches the half-width is zero.

For an M/M/1 queue with arrival rate λ and service rate μ, the simulator
matches ρ = λ/μ, W = 1/(μ−λ), Wq = ρ/(μ−λ), L = ρ/(1−ρ) and
Lq = ρ²/(1−ρ) (see `TestMM1Analytic`).

## Checkpoints

`Checkpoint()` / `WriteCheckpoint(w)` capture the full simulation state:
clock, RNG state, future event list (stored in processing order for stable
bytes), every queued/running/blocked job including service progress and
preemption bookkeeping, and every statistics accumulator. `Resume(cfg, cp)`
or `ReadCheckpoint(cfg, r)` reconstruct an identical simulator. Resuming and
running to time T produces results `reflect.DeepEqual` to a single
continuous run to T (see `TestCheckpointResumeEquality` and
`TestCheckpointMidBlock`).

## Concurrent replications

`RunReplications(cfg, seeds, until)` runs one independent simulator per seed
concurrently. Replications share no mutable state: each constructs its own
simulator, RNG stream and accumulators. Results are returned in seed order
(independent of scheduling), and `ReplicationSummary` merges point estimates
as across-replication means with Student-t half-widths and pools raw samples
for quantiles.

## Minimal example

```go
cfg := qns.Config{
    Stations: []qns.StationConfig{
        {Name: "web", Servers: 4, Capacity: -1,
            Policy: qns.FCFS, Service: qns.Exponential{Rate: 100}},
        {Name: "db", Servers: 2, Capacity: 50,
            Policy: qns.PriorityNonPreemptive, Service: qns.Exponential{Rate: 60}},
    },
    Blocking: qns.Reject,
    Open: &qns.OpenModel{
        Arrivals: qns.PoissonProcess{Rate: 80},
        Entry:    []qns.Route{{To: 0, Prob: 1}},
        Transitions: [][]qns.Route{
            {{To: 1, Prob: 0.8}}, // web -> db 80%, else leaves
            {},                    // db leaves
        },
    },
    Warmup:  200,
    Batches: 10,
}
sim := qns.New(cfg, 42)
if err := sim.Run(2000); err != nil {
    log.Fatal(err)
}
for _, r := range sim.Results().Stations {
    fmt.Printf("%s: util=%.2f p95sojourn=%.3f\n",
        r.Name, r.Utilization, r.SojournQuantiles[0.95])
}
```

## Run tests

```sh
go test ./...
go test -race ./...
```
