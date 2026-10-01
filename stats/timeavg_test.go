package stats

import "testing"

func TestTimeAverageBasic(t *testing.T) {
	ta := NewTimeAverage()
	ta.Update(0, 0)
	ta.Update(2, 3)
	ta.Update(5, 1)
	ta.Close(10)
	// 面积 = 0*2 + 3*3 + 1*5 = 14，均值 = 1.4
	if got := ta.Mean(); got != 1.4 {
		t.Fatalf("mean = %v, want 1.4", got)
	}
	if got := ta.IntervalMean(2, 5); got != 3 {
		t.Fatalf("interval [2,5) mean = %v, want 3", got)
	}
	if got := ta.IntervalMean(0, 2); got != 0 {
		t.Fatalf("interval [0,2) mean = %v, want 0", got)
	}
}

func TestTimeAverageSameTimeWins(t *testing.T) {
	ta := NewTimeAverage()
	ta.Update(0, 1)
	ta.Update(0, 2) // 同刻改写
	ta.Update(3, 5)
	ta.Close(4)
	// [0,3)=2, [3,4)=5 => (6+5)/4 = 2.75
	if got := ta.Mean(); got != 2.75 {
		t.Fatalf("mean = %v, want 2.75", got)
	}
}

func TestTimeAverageRestore(t *testing.T) {
	ta := NewTimeAverage()
	ta.Update(0, 0)
	ta.Update(1, 4)
	st := ta.State()

	ta2 := NewTimeAverage()
	ta2.RestoreState(st)
	ta2.Update(3, 0)
	ta2.Close(4)
	// [0,1)=0, [1,3)=4, [3,4)=0 => 8/4 = 2
	if got := ta2.Mean(); got != 2 {
		t.Fatalf("restored mean = %v, want 2", got)
	}
}
