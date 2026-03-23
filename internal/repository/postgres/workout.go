package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Workout struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewWorkout(pool *pgxpool.Pool, q *sqlcgen.Queries) *Workout {
	return &Workout{pool: pool, q: q}
}

func (w *Workout) StartWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return domain.Workout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.q.WithTx(tx)
	wo, err := qtx.CreateWorkout(ctx, userID)
	if err != nil {
		return domain.Workout{}, err
	}
	if err := qtx.CompleteActiveWorkoutsExcept(ctx, sqlcgen.CompleteActiveWorkoutsExceptParams{
		UserID: userID,
		ID:     wo.ID,
	}); err != nil {
		return domain.Workout{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Workout{}, err
	}
	return mapWorkout(wo), nil
}

func (w *Workout) GetActiveWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	wo, err := w.q.GetActiveWorkoutForUser(ctx, userID)
	if err != nil {
		return domain.Workout{}, mapErr(err)
	}
	return mapWorkout(wo), nil
}

func (w *Workout) GetWorkoutForUser(ctx context.Context, workoutID int, userID int64) (domain.Workout, error) {
	wo, err := w.q.GetWorkoutByIDForUser(ctx, sqlcgen.GetWorkoutByIDForUserParams{
		ID:     int32(workoutID),
		UserID: userID,
	})
	if err != nil {
		return domain.Workout{}, mapErr(err)
	}
	return mapWorkout(wo), nil
}

func (w *Workout) CompleteWorkout(ctx context.Context, userID int64, workoutID int) error {
	err := w.q.CompleteWorkout(ctx, sqlcgen.CompleteWorkoutParams{
		ID:     int32(workoutID),
		UserID: userID,
	})
	return mapErr(err)
}

func (w *Workout) UpsertWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int, sets []domain.SetEntry) error {
	if err := w.ensureWorkoutOwner(ctx, workoutID, userID); err != nil {
		return err
	}
	b, err := json.Marshal(sets)
	if err != nil {
		return err
	}
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := w.q.WithTx(tx)
	_, err = qtx.UpsertWorkoutExercise(ctx, sqlcgen.UpsertWorkoutExerciseParams{
		WorkoutID:  int32(workoutID),
		ExerciseID: int32(exerciseID),
		Sets:       b,
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Workout) ensureWorkoutOwner(ctx context.Context, workoutID int, userID int64) error {
	_, err := w.q.GetWorkoutByIDForUser(ctx, sqlcgen.GetWorkoutByIDForUserParams{
		ID:     int32(workoutID),
		UserID: userID,
	})
	return mapErr(err)
}

func (w *Workout) GetLastExerciseData(ctx context.Context, userID int64, exerciseID int) ([]domain.SetEntry, time.Time, error) {
	row, err := w.q.GetLastWorkoutExerciseData(ctx, sqlcgen.GetLastWorkoutExerciseDataParams{
		UserID:     userID,
		ExerciseID: int32(exerciseID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, domain.ErrNoLastExerciseData
		}
		return nil, time.Time{}, err
	}
	sets, err := parseSets(row.Sets)
	if err != nil {
		return nil, time.Time{}, err
	}
	var at time.Time
	if row.Date.Valid {
		at = row.Date.Time
	}
	return sets, at, nil
}

func (w *Workout) ListCompletedWorkouts(ctx context.Context, userID int64, limit int) ([]domain.Workout, error) {
	rows, err := w.q.ListCompletedWorkoutsForUser(ctx, sqlcgen.ListCompletedWorkoutsForUserParams{
		UserID: userID,
		Limit:  int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.Workout, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapWorkout(r))
	}
	return out, nil
}

func mapWorkout(w sqlcgen.Workout) domain.Workout {
	var t time.Time
	if w.Date.Valid {
		t = w.Date.Time
	}
	return domain.Workout{
		ID:     int(w.ID),
		UserID: w.UserID,
		Date:   t,
		Status: domain.WorkoutStatus(w.Status),
	}
}

func parseSets(b []byte) ([]domain.SetEntry, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var s []domain.SetEntry
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return s, nil
}
