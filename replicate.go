package qns

import (
	"sort"
	"sync"
)

// ReplicationSummary holds merged statistics across independent replications.
// Point estimates are the across-replication means; half-widths are Student-t
// 95% half-widths of the replication means. Quantiles are pooled from all
// post-warmup samples across replications.
type ReplicationSummary struct {
	Replications int
	Stations     []StationReport
	Rejected     int64
	Completed    int64
}

// RunReplications executes len(seeds) independent replications concurrently.
// Each replication gets its own Simulator, RNG stream and accumulators; no
// mutable state is shared. Results are returned in the same order as seeds.
func RunReplications(cfg Config, seeds []int64, until float64) ([]Results, ReplicationSummary, error) {
	type outcome struct {
		idx int
		res Results
		err error
	}
	results := make([]Results, len(seeds))
	outcomes := make(chan outcome, len(seeds))

	var wg sync.WaitGroup
	// A semaphore caps concurrent simulations; each one is fully independent.
	sem := make(chan struct{}, len(seeds))
	for i, seed := range seeds {
		wg.Add(1)
		go func(i int, seed int64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			sim := New(cfg, seed)
			if err := sim.Run(until); err != nil {
				outcomes <- outcome{idx: i, err: err}
				return
			}
			outcomes <- outcome{idx: i, res: sim.Results()}
		}(i, seed)
	}
	wg.Wait()
	close(outcomes)

	for o := range outcomes {
		if o.err != nil {
			return nil, ReplicationSummary{}, o.err
		}
		results[o.idx] = o.res
	}
	return results, mergeReplications(results), nil
}

// mergeReplications combines per-run results into a single summary.
func mergeReplications(runs []Results) ReplicationSummary {
	sum := ReplicationSummary{Replications: len(runs)}
	if len(runs) == 0 {
		return sum
	}
	n := len(runs[0].Stations)
	sum.Stations = make([]StationReport, n)

	for i := 0; i < n; i++ {
		merged := StationReport{
			Name:             runs[0].Stations[i].Name,
			WaitQuantiles:    map[float64]float64{},
			SojournQuantiles: map[float64]float64{},
		}
		var (
			waitPool, sojPool []float64
			throughputs       []float64
			utils             []float64
			meanWaits         []float64
			meanSojourns      []float64
			timeAvgQLs        []float64
			arrivalQLs        []float64
			timeAvgISs        []float64
		)
		for _, r := range runs {
			st := r.Stations[i]
			merged.Completed += st.Completed
			merged.Rejected += st.Rejected
			throughputs = append(throughputs, st.Throughput)
			utils = append(utils, st.Utilization)
			meanWaits = append(meanWaits, st.MeanWait)
			meanSojourns = append(meanSojourns, st.MeanSojourn)
			timeAvgQLs = append(timeAvgQLs, st.TimeAvgQueueLength)
			arrivalQLs = append(arrivalQLs, st.MeanQueueLength)
			timeAvgISs = append(timeAvgISs, st.TimeAvgInSystem)
			waitPool = append(waitPool, st.WaitSamples...)
			sojPool = append(sojPool, st.SojournSamples...)
		}

		sort.Float64s(waitPool)
		sort.Float64s(sojPool)
		for _, p := range []float64{0.5, 0.9, 0.95, 0.99} {
			merged.WaitQuantiles[p] = quantile(waitPool, p)
			merged.SojournQuantiles[p] = quantile(sojPool, p)
		}
		merged.WaitSamples = waitPool
		merged.SojournSamples = sojPool
		merged.Throughput = mean(throughputs)
		merged.Utilization = mean(utils)
		merged.MeanWait = mean(meanWaits)
		merged.MeanSojourn = mean(meanSojourns)
		merged.TimeAvgQueueLength = mean(timeAvgQLs)
		merged.MeanQueueLength = mean(arrivalQLs)
		merged.TimeAvgInSystem = mean(timeAvgISs)
		merged.WaitCI = ciFromMeans(meanWaits, merged.MeanWait)
		merged.SojournCI = ciFromMeans(meanSojourns, merged.MeanSojourn)
		merged.QueueLengthCI = ciFromMeans(timeAvgQLs, merged.TimeAvgQueueLength)
		sum.Stations[i] = merged
	}
	for _, r := range runs {
		sum.Completed += r.Completed
		sum.Rejected += r.Rejected
	}
	return sum
}
