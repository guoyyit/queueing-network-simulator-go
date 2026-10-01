package qsim

import (
	"testing"

	"github.com/guoyyit/queueing-network-simulator-go/dist"
)

func TestCheckpointResumeIdentical(t *testing.T) {
	cfg := mm1Config(4242, 4, 5, 400, 40)
	cfg.BatchCount = 10

	// 连续运行
	cont := New(cfg)
	cont.Run(0)
	want := cont.Report()

	// 分段运行：跑到中点保存，恢复后续跑
	seg := New(cfg)
	mid := 200.0
	for {
		e := seg.cal.Peek()
		if e == nil || e.Time >= mid {
			break
		}
		seg.Step()
	}
	seg.now = mid
	snap, err := seg.Save()
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := snap.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	loaded, err := UnmarshalSnapshot(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resumed, err := RestoreSnapshot(loaded)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	resumed.Run(0)
	got := resumed.Report()

	if !reportsEqual(want, got) {
		t.Fatal("checkpoint-resumed report differs from continuous run")
	}
	if got.Rejected != want.Rejected || got.Completed != want.Completed {
		t.Fatal("reject/complete counts differ after resume")
	}
}

func TestCheckpointMultipleCuts(t *testing.T) {
	cfg := func() *Config { return mm1Config(555, 4, 5, 300, 30) }
	cont := New(cfg())
	cont.Run(0)
	want := cont.Report()

	seg := New(cfg())
	cuts := []float64{37, 91, 150, 222}
	for _, cut := range cuts {
		for {
			e := seg.cal.Peek()
			if e == nil || e.Time >= cut {
				break
			}
			seg.Step()
		}
		seg.now = cut
		snap, _ := seg.Save()
		data, _ := snap.Marshal()
		loaded, _ := UnmarshalSnapshot(data)
		seg, _ = RestoreSnapshot(loaded)
	}
	seg.Run(0)
	if !reportsEqual(want, seg.Report()) {
		t.Fatal("multi-cut checkpoint resume differs")
	}
}

func TestCheckpointBlocking(t *testing.T) {
	build := func() *Config {
		return &Config{
			Blocking: Block, Seed: 3, EndTime: 80, Warmup: 0,
			Network: &NetworkConfig{
				Model:          Open,
				ExternalArrive: []float64{0.8, 0},
				Routing: [][]float64{
					{0, 1},
					{0, 0},
				},
				Stations: []StationConfig{
					{Name: "src", Servers: 1, Capacity: -1, Policy: FCFS,
						Service: dist.Deterministic{Value: 1}},
					{Name: "sink", Servers: 1, Capacity: 1, Policy: FCFS,
						Service: dist.Deterministic{Value: 4}},
				},
			},
		}
	}
	cont := New(build())
	cont.Run(0)
	want := cont.Report()

	seg := New(build())
	for _, cut := range []float64{5, 12.5, 30, 55} {
		for {
			e := seg.cal.Peek()
			if e == nil || e.Time >= cut {
				break
			}
			seg.Step()
		}
		seg.now = cut
		snap, _ := seg.Save()
		data, _ := snap.Marshal()
		loaded, _ := UnmarshalSnapshot(data)
		seg, _ = RestoreSnapshot(loaded)
	}
	seg.Run(0)
	got := seg.Report()
	if got.Rejected != want.Rejected || got.Completed != want.Completed {
		t.Fatalf("blocking checkpoint differs: rej %d/%d done %d/%d",
			got.Rejected, want.Rejected, got.Completed, want.Completed)
	}
	for i := range want.Stations {
		if want.Stations[i].Completed != got.Stations[i].Completed {
			t.Fatalf("station %d completions differ: %d/%d",
				i, got.Stations[i].Completed, want.Stations[i].Completed)
		}
	}
}
