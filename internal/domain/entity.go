package domain

import "time"

type MuscleGroup struct {
	ID   int
	Name string
}

type Exercise struct {
	ID            int
	Name          string
	MuscleGroupID int
}

type WorkoutStatus string

const (
	WorkoutActive    WorkoutStatus = "active"
	WorkoutCompleted WorkoutStatus = "completed"
)

type Workout struct {
	ID     int
	UserID int64
	Date   time.Time
	Status WorkoutStatus
}

type SetEntry struct {
	Reps   int     `json:"reps"`
	Weight float64 `json:"weight"`
}

type WorkoutExercise struct {
	ID         int
	WorkoutID  int
	ExerciseID int
	Sets       []SetEntry
}
