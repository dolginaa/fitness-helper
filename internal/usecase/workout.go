package usecase

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

type Service struct {
	Catalog domain.CatalogRepository
	Workout domain.WorkoutRepository
	Session domain.SessionRepository
}

func NewService(c domain.CatalogRepository, w domain.WorkoutRepository, s domain.SessionRepository) *Service {
	return &Service{Catalog: c, Workout: w, Session: s}
}

func (s *Service) StartNewWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	w, err := s.Workout.StartWorkout(ctx, userID)
	if err != nil {
		return domain.Workout{}, err
	}
	wid := w.ID
	st := domain.SessionState{
		Phase:    domain.PhaseSelectGroup,
		LastFlow: "workout",
	}
	if err := s.Session.Save(ctx, userID, &wid, st); err != nil {
		return domain.Workout{}, err
	}
	return w, nil
}

func (s *Service) ensureWorkoutSession(ctx context.Context, userID int64) (domain.Workout, domain.SessionState, error) {
	wid, st, err := s.Session.Get(ctx, userID)
	if err != nil {
		return domain.Workout{}, st, err
	}
	if wid == nil {
		return domain.Workout{}, st, domain.ErrNoActiveWorkout
	}
	w, err := s.Workout.GetWorkoutForUser(ctx, *wid, userID)
	if err != nil {
		return domain.Workout{}, st, err
	}
	if w.Status != domain.WorkoutActive {
		return domain.Workout{}, st, domain.ErrNoActiveWorkout
	}
	return w, st, nil
}

func (s *Service) SelectMuscleGroup(ctx context.Context, userID int64, muscleGroupID int) error {
	w, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return err
	}
	_, err = s.Catalog.ListExercisesByGroup(ctx, muscleGroupID)
	if err != nil {
		return err
	}
	st.Phase = domain.PhaseSelectExercise
	st.MuscleGroupID = muscleGroupID
	return s.Session.Save(ctx, userID, &w.ID, st)
}

func (s *Service) CurrentState(ctx context.Context, userID int64) (*int, domain.SessionState, error) {
	return s.Session.Get(ctx, userID)
}

func (s *Service) ListExercisesWithLast(ctx context.Context, userID int64, muscleGroupID int) ([]ExerciseRow, error) {
	exercises, err := s.Catalog.ListExercisesByGroup(ctx, muscleGroupID)
	if err != nil {
		return nil, err
	}
	rows := make([]ExerciseRow, 0, len(exercises))
	for _, ex := range exercises {
		summary := "нет данных"
		sets, _, err := s.Workout.GetLastExerciseData(ctx, userID, ex.ID)
		if err != nil {
			if !errors.Is(err, domain.ErrNoLastExerciseData) {
				return nil, err
			}
		} else if len(sets) > 0 {
			summary = FormatSets(sets)
		}
		rows = append(rows, ExerciseRow{Exercise: ex, LastSummary: summary})
	}
	return rows, nil
}

func (s *Service) ListExercisesForCurrentGroup(ctx context.Context, userID int64) ([]ExerciseRow, error) {
	_, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return nil, err
	}
	if st.MuscleGroupID == 0 {
		return nil, domain.ErrInvalidState
	}
	return s.ListExercisesWithLast(ctx, userID, st.MuscleGroupID)
}

func (s *Service) BeginExercise(ctx context.Context, userID int64, exerciseID int) (domain.Exercise, []domain.SetEntry, error) {
	w, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return domain.Exercise{}, nil, err
	}
	if st.MuscleGroupID == 0 {
		return domain.Exercise{}, nil, domain.ErrInvalidState
	}
	ex, err := s.Catalog.GetExerciseInGroup(ctx, exerciseID, st.MuscleGroupID)
	if err != nil {
		return domain.Exercise{}, nil, err
	}
	var last []domain.SetEntry
	if sets, _, err := s.Workout.GetLastExerciseData(ctx, userID, ex.ID); err == nil {
		last = sets
	}
	st.Phase = domain.PhaseLoggingSets
	st.ExerciseID = ex.ID
	st.DraftSets = nil
	st.Awaiting = "reps"
	st.PendingReps = 0
	return ex, last, s.Session.Save(ctx, userID, &w.ID, st)
}

func (s *Service) ProcessLogInput(ctx context.Context, userID int64, text string) (done bool, err error) {
	w, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return false, err
	}
	if st.Phase != domain.PhaseLoggingSets {
		return false, domain.ErrInvalidState
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	if st.Awaiting == "reps" {
		reps, err := strconv.Atoi(text)
		if err != nil || reps < 0 {
			return false, domain.ErrInvalidState
		}
		st.PendingReps = reps
		st.Awaiting = "weight"
		return false, s.Session.Save(ctx, userID, &w.ID, st)
	}
	if st.Awaiting == "weight" {
		val, err := strconv.ParseFloat(text, 64)
		if err != nil || val < 0 {
			return false, domain.ErrInvalidState
		}
		st.DraftSets = append(st.DraftSets, domain.SetEntry{Reps: st.PendingReps, Weight: val})
		st.Awaiting = "reps"
		st.PendingReps = 0
		return false, s.Session.Save(ctx, userID, &w.ID, st)
	}
	return false, domain.ErrInvalidState
}

func (s *Service) FinishExercise(ctx context.Context, userID int64) error {
	w, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return err
	}
	if st.Phase != domain.PhaseLoggingSets || st.ExerciseID == 0 {
		return domain.ErrInvalidState
	}
	if len(st.DraftSets) == 0 {
		return domain.ErrInvalidState
	}
	if err := s.Workout.UpsertWorkoutExercise(ctx, userID, w.ID, st.ExerciseID, st.DraftSets); err != nil {
		return err
	}
	st.Phase = domain.PhaseSelectExercise
	st.ExerciseID = 0
	st.DraftSets = nil
	st.Awaiting = ""
	st.PendingReps = 0
	return s.Session.Save(ctx, userID, &w.ID, st)
}

func (s *Service) NextMuscleGroup(ctx context.Context, userID int64) error {
	w, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return err
	}
	st.Phase = domain.PhaseSelectGroup
	st.MuscleGroupID = 0
	st.ExerciseID = 0
	st.DraftSets = nil
	st.Awaiting = ""
	return s.Session.Save(ctx, userID, &w.ID, st)
}

func (s *Service) EndWorkout(ctx context.Context, userID int64) error {
	w, st, err := s.ensureWorkoutSession(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.Workout.CompleteWorkout(ctx, userID, w.ID); err != nil {
		return err
	}
	_ = st
	if err := s.Session.Clear(ctx, userID); err != nil {
		return err
	}
	return nil
}

func (s *Service) History(ctx context.Context, userID int64, limit int) ([]domain.Workout, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	return s.Workout.ListCompletedWorkouts(ctx, userID, limit)
}

func (s *Service) BeginLastCommand(ctx context.Context, userID int64) error {
	st := domain.SessionState{
		Phase:    domain.PhaseLastPickGroup,
		LastFlow: "last_cmd",
	}
	return s.Session.Save(ctx, userID, nil, st)
}

func (s *Service) LastSelectGroup(ctx context.Context, userID int64, muscleGroupID int) error {
	_, err := s.Catalog.ListExercisesByGroup(ctx, muscleGroupID)
	if err != nil {
		return err
	}
	st := domain.SessionState{
		Phase:         domain.PhaseLastPickExercise,
		MuscleGroupID: muscleGroupID,
		LastFlow:      "last_cmd",
	}
	return s.Session.Save(ctx, userID, nil, st)
}

func (s *Service) LastSelectExercise(ctx context.Context, userID int64, exerciseID int) (domain.Exercise, []domain.SetEntry, time.Time, error) {
	_, st, err := s.Session.Get(ctx, userID)
	if err != nil {
		return domain.Exercise{}, nil, time.Time{}, err
	}
	if st.Phase != domain.PhaseLastPickExercise || st.MuscleGroupID == 0 {
		return domain.Exercise{}, nil, time.Time{}, domain.ErrInvalidState
	}
	ex, err := s.Catalog.GetExerciseInGroup(ctx, exerciseID, st.MuscleGroupID)
	if err != nil {
		return domain.Exercise{}, nil, time.Time{}, err
	}
	sets, at, err := s.Workout.GetLastExerciseData(ctx, userID, ex.ID)
	if err != nil {
		return ex, nil, time.Time{}, err
	}
	return ex, sets, at, nil
}

func (s *Service) ResetLastFlow(ctx context.Context, userID int64) error {
	return s.Session.Clear(ctx, userID)
}
