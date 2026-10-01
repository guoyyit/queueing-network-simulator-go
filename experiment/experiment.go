// Package experiment 提供排队网络仿真的并行独立复制与跨复制统计合并。
package experiment

import (
	"fmt"
	"math"
	"sync"

	"github.com/guoyyit/queueing-network-simulator-go/qsim"
	"github.com/guoyyit/queueing-network-simulator-go/rng"
)

// ReplicationResult 是单次复制的结果。
type ReplicationResult struct {
	Index  int
	Seed   uint64
	Report *qsim.SimulationReport
}

// StationSummary 是某工作站跨复制合并后的统计。
type StationSummary struct {
	Name string
	N    int

	MeanThroughput  float64
	MeanUtilization float64
	MeanWait        float64
	MeanSojourn     float64
	MeanQueueLength float64
	MeanNumber      float64

	// 跨复制各复制点估计的样本标准差。
	StdThroughput  float64
	StdUtilization float64
	StdWait        float64
	StdSojourn     float64
	StdQueueLength float64
	StdNumber      float64

	TotalRejected  int64
	TotalCompleted int64

	// 内部：平方偏差累加
	s2Throughput  float64
	s2Utilization float64
	s2Wait        float64
	s2Sojourn     float64
	s2QueueLength float64
	s2Number      float64
}

// Summary 是整组复制的合并结果。
type Summary struct {
	Replications int
	Stations     []StationSummary
	Results      []ReplicationResult
}

// cloneConfig 深拷贝配置，使各复制不共享任何可变对象。
// 网络结构（数值矩阵）逐片复制；分布为无状态值类型，直接复用。
func cloneConfig(cfg *qsim.Config) *qsim.Config {
	c := *cfg
	if cfg.Network != nil {
		n := *cfg.Network
		n.Stations = append([]qsim.StationConfig(nil), cfg.Network.Stations...)
		if cfg.Network.Routing != nil {
			n.Routing = make([][]float64, len(cfg.Network.Routing))
			for i, row := range cfg.Network.Routing {
				n.Routing[i] = append([]float64(nil), row...)
			}
		}
		n.ExternalArrive = append([]float64(nil), cfg.Network.ExternalArrive...)
		c.Network = &n
	}
	return &c
}

// Run 并发执行 replications 个相互独立的复制并返回每次的结果。
// 每个复制：
//   - 使用 rng.ChildSeed 派生的不同种子（相同实验种子下整体可复现）；
//   - 获得配置的深拷贝，各自构造独立仿真器，不共享可变状态或随机流。
func Run(cfg *qsim.Config, replications int) ([]ReplicationResult, error) {
	if replications <= 0 {
		return nil, fmt.Errorf("experiment: replications must be positive, got %d", replications)
	}
	results := make([]ReplicationResult, replications)
	var wg sync.WaitGroup
	for i := 0; i < replications; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			seed := rng.ChildSeed(cfg.Seed, i)
			rc := cloneConfig(cfg)
			rc.Seed = seed
			sim := qsim.New(rc)
			sim.Run(0)
			results[i] = ReplicationResult{Index: i, Seed: seed, Report: sim.Report()}
		}(i)
	}
	wg.Wait()
	return results, nil
}

// Aggregate 合并多次复制的点估计，给出跨复制均值与样本标准差。
func Aggregate(results []ReplicationResult) *Summary {
	if len(results) == 0 {
		return &Summary{}
	}
	nStation := len(results[0].Report.Stations)
	sum := &Summary{Replications: len(results), Results: results,
		Stations: make([]StationSummary, nStation)}

	for k := 0; k < nStation; k++ {
		sum.Stations[k].Name = results[0].Report.Stations[k].Name
	}

	// 累加和
	for _, r := range results {
		for k := range r.Report.Stations {
			st := r.Report.Stations[k]
			s := &sum.Stations[k]
			s.N++
			s.MeanThroughput += st.Throughput
			s.MeanUtilization += st.Utilization
			s.MeanWait += st.MeanWait
			s.MeanSojourn += st.MeanSojourn
			s.MeanQueueLength += st.MeanQueueLength
			s.MeanNumber += st.MeanNumberInSystem
			s.TotalRejected += st.Rejected
			s.TotalCompleted += st.Completed
		}
	}
	for k := range sum.Stations {
		s := &sum.Stations[k]
		n := float64(s.N)
		s.MeanThroughput /= n
		s.MeanUtilization /= n
		s.MeanWait /= n
		s.MeanSojourn /= n
		s.MeanQueueLength /= n
		s.MeanNumber /= n
	}

	// 样本标准差（每工作站独立）
	for _, r := range results {
		for k := range r.Report.Stations {
			st := r.Report.Stations[k]
			s := &sum.Stations[k]
			s.s2Throughput += (st.Throughput - s.MeanThroughput) * (st.Throughput - s.MeanThroughput)
			s.s2Utilization += (st.Utilization - s.MeanUtilization) * (st.Utilization - s.MeanUtilization)
			s.s2Wait += (st.MeanWait - s.MeanWait) * (st.MeanWait - s.MeanWait)
			s.s2Sojourn += (st.MeanSojourn - s.MeanSojourn) * (st.MeanSojourn - s.MeanSojourn)
			s.s2QueueLength += (st.MeanQueueLength - s.MeanQueueLength) * (st.MeanQueueLength - s.MeanQueueLength)
			s.s2Number += (st.MeanNumberInSystem - s.MeanNumber) * (st.MeanNumberInSystem - s.MeanNumber)
		}
	}
	denom := 1.0
	if len(results) > 1 {
		denom = float64(len(results) - 1)
	}
	for k := range sum.Stations {
		s := &sum.Stations[k]
		s.StdThroughput = math.Sqrt(s.s2Throughput / denom)
		s.StdUtilization = math.Sqrt(s.s2Utilization / denom)
		s.StdWait = math.Sqrt(s.s2Wait / denom)
		s.StdSojourn = math.Sqrt(s.s2Sojourn / denom)
		s.StdQueueLength = math.Sqrt(s.s2QueueLength / denom)
		s.StdNumber = math.Sqrt(s.s2Number / denom)
	}
	return sum
}

// RunAndAggregate 并发执行复制并直接返回合并摘要。
func RunAndAggregate(cfg *qsim.Config, replications int) (*Summary, error) {
	results, err := Run(cfg, replications)
	if err != nil {
		return nil, err
	}
	return Aggregate(results), nil
}
