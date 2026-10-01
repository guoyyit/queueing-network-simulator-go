package qns

import (
	"container/heap"
	"sort"
)

type eventType int

const (
	evArrival    eventType = iota // external or routed arrival at a station
	evCompletion                  // a server finishes a job
	evBlockRetry                  // reserved for future block-retry policies
)

// event is a single scheduled activity. Tie-breaking uses (time, type order,
// station index, job id, sequence) and is documented in README.md.
type event struct {
	time    float64
	typ     eventType
	station int
	jobID   int64
	seq     int64
}

// less defines the deterministic same-time processing order:
//  1. earlier simulated time,
//  2. event type: arrivals before completions before block retries
//     (an arrival sees the server state left by earlier events at that time),
//  3. lower station index,
//  4. lower job id,
//  5. lower insertion sequence (FIFO among otherwise-identical events).
func (e event) less(o event) bool {
	if e.time != o.time {
		return e.time < o.time
	}
	if e.typ != o.typ {
		return e.typ < o.typ
	}
	if e.station != o.station {
		return e.station < o.station
	}
	if e.jobID != o.jobID {
		return e.jobID < o.jobID
	}
	return e.seq < o.seq
}

// eventList is the time-ordered future event list, backed by a binary heap.
type eventList struct {
	h eventHeap
}

type eventHeap []event

func (h eventHeap) Len() int { return len(h) }

func (h eventHeap) Less(i, j int) bool { return h[i].less(h[j]) }

func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *eventHeap) Push(x any) { *h = append(*h, x.(event)) }

func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

func newEventList() *eventList {
	l := &eventList{}
	heap.Init(&l.h)
	return l
}

func (l *eventList) push(e event) { heap.Push(&l.h, e) }

func (l *eventList) pop() (event, bool) {
	if l.h.Len() == 0 {
		return event{}, false
	}
	return heap.Pop(&l.h).(event), true
}

func (l *eventList) peek() (event, bool) {
	if l.h.Len() == 0 {
		return event{}, false
	}
	return l.h[0], true
}

func (l *eventList) len() int { return l.h.Len() }

// removeCompletions removes pending completion events of jobID at station.
func (l *eventList) removeCompletions(station int, jobID int64) {
	out := l.h[:0]
	for _, e := range l.h {
		if e.typ == evCompletion && e.station == station && e.jobID == jobID {
			continue
		}
		out = append(out, e)
	}
	l.h = out
	heap.Init(&l.h)
}

// sorted returns a copy of all events in processing order (used by checkpoints
// to guarantee byte-stable snapshots).
func (l *eventList) sorted() []event {
	out := append([]event(nil), l.h...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].less(out[j]) })
	return out
}
