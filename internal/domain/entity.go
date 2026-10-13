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

// Upper bounds of a single set; anything above is treated as a typo.
const (
	MaxReps   = 1000
	MaxWeight = 1000
)

type SetEntry struct {
	Reps   int     `json:"reps"`
	Weight float64 `json:"weight"`
}

func (s SetEntry) Valid() bool {
	return s.Reps >= 1 && s.Reps <= MaxReps && s.Weight >= 0 && s.Weight <= MaxWeight
}

type WorkoutExercise struct {
	ID         int
	WorkoutID  int
	ExerciseID int
	Sets       []SetEntry
}
