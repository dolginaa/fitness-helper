package usecase

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/adolgina/fitness-helper/internal/domain"
)

type Service struct {
	catalog   domain.CatalogRepository
	workouts  domain.WorkoutRepository
	sessions  domain.SessionRepository
	analytics domain.AnalyticsRepository
	backup    domain.BackupRepository
	settings  domain.SettingsRepository
}

func NewService(
	c domain.CatalogRepository,
	w domain.WorkoutRepository,
	s domain.SessionRepository,
	a domain.AnalyticsRepository,
	b domain.BackupRepository,
	st domain.SettingsRepository,
) *Service {
	return &Service{catalog: c, workouts: w, sessions: s, analytics: a, backup: b, settings: st}
}

const (
	minExerciseNameLen = 2
	maxExerciseNameLen = 100
)

func (s *Service) ListMuscleGroups(ctx context.Context) ([]domain.MuscleGroup, error) {
	return s.catalog.ListMuscleGroups(ctx)
}

func (s *Service) SessionState(ctx context.Context, userID int64) (domain.SessionState, error) {
	_, st, err := s.sessions.Get(ctx, userID)
	return st, err
}

// ActiveWorkout reports the unfinished workout of the user, if any.
func (s *Service) ActiveWorkout(ctx context.Context, userID int64) (domain.Workout, bool, error) {
	w, err := s.workouts.GetActiveWorkout(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Workout{}, false, nil
	}
	if err != nil {
		return domain.Workout{}, false, err
	}
	return w, true, nil
}

// StartNewWorkout closes the unfinished workout (if any) and starts a new one.
func (s *Service) StartNewWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	prev, active, err := s.ActiveWorkout(ctx, userID)
	if err != nil {
		return domain.Workout{}, err
	}
	if active {
		if _, err := s.closeWorkout(ctx, userID, prev); err != nil {
			return domain.Workout{}, err
		}
	}
	w, err := s.workouts.StartWorkout(ctx, userID)
	if err != nil {
		return domain.Workout{}, err
	}
	st := domain.SessionState{Phase: domain.PhaseSelectGroup}
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return domain.Workout{}, err
	}
	return w, nil
}

// Resume returns where the user is in the active workout.
func (s *Service) Resume(ctx context.Context, userID int64) (domain.SessionState, error) {
	_, st, err := s.activeSession(ctx, userID)
	return st, err
}

// activeSession loads the active workout and the dialog state. If the state is lost
// but the workout is still active in storage, the dialog restarts from the group choice.
func (s *Service) activeSession(ctx context.Context, userID int64) (domain.Workout, domain.SessionState, error) {
	wid, st, err := s.sessions.Get(ctx, userID)
	if err != nil {
		return domain.Workout{}, st, err
	}
	if wid != nil {
		w, err := s.workouts.GetWorkoutForUser(ctx, *wid, userID)
		if err == nil && w.Status == domain.WorkoutActive {
			return w, st, nil
		}
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return domain.Workout{}, st, err
		}
	}
	w, active, err := s.ActiveWorkout(ctx, userID)
	if err != nil {
		return domain.Workout{}, st, err
	}
	if !active {
		return domain.Workout{}, st, domain.ErrNoActiveWorkout
	}
	st = domain.SessionState{Phase: domain.PhaseSelectGroup}
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return domain.Workout{}, st, err
	}
	return w, st, nil
}

func (s *Service) SelectMuscleGroup(ctx context.Context, userID int64, muscleGroupID int) (ExerciseList, error) {
	w, st, err := s.activeSession(ctx, userID)
	if err != nil {
		return ExerciseList{}, err
	}
	group, err := s.catalog.GetMuscleGroup(ctx, muscleGroupID)
	if err != nil {
		return ExerciseList{}, err
	}
	st = domain.SessionState{Phase: domain.PhaseSelectExercise, MuscleGroupID: group.ID}
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseList{}, err
	}
	return s.exerciseList(ctx, userID, group, &w)
}

// BackToExercises leaves the exercise being logged (or the "new exercise" prompt)
// and returns the exercise list of the current muscle group.
func (s *Service) BackToExercises(ctx context.Context, userID int64) (ExerciseList, error) {
	w, st, err := s.activeSession(ctx, userID)
	if err != nil {
		return ExerciseList{}, err
	}
	if st.MuscleGroupID == 0 {
		return ExerciseList{}, domain.ErrInvalidState
	}
	group, err := s.catalog.GetMuscleGroup(ctx, st.MuscleGroupID)
	if err != nil {
		return ExerciseList{}, err
	}
	st = domain.SessionState{Phase: domain.PhaseSelectExercise, MuscleGroupID: group.ID}
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseList{}, err
	}
	return s.exerciseList(ctx, userID, group, &w)
}

func (s *Service) BackToGroups(ctx context.Context, userID int64) error {
	w, _, err := s.activeSession(ctx, userID)
	if err != nil {
		return err
	}
	return s.sessions.Save(ctx, userID, &w.ID, domain.SessionState{Phase: domain.PhaseSelectGroup})
}

// exerciseList builds the exercises of the group with their previous results.
// With a workout it also fills the sets already logged in it.
func (s *Service) exerciseList(ctx context.Context, userID int64, group domain.MuscleGroup, w *domain.Workout) (ExerciseList, error) {
	exercises, err := s.catalog.ListExercisesByGroup(ctx, group.ID)
	if err != nil {
		return ExerciseList{}, err
	}
	var today domain.WorkoutDetail
	if w != nil {
		if today, err = s.analytics.GetWorkoutDetail(ctx, userID, w.ID); err != nil {
			return ExerciseList{}, err
		}
	}
	rows := make([]ExerciseRow, 0, len(exercises))
	for _, ex := range exercises {
		row := ExerciseRow{Exercise: ex, TodaySets: today.SetsOf(ex.ID)}
		if row.LastSets, row.LastAt, err = s.lastResult(ctx, userID, ex.ID); err != nil {
			return ExerciseList{}, err
		}
		rows = append(rows, row)
	}
	return ExerciseList{Group: group, Rows: rows}, nil
}

func (s *Service) lastResult(ctx context.Context, userID int64, exerciseID int) ([]domain.SetEntry, time.Time, error) {
	sets, at, err := s.workouts.GetLastExerciseData(ctx, userID, exerciseID)
	if errors.Is(err, domain.ErrNoLastExerciseData) {
		return nil, time.Time{}, nil
	}
	return sets, at, err
}

func (s *Service) BeginExercise(ctx context.Context, userID int64, exerciseID int) (ExerciseLog, error) {
	w, prev, err := s.activeSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	ex, err := s.catalog.GetExercise(ctx, exerciseID)
	if err != nil {
		return ExerciseLog{}, err
	}
	return s.beginExercise(ctx, userID, w, ex, prev)
}

func (s *Service) beginExercise(ctx context.Context, userID int64, w domain.Workout, ex domain.Exercise, prev domain.SessionState) (ExerciseLog, error) {
	kind, err := s.startingKind(ctx, userID, w, ex, prev)
	if err != nil {
		return ExerciseLog{}, err
	}
	st := domain.SessionState{
		Phase:         domain.PhaseLoggingSets,
		MuscleGroupID: ex.MuscleGroupID,
		ExerciseID:    ex.ID,
		Kind:          kind,
	}
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

// startingKind picks the kind of load to enter sets in: the one already chosen for this exercise,
// else the one of today's last set, else the one of the last workout, else the default of the exercise.
func (s *Service) startingKind(ctx context.Context, userID int64, w domain.Workout, ex domain.Exercise, prev domain.SessionState) (domain.LoadKind, error) {
	if prev.Phase == domain.PhaseLoggingSets && prev.ExerciseID == ex.ID && ex.Allows(prev.Kind) {
		return prev.Kind, nil
	}
	today, err := s.analytics.GetWorkoutDetail(ctx, userID, w.ID)
	if err != nil {
		return "", err
	}
	last, _, err := s.lastResult(ctx, userID, ex.ID)
	if err != nil {
		return "", err
	}
	for _, sets := range [][]domain.SetEntry{today.SetsOf(ex.ID), last} {
		if n := len(sets); n > 0 && ex.Allows(sets[n-1].Load()) {
			return sets[n-1].Load(), nil
		}
	}
	return ex.Offered()[0], nil
}

// currentKind is the kind the next set is entered in.
func currentKind(ex domain.Exercise, st domain.SessionState) domain.LoadKind {
	if st.Kind != "" && ex.Allows(st.Kind) {
		return st.Kind
	}
	return ex.Offered()[0]
}

// CurrentExerciseLog returns the exercise being logged right now.
func (s *Service) CurrentExerciseLog(ctx context.Context, userID int64) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

func (s *Service) loggingSession(ctx context.Context, userID int64) (domain.Workout, domain.SessionState, error) {
	w, st, err := s.activeSession(ctx, userID)
	if err != nil {
		return domain.Workout{}, st, err
	}
	if st.Phase != domain.PhaseLoggingSets || st.ExerciseID == 0 {
		return domain.Workout{}, st, domain.ErrInvalidState
	}
	return w, st, nil
}

func (s *Service) exerciseLog(ctx context.Context, userID int64, w domain.Workout, st domain.SessionState) (ExerciseLog, error) {
	ex, err := s.catalog.GetExercise(ctx, st.ExerciseID)
	if err != nil {
		return ExerciseLog{}, err
	}
	today, err := s.analytics.GetWorkoutDetail(ctx, userID, w.ID)
	if err != nil {
		return ExerciseLog{}, err
	}
	log := ExerciseLog{Exercise: ex, Kind: currentKind(ex, st), Sets: today.SetsOf(ex.ID), PendingReps: st.PendingReps}
	if log.LastSets, log.LastAt, err = s.lastResult(ctx, userID, ex.ID); err != nil {
		return ExerciseLog{}, err
	}
	log.Suggestion = suggestSet(log.Sets, log.LastSets, log.Kind)
	return log, nil
}

// suggestSet guesses the next set of the given kind. While today repeats the last workout set by set
// (a warm-up ladder, for example), the guess is its next set; otherwise it is today's previous set.
func suggestSet(today, last []domain.SetEntry, kind domain.LoadKind) *domain.SetEntry {
	today, last = ofKind(today, kind), ofKind(last, kind)
	n := len(today)
	switch {
	case n == 0 && len(last) > 0:
		return &last[0]
	case n > 0 && n < len(last) && today[n-1] == last[n-1]:
		return &last[n]
	case n > 0:
		return &today[n-1]
	}
	return nil
}

func ofKind(sets []domain.SetEntry, kind domain.LoadKind) []domain.SetEntry {
	out := make([]domain.SetEntry, 0, len(sets))
	for _, s := range sets {
		if s.Load() == kind {
			out = append(out, s)
		}
	}
	return out
}

// LogInput handles a text message sent while an exercise is being logged.
// Sets are stored in the workout right away, so nothing is lost if the user never taps "done".
func (s *Service) LogInput(ctx context.Context, userID int64, text string) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	ex, err := s.catalog.GetExercise(ctx, st.ExerciseID)
	if err != nil {
		return ExerciseLog{}, err
	}
	kind := currentKind(ex, st)
	in, err := ParseSetInput(text, kind)
	if err != nil {
		return ExerciseLog{}, err
	}
	add := in.Sets
	switch {
	case len(add) > 0:
	case st.PendingReps == 0:
		reps, ok := wholeNumber(in.Number)
		if !ok || reps < 1 {
			return ExerciseLog{}, domain.ErrInvalidInput
		}
		st.PendingReps = reps
	default:
		set, ok := completeSet(kind, st.PendingReps, in.Number)
		if !ok {
			return ExerciseLog{}, domain.ErrInvalidInput
		}
		add = []domain.SetEntry{set}
	}
	if len(add) > 0 {
		st.PendingReps = 0
		if err := s.appendSets(ctx, userID, w, st.ExerciseID, add); err != nil {
			return ExerciseLog{}, err
		}
	}
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

// AddSet logs one set of the exercise being logged.
// exerciseID guards against a screen of an exercise that is no longer being logged.
func (s *Service) AddSet(ctx context.Context, userID int64, exerciseID int, set domain.SetEntry) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if st.ExerciseID != exerciseID {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	set = set.Normalize()
	if !set.Valid() {
		return ExerciseLog{}, domain.ErrInvalidInput
	}
	ex, err := s.catalog.GetExercise(ctx, exerciseID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if !ex.Allows(set.Load()) {
		return ExerciseLog{}, domain.ErrInvalidInput
	}
	if err := s.appendSets(ctx, userID, w, exerciseID, []domain.SetEntry{set}); err != nil {
		return ExerciseLog{}, err
	}
	st.PendingReps = 0
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

// RepeatSet adds the suggested set (see suggestSet).
// exerciseID guards against buttons of an exercise that is no longer being logged.
func (s *Service) RepeatSet(ctx context.Context, userID int64, exerciseID int) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if st.ExerciseID != exerciseID {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	log, err := s.exerciseLog(ctx, userID, w, st)
	if err != nil {
		return ExerciseLog{}, err
	}
	if log.Suggestion == nil {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	if err := s.workouts.UpsertWorkoutExercise(ctx, userID, w.ID, exerciseID, append(log.Sets, *log.Suggestion)); err != nil {
		return ExerciseLog{}, err
	}
	st.PendingReps = 0
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

// UndoLastSet drops the pending reps or, if there are none, the last logged set.
func (s *Service) UndoLastSet(ctx context.Context, userID int64, exerciseID int) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if st.ExerciseID != exerciseID {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	if st.PendingReps > 0 {
		st.PendingReps = 0
		if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
			return ExerciseLog{}, err
		}
		return s.exerciseLog(ctx, userID, w, st)
	}
	today, err := s.analytics.GetWorkoutDetail(ctx, userID, w.ID)
	if err != nil {
		return ExerciseLog{}, err
	}
	sets := today.SetsOf(exerciseID)
	switch len(sets) {
	case 0:
		return ExerciseLog{}, domain.ErrInvalidState
	case 1:
		err = s.workouts.DeleteWorkoutExercise(ctx, userID, w.ID, exerciseID)
	default:
		err = s.workouts.UpsertWorkoutExercise(ctx, userID, w.ID, exerciseID, sets[:len(sets)-1])
	}
	if err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

func (s *Service) appendSets(ctx context.Context, userID int64, w domain.Workout, exerciseID int, add []domain.SetEntry) error {
	today, err := s.analytics.GetWorkoutDetail(ctx, userID, w.ID)
	if err != nil {
		return err
	}
	sets := append(today.SetsOf(exerciseID), add...)
	return s.workouts.UpsertWorkoutExercise(ctx, userID, w.ID, exerciseID, sets)
}

// SetLoadKind chooses the kind of load the next sets of the exercise are entered in.
func (s *Service) SetLoadKind(ctx context.Context, userID int64, exerciseID int, kind domain.LoadKind) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if st.ExerciseID != exerciseID {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	ex, err := s.catalog.GetExercise(ctx, exerciseID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if !ex.Allows(kind) {
		return ExerciseLog{}, domain.ErrInvalidInput
	}
	st.Kind = kind
	st.PendingReps = 0
	if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
		return ExerciseLog{}, err
	}
	return s.exerciseLog(ctx, userID, w, st)
}

// ToggleLoad adds the kind of load to the exercise or removes it; one kind always stays.
// The exercise is shared by everyone who uses the bot.
func (s *Service) ToggleLoad(ctx context.Context, userID int64, exerciseID int, kind domain.LoadKind) (ExerciseLog, error) {
	w, st, err := s.loggingSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if st.ExerciseID != exerciseID {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	if !slices.Contains(domain.AllLoads, kind) {
		return ExerciseLog{}, domain.ErrInvalidInput
	}
	ex, err := s.catalog.GetExercise(ctx, exerciseID)
	if err != nil {
		return ExerciseLog{}, err
	}
	loads := toggleLoad(ex.Offered(), kind)
	if len(loads) == 0 {
		return ExerciseLog{}, domain.ErrInvalidInput
	}
	if err := s.catalog.SetExerciseLoads(ctx, exerciseID, loads); err != nil {
		return ExerciseLog{}, err
	}
	if !slices.Contains(loads, st.Kind) {
		st.Kind = loads[0]
		st.PendingReps = 0
		if err := s.sessions.Save(ctx, userID, &w.ID, st); err != nil {
			return ExerciseLog{}, err
		}
	}
	return s.exerciseLog(ctx, userID, w, st)
}

// SetExerciseLoads sets the kinds of load an exercise can be done with; at least one is needed.
func (s *Service) SetExerciseLoads(ctx context.Context, exerciseID int, loads []domain.LoadKind) (domain.Exercise, error) {
	loads = normalizeLoads(loads)
	if len(loads) == 0 {
		return domain.Exercise{}, domain.ErrInvalidInput
	}
	if err := s.catalog.SetExerciseLoads(ctx, exerciseID, loads); err != nil {
		return domain.Exercise{}, err
	}
	return s.catalog.GetExercise(ctx, exerciseID)
}

func toggleLoad(loads []domain.LoadKind, kind domain.LoadKind) []domain.LoadKind {
	if i := slices.Index(loads, kind); i >= 0 {
		return slices.Delete(slices.Clone(loads), i, i+1)
	}
	return append(slices.Clone(loads), kind)
}

// normalizeLoads drops repeated and unknown kinds.
func normalizeLoads(loads []domain.LoadKind) []domain.LoadKind {
	out := make([]domain.LoadKind, 0, len(loads))
	for _, l := range loads {
		if slices.Contains(domain.AllLoads, l) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// BeginAddExercise makes the next text message the name of a new exercise in the current group.
func (s *Service) BeginAddExercise(ctx context.Context, userID int64) (domain.MuscleGroup, error) {
	w, st, err := s.activeSession(ctx, userID)
	if err != nil {
		return domain.MuscleGroup{}, err
	}
	if st.MuscleGroupID == 0 {
		return domain.MuscleGroup{}, domain.ErrInvalidState
	}
	group, err := s.catalog.GetMuscleGroup(ctx, st.MuscleGroupID)
	if err != nil {
		return domain.MuscleGroup{}, err
	}
	st = domain.SessionState{Phase: domain.PhaseAddingExercise, MuscleGroupID: group.ID}
	return group, s.sessions.Save(ctx, userID, &w.ID, st)
}

// AddExercise creates the exercise in the current group (or reuses one with the same name)
// and starts logging it.
func (s *Service) AddExercise(ctx context.Context, userID int64, name string) (ExerciseLog, error) {
	w, st, err := s.activeSession(ctx, userID)
	if err != nil {
		return ExerciseLog{}, err
	}
	if st.Phase != domain.PhaseAddingExercise || st.MuscleGroupID == 0 {
		return ExerciseLog{}, domain.ErrInvalidState
	}
	ex, err := s.CreateExercise(ctx, st.MuscleGroupID, name, nil)
	if err != nil {
		return ExerciseLog{}, err
	}
	return s.beginExercise(ctx, userID, w, ex, st)
}

// CreateExercise adds an exercise to the muscle group; no loads means weight only.
// A name that is the same as an existing one apart from case, spaces and punctuation
// (see domain.ExerciseKey) returns the existing exercise.
func (s *Service) CreateExercise(ctx context.Context, muscleGroupID int, name string, loads []domain.LoadKind) (domain.Exercise, error) {
	name = strings.Join(strings.Fields(name), " ")
	if n := utf8.RuneCountInString(name); n < minExerciseNameLen || n > maxExerciseNameLen || domain.ExerciseKey(name) == "" {
		return domain.Exercise{}, domain.ErrInvalidInput
	}
	group, err := s.catalog.GetMuscleGroup(ctx, muscleGroupID)
	if err != nil {
		return domain.Exercise{}, err
	}
	existing, err := s.catalog.ListExercisesByGroup(ctx, group.ID)
	if err != nil {
		return domain.Exercise{}, err
	}
	for _, ex := range existing {
		if domain.ExerciseKey(ex.Name) == domain.ExerciseKey(name) {
			return ex, nil
		}
	}
	return s.catalog.CreateExercise(ctx, name, group.ID, normalizeLoads(loads))
}

// CurrentWorkout returns everything logged in the active workout so far.
func (s *Service) CurrentWorkout(ctx context.Context, userID int64) (domain.WorkoutDetail, error) {
	w, _, err := s.activeSession(ctx, userID)
	if err != nil {
		return domain.WorkoutDetail{}, err
	}
	return s.analytics.GetWorkoutDetail(ctx, userID, w.ID)
}

// EndWorkout completes the active workout and returns it.
// A workout without exercises is discarded: the returned detail has no exercises.
func (s *Service) EndWorkout(ctx context.Context, userID int64) (domain.WorkoutDetail, error) {
	w, _, err := s.activeSession(ctx, userID)
	if err != nil {
		return domain.WorkoutDetail{}, err
	}
	detail, err := s.closeWorkout(ctx, userID, w)
	if err != nil {
		return domain.WorkoutDetail{}, err
	}
	return detail, s.sessions.Clear(ctx, userID)
}

func (s *Service) closeWorkout(ctx context.Context, userID int64, w domain.Workout) (domain.WorkoutDetail, error) {
	detail, err := s.analytics.GetWorkoutDetail(ctx, userID, w.ID)
	if err != nil {
		return domain.WorkoutDetail{}, err
	}
	if len(detail.Exercises) == 0 {
		return detail, s.workouts.DeleteWorkout(ctx, userID, w.ID)
	}
	finished := time.Now()
	if err := s.workouts.CompleteWorkout(ctx, userID, w.ID, finished); err != nil {
		return domain.WorkoutDetail{}, err
	}
	detail.Workout.Status = domain.WorkoutCompleted
	detail.Workout.FinishedAt = finished
	return detail, nil
}
