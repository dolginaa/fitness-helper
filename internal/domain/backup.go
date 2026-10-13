package domain

import "time"

const ExportVersion = 1

type ExportBundle struct {
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Workouts   []ExportWorkout `json:"workouts"`
}

type ExportWorkout struct {
	Date      time.Time               `json:"date"`
	Status    string                  `json:"status"`
	Exercises []ExportWorkoutExercise `json:"exercises"`
}

type ExportWorkoutExercise struct {
	ExerciseName string     `json:"exercise_name"`
	MuscleGroup  string     `json:"muscle_group"`
	Sets         []SetEntry `json:"sets"`
}

type ImportMode string

const (
	ImportModeMerge   ImportMode = "merge"
	ImportModeReplace ImportMode = "replace"
)

type ImportResult struct {
	WorkoutsAdded   int
	ExercisesAdded  int
	WorkoutsSkipped int
}

type UserSettings struct {
	UserID              int64
	WeeklyBackupEnabled bool
	LastBackupAt        *time.Time
}
