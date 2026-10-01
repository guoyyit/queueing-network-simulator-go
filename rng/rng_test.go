package rng

import "testing"

func TestReproducible(t *testing.T) {
	a := New(42)
	b := New(42)
	for i := 0; i < 1000; i++ {
		if x, y := a.Uint64(), b.Uint64(); x != y {
			t.Fatalf("same seed diverged at %d: %d != %d", i, x, y)
		}
	}
}

func TestRestoreContinuesIdentically(t *testing.T) {
	full := New(7)
	var head []uint64
	for i := 0; i < 100; i++ {
		head = append(head, full.Uint64())
	}

	part := New(7)
	for i := 0; i < 100; i++ {
		part.Uint64()
	}
	snap := part.StateOf()
	resumed := Restore(snap)

	for i := 0; i < 500; i++ {
		want := full.Uint64()
		if got := resumed.Uint64(); got != want {
			t.Fatalf("restored stream diverged at %d: %d != %d", i, got, want)
		}
	}

	if resumed.Position() != 600 {
		t.Fatalf("position = %d, want 600", resumed.Position())
	}
}

func TestDifferentSeedsDiffer(t *testing.T) {
	if New(1).Uint64() == New(2).Uint64() {
		t.Fatal("different seeds produced identical first draw")
	}
}

func TestFloat64Range(t *testing.T) {
	r := New(99)
	for i := 0; i < 10000; i++ {
		v := r.Float64()
		if v < 0 || v >= 1 {
			t.Fatalf("Float64 out of range: %v", v)
		}
	}
}

func TestChildSeedDistinct(t *testing.T) {
	seen := map[uint64]bool{}
	for i := 0; i < 16; i++ {
		z := ChildSeed(123, i)
		if seen[z] {
			t.Fatalf("duplicate child seed at %d", i)
		}
		seen[z] = true
	}
}
