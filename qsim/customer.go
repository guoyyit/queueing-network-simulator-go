package qsim

// Customer 是网络中的任务（顾客）实体。
type Customer struct {
	ID          uint64
	Class       int
	Priority    int
	BirthTime   float64 // 进入（或在封闭模型中创建）系统的时刻
	ArrivalTime float64 // 到达当前工作站的时刻
	Remaining   float64 // 当前工作站剩余服务时间（用于抢占恢复）
	Station     int     // 当前/目标工作站编号
	Preempted   bool    // 是否为被抢占后等待恢复的任务
}
