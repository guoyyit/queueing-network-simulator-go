// Command qsimdemo 在本地运行一个两台工作站的开放排队网络仿真，
// 打印稳态指标、批均值置信区间，并演示检查点续跑与并行复制。
package main

import (
	"fmt"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
	"github.com/guoyyit/queueing-network-simulator-go/experiment"
	"github.com/guoyyit/queueing-network-simulator-go/qsim"
)

func main() {
	cfg := &qsim.Config{
		Blocking:   qsim.Block,
		Seed:       20261002,
		Warmup:     2000,
		EndTime:    20000,
		BatchCount: 20,
		Confidence: 0.95,
		Network: &qsim.NetworkConfig{
			Model:          qsim.Open,
			ExternalArrive: []float64{4.0, 0.0},
			Routing: [][]float64{
				{0.2, 0.7}, // 工作站0：20% 离开，70% 去工作站1，其余离开
				{0.1, 0.0}, // 工作站1：10% 返回工作站0，其余离开
			},
			Stations: []qsim.StationConfig{
				{Name: "gateway", Servers: 2, Capacity: -1, Policy: qsim.FCFS,
					Service: dist.Exponential{Rate: 3.0}},
				{Name: "worker", Servers: 3, Capacity: 50, Policy: qsim.PriorityNP,
					Service: dist.Erlang{K: 2, Rate: 4.0}},
			},
		},
	}

	sim := qsim.New(cfg)
	sim.Run(0)
	rep := sim.Report()

	fmt.Println("== 单次连续运行（开放网络，发送方阻断）==")
	fmt.Printf("拒绝=%d 离开=%d\n\n", rep.Rejected, rep.Completed)
	for _, st := range rep.Stations {
		fmt.Printf("工作站 %s\n", st.Name)
		fmt.Printf("  吞吐=%.4f  利用率=%.4f\n", st.Throughput, st.Utilization)
		fmt.Printf("  平均排队=%.4f  平均逗留=%.4f\n", st.MeanWait, st.MeanSojourn)
		fmt.Printf("  p50/p90/p95/p99 逗留=%.3f/%.3f/%.3f/%.3f\n",
			st.SojournQuantiles[0], st.SojournQuantiles[1],
			st.SojournQuantiles[2], st.SojournQuantiles[3])
		fmt.Printf("  时间平均等待队长=%.4f  时间平均站内人数=%.4f\n",
			st.MeanQueueLength, st.MeanNumberInSystem)
		fmt.Printf("  排队时长 95%%CI=%.4f ± %.4f\n", st.WaitCI.Mean, st.WaitCI.HalfWidth)
		fmt.Printf("  利用率   95%%CI=%.4f ± %.4f\n\n", st.UtilCI.Mean, st.UtilCI.HalfWidth)
	}

	// 检查点：在约 1/3 处保存并恢复续跑，结果应与连续运行一致。
	cut := cfg.Warmup + (cfg.EndTime-cfg.Warmup)/3
	part := qsim.New(cfg)
	part.StepUntil(cut)
	snap, _ := part.Save()
	data, _ := snap.Marshal()
	loaded, _ := qsim.UnmarshalSnapshot(data)
	resumed, _ := qsim.RestoreSnapshot(loaded)
	resumed.Run(0)
	r2 := resumed.Report()
	match := r2.Completed == rep.Completed && r2.Rejected == rep.Rejected
	fmt.Printf("== 检查点续跑一致性 ==\n一致=%v（连续离开=%d，续跑离开=%d）\n\n",
		match, rep.Completed, r2.Completed)

	// 并行复制
	sum, err := experiment.RunAndAggregate(cfg, 12)
	if err != nil {
		panic(err)
	}
	fmt.Println("== 12 个独立复制（并发）合并 ==")
	for _, st := range sum.Stations {
		fmt.Printf("  %s: 利用率=%.4f±%.4f 平均逗留=%.4f±%.4f\n",
			st.Name, st.MeanUtilization, 1.96*st.StdUtilization,
			st.MeanSojourn, 1.96*st.StdSojourn)
	}
}
