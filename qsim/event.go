package qsim

import "container/heap"

// EventType 标识仿真事件种类。
type EventType int

const (
	// EvArrival 到达事件（外部到达或阻断解除后转入目标工作站）。
	EvArrival EventType = iota
	// EvCompletion 服务完成事件。
	evCompletion
)

func (t EventType) order() int {
	if t == EvArrival {
		return 0 // 同刻先处理到达，再处理完成
	}
	return 1
}

// Event 是未来事件表中的一条事件（骨架）。
type Event struct {
	Type     EventType
	Time     float64
	Station  int
	Customer *Customer
	Server   int
	Seq      uint64 // 同刻排序序号（入表时分配，单调递增）
}

// less 定义事件的确定性先后次序：
//  1. 时刻更早者先；
//  2. 同刻：到达事件先于完成事件；
//  3. 再同：工作站编号小者先；
//  4. 再同：顾客优先级数值小者（高优先级）先；
//  5. 再同：顾客 ID 小者先；
//  6. 最后以入表序号 Seq 兜底（入表顺序本身也是确定的）。
func (e *Event) less(o *Event) bool {
	if e.Time != o.Time {
		return e.Time < o.Time
	}
	if a, b := e.Type.order(), o.Type.order(); a != b {
		return a < b
	}
	if e.Station != o.Station {
		return e.Station < o.Station
	}
	ep, op := customerPriority(e.Customer), customerPriority(o.Customer)
	if ep != op {
		return ep < op
	}
	id1, id2 := customerID(e.Customer), customerID(o.Customer)
	if id1 != id2 {
		return id1 < id2
	}
	return e.Seq < o.Seq
}

func customerPriority(c *Customer) int {
	if c == nil {
		return 0
	}
	return c.Priority
}

func customerID(c *Customer) uint64 {
	if c == nil {
		return 0
	}
	return c.ID
}

// Calendar 是按上述规则排序的未来事件表（最小堆）。
type Calendar struct {
	items []*Event
	next  uint64
}

// NewCalendar 创建空事件表。
func NewCalendar() *Calendar { return &Calendar{} }

// Push 将事件加入表中，并为其分配单调递增的同刻兜底序号。
func (c *Calendar) Push(e *Event) {
	c.next++
	e.Seq = c.next
	heap.Push((*eventHeap)(c), e)
}

// Pop 取出并返回当前最优先的事件。
func (c *Calendar) Pop() *Event {
	if len(c.items) == 0 {
		return nil
	}
	return heap.Pop((*eventHeap)(c)).(*Event)
}

// Peek 返回但不移除当前最优先的事件。
func (c *Calendar) Peek() *Event {
	if len(c.items) == 0 {
		return nil
	}
	return c.items[0]
}

// Len 返回事件数量。
func (c *Calendar) Len() int { return len(c.items) }

// Remove 从表中删除指定事件指针（保留其余事件的 Seq 不变）。
// 用于取消在途完成事件（抢占时）。找不到时返回 false。
func (c *Calendar) Remove(target *Event) bool {
	idx := -1
	for i, e := range c.items {
		if e == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	heap.Remove((*eventHeap)(c), idx)
	return true
}

// findAndRemove 删除第一个满足 pred 的事件（保留其余事件 Seq），返回是否删除。
func (c *Calendar) findAndRemove(pred func(*Event) bool) bool {
	for _, e := range c.items {
		if pred(e) {
			return c.Remove(e)
		}
	}
	return false
}

// Items 返回事件快照（供检查点序列化）。
func (c *Calendar) Items() []*Event {
	out := make([]*Event, len(c.items))
	copy(out, c.items)
	return out
}

// RestoreItems 用事件快照重建事件表（重新堆化，保留原 Seq）。
func (c *Calendar) RestoreItems(items []*Event, seq uint64) {
	c.items = make([]*Event, len(items))
	copy(c.items, items)
	c.next = seq
	heap.Init((*eventHeap)(c))
}

// PushRaw 直接插入事件而不重新分配 Seq（检查点恢复用）。
func (c *Calendar) PushRaw(e *Event) {
	heap.Push((*eventHeap)(c), e)
}

// Seq 返回已分配到的最大序号。
func (c *Calendar) Seq() uint64 { return c.next }

// RestoreSeq 恢复序号计数器（检查点恢复用）。
func (c *Calendar) RestoreSeq(seq uint64) { c.next = seq }

// eventHeap 为 Calendar 实现 heap.Interface。
type eventHeap Calendar

func (h eventHeap) Len() int { return len(h.items) }

func (h eventHeap) Less(i, j int) bool { return h.items[i].less(h.items[j]) }

func (h eventHeap) Swap(i, j int) { h.items[i], h.items[j] = h.items[j], h.items[i] }

func (h *eventHeap) Push(x any) { h.items = append(h.items, x.(*Event)) }

func (h *eventHeap) Pop() any {
	n := len(h.items)
	it := h.items[n-1]
	h.items = h.items[:n-1]
	return it
}
