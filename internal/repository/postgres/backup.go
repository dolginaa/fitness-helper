package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Backup struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewBackup(pool *pgxpool.Pool, q *sqlcgen.Queries) *Backup {
	return &Backup{pool: pool, q: q}
}

func (b *Backup) ExportRows(ctx context.Context, userID int64) ([]domain.ExportWorkout, error) {
	rows, err := b.q.ExportUserWorkoutRows(ctx, userID)
	if err != nil {
		return nil, err
	}
	byID := map[int32]*domain.ExportWorkout{}
	order := []int32{}
	for _, r := range rows {
		if _, ok := byID[r.WorkoutID]; !ok {
			byID[r.WorkoutID] = &domain.ExportWorkout{
				Date:   timeFromPg(r.Date),
				Status: r.Status,
			}
			order = append(order, r.WorkoutID)
		}
		if !r.ExerciseID.Valid {
			continue
		}
		sets, err := parseSets(r.Sets)
		if err != nil {
			return nil, err
		}
		byID[r.WorkoutID].Exercises = append(byID[r.WorkoutID].Exercises, domain.ExportWorkoutExercise{
			ExerciseName: r.ExerciseName.String,
			MuscleGroup:  r.MuscleGroupName.String,
			Sets:         sets,
		})
	}
	out := make([]domain.ExportWorkout, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

func (b *Backup) WorkoutExerciseExistsOnDate(ctx context.Context, userID int64, exerciseID int, date time.Time) (bool, error) {
	return b.q.WorkoutExerciseExistsOnDate(ctx, sqlcgen.WorkoutExerciseExistsOnDateParams{
		UserID:     userID,
		ExerciseID: int32(exerciseID),
		Column3:    pgDateFromTime(date),
	})
}

func (b *Backup) CreateImportedWorkout(ctx context.Context, userID int64, date time.Time) (int, error) {
	wo, err := b.q.InsertImportedWorkout(ctx, sqlcgen.InsertImportedWorkoutParams{
		UserID: userID,
		Date:   pgTimestamptz(date),
	})
	if err != nil {
		return 0, err
	}
	return int(wo.ID), nil
}

func (b *Backup) AddExerciseToWorkout(ctx context.Context, workoutID, exerciseID int, sets []domain.SetEntry) error {
	raw, err := json.Marshal(sets)
	if err != nil {
		return err
	}
	_, err = b.q.UpsertWorkoutExercise(ctx, sqlcgen.UpsertWorkoutExerciseParams{
		WorkoutID:  int32(workoutID),
		ExerciseID: int32(exerciseID),
		Sets:       raw,
	})
	return err
}

func (b *Backup) DeleteAllUserWorkouts(ctx context.Context, userID int64) error {
	return b.q.DeleteUserWorkouts(ctx, userID)
}

var _ domain.BackupRepository = (*Backup)(nil)
