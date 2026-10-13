package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

// pullups is an exercise done with a weight, a band or the body.
func (f fixture) pullups() domain.Exercise {
	return f.store.AddExercise(f.legs.ID, "Подтягивания", domain.LoadWeight, domain.LoadBand, domain.LoadBodyweight)
}

func (f fixture) begin(t *testing.T, ex domain.Exercise) ExerciseLog {
	t.Helper()
	ctx := context.Background()
	if _, active, _ := f.svc.ActiveWorkout(ctx, testUser); !active {
		if _, err := f.svc.StartNewWorkout(ctx, testUser); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.SelectMuscleGroup(ctx, testUser, ex.MuscleGroupID); err != nil {
		t.Fatal(err)
	}
	log, err := f.svc.BeginExercise(ctx, testUser, ex.ID)
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func TestLoggingBandAndBodyweightSets(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	pull := f.pullups()
	log := f.begin(t, pull)
	if log.Kind != domain.LoadWeight || !reflect.DeepEqual(log.Exercise.Offered(), []domain.LoadKind{domain.LoadWeight, domain.LoadBand, domain.LoadBodyweight}) {
		t.Fatalf("an exercise starts in its first kind: %+v", log)
	}

	log, err := f.svc.SetLoadKind(ctx, testUser, pull.ID, domain.LoadBand)
	if err != nil || log.Kind != domain.LoadBand {
		t.Fatalf("switch to band: %+v, %v", log.Kind, err)
	}
	// Band sets: the level follows the reps, or both come in one message.
	if log, err = f.svc.LogInput(ctx, testUser, "12"); err != nil || log.PendingReps != 12 {
		t.Fatalf("reps must be pending: %+v, %v", log, err)
	}
	if log, err = f.svc.LogInput(ctx, testUser, "4"); err != nil {
		t.Fatal(err)
	}
	if log, err = f.svc.LogInput(ctx, testUser, "10 3"); err != nil {
		t.Fatal(err)
	}
	if want := []domain.SetEntry{band(12, 4), band(10, 3)}; !reflect.DeepEqual(log.Sets, want) {
		t.Fatalf("got %+v want %+v", log.Sets, want)
	}
	if log.Suggestion == nil || *log.Suggestion != band(10, 3) {
		t.Fatalf("the suggestion must be a band set: %+v", log.Suggestion)
	}
	if _, err = f.svc.LogInput(ctx, testUser, "10 15"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("band level 15 does not exist: %v", err)
	}

	// Bodyweight sets go in one number each and are suggested apart from the band ones.
	if log, err = f.svc.SetLoadKind(ctx, testUser, pull.ID, domain.LoadBodyweight); err != nil || log.Suggestion != nil {
		t.Fatalf("no bodyweight sets yet, so nothing to suggest: %+v, %v", log.Suggestion, err)
	}
	if log, err = f.svc.LogInput(ctx, testUser, "8, 6"); err != nil {
		t.Fatal(err)
	}
	if got := log.Sets[2:]; !reflect.DeepEqual(got, []domain.SetEntry{bodyweight(8, 0), bodyweight(6, 0)}) {
		t.Fatalf("got %+v", got)
	}

	// Coming back to the exercise keeps the chosen kind.
	if log, err = f.svc.BeginExercise(ctx, testUser, pull.ID); err != nil || log.Kind != domain.LoadBodyweight {
		t.Fatalf("the kind must be kept: %v, %v", log.Kind, err)
	}
	// A kind the exercise cannot be done with is refused.
	if _, err = f.svc.SetLoadKind(ctx, testUser, f.calf.ID, domain.LoadBand); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("not the exercise being logged: want ErrInvalidState, got %v", err)
	}
	f.begin(t, f.calf)
	if _, err = f.svc.SetLoadKind(ctx, testUser, f.calf.ID, domain.LoadBand); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("calves have no band: want ErrInvalidInput, got %v", err)
	}
}

func TestNextWorkoutStartsInTheLastKind(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	pull := f.pullups()
	f.begin(t, pull)
	if _, err := f.svc.SetLoadKind(ctx, testUser, pull.ID, domain.LoadBand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LogInput(ctx, testUser, "12 4"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.EndWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}

	log := f.begin(t, pull)
	if log.Kind != domain.LoadBand {
		t.Fatalf("the exercise must start in the kind of last time, got %v", log.Kind)
	}
	if log.Suggestion == nil || *log.Suggestion != band(12, 4) {
		t.Fatalf("the first set is suggested from last time: %+v", log.Suggestion)
	}
}

func TestToggleLoad(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	f.begin(t, f.press)

	log, err := f.svc.ToggleLoad(ctx, testUser, f.press.ID, domain.LoadBand)
	if err != nil || !reflect.DeepEqual(log.Exercise.Offered(), []domain.LoadKind{domain.LoadWeight, domain.LoadBand}) {
		t.Fatalf("band must be added: %+v, %v", log.Exercise.Loads, err)
	}
	if _, err = f.svc.SetLoadKind(ctx, testUser, f.press.ID, domain.LoadBand); err != nil {
		t.Fatal(err)
	}
	// Taking away the kind in use moves the dialog to the first kind left.
	if log, err = f.svc.ToggleLoad(ctx, testUser, f.press.ID, domain.LoadBand); err != nil || log.Kind != domain.LoadWeight {
		t.Fatalf("kind %v, %v", log.Kind, err)
	}
	if _, err = f.svc.ToggleLoad(ctx, testUser, f.press.ID, domain.LoadWeight); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("the last kind cannot be taken away: %v", err)
	}
	if _, err = f.svc.ToggleLoad(ctx, testUser, f.press.ID, "kettlebell"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("an unknown kind must not be added: %v", err)
	}
}

func TestAddSetKinds(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	pull := f.pullups()
	f.begin(t, pull)

	log, err := f.svc.AddSet(ctx, testUser, pull.ID, domain.SetEntry{Reps: 10, Weight: 20, Kind: domain.LoadWeight})
	if err != nil || !reflect.DeepEqual(log.Sets, sets(10, 20)) {
		t.Fatalf("an explicit weight kind is stored as the plain set: %+v, %v", log.Sets, err)
	}
	if log, err = f.svc.AddSet(ctx, testUser, pull.ID, band(12, 4)); err != nil || len(log.Sets) != 2 {
		t.Fatalf("band set: %+v, %v", log.Sets, err)
	}
	for _, bad := range []domain.SetEntry{band(12, 0), band(12, 11), {Reps: 10, Weight: 5, Kind: domain.LoadBand, Band: 3}} {
		if _, err := f.svc.AddSet(ctx, testUser, pull.ID, bad); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%+v: want ErrInvalidInput, got %v", bad, err)
		}
	}
	f.begin(t, f.press)
	if _, err := f.svc.AddSet(ctx, testUser, f.press.ID, band(12, 4)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("an exercise without a band refuses band sets: %v", err)
	}
}

func TestCreateExerciseWithLoads(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ex, err := f.svc.CreateExercise(context.Background(), f.legs.ID, "Отжимания", []domain.LoadKind{domain.LoadBodyweight, "kettlebell", domain.LoadBodyweight, domain.LoadBand})
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.LoadKind{domain.LoadBodyweight, domain.LoadBand}; !reflect.DeepEqual(ex.Loads, want) {
		t.Fatalf("got %v want %v", ex.Loads, want)
	}
}

func TestImportKeepsKindsAndFindsExercisesByKey(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	pull := f.store.AddExercise(f.legs.ID, "Подтягивания")

	bundle := domain.ExportBundle{Version: domain.ExportVersion, Workouts: []domain.ExportWorkout{{
		Date: time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
		Exercises: []domain.ExportWorkoutExercise{
			{ExerciseName: "подтягивания ", MuscleGroup: "НОГИ", Sets: []domain.SetEntry{band(12, 4)}},
			{ExerciseName: "Отжимания", MuscleGroup: "ноги", Sets: []domain.SetEntry{bodyweight(20, 0)}},
		},
	}}}
	res, err := f.svc.ImportUserData(ctx, testUser, zipBundle(t, bundle), domain.ImportModeMerge)
	if err != nil || res.WorkoutsAdded != 1 {
		t.Fatalf("import: %+v, %v", res, err)
	}
	groups, _ := f.store.ListMuscleGroups(ctx)
	if len(groups) != 1 {
		t.Fatalf("a group written in capitals is the same group: %+v", groups)
	}
	exercises, _ := f.store.ListExercisesByGroup(ctx, f.legs.ID)
	byName := map[string]domain.Exercise{}
	for _, e := range exercises {
		byName[e.Name] = e
	}
	if len(byName) != 4 {
		t.Fatalf("pull-ups written differently must not be created twice: %+v", byName)
	}
	if got := byName["Подтягивания"]; got.ID != pull.ID || !got.Allows(domain.LoadBand) || !got.Allows(domain.LoadWeight) {
		t.Fatalf("the existing exercise must learn the band: %+v", got)
	}
	if !byName["Отжимания"].Allows(domain.LoadBodyweight) {
		t.Fatalf("a new exercise must allow the kind its sets were done with: %+v", byName["Отжимания"])
	}
}

func zipBundle(t *testing.T, b domain.ExportBundle) []byte {
	t.Helper()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("export.json")
	_, _ = w.Write(raw)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
