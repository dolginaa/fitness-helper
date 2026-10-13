// Package memory is an in-memory implementation of the domain ports for tests.
package memory

import (
	"context"
	"sort"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

type session struct {
	workoutID *int
	state     domain.SessionState
}

// Store implements every repository port. It is not safe for concurrent use.
type Store struct {
	Now func() time.Time

	groups    []domain.MuscleGroup
	exercises []domain.Exercise
	workouts  []domain.Workout
	// logged keeps workout exercises in the order they were first logged.
	logged   []domain.WorkoutExercise
	sessions map[int64]session
	settings map[int64]domain.UserSettings
	nextID   int
}

func NewStore() *Store {
	return &Store{
		Now:      time.Now,
		sessions: map[int64]session{},
		settings: map[int64]domain.UserSettings{},
	}
}

func (s *Store) id() int {
	s.nextID++
	return s.nextID
}

// AddGroup seeds a muscle group.
func (s *Store) AddGroup(name string) domain.MuscleGroup {
	g := domain.MuscleGroup{ID: s.id(), Name: name}
	s.groups = append(s.groups, g)
	return g
}

// AddExercise seeds an exercise of an existing group.
func (s *Store) AddExercise(groupID int, name string, loads ...domain.LoadKind) domain.Exercise {
	e, _ := s.CreateExercise(context.Background(), name, groupID, loads)
	return e
}

// AddCompletedWorkout seeds a finished workout with one exercise.
func (s *Store) AddCompletedWorkout(userID int64, at time.Time, exerciseID int, sets ...domain.SetEntry) domain.Workout {
	w := domain.Workout{ID: s.id(), UserID: userID, Date: at, Status: domain.WorkoutCompleted}
	s.workouts = append(s.workouts, w)
	s.logged = append(s.logged, domain.WorkoutExercise{ID: s.id(), WorkoutID: w.ID, ExerciseID: exerciseID, Sets: sets})
	return w
}

// Catalog

func (s *Store) ListMuscleGroups(ctx context.Context) ([]domain.MuscleGroup, error) {
	return append([]domain.MuscleGroup(nil), s.groups...), nil
}

func (s *Store) GetMuscleGroup(ctx context.Context, id int) (domain.MuscleGroup, error) {
	for _, g := range s.groups {
		if g.ID == id {
			return g, nil
		}
	}
	return domain.MuscleGroup{}, domain.ErrNotFound
}

func (s *Store) ListExercisesByGroup(ctx context.Context, muscleGroupID int) ([]domain.Exercise, error) {
	var out []domain.Exercise
	for _, e := range s.exercises {
		if e.MuscleGroupID == muscleGroupID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *Store) GetExercise(ctx context.Context, id int) (domain.Exercise, error) {
	for _, e := range s.exercises {
		if e.ID == id {
			return e, nil
		}
	}
	return domain.Exercise{}, domain.ErrNotFound
}

func (s *Store) CreateExercise(ctx context.Context, name string, muscleGroupID int, loads []domain.LoadKind) (domain.Exercise, error) {
	e := domain.Exercise{ID: s.id(), Name: name, MuscleGroupID: muscleGroupID, Loads: append([]domain.LoadKind(nil), loads...)}
	s.exercises = append(s.exercises, e)
	return e, nil
}

func (s *Store) SetExerciseLoads(ctx context.Context, id int, loads []domain.LoadKind) error {
	for i := range s.exercises {
		if s.exercises[i].ID == id {
			s.exercises[i].Loads = append([]domain.LoadKind(nil), loads...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (s *Store) GetOrCreateExerciseByNameAndGroup(ctx context.Context, name, muscleGroup string) (domain.Exercise, error) {
	var group *domain.MuscleGroup
	for i := range s.groups {
		if domain.ExerciseKey(s.groups[i].Name) == domain.ExerciseKey(muscleGroup) {
			group = &s.groups[i]
		}
	}
	if group == nil {
		g := s.AddGroup(muscleGroup)
		group = &g
	}
	for _, e := range s.exercises {
		if e.MuscleGroupID == group.ID && domain.ExerciseKey(e.Name) == domain.ExerciseKey(name) {
			return e, nil
		}
	}
	return s.CreateExercise(ctx, name, group.ID, nil)
}

func (s *Store) ListExerciseUsage(ctx context.Context) ([]domain.ExerciseUsage, error) {
	out := make([]domain.ExerciseUsage, 0, len(s.exercises))
	for _, e := range s.exercises {
		g, _ := s.GetMuscleGroup(ctx, e.MuscleGroupID)
		u := domain.ExerciseUsage{Exercise: e, Group: g.Name}
		for _, we := range s.logged {
			if we.ExerciseID == e.ID {
				u.Workouts++
			}
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *Store) MergeExercises(ctx context.Context, from, to int) error {
	if from == to {
		return domain.ErrInvalidInput
	}
	src, err := s.GetExercise(ctx, from)
	if err != nil {
		return err
	}
	target, err := s.GetExercise(ctx, to)
	if err != nil {
		return err
	}
	var kept []domain.WorkoutExercise
	for _, we := range s.logged {
		if we.ExerciseID != from {
			kept = append(kept, we)
		}
	}
	for _, we := range s.logged {
		if we.ExerciseID != from {
			continue
		}
		joined := false
		for i := range kept {
			if kept[i].WorkoutID == we.WorkoutID && kept[i].ExerciseID == to {
				kept[i].Sets = append(append([]domain.SetEntry(nil), kept[i].Sets...), we.Sets...)
				joined = true
			}
		}
		if !joined {
			we.ExerciseID = to
			kept = append(kept, we)
		}
	}
	s.logged = kept
	for _, k := range src.Loads {
		if !target.Allows(k) {
			target.Loads = append(target.Offered(), k)
		}
	}
	for i := range s.exercises {
		if s.exercises[i].ID == to {
			s.exercises[i].Loads = target.Loads
		}
	}
	rest := s.exercises[:0]
	for _, e := range s.exercises {
		if e.ID != from {
			rest = append(rest, e)
		}
	}
	s.exercises = rest
	return nil
}

func (s *Store) DeleteUnusedExercise(ctx context.Context, id int) error {
	for _, we := range s.logged {
		if we.ExerciseID == id {
			return domain.ErrInUse
		}
	}
	rest := s.exercises[:0]
	for _, e := range s.exercises {
		if e.ID != id {
			rest = append(rest, e)
		}
	}
	s.exercises = rest
	return nil
}

// Workouts

func (s *Store) workout(workoutID int, userID int64) *domain.Workout {
	for i := range s.workouts {
		if s.workouts[i].ID == workoutID && s.workouts[i].UserID == userID {
			return &s.workouts[i]
		}
	}
	return nil
}

func (s *Store) StartWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	for i := range s.workouts {
		if s.workouts[i].UserID == userID && s.workouts[i].Status == domain.WorkoutActive {
			s.workouts[i].Status = domain.WorkoutCompleted
		}
	}
	w := domain.Workout{ID: s.id(), UserID: userID, Date: s.Now(), Status: domain.WorkoutActive}
	s.workouts = append(s.workouts, w)
	return w, nil
}

func (s *Store) GetActiveWorkout(ctx context.Context, userID int64) (domain.Workout, error) {
	for _, w := range s.workouts {
		if w.UserID == userID && w.Status == domain.WorkoutActive {
			return w, nil
		}
	}
	return domain.Workout{}, domain.ErrNotFound
}

func (s *Store) GetWorkoutForUser(ctx context.Context, workoutID int, userID int64) (domain.Workout, error) {
	if w := s.workout(workoutID, userID); w != nil {
		return *w, nil
	}
	return domain.Workout{}, domain.ErrNotFound
}

func (s *Store) CompleteWorkout(ctx context.Context, userID int64, workoutID int, at time.Time) error {
	if w := s.workout(workoutID, userID); w != nil {
		w.Status = domain.WorkoutCompleted
		w.FinishedAt = at
	}
	return nil
}

func (s *Store) DeleteWorkout(ctx context.Context, userID int64, workoutID int) error {
	if s.workout(workoutID, userID) == nil {
		return nil
	}
	s.deleteWorkouts(func(w domain.Workout) bool { return w.ID == workoutID })
	return nil
}

func (s *Store) deleteWorkouts(match func(domain.Workout) bool) {
	gone := map[int]bool{}
	kept := s.workouts[:0]
	for _, w := range s.workouts {
		if match(w) {
			gone[w.ID] = true
			continue
		}
		kept = append(kept, w)
	}
	s.workouts = kept
	logged := s.logged[:0]
	for _, we := range s.logged {
		if !gone[we.WorkoutID] {
			logged = append(logged, we)
		}
	}
	s.logged = logged
	for uid, sess := range s.sessions {
		if sess.workoutID != nil && gone[*sess.workoutID] {
			sess.workoutID = nil
			s.sessions[uid] = sess
		}
	}
}

func (s *Store) UpsertWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int, sets []domain.SetEntry) error {
	if s.workout(workoutID, userID) == nil {
		return domain.ErrNotFound
	}
	return s.AddExerciseToWorkout(ctx, workoutID, exerciseID, sets)
}

func (s *Store) DeleteWorkoutExercise(ctx context.Context, userID int64, workoutID int, exerciseID int) error {
	if s.workout(workoutID, userID) == nil {
		return domain.ErrNotFound
	}
	logged := s.logged[:0]
	for _, we := range s.logged {
		if we.WorkoutID != workoutID || we.ExerciseID != exerciseID {
			logged = append(logged, we)
		}
	}
	s.logged = logged
	return nil
}

func (s *Store) GetLastExerciseData(ctx context.Context, userID int64, exerciseID int) ([]domain.SetEntry, time.Time, error) {
	var sets []domain.SetEntry
	var at time.Time
	for _, we := range s.logged {
		w := s.workout(we.WorkoutID, userID)
		if w == nil || w.Status != domain.WorkoutCompleted || we.ExerciseID != exerciseID {
			continue
		}
		if sets == nil || w.Date.After(at) {
			sets, at = we.Sets, w.Date
		}
	}
	if sets == nil {
		return nil, time.Time{}, domain.ErrNoLastExerciseData
	}
	return sets, at, nil
}

// Session

func (s *Store) Get(ctx context.Context, userID int64) (*int, domain.SessionState, error) {
	sess, ok := s.sessions[userID]
	if !ok {
		return nil, domain.SessionState{Phase: domain.PhaseIdle}, nil
	}
	return sess.workoutID, sess.state, nil
}

func (s *Store) Save(ctx context.Context, userID int64, activeWorkoutID *int, state domain.SessionState) error {
	var wid *int
	if activeWorkoutID != nil {
		v := *activeWorkoutID
		wid = &v
	}
	s.sessions[userID] = session{workoutID: wid, state: state}
	return nil
}

func (s *Store) Clear(ctx context.Context, userID int64) error {
	delete(s.sessions, userID)
	return nil
}

// Analytics

func (s *Store) completed(userID int64) []domain.Workout {
	var out []domain.Workout
	for _, w := range s.workouts {
		if w.UserID == userID && w.Status == domain.WorkoutCompleted {
			out = append(out, w)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out
}

func (s *Store) ListCompletedWorkoutsInRange(ctx context.Context, userID int64, since, until time.Time) ([]domain.Workout, error) {
	var out []domain.Workout
	for _, w := range s.completed(userID) {
		if !w.Date.Before(since) && w.Date.Before(until) {
			out = append(out, w)
		}
	}
	return out, nil
}

func (s *Store) ListWorkoutSummaries(ctx context.Context, userID int64, limit, offset int) ([]domain.WorkoutSummary, error) {
	var out []domain.WorkoutSummary
	for _, w := range s.completed(userID) {
		d, _ := s.GetWorkoutDetail(ctx, userID, w.ID)
		if len(d.Exercises) == 0 {
			continue
		}
		sum := domain.WorkoutSummary{Workout: w, ExerciseCount: len(d.Exercises)}
		seen := map[string]bool{}
		for _, ex := range d.Exercises {
			if !seen[ex.MuscleGroup] {
				seen[ex.MuscleGroup] = true
				sum.MuscleGroups = append(sum.MuscleGroups, ex.MuscleGroup)
			}
		}
		sort.Strings(sum.MuscleGroups)
		out = append(out, sum)
	}
	if offset >= len(out) {
		return nil, nil
	}
	out = out[offset:]
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) GetWorkoutDetail(ctx context.Context, userID int64, workoutID int) (domain.WorkoutDetail, error) {
	w := s.workout(workoutID, userID)
	if w == nil {
		return domain.WorkoutDetail{}, domain.ErrNotFound
	}
	d := domain.WorkoutDetail{Workout: *w}
	for _, we := range s.logged {
		if we.WorkoutID != workoutID {
			continue
		}
		ex, _ := s.GetExercise(ctx, we.ExerciseID)
		g, _ := s.GetMuscleGroup(ctx, ex.MuscleGroupID)
		d.Exercises = append(d.Exercises, domain.WorkoutExerciseDetail{
			ExerciseID:   ex.ID,
			ExerciseName: ex.Name,
			MuscleGroup:  g.Name,
			Sets:         append([]domain.SetEntry(nil), we.Sets...),
		})
	}
	return d, nil
}

func (s *Store) ListExerciseProgress(ctx context.Context, userID int64, exerciseID int, since time.Time) ([]domain.ExerciseProgressPoint, error) {
	var out []domain.ExerciseProgressPoint
	ws := s.completed(userID)
	for i := len(ws) - 1; i >= 0; i-- {
		if ws[i].Date.Before(since) {
			continue
		}
		for _, we := range s.logged {
			if we.WorkoutID == ws[i].ID && we.ExerciseID == exerciseID {
				out = append(out, domain.NewProgressPoint(ws[i].ID, ws[i].Date, we.Sets))
			}
		}
	}
	return out, nil
}

func (s *Store) ListMuscleGroupProgress(ctx context.Context, userID int64, muscleGroupID int, since time.Time) ([]domain.ExerciseProgressSeries, error) {
	var out []domain.ExerciseProgressSeries
	exercises, _ := s.ListExercisesByGroup(ctx, muscleGroupID)
	for _, ex := range exercises {
		points, _ := s.ListExerciseProgress(ctx, userID, ex.ID, since)
		if len(points) > 0 {
			out = append(out, domain.ExerciseProgressSeries{ExerciseID: ex.ID, ExerciseName: ex.Name, Points: points})
		}
	}
	return out, nil
}

// Backup

func (s *Store) ExportRows(ctx context.Context, userID int64) ([]domain.ExportWorkout, error) {
	var out []domain.ExportWorkout
	ws := s.completed(userID)
	for i := len(ws) - 1; i >= 0; i-- {
		d, _ := s.GetWorkoutDetail(ctx, userID, ws[i].ID)
		ew := domain.ExportWorkout{Date: ws[i].Date, Status: string(ws[i].Status)}
		for _, ex := range d.Exercises {
			ew.Exercises = append(ew.Exercises, domain.ExportWorkoutExercise{
				ExerciseName: ex.ExerciseName,
				MuscleGroup:  ex.MuscleGroup,
				Sets:         ex.Sets,
			})
		}
		out = append(out, ew)
	}
	return out, nil
}

func (s *Store) WorkoutExerciseExistsOnDate(ctx context.Context, userID int64, exerciseID int, date time.Time) (bool, error) {
	day := date.UTC().Format(time.DateOnly)
	for _, we := range s.logged {
		w := s.workout(we.WorkoutID, userID)
		if w != nil && w.Status == domain.WorkoutCompleted && we.ExerciseID == exerciseID && w.Date.UTC().Format(time.DateOnly) == day {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) CreateImportedWorkout(ctx context.Context, userID int64, date time.Time) (int, error) {
	w := domain.Workout{ID: s.id(), UserID: userID, Date: date, Status: domain.WorkoutCompleted}
	s.workouts = append(s.workouts, w)
	return w.ID, nil
}

func (s *Store) AddExerciseToWorkout(ctx context.Context, workoutID, exerciseID int, sets []domain.SetEntry) error {
	sets = append([]domain.SetEntry(nil), sets...)
	for i := range s.logged {
		if s.logged[i].WorkoutID == workoutID && s.logged[i].ExerciseID == exerciseID {
			s.logged[i].Sets = sets
			return nil
		}
	}
	s.logged = append(s.logged, domain.WorkoutExercise{ID: s.id(), WorkoutID: workoutID, ExerciseID: exerciseID, Sets: sets})
	return nil
}

func (s *Store) DeleteAllUserWorkouts(ctx context.Context, userID int64) error {
	s.deleteWorkouts(func(w domain.Workout) bool { return w.UserID == userID })
	return nil
}

// Settings

func (s *Store) GetSettings(ctx context.Context, userID int64) (domain.UserSettings, error) {
	if st, ok := s.settings[userID]; ok {
		return st, nil
	}
	return domain.UserSettings{UserID: userID}, nil
}

func (s *Store) SetWeeklyBackup(ctx context.Context, userID int64, enabled bool) error {
	st, _ := s.GetSettings(ctx, userID)
	st.WeeklyBackupEnabled = enabled
	s.settings[userID] = st
	return nil
}

func (s *Store) SetLastBackupAt(ctx context.Context, userID int64, at time.Time) error {
	st, _ := s.GetSettings(ctx, userID)
	st.LastBackupAt = &at
	s.settings[userID] = st
	return nil
}

func (s *Store) ListUsersDueWeeklyBackup(ctx context.Context, before time.Time) ([]int64, error) {
	var out []int64
	for uid, st := range s.settings {
		if st.WeeklyBackupEnabled && (st.LastBackupAt == nil || st.LastBackupAt.Before(before)) {
			out = append(out, uid)
		}
	}
	return out, nil
}

var (
	_ domain.CatalogRepository   = (*Store)(nil)
	_ domain.WorkoutRepository   = (*Store)(nil)
	_ domain.SessionRepository   = (*Store)(nil)
	_ domain.AnalyticsRepository = (*Store)(nil)
	_ domain.BackupRepository    = (*Store)(nil)
	_ domain.SettingsRepository  = (*Store)(nil)
)
