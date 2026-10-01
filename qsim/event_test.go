package qsim

import "testing"

func TestCalendarOrdering(t *testing.T) {
	c := NewCalendar()
	c1 := &Customer{ID: 1, Priority: 0}
	c2 := &Customer{ID: 2, Priority: 0}
	c3 := &Customer{ID: 3, Priority: 0}

	c.Push(&Event{Type: evCompletion, Time: 5, Station: 0, Customer: c1})
	c.Push(&Event{Type: EvArrival, Time: 1, Station: 0, Customer: c2})
	c.Push(&Event{Type: EvArrival, Time: 5, Station: 0, Customer: c3})

	got := c.Pop()
	if got.Time != 1 || got.Customer.ID != 2 {
		t.Fatalf("first event = t%v id%d, want t1 id2", got.Time, got.Customer.ID)
	}
	got = c.Pop()
	if got.Type != EvArrival || got.Customer.ID != 3 {
		t.Fatalf("same-time arrival should precede completion, got %+v", got)
	}
	got = c.Pop()
	if got.Type != evCompletion || got.Customer.ID != 1 {
		t.Fatalf("last should be completion id1, got %+v", got)
	}
}

func TestCalendarSameTimeStationAndPriority(t *testing.T) {
	c := NewCalendar()
	hi := &Customer{ID: 1, Priority: 0}
	lo := &Customer{ID: 2, Priority: 5}
	c.Push(&Event{Type: EvArrival, Time: 3, Station: 1, Customer: lo})
	c.Push(&Event{Type: EvArrival, Time: 3, Station: 0, Customer: lo})
	c.Push(&Event{Type: EvArrival, Time: 3, Station: 1, Customer: hi})

	wantStations := []int{0, 1, 1}
	wantIDs := []uint64{2, 1, 2}
	for i := range wantStations {
		e := c.Pop()
		if e.Station != wantStations[i] || e.Customer.ID != wantIDs[i] {
			t.Fatalf("step %d got station %d id%d, want station %d id%d",
				i, e.Station, e.Customer.ID, wantStations[i], wantIDs[i])
		}
	}
}

func TestCalendarRestoreHeap(t *testing.T) {
	c := NewCalendar()
	for i := uint64(1); i <= 5; i++ {
		c.Push(&Event{Type: EvArrival, Time: float64(i * 2), Customer: &Customer{ID: i}})
	}
	items := c.Items()
	seq := c.Seq()

	c2 := NewCalendar()
	c2.RestoreItems(items, seq)
	c2.Push(&Event{Type: EvArrival, Time: 1, Customer: &Customer{ID: 99}})
	e := c2.Pop()
	if e.Customer.ID != 99 {
		t.Fatalf("after restore, head id = %d, want 99", e.Customer.ID)
	}
	if e2 := c2.Pop(); e2.Customer.ID != 1 {
		t.Fatalf("next id = %d, want 1", e2.Customer.ID)
	}
}
