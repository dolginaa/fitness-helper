package domain

import "time"

type WorkoutDetail struct {
	Workout   Workout
	Exercises []WorkoutExerciseDetail
}

// SetsOf returns the sets logged for the exercise in this workout (nil if none).
func (d WorkoutDetail) SetsOf(exerciseID int) []SetEntry {
	for _, ex := range d.Exercises {
		if ex.ExerciseID == exerciseID {
			return ex.Sets
		}
	}
	return nil
}

type WorkoutExerciseDetail struct {
	ExerciseID   int
	ExerciseName string
	MuscleGroup  string
	Sets         []SetEntry
}

// WorkoutSummary is a history list item: a workout and what was trained in it.
type WorkoutSummary struct {
	Workout       Workout
	MuscleGroups  []string
	ExerciseCount int
}

type ExerciseProgressPoint struct {
	Date          time.Time
	MaxWeight     float64
	WorkingWeight float64
	Sets          []SetEntry
}

type ExerciseProgressSeries struct {
	ExerciseID   int
	ExerciseName string
	Points       []ExerciseProgressPoint
}

// MaxWeightFromSets returns the maximum weight across sets (0 if empty).
func MaxWeightFromSets(sets []SetEntry) float64 {
	var max float64
	for _, s := range sets {
		if s.Weight > max {
			max = s.Weight
		}
	}
	return max
}

// WorkingWeightFromSets returns sum(weight*reps)/sum(reps) (0 if no reps).
func WorkingWeightFromSets(sets []SetEntry) float64 {
	var weighted float64
	var reps int
	for _, s := range sets {
		weighted += s.Weight * float64(s.Reps)
		reps += s.Reps
	}
	if reps == 0 {
		return 0
	}
	return weighted / float64(reps)
}

// VolumeFromSets returns the lifted volume: sum(weight*reps).
func VolumeFromSets(sets []SetEntry) float64 {
	var volume float64
	for _, s := range sets {
		volume += s.Weight * float64(s.Reps)
	}
	return volume
}
