package postgres

import (
	"context"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
)

type Analytics struct {
	q *sqlcgen.Queries
}

func NewAnalytics(q *sqlcgen.Queries) *Analytics {
	return &Analytics{q: q}
}

func (a *Analytics) ListCompletedWorkoutsInRange(ctx context.Context, userID int64, since, until time.Time) ([]domain.Workout, error) {
	rows, err := a.q.ListCompletedWorkoutsInRange(ctx, sqlcgen.ListCompletedWorkoutsInRangeParams{
		UserID: userID,
		Date:   pgTimestamptz(since),
		Date_2: pgTimestamptz(until),
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

func (a *Analytics) ListWorkoutSummaries(ctx context.Context, userID int64, limit, offset int) ([]domain.WorkoutSummary, error) {
	rows, err := a.q.ListWorkoutSummaries(ctx, sqlcgen.ListWorkoutSummariesParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkoutSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.WorkoutSummary{
			Workout: domain.Workout{
				ID:     int(r.ID),
				UserID: r.UserID,
				Date:   timeFromPg(r.Date),
				Status: domain.WorkoutStatus(r.Status),
			},
			MuscleGroups:  r.MuscleGroups,
			ExerciseCount: int(r.ExerciseCount),
		})
	}
	return out, nil
}

func (a *Analytics) GetWorkoutDetail(ctx context.Context, userID int64, workoutID int) (domain.WorkoutDetail, error) {
	rows, err := a.q.GetWorkoutDetailRows(ctx, sqlcgen.GetWorkoutDetailRowsParams{
		ID:     int32(workoutID),
		UserID: userID,
	})
	if err != nil {
		return domain.WorkoutDetail{}, err
	}
	if len(rows) == 0 {
		return domain.WorkoutDetail{}, domain.ErrNotFound
	}
	return buildWorkoutDetail(rows), nil
}

func buildWorkoutDetail(rows []sqlcgen.GetWorkoutDetailRowsRow) domain.WorkoutDetail {
	r0 := rows[0]
	detail := domain.WorkoutDetail{
		Workout: domain.Workout{
			ID:     int(r0.WorkoutID),
			UserID: r0.UserID,
			Date:   timeFromPg(r0.Date),
			Status: domain.WorkoutStatus(r0.Status),
		},
	}
	for _, r := range rows {
		if !r.ExerciseID.Valid {
			continue
		}
		sets, _ := parseSets(r.Sets)
		detail.Exercises = append(detail.Exercises, domain.WorkoutExerciseDetail{
			ExerciseID:   int(r.ExerciseID.Int32),
			ExerciseName: r.ExerciseName.String,
			MuscleGroup:  r.MuscleGroupName.String,
			Sets:         sets,
		})
	}
	return detail
}

func (a *Analytics) ListExerciseProgress(ctx context.Context, userID int64, exerciseID int, since time.Time) ([]domain.ExerciseProgressPoint, error) {
	rows, err := a.q.ListExerciseProgress(ctx, sqlcgen.ListExerciseProgressParams{
		UserID:     userID,
		ExerciseID: int32(exerciseID),
		Date:       pgTimestamptz(since),
	})
	if err != nil {
		return nil, err
	}
	points := make([]domain.ExerciseProgressPoint, 0, len(rows))
	for _, r := range rows {
		sets, err := parseSets(r.Sets)
		if err != nil {
			return nil, err
		}
		points = append(points, domain.ExerciseProgressPoint{
			Date:          timeFromPg(r.Date),
			MaxWeight:     domain.MaxWeightFromSets(sets),
			WorkingWeight: domain.WorkingWeightFromSets(sets),
			Sets:          sets,
		})
	}
	return points, nil
}

func (a *Analytics) ListMuscleGroupProgress(ctx context.Context, userID int64, muscleGroupID int, since time.Time) ([]domain.ExerciseProgressSeries, error) {
	rows, err := a.q.ListMuscleGroupProgress(ctx, sqlcgen.ListMuscleGroupProgressParams{
		UserID:        userID,
		MuscleGroupID: int32(muscleGroupID),
		Date:          pgTimestamptz(since),
	})
	if err != nil {
		return nil, err
	}
	byEx := map[int]*domain.ExerciseProgressSeries{}
	order := []int{}
	for _, r := range rows {
		sets, err := parseSets(r.Sets)
		if err != nil {
			return nil, err
		}
		exID := int(r.ExerciseID)
		if byEx[exID] == nil {
			byEx[exID] = &domain.ExerciseProgressSeries{
				ExerciseID:   exID,
				ExerciseName: r.ExerciseName,
			}
			order = append(order, exID)
		}
		byEx[exID].Points = append(byEx[exID].Points, domain.ExerciseProgressPoint{
			Date:          timeFromPg(r.Date),
			MaxWeight:     domain.MaxWeightFromSets(sets),
			WorkingWeight: domain.WorkingWeightFromSets(sets),
			Sets:          sets,
		})
	}
	out := make([]domain.ExerciseProgressSeries, 0, len(order))
	for _, id := range order {
		out = append(out, *byEx[id])
	}
	return out, nil
}

var _ domain.AnalyticsRepository = (*Analytics)(nil)
