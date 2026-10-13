package domain

import "testing"

func TestMaxWeightFromSets(t *testing.T) {
	t.Parallel()
	if MaxWeightFromSets([]SetEntry{{Reps: 10, Weight: 50}, {Reps: 8, Weight: 52.5}}) != 52.5 {
		t.Fatal("expected max 52.5")
	}
	if MaxWeightFromSets(nil) != 0 {
		t.Fatal("expected 0 for empty")
	}
}

func TestWorkingWeightFromSets(t *testing.T) {
	t.Parallel()
	got := WorkingWeightFromSets([]SetEntry{{Reps: 10, Weight: 50}, {Reps: 8, Weight: 52.5}})
	want := (10*50.0 + 8*52.5) / (10 + 8)
	if got != want {
		t.Fatalf("got %v want %v", got, want)
	}
	if WorkingWeightFromSets(nil) != 0 {
		t.Fatal("expected 0 for empty")
	}
}

func TestVolumeFromSets(t *testing.T) {
	t.Parallel()
	if got := VolumeFromSets([]SetEntry{{Reps: 10, Weight: 50}, {Reps: 8, Weight: 52.5}}); got != 920 {
		t.Fatalf("got %v want 920", got)
	}
}

func TestSetEntryValid(t *testing.T) {
	t.Parallel()
	valid := []SetEntry{{Reps: 1, Weight: 0}, {Reps: 12, Weight: 52.5}}
	for _, s := range valid {
		if !s.Valid() {
			t.Fatalf("%+v must be valid", s)
		}
	}
	invalid := []SetEntry{{Reps: 0, Weight: 50}, {Reps: 10, Weight: -1}, {Reps: MaxReps + 1, Weight: 1}, {Reps: 1, Weight: MaxWeight + 1}}
	for _, s := range invalid {
		if s.Valid() {
			t.Fatalf("%+v must be invalid", s)
		}
	}
}
