package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

func (f fixture) workoutAt(t *testing.T, at time.Time, exercises map[int][]domain.SetEntry) domain.Workout {
	t.Helper()
	ctx := context.Background()
	id, err := f.store.CreateImportedWorkout(ctx, testUser, at)
	if err != nil {
		t.Fatal(err)
	}
	for ex, set := range exercises {
		if err := f.store.AddExerciseToWorkout(ctx, id, ex, set); err != nil {
			t.Fatal(err)
		}
	}
	w, err := f.store.GetWorkoutForUser(ctx, id, testUser)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestCatalogReportAndMerge(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	day := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	old := f.store.AddExercise(f.legs.ID, "подтягивания")
	pull := f.store.AddExercise(f.legs.ID, "Подтягивания", domain.LoadWeight, domain.LoadBand)
	plankOld := f.store.AddExercise(f.legs.ID, "планка", domain.LoadBodyweight)
	plank := f.store.AddExercise(f.legs.ID, "Планка")
	lying := f.store.AddExercise(f.legs.ID, "Сгибания ног лежа")
	f.store.AddExercise(f.legs.ID, "Сгибания ног сидя")
	f.store.AddExercise(f.legs.ID, "Сгибание ног сидя")
	f.store.AddExercise(f.legs.ID, "Тяга верхнего блока")
	f.store.AddExercise(f.legs.ID, "Тяга нижнего блока")

	// "подтягивания" was done in a workout that also has "Подтягивания": their sets must be joined.
	both := f.workoutAt(t, day, map[int][]domain.SetEntry{old.ID: sets(10, 20, 8, 20), pull.ID: sets(12, 15)})
	oldOnly := f.workoutAt(t, day.AddDate(0, 0, 3), map[int][]domain.SetEntry{old.ID: sets(9, 25), lying.ID: sets(10, 30)})
	f.workoutAt(t, day.AddDate(0, 0, 6), map[int][]domain.SetEntry{pull.ID: sets(7, 30)})
	f.workoutAt(t, day.AddDate(0, 0, 7), map[int][]domain.SetEntry{plankOld.ID: {bodyweight(60, 0)}})

	report, err := f.svc.ExerciseCatalogReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Merges) != 2 {
		t.Fatalf("pull-ups and the plank are written twice: %+v", report.Merges)
	}
	byKeep := map[int]MergePlan{}
	for _, m := range report.Merges {
		byKeep[m.Keep.Exercise.ID] = m
	}
	if m, ok := byKeep[pull.ID]; !ok || len(m.Merge) != 1 || m.Merge[0].Exercise.ID != old.ID {
		t.Fatalf("the capitalized spelling must stay: %+v", byKeep)
	}
	if m, ok := byKeep[plank.ID]; !ok || m.Merge[0].Exercise.ID != plankOld.ID {
		t.Fatalf("the capitalized plank must stay even if the other was used more: %+v", byKeep)
	}
	if len(report.Similar) != 1 || report.Similar[0].A.Exercise.Name != "Сгибания ног сидя" {
		t.Fatalf("only the one-letter difference is similar, not upper and lower block: %+v", report.Similar)
	}
	unused := map[string]bool{}
	for _, u := range report.Unused {
		unused[u.Exercise.Name] = true
	}
	if !unused["Тяга верхнего блока"] || unused["Подтягивания"] || len(unused) != 7 {
		t.Fatalf("unused: %v", unused)
	}

	setsBefore := countAllSets(t, f)
	if err := f.svc.ApplyMerges(ctx, report.Merges); err != nil {
		t.Fatal(err)
	}
	if got := countAllSets(t, f); got != setsBefore {
		t.Fatalf("%d sets before, %d after: the merge lost records", setsBefore, got)
	}
	d, err := f.svc.GetWorkout(ctx, testUser, both.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := d.SetsOf(pull.ID), sets(12, 15, 10, 20, 8, 20); !reflect.DeepEqual(got, want) {
		t.Fatalf("sets of the same workout must be joined: got %+v want %+v", got, want)
	}
	if len(d.Exercises) != 1 {
		t.Fatalf("no second record of the same exercise is left: %+v", d.Exercises)
	}
	d, _ = f.svc.GetWorkout(ctx, testUser, oldOnly.ID)
	if got := d.SetsOf(pull.ID); !reflect.DeepEqual(got, sets(9, 25)) {
		t.Fatalf("a record must move to the kept exercise: %+v", got)
	}
	if _, err := f.store.GetExercise(ctx, old.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the merged exercise must be gone: %v", err)
	}
	kept, _ := f.store.GetExercise(ctx, plank.ID)
	if !kept.Allows(domain.LoadBodyweight) {
		t.Fatalf("the kept exercise takes over the kinds of load: %+v", kept)
	}
	again, _ := f.svc.ExerciseCatalogReport(ctx)
	if len(again.Merges) != 0 {
		t.Fatalf("nothing is left to merge: %+v", again.Merges)
	}
}

func countAllSets(t *testing.T, f fixture) int {
	t.Helper()
	rows, err := f.store.ExportRows(context.Background(), testUser)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, w := range rows {
		for _, ex := range w.Exercises {
			n += len(ex.Sets)
		}
	}
	return n
}

func TestDeleteUnusedExercise(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.workoutAt(t, time.Now(), map[int][]domain.SetEntry{f.press.ID: sets(10, 100)})
	if err := f.svc.DeleteUnusedExercise(ctx, f.press.ID); !errors.Is(err, domain.ErrInUse) {
		t.Fatalf("an exercise in a workout must stay: %v", err)
	}
	if err := f.svc.DeleteUnusedExercise(ctx, f.calf.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetExercise(ctx, f.calf.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("the unused exercise must be gone: %v", err)
	}
}

func TestMergeIntoItself(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.svc.MergeExercises(context.Background(), f.press.ID, f.press.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
}
