package usecase

import (
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

// ExerciseRow is an exercise with its previous result and, inside a workout, today's sets.
type ExerciseRow struct {
	Exercise  domain.Exercise
	LastSets  []domain.SetEntry
	LastAt    time.Time
	TodaySets []domain.SetEntry
}

type ExerciseList struct {
	Group domain.MuscleGroup
	Rows  []ExerciseRow
}

// ExerciseLog is the state of the exercise being logged in the active workout.
type ExerciseLog struct {
	Exercise domain.Exercise
	// Kind is the kind of load the next set is entered in; it is one of Exercise.Offered().
	Kind     domain.LoadKind
	Sets     []domain.SetEntry
	LastSets []domain.SetEntry
	LastAt   time.Time
	// PendingReps > 0: reps are entered, the weight is expected next.
	PendingReps int
	// Suggestion is the most likely next set, added by a one-tap repeat (nil if there is nothing to go by).
	Suggestion *domain.SetEntry
}

type HistoryPage struct {
	Items   []domain.WorkoutSummary
	Page    int
	HasNext bool
}

type ExerciseProgress struct {
	Exercise domain.Exercise
	Points   []domain.ExerciseProgressPoint
}

type GroupProgress struct {
	Group  domain.MuscleGroup
	Series []domain.ExerciseProgressSeries
}
