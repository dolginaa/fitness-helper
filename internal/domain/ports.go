package domain

import (
	"context"
	"time"
)

type CatalogRepository interface {
	ListMuscleGroups(ctx context.Context) ([]MuscleGroup, error)
	GetMuscleGroup(ctx context.Context, id int) (MuscleGroup, error)
	ListExercisesByGroup(ctx context.Context, muscleGroupID int) ([]Exercise, error)
	GetExercise(ctx context.Context, id int) (Exercise, error)
	CreateExercise(ctx context.Context, name string, muscleGroupID int) (Exercise, error)
	GetOrCreateExerciseByNameAndGroup(ctx context.Context, name, muscleGroup string) (Exercise, error)
}

type WorkoutRepository interface {
	StartWorkout(ctx context.Context, userID int64) (Workout, error)
	GetActiveWorkout(ctx context.Context, userID int64) (Workout, error)
	GetWorkoutForUser(ctx context.Context, workoutID int, userID int64) (Workout, error)
	CompleteWorkout(ctx context.Context, userID int64, workoutID int) error
	DeleteWorkout(ctx context.Context, userID int64, workoutID int) error
	UpsertWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int, sets []SetEntry) error
	DeleteWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int) error
	GetLastExerciseData(ctx context.Context, userID int64, exerciseID int) (sets []SetEntry, at time.Time, err error)
}

type SessionRepository interface {
	Get(ctx context.Context, userID int64) (activeWorkoutID *int, state SessionState, err error)
	Save(ctx context.Context, userID int64, activeWorkoutID *int, state SessionState) error
	Clear(ctx context.Context, userID int64) error
}

type AnalyticsRepository interface {
	ListCompletedWorkoutsInRange(ctx context.Context, userID int64, since, until time.Time) ([]Workout, error)
	ListWorkoutSummaries(ctx context.Context, userID int64, limit, offset int) ([]WorkoutSummary, error)
	GetWorkoutDetail(ctx context.Context, userID int64, workoutID int) (WorkoutDetail, error)
	ListExerciseProgress(ctx context.Context, userID int64, exerciseID int, since time.Time) ([]ExerciseProgressPoint, error)
	ListMuscleGroupProgress(ctx context.Context, userID int64, muscleGroupID int, since time.Time) ([]ExerciseProgressSeries, error)
}

type BackupRepository interface {
	ExportRows(ctx context.Context, userID int64) ([]ExportWorkout, error)
	WorkoutExerciseExistsOnDate(ctx context.Context, userID int64, exerciseID int, date time.Time) (bool, error)
	CreateImportedWorkout(ctx context.Context, userID int64, date time.Time) (int, error)
	AddExerciseToWorkout(ctx context.Context, workoutID, exerciseID int, sets []SetEntry) error
	DeleteAllUserWorkouts(ctx context.Context, userID int64) error
}

type SettingsRepository interface {
	GetSettings(ctx context.Context, userID int64) (UserSettings, error)
	SetWeeklyBackup(ctx context.Context, userID int64, enabled bool) error
	SetLastBackupAt(ctx context.Context, userID int64, at time.Time) error
	ListUsersDueWeeklyBackup(ctx context.Context, before time.Time) ([]int64, error)
}
