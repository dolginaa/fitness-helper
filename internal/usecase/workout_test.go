package usecase

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

func TestFormatSets(t *testing.T) {
	t.Parallel()
	s := FormatSets([]domain.SetEntry{{Reps: 10, Weight: 50}, {Reps: 8, Weight: 52.5}})
	if !strings.Contains(s, "10×50.0") || !strings.Contains(s, "8×52.5") {
		t.Fatalf("unexpected format: %q", s)
	}
	if FormatSets(nil) != "нет данных" {
		t.Fatal("nil sets")
	}
}

type fakeCatalog struct {
	groups   []domain.MuscleGroup
	exercise domain.Exercise
}

func (f *fakeCatalog) ListMuscleGroups(ctx context.Context) ([]domain.MuscleGroup, error) {
	return f.groups, nil
}

func (f *fakeCatalog) ListExercisesByGroup(ctx context.Context, muscleGroupID int) ([]domain.Exercise, error) {
	return []domain.Exercise{f.exercise}, nil
}

func (f *fakeCatalog) GetExercise(ctx context.Context, id int) (domain.Exercise, error) {
	return f.exercise, nil
}

func (f *fakeCatalog) GetExerciseInGroup(ctx context.Context, exerciseID, muscleGroupID int) (domain.Exercise, error) {
	return f.exercise, nil
}

type fakeWorkout struct {
	last domain.Workout
}

func (f *fakeWorkout) StartWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	f.last = domain.Workout{ID: 1, UserID: userID, Date: time.Now(), Status: domain.WorkoutActive}
	return f.last, nil
}

func (f *fakeWorkout) GetActiveWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	return f.last, nil
}

func (f *fakeWorkout) GetWorkoutForUser(ctx context.Context, workoutID int, userID int64) (domain.Workout, error) {
	return f.last, nil
}

func (f *fakeWorkout) CompleteWorkout(ctx context.Context, userID int64, workoutID int) error {
	f.last.Status = domain.WorkoutCompleted
	return nil
}

func (f *fakeWorkout) UpsertWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int, sets []domain.SetEntry) error {
	return nil
}

func (f *fakeWorkout) GetLastExerciseData(ctx context.Context, userID int64, exerciseID int) ([]domain.SetEntry, time.Time, error) {
	return nil, time.Time{}, domain.ErrNoLastExerciseData
}

func (f *fakeWorkout) ListCompletedWorkouts(ctx context.Context, userID int64, limit int) ([]domain.Workout, error) {
	return nil, nil
}

type fakeSession struct {
	wid   *int
	state domain.SessionState
}

func (f *fakeSession) Get(ctx context.Context, userID int64) (*int, domain.SessionState, error) {
	return f.wid, f.state, nil
}

func (f *fakeSession) Save(ctx context.Context, userID int64, activeWorkoutID *int, state domain.SessionState) error {
	f.wid = activeWorkoutID
	f.state = state
	return nil
}

func (f *fakeSession) Clear(ctx context.Context, userID int64) error {
	f.wid = nil
	f.state = domain.SessionState{}
	return nil
}

func TestStartNewWorkoutSetsState(t *testing.T) {
	t.Parallel()
	cat := &fakeCatalog{groups: []domain.MuscleGroup{{ID: 1, Name: "грудь"}}, exercise: domain.Exercise{ID: 1, Name: "жим", MuscleGroupID: 1}}
	w := &fakeWorkout{}
	s := &fakeSession{}
	svc := NewService(cat, w, s)
	wo, err := svc.StartNewWorkout(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if wo.Status != domain.WorkoutActive || s.state.Phase != domain.PhaseSelectGroup || s.wid == nil {
		t.Fatalf("bad start: %#v state=%#v wid=%v", wo, s.state, s.wid)
	}
}

func TestFinishExerciseRequiresSets(t *testing.T) {
	t.Parallel()
	cat := &fakeCatalog{exercise: domain.Exercise{ID: 1, MuscleGroupID: 1}}
	w := &fakeWorkout{
		last: domain.Workout{ID: 1, UserID: 42, Status: domain.WorkoutActive, Date: time.Now()},
	}
	wid := 1
	s := &fakeSession{
		wid: &wid,
		state: domain.SessionState{
			Phase:         domain.PhaseLoggingSets,
			MuscleGroupID: 1,
			ExerciseID:    1,
			DraftSets:     nil,
		},
	}
	svc := NewService(cat, w, s)
	err := svc.FinishExercise(context.Background(), 42)
	if err != domain.ErrInvalidState {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}
