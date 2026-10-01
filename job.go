package qns

// Job is a single customer/task moving through the network.
type Job struct {
	ID             int64
	Priority       int
	BirthTime      float64 // time the job entered the network
	StationArrival float64 // time the job entered its current station
	ServiceStart   float64 // time the current service segment began
	ServiceTime    float64 // total service time of the current segment
	Remaining      float64 // remaining service time of the current segment
	Station        int     // current station index
	routedFrom     int     // sender server index when routed; -1 for external arrivals
	firstService   float64 // first time service began for this visit
	waitExtra      float64 // accumulated queueing time after preemption(s)
	enqueuedAt     float64 // time the job last entered the waiting room
	everQueued     bool    // whether the job spent time in the waiting room
}
