package qsim

// FlowModel 为流量模型。
type FlowModel int

const (
	// Open 开放模型：外部到达进入系统，按转移概率路由或离开。
	Open FlowModel = iota
	// Closed 封闭模型：任务总数固定，无外部到达。
	Closed
)

// NetworkConfig 描述工作站网络的静态拓扑与流量参数（骨架，后续填充）。
type NetworkConfig struct {
	Model          FlowModel
	Stations       []StationConfig
	Routing        [][]float64 // Routing[i][j]：从工作站 i 转移到 j 的概率
	ExternalArrive []float64   // 开放模型：外部到达各工作站的速率
	Population     int         // 封闭模型：常驻任务总数
}

// Network 是网络的运行时结构（骨架）。
type Network struct {
	cfg *NetworkConfig
}
