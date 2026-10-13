package usecase

import (
	"context"
	"sort"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

const (
	HalfYearDays    = 180
	HistoryPageSize = 8

	// maxGroupChartSeries limits the muscle group chart to the most trained exercises.
	maxGroupChartSeries = 8
)

// History returns a page of completed workouts, newest first. Pages start at 0.
func (s *Service) History(ctx context.Context, userID int64, page int) (HistoryPage, error) {
	if page < 0 {
		page = 0
	}
	items, err := s.analytics.ListWorkoutSummaries(ctx, userID, HistoryPageSize+1, page*HistoryPageSize)
	if err != nil {
		return HistoryPage{}, err
	}
	hasNext := len(items) > HistoryPageSize
	if hasNext {
		items = items[:HistoryPageSize]
	}
	return HistoryPage{Items: items, Page: page, HasNext: hasNext}, nil
}

func (s *Service) GetWorkout(ctx context.Context, userID int64, workoutID int) (domain.WorkoutDetail, error) {
	return s.analytics.GetWorkoutDetail(ctx, userID, workoutID)
}

// WorkoutsOnDate returns the completed workouts of the calendar day that starts at day,
// in chronological order.
func (s *Service) WorkoutsOnDate(ctx context.Context, userID int64, day time.Time) ([]domain.WorkoutDetail, error) {
	ws, err := s.analytics.ListCompletedWorkoutsInRange(ctx, userID, day, day.AddDate(0, 0, 1))
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkoutDetail, 0, len(ws))
	for i := len(ws) - 1; i >= 0; i-- {
		d, err := s.analytics.GetWorkoutDetail(ctx, userID, ws[i].ID)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ExerciseOverview lists the exercises of the group with the last result of each.
func (s *Service) ExerciseOverview(ctx context.Context, userID int64, muscleGroupID int) (ExerciseList, error) {
	group, err := s.catalog.GetMuscleGroup(ctx, muscleGroupID)
	if err != nil {
		return ExerciseList{}, err
	}
	return s.exerciseList(ctx, userID, group, nil)
}

// ExerciseProgress returns the results of the exercise over the last half a year, oldest first.
func (s *Service) ExerciseProgress(ctx context.Context, userID int64, exerciseID int) (ExerciseProgress, error) {
	ex, err := s.catalog.GetExercise(ctx, exerciseID)
	if err != nil {
		return ExerciseProgress{}, err
	}
	points, err := s.analytics.ListExerciseProgress(ctx, userID, exerciseID, halfYearAgo())
	if err != nil {
		return ExerciseProgress{}, err
	}
	return ExerciseProgress{Exercise: ex, Points: points}, nil
}

// MuscleGroupProgress returns the most trained exercises of the group over the last half a year.
func (s *Service) MuscleGroupProgress(ctx context.Context, userID int64, muscleGroupID int) (GroupProgress, error) {
	group, err := s.catalog.GetMuscleGroup(ctx, muscleGroupID)
	if err != nil {
		return GroupProgress{}, err
	}
	series, err := s.analytics.ListMuscleGroupProgress(ctx, userID, muscleGroupID, halfYearAgo())
	if err != nil {
		return GroupProgress{}, err
	}
	sort.SliceStable(series, func(i, j int) bool {
		return len(series[i].Points) > len(series[j].Points)
	})
	if len(series) > maxGroupChartSeries {
		series = series[:maxGroupChartSeries]
	}
	return GroupProgress{Group: group, Series: series}, nil
}

func halfYearAgo() time.Time {
	return time.Now().AddDate(0, 0, -HalfYearDays)
}
