package qns

import (
	"bytes"
	"encoding/json"
	"io"
)

// Checkpoint is a fully serializable snapshot of a simulation.
type Checkpoint struct {
	state simState
}

// simState is the complete, config-independent simulation state. It captures
// the RNG position and every event/job/statistics datum, so a resumed run
// produces results identical to one uninterrupted run.
type simState struct {
	Version      int            `json:"version"`
	Now          float64        `json:"now"`
	Until        float64        `json:"until"`
	RNGState     uint64         `json:"rng_state"`
	Seed         int64          `json:"seed"`
	NextJobID    int64          `json:"next_job_id"`
	NextSeq      int64          `json:"next_seq"`
	Completed    int64          `json:"completed"`
	Rejected     int64          `json:"rejected"`
	Initialized  bool           `json:"initialized"`
	LevelsSeeded bool           `json:"levels_seeded"`
	Events       []eventState   `json:"events"`
	Stations     []stationState `json:"stations"`
}

// eventState is the serializable form of an internal event.
type eventState struct {
	Time    float64 `json:"time"`
	Type    int     `json:"type"`
	Station int     `json:"station"`
	JobID   int64   `json:"job_id"`
	Seq     int64   `json:"seq"`
}

// stationState captures one workstation.
type stationState struct {
	Waiting []*jobState     `json:"waiting"`
	Service []*jobState     `json:"service"`
	Blocked []*blockedState `json:"blocked"`
	Stats   statsState      `json:"stats"`
}

type jobState struct {
	ID             int64   `json:"id"`
	Priority       int     `json:"priority"`
	BirthTime      float64 `json:"birth_time"`
	StationArrival float64 `json:"station_arrival"`
	ServiceStart   float64 `json:"service_start"`
	ServiceTime    float64 `json:"service_time"`
	Remaining      float64 `json:"remaining"`
	Station        int     `json:"station"`
	RoutedFrom     int     `json:"routed_from"`
	FirstService   float64 `json:"first_service"`
	WaitExtra      float64 `json:"wait_extra"`
	EnqueuedAt     float64 `json:"enqueued_at"`
	EverQueued     bool    `json:"ever_queued"`
}

type blockedState struct {
	Server int       `json:"server"`
	Job    *jobState `json:"job"`
	To     int       `json:"to"`
}

// Checkpoint returns an in-memory snapshot of the complete simulation state.
func (s *Simulator) Checkpoint() (*Checkpoint, error) {
	return &Checkpoint{state: *s.exportState()}, nil
}

// WriteCheckpoint serializes a full snapshot to w as deterministic JSON
// (map keys sorted, event list stored in processing order).
func (s *Simulator) WriteCheckpoint(w io.Writer) error {
	cp, err := s.Checkpoint()
	if err != nil {
		return err
	}
	return cp.Write(w)
}

// Write serializes the checkpoint to w.
func (c *Checkpoint) Write(w io.Writer) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(&c.state); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// Resume reconstructs a simulator from a checkpoint and its original config.
func Resume(cfg Config, cp *Checkpoint) (*Simulator, error) {
	s := &Simulator{cfg: cfg}
	if err := s.importState(&cp.state); err != nil {
		return nil, err
	}
	return s, nil
}

// ReadCheckpoint deserializes a snapshot from r and resumes it.
func ReadCheckpoint(cfg Config, r io.Reader) (*Simulator, error) {
	cp, err := ReadCheckpointData(r)
	if err != nil {
		return nil, err
	}
	return Resume(cfg, cp)
}

// ReadCheckpointData decodes a checkpoint without a config.
func ReadCheckpointData(r io.Reader) (*Checkpoint, error) {
	var st simState
	dec := json.NewDecoder(r)
	if err := dec.Decode(&st); err != nil {
		return nil, err
	}
	return &Checkpoint{state: st}, nil
}
