package qns

import "testing"

// TestE2ESimultaneousCompletions constructs two jobs already in service at
// distinct stations with completion events scheduled at exactly the same
// time, then asserts the documented tie-break: lower station index first.
func TestE2ESimultaneousCompletions(t *testing.T) {
	cfg := Config{
		Stations: []StationConfig{
			{Name: "a", Servers: 1, Policy: FCFS, Service: Constant{Value: 7}},
			{Name: "b", Servers: 1, Policy: FCFS, Service: Constant{Value: 7}},
		},
		Blocking: Reject,
		Closed: &ClosedModel{
			Population:   2,
			StartStation: 0,
			Transitions: [][]Route{
				{{To: 1, Prob: 1}},
				{{To: 0, Prob: 1}},
			},
		},
	}
	sim := New(cfg, 5)
	if err := sim.initState(); err != nil {
		t.Fatal(err)
	}
	// Rebuild the state by hand: job 10 serves station 0 and job 20 serves
	// station 1, both completing at t=7 with distinct completion events.
	jobA := &Job{ID: 10, Station: 0, StationArrival: 0, firstService: 0, ServiceStart: 0, ServiceTime: 7, Remaining: 7}
	jobB := &Job{ID: 20, Station: 1, StationArrival: 0, firstService: 0, ServiceStart: 0, ServiceTime: 7, Remaining: 7}
	sim.st[0].service[0] = jobA
	sim.st[1].service[0] = jobB
	sim.fel = newEventList()
	sim.schedule(evCompletion, 7, 0, 10)
	sim.schedule(evCompletion, 7, 1, 20)
	sim.completionOrder = []int64{}

	if err := sim.Run(7); err != nil {
		t.Fatal(err)
	}
	want := []int64{10, 20}
	if len(sim.completionOrder) < 2 ||
		sim.completionOrder[0] != want[0] || sim.completionOrder[1] != want[1] {
		t.Fatalf("same-time completions not ordered by station: got %v want prefix %v",
			sim.completionOrder, want)
	}
}
