package domain

import (
	"testing"
	"time"
)

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

func TestBandAndBodyweightSets(t *testing.T) {
	t.Parallel()
	sets := []SetEntry{
		{Reps: 10, Weight: 50},
		{Reps: 15, Kind: LoadBand, Band: 4},
		{Reps: 20, Kind: LoadBodyweight},
		{Reps: 8, Weight: 5, Kind: LoadBodyweight},
	}
	if got := MaxWeightFromSets(sets); got != 50 {
		t.Fatalf("max weight must ignore bands and bare bodyweight: %v", got)
	}
	// Only counted sets weigh in: 10×50 and 8×5.
	if got, want := WorkingWeightFromSets(sets), (10*50.0+8*5)/18; got != want {
		t.Fatalf("working weight: got %v want %v", got, want)
	}
	if got := VolumeFromSets(sets); got != 540 {
		t.Fatalf("volume: got %v want 540", got)
	}
	p := NewProgressPoint(7, time.Now(), sets)
	if p.TotalReps != 53 || p.MaxReps != 20 || p.Band != 4 || p.WorkoutID != 7 {
		t.Fatalf("unexpected point: %+v", p)
	}
}

func TestSetEntryKinds(t *testing.T) {
	t.Parallel()
	valid := []SetEntry{
		{Reps: 10, Weight: 50},
		{Reps: 10, Weight: 50, Kind: LoadWeight},
		{Reps: 15, Kind: LoadBodyweight},
		{Reps: 8, Weight: 5, Kind: LoadBodyweight},
		{Reps: 12, Kind: LoadBand, Band: 1},
		{Reps: 12, Kind: LoadBand, Band: MaxBand},
	}
	for _, s := range valid {
		if !s.Valid() {
			t.Fatalf("%+v must be valid", s)
		}
	}
	invalid := []SetEntry{
		{Reps: 12, Kind: LoadBand},
		{Reps: 12, Kind: LoadBand, Band: MaxBand + 1},
		{Reps: 12, Weight: 5, Kind: LoadBand, Band: 3},
		{Reps: 12, Weight: 5, Band: 3},
		{Reps: 12, Kind: "kettlebell"},
		{Reps: 12, Kind: LoadBodyweight, Weight: -1},
	}
	for _, s := range invalid {
		if s.Valid() {
			t.Fatalf("%+v must be invalid", s)
		}
	}
	if (SetEntry{Reps: 1, Weight: 5, Kind: LoadWeight}).Normalize() != (SetEntry{Reps: 1, Weight: 5}) {
		t.Fatal("the weight kind is written as an empty kind")
	}
}

func TestDominantLoadAndChartSeries(t *testing.T) {
	t.Parallel()
	if DominantLoad(nil) != LoadWeight {
		t.Fatal("no sets means weight")
	}
	if got := DominantLoad([]SetEntry{{Reps: 5, Weight: 5}, {Reps: 5, Kind: LoadBand, Band: 3}, {Reps: 5, Kind: LoadBand, Band: 3}}); got != LoadBand {
		t.Fatalf("most sets win, got %v", got)
	}
	if got := DominantLoad([]SetEntry{{Reps: 5, Kind: LoadBand, Band: 3}, {Reps: 5, Weight: 5}}); got != LoadBand {
		t.Fatalf("a tie goes to the first set, got %v", got)
	}

	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	point := func(i int, k LoadKind) ExerciseProgressPoint {
		return ExerciseProgressPoint{WorkoutID: i, Date: day.AddDate(0, 0, i), Kind: k}
	}
	// The latest kind has enough points: only they are drawn.
	kind, got := ChartSeries([]ExerciseProgressPoint{point(1, LoadWeight), point(2, LoadBand), point(3, LoadWeight), point(4, LoadBand)})
	if kind != LoadBand || len(got) != 2 || got[0].WorkoutID != 2 {
		t.Fatalf("latest kind: %v %+v", kind, got)
	}
	// The latest kind has one point: fall back to the one with more.
	kind, got = ChartSeries([]ExerciseProgressPoint{point(1, LoadWeight), point(2, LoadWeight), point(3, LoadBand)})
	if kind != LoadWeight || len(got) != 2 {
		t.Fatalf("fallback: %v %+v", kind, got)
	}
	if _, got = ChartSeries(nil); len(got) != 0 {
		t.Fatal("no points, no series")
	}
}

func TestExerciseKey(t *testing.T) {
	t.Parallel()
	same := [][]string{
		{"Подтягивания", "подтягивания", " Подтягивания  "},
		{"Подъёмы на икры", "подъемы на икры", "ПОДЪЁМЫ НА ИКРЫ."},
		{"Жим гантель на скамье под 45", "Жим гантель на скамье под 45°", "жим-гантель на скамье под  45"},
		{"Glutes isolator", "glutes Isolator"},
	}
	for _, names := range same {
		for _, n := range names[1:] {
			if ExerciseKey(n) != ExerciseKey(names[0]) {
				t.Fatalf("%q and %q must be one exercise: %q vs %q", names[0], n, ExerciseKey(names[0]), ExerciseKey(n))
			}
		}
	}
	for _, pair := range [][2]string{
		{"Подтягивания", "Негативные подтягивания"},
		{"Тяга верхнего блока", "Тяга нижнего блока"},
		{"Жим под 30 в тренажере", "Жим под 45 в тренажере"},
	} {
		if ExerciseKey(pair[0]) == ExerciseKey(pair[1]) {
			t.Fatalf("%q and %q are different exercises", pair[0], pair[1])
		}
	}
	e := Exercise{}
	if !e.Allows(LoadWeight) || e.Allows(LoadBand) || len(e.Offered()) != 1 {
		t.Fatal("an exercise without loads is weight only")
	}
	e.Loads = []LoadKind{LoadBodyweight, LoadBand}
	if !e.Allows(LoadBand) || e.Allows(LoadWeight) || e.Offered()[0] != LoadBodyweight {
		t.Fatal("unexpected loads")
	}
}
