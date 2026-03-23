package domain

import (
	"context"
	"time"
)

type CatalogRepository interface {
	ListMuscleGroups(ctx context.Context) ([]MuscleGroup, error)
	ListExercisesByGroup(ctx context.Context, muscleGroupID int) ([]Exercise, error)
	GetExercise(ctx context.Context, id int) (Exercise, error)
	GetExerciseInGroup(ctx context.Context, exerciseID, muscleGroupID int) (Exercise, error)
}

type WorkoutRepository interface {
	// StartWorkout creates a new active workout and completes other active workouts for the user (transaction).
	StartWorkout(ctx context.Context, userID int64) (Workout, error)
	GetActiveWorkout(ctx context.Context, userID int64) (Workout, error)
	GetWorkoutForUser(ctx context.Context, workoutID int, userID int64) (Workout, error)
	CompleteWorkout(ctx context.Context, userID int64, workoutID int) error
	UpsertWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int, sets []SetEntry) error
	GetLastExerciseData(ctx context.Context, userID int64, exerciseID int) (sets []SetEntry, at time.Time, err error)
	ListCompletedWorkouts(ctx context.Context, userID int64, limit int) ([]Workout, error)
}

type SessionRepository interface {
	Get(ctx context.Context, userID int64) (activeWorkoutID *int, state SessionState, err error)
	Save(ctx context.Context, userID int64, activeWorkoutID *int, state SessionState) error
	Clear(ctx context.Context, userID int64) error
}
