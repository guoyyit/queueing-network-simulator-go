package qsim

// Policy 为工作站队列调度策略。
type Policy int

const (
	// FCFS 先到先服务。
	FCFS Policy = iota
	// LCFS 后到先服务（非抢占）。
	LCFS
	// PriorityNP 优先级非抢占：数值越小优先级越高。
	PriorityNP
	// PriorityPP 优先级抢占：高优先级到达可抢占正在服务的低优先级任务。
	PriorityPP
)
