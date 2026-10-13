package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/memory"
)

const testUser int64 = 42

type fixture struct {
	svc   *Service
	store *memory.Store
	legs  domain.MuscleGroup
	press domain.Exercise
	calf  domain.Exercise
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	store := memory.NewStore()
	legs := store.AddGroup("ноги")
	press, _ := store.CreateExercise(context.Background(), "Жим ногами", legs.ID)
	calf, _ := store.CreateExercise(context.Background(), "Икры", legs.ID)
	return fixture{
		svc:   NewService(store, store, store, store, store, store),
		store: store,
		legs:  legs,
		press: press,
		calf:  calf,
	}
}

// startLogging brings the user to the point where sets of "Жим ногами" are expected.
func (f fixture) startLogging(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.svc.StartNewWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SelectMuscleGroup(ctx, testUser, f.legs.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BeginExercise(ctx, testUser, f.press.ID); err != nil {
		t.Fatal(err)
	}
}

func sets(pairs ...float64) []domain.SetEntry {
	out := make([]domain.SetEntry, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, domain.SetEntry{Reps: int(pairs[i]), Weight: pairs[i+1]})
	}
	return out
}

func TestStartNewWorkoutSetsState(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	wo, err := f.svc.StartNewWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.SessionState(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if wo.Status != domain.WorkoutActive || st.Phase != domain.PhaseSelectGroup {
		t.Fatalf("bad start: %#v state=%#v", wo, st)
	}
}

func TestLogInputStoresSetsImmediately(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()

	log, err := f.svc.LogInput(ctx, testUser, "10 100")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(log.Sets, sets(10, 100)) {
		t.Fatalf("after one message: %+v", log.Sets)
	}
	if log, err = f.svc.LogInput(ctx, testUser, "8x110, 6 х 112,5"); err != nil {
		t.Fatal(err)
	}
	want := sets(10, 100, 8, 110, 6, 112.5)
	if !reflect.DeepEqual(log.Sets, want) {
		t.Fatalf("got %+v want %+v", log.Sets, want)
	}
	// Sets are already in the workout, without any "finish exercise" step.
	d, err := f.svc.CurrentWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.SetsOf(f.press.ID), want) {
		t.Fatalf("workout has %+v", d.Exercises)
	}
}

func TestLogInputRepsThenWeight(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()

	log, err := f.svc.LogInput(ctx, testUser, "12")
	if err != nil {
		t.Fatal(err)
	}
	if log.PendingReps != 12 || len(log.Sets) != 0 {
		t.Fatalf("reps must be pending: %+v", log)
	}
	if log, err = f.svc.LogInput(ctx, testUser, "52,5"); err != nil {
		t.Fatal(err)
	}
	if log.PendingReps != 0 || !reflect.DeepEqual(log.Sets, sets(12, 52.5)) {
		t.Fatalf("set must be stored: %+v", log)
	}
}

func TestLogInputRejectsGarbage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	for _, text := range []string{"", "десять", "0 50", "10 50 60", "10.5", "10 -5", "5000 20"} {
		if _, err := f.svc.LogInput(context.Background(), testUser, text); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%q: want ErrInvalidInput, got %v", text, err)
		}
	}
}

func TestRepeatAndUndo(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.store.AddCompletedWorkout(testUser, time.Now().AddDate(0, 0, -7), f.press.ID, sets(10, 90, 8, 95)...)
	f.startLogging(t)

	log, err := f.svc.CurrentExerciseLog(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if log.Suggestion == nil || *log.Suggestion != (domain.SetEntry{Reps: 10, Weight: 90}) {
		t.Fatalf("first suggestion must come from the last workout: %+v", log.Suggestion)
	}
	if _, err = f.svc.RepeatSet(ctx, testUser, f.press.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.LogInput(ctx, testUser, "9 92.5"); err != nil {
		t.Fatal(err)
	}
	if log, err = f.svc.RepeatSet(ctx, testUser, f.press.ID); err != nil {
		t.Fatal(err)
	}
	if want := sets(10, 90, 9, 92.5, 9, 92.5); !reflect.DeepEqual(log.Sets, want) {
		t.Fatalf("got %+v want %+v", log.Sets, want)
	}

	for i := 0; i < 3; i++ {
		if log, err = f.svc.UndoLastSet(ctx, testUser, f.press.ID); err != nil {
			t.Fatal(err)
		}
	}
	if len(log.Sets) != 0 {
		t.Fatalf("all sets must be undone: %+v", log.Sets)
	}
	if _, err = f.svc.UndoLastSet(ctx, testUser, f.press.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("nothing to undo: got %v", err)
	}
	// A button of another exercise must not touch the current one.
	if _, err = f.svc.RepeatSet(ctx, testUser, f.calf.ID); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("stale button: got %v", err)
	}
}

func TestExerciseListShowsTodayAndLast(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.store.AddCompletedWorkout(testUser, time.Now().AddDate(0, 0, -7), f.calf.ID, sets(20, 40)...)
	f.startLogging(t)
	if _, err := f.svc.LogInput(ctx, testUser, "10 100"); err != nil {
		t.Fatal(err)
	}
	list, err := f.svc.BackToExercises(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Rows) != 2 || list.Group.ID != f.legs.ID {
		t.Fatalf("unexpected list: %+v", list)
	}
	if !reflect.DeepEqual(list.Rows[0].TodaySets, sets(10, 100)) || list.Rows[0].LastSets != nil {
		t.Fatalf("press row: %+v", list.Rows[0])
	}
	if list.Rows[1].TodaySets != nil || !reflect.DeepEqual(list.Rows[1].LastSets, sets(20, 40)) {
		t.Fatalf("calf row: %+v", list.Rows[1])
	}
}

func TestReopenedExerciseKeepsSets(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()
	if _, err := f.svc.LogInput(ctx, testUser, "10 100"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BackToExercises(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.BeginExercise(ctx, testUser, f.press.ID); err != nil {
		t.Fatal(err)
	}
	log, err := f.svc.LogInput(ctx, testUser, "8 110")
	if err != nil {
		t.Fatal(err)
	}
	if want := sets(10, 100, 8, 110); !reflect.DeepEqual(log.Sets, want) {
		t.Fatalf("got %+v want %+v", log.Sets, want)
	}
}

func TestEndWorkout(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()
	if _, err := f.svc.LogInput(ctx, testUser, "10 100"); err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.EndWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if d.Workout.Status != domain.WorkoutCompleted || len(d.Exercises) != 1 {
		t.Fatalf("unexpected detail: %+v", d)
	}
	if _, active, _ := f.svc.ActiveWorkout(ctx, testUser); active {
		t.Fatal("workout must not stay active")
	}
	page, err := f.svc.History(ctx, testUser, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ExerciseCount != 1 {
		t.Fatalf("history: %+v", page)
	}
	if _, err := f.svc.LogInput(ctx, testUser, "10 100"); !errors.Is(err, domain.ErrNoActiveWorkout) {
		t.Fatalf("want ErrNoActiveWorkout, got %v", err)
	}
}

func TestEmptyWorkoutIsDiscarded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.svc.StartNewWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	// Starting again must not leave the empty workout in the history either.
	if _, err := f.svc.StartNewWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.EndWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Exercises) != 0 {
		t.Fatalf("unexpected exercises: %+v", d)
	}
	rows, _ := f.store.ExportRows(ctx, testUser)
	if len(rows) != 0 {
		t.Fatalf("empty workouts must be deleted, got %+v", rows)
	}
}

func TestSessionIsRestoredFromActiveWorkout(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()
	if _, err := f.svc.LogInput(ctx, testUser, "10 100"); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Clear(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.Resume(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != domain.PhaseSelectGroup {
		t.Fatalf("unexpected phase %q", st.Phase)
	}
	d, err := f.svc.CurrentWorkout(ctx, testUser)
	if err != nil || len(d.Exercises) != 1 {
		t.Fatalf("workout must be intact: %+v err=%v", d, err)
	}
}

func TestAddExercise(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()
	if _, err := f.svc.BeginAddExercise(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddExercise(ctx, testUser, " "); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
	log, err := f.svc.AddExercise(ctx, testUser, "  Болгарские   выпады ")
	if err != nil {
		t.Fatal(err)
	}
	if log.Exercise.Name != "Болгарские выпады" || log.Exercise.MuscleGroupID != f.legs.ID {
		t.Fatalf("unexpected exercise: %+v", log.Exercise)
	}
	st, _ := f.svc.SessionState(ctx, testUser)
	if st.Phase != domain.PhaseLoggingSets || st.ExerciseID != log.Exercise.ID {
		t.Fatalf("must start logging the new exercise: %+v", st)
	}

	// The same name in another case reuses the existing exercise.
	if _, err := f.svc.BeginAddExercise(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	again, err := f.svc.AddExercise(ctx, testUser, "жим НОГАМИ")
	if err != nil {
		t.Fatal(err)
	}
	if again.Exercise.ID != f.press.ID {
		t.Fatalf("duplicate created: %+v", again.Exercise)
	}
}

func TestHistoryPaging(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < HistoryPageSize+2; i++ {
		f.store.AddCompletedWorkout(testUser, now.AddDate(0, 0, -i), f.press.ID, sets(10, 100)...)
	}
	first, err := f.svc.History(ctx, testUser, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != HistoryPageSize || !first.HasNext {
		t.Fatalf("first page: %d items, hasNext=%v", len(first.Items), first.HasNext)
	}
	if !first.Items[0].Workout.Date.Equal(now) {
		t.Fatal("newest workout must come first")
	}
	second, err := f.svc.History(ctx, testUser, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 2 || second.HasNext {
		t.Fatalf("second page: %d items, hasNext=%v", len(second.Items), second.HasNext)
	}
}

func TestWorkoutsOnDate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)
	f.store.AddCompletedWorkout(testUser, day.Add(-time.Minute), f.press.ID, sets(10, 100)...)
	morning := f.store.AddCompletedWorkout(testUser, day.Add(30*time.Minute), f.press.ID, sets(10, 100)...)
	evening := f.store.AddCompletedWorkout(testUser, day.Add(23*time.Hour), f.calf.ID, sets(20, 40)...)
	f.store.AddCompletedWorkout(testUser, day.Add(24*time.Hour), f.press.ID, sets(10, 100)...)

	got, err := f.svc.WorkoutsOnDate(ctx, testUser, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Workout.ID != morning.ID || got[1].Workout.ID != evening.ID {
		t.Fatalf("unexpected workouts: %+v", got)
	}
}

func TestAddSet(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.startLogging(t)
	ctx := context.Background()

	log, err := f.svc.AddSet(ctx, testUser, f.press.ID, domain.SetEntry{Reps: 10, Weight: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(log.Sets, sets(10, 100)) {
		t.Fatalf("unexpected sets: %+v", log.Sets)
	}
	if _, err := f.svc.AddSet(ctx, testUser, f.press.ID, domain.SetEntry{Reps: 0, Weight: 100}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
	if _, err := f.svc.AddSet(ctx, testUser, f.calf.ID, domain.SetEntry{Reps: 10, Weight: 40}); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("a set for another exercise: want ErrInvalidState, got %v", err)
	}
}

func TestCreateExercise(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	ex, err := f.svc.CreateExercise(ctx, f.legs.ID, " Выпады  назад ")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Name != "Выпады назад" || ex.MuscleGroupID != f.legs.ID {
		t.Fatalf("unexpected exercise: %+v", ex)
	}
	if same, _ := f.svc.CreateExercise(ctx, f.legs.ID, "выпады назад"); same.ID != ex.ID {
		t.Fatalf("duplicate created: %+v", same)
	}
	if _, err := f.svc.CreateExercise(ctx, f.legs.ID, "я"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
	if _, err := f.svc.CreateExercise(ctx, 999, "Выпады"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown group: want ErrNotFound, got %v", err)
	}
}

func TestSuggestSet(t *testing.T) {
	t.Parallel()
	ladder := sets(15, 45, 15, 85, 15, 100, 13, 100)
	cases := []struct {
		name        string
		today, last []domain.SetEntry
		want        *domain.SetEntry
	}{
		{"nothing to go by", nil, nil, nil},
		{"first set comes from the last workout", nil, ladder, &ladder[0]},
		{"following the ladder", sets(15, 45, 15, 85), ladder, &ladder[2]},
		{"went heavier than last time: repeat today", sets(15, 45, 15, 90), ladder, &domain.SetEntry{Reps: 15, Weight: 90}},
		{"more sets than last time: repeat today", sets(10, 50, 10, 50), sets(10, 50), &domain.SetEntry{Reps: 10, Weight: 50}},
		{"no history: repeat today", sets(10, 50), nil, &domain.SetEntry{Reps: 10, Weight: 50}},
	}
	for _, c := range cases {
		got := suggestSet(c.today, c.last)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Fatalf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}
