package domain

type Phase string

const (
	PhaseIdle           Phase = "idle"
	PhaseSelectGroup    Phase = "select_group"
	PhaseSelectExercise Phase = "select_exercise"
	PhaseLoggingSets    Phase = "logging_sets"
	PhaseAddingExercise Phase = "adding_exercise"
)

// SessionState is persisted as JSON in user_workout_state.state.
// Logged sets are not kept here: they go straight to the workout.
type SessionState struct {
	Phase         Phase `json:"phase"`
	MuscleGroupID int   `json:"muscle_group_id,omitempty"`
	ExerciseID    int   `json:"exercise_id,omitempty"`
	// Kind is the load kind the next set of the exercise is entered in.
	Kind LoadKind `json:"kind,omitempty"`
	// PendingReps > 0 means reps were sent on their own and the weight is expected next.
	PendingReps int `json:"pending_reps,omitempty"`
}
