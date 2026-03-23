package domain

type Phase string

const (
	PhaseIdle             Phase = "idle"
	PhaseSelectGroup      Phase = "select_group"
	PhaseSelectExercise   Phase = "select_exercise"
	PhaseLoggingSets      Phase = "logging_sets"
	PhaseLastPickGroup    Phase = "last_pick_group"
	PhaseLastPickExercise Phase = "last_pick_exercise"
)

// SessionState is persisted as JSON in user_workout_state.state.
type SessionState struct {
	Phase         Phase       `json:"phase"`
	MuscleGroupID int         `json:"muscle_group_id,omitempty"`
	ExerciseID    int         `json:"exercise_id,omitempty"`
	DraftSets     []SetEntry  `json:"draft_sets,omitempty"`
	Awaiting      string      `json:"awaiting,omitempty"` // "reps" | "weight"
	PendingReps   int         `json:"pending_reps,omitempty"`
	LastFlow      string      `json:"last_flow,omitempty"` // "workout" | "last_cmd"
}

func (s SessionState) IsLogging() bool {
	return s.Phase == PhaseLoggingSets
}
