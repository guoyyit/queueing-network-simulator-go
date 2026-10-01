package qns

// Route is one probabilistic routing target.
type Route struct {
	To   int
	Prob float64
}

// BlockingMode defines what happens when a job targets a full queue.
type BlockingMode int

const (
	// Reject rejects the arriving job and counts it in rejection statistics.
	Reject BlockingMode = iota
	// Block keeps the sender server occupied ("blocked") until space appears.
	Block
)

// OpenModel describes externally arriving traffic.
type OpenModel struct {
	Arrivals    ArrivalProcess
	Entry       []Route   // probability distribution over entry stations
	Transitions [][]Route // per-station probabilistic routes; residual mass leaves
}

// ClosedModel describes a fixed population with no external arrivals.
type ClosedModel struct {
	Population   int
	StartStation int
	Transitions  [][]Route // must have zero residual mass (no leaving)
}

// Config is the full network/simulation configuration.
type Config struct {
	Stations []StationConfig
	Blocking BlockingMode
	Open     *OpenModel
	Closed   *ClosedModel
	Warmup   float64 // statistics-collection warm-up period
	Batches  int     // number of batches for batch-means variance estimation
}
