package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/postgres"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testUser int64 = 42

// newBarePool connects to TEST_DATABASE_URL and creates a throwaway schema, so the test never
// touches existing data. Without the variable the test is skipped.
func newBarePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	return pool
}

// applyFiles runs SQL files the way the migration tool does: each file as one batch.
func applyFiles(t *testing.T, pool *pgxpool.Pool, paths ...string) {
	t.Helper()
	for _, path := range paths {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

// newPool is a database with every migration applied.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := newBarePool(t)
	migrations, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("migrations not found: %v", err)
	}
	sort.Strings(migrations)
	applyFiles(t, pool, migrations...)
	return pool
}

func newService(pool *pgxpool.Pool) *usecase.Service {
	q := sqlcgen.New(pool)
	return usecase.NewService(
		postgres.NewCatalog(pool, q),
		postgres.NewWorkout(pool, q),
		postgres.NewSession(q),
		postgres.NewAnalytics(q),
		postgres.NewBackup(pool, q),
		postgres.NewSettings(q),
	)
}

func group(t *testing.T, groups []domain.MuscleGroup, name string) domain.MuscleGroup {
	t.Helper()
	for _, g := range groups {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("no muscle group %q in %+v", name, groups)
	return domain.MuscleGroup{}
}

func TestWorkoutFlow(t *testing.T) {
	pool := newPool(t)
	svc := newService(pool)
	ctx := context.Background()

	groups, err := svc.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	legs, back := group(t, groups, "ноги"), group(t, groups, "спина")

	// An empty workout is discarded when the next one starts.
	if _, err := svc.StartNewWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartNewWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}

	list, err := svc.SelectMuscleGroup(ctx, testUser, legs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if list.Group.Name != "ноги" || len(list.Rows) == 0 {
		t.Fatalf("unexpected list: %+v", list)
	}
	press := list.Rows[0].Exercise

	if _, err := svc.BeginExercise(ctx, testUser, press.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LogInput(ctx, testUser, "10 100, 8 110"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RepeatSet(ctx, testUser, press.ID); err != nil {
		t.Fatal(err)
	}
	log, err := svc.UndoLastSet(ctx, testUser, press.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SetEntry{{Reps: 10, Weight: 100}, {Reps: 8, Weight: 110}}
	if !reflect.DeepEqual(log.Sets, want) {
		t.Fatalf("got %+v want %+v", log.Sets, want)
	}

	// A custom exercise, logged and then fully undone, must leave no trace in the workout.
	if _, err := svc.BackToExercises(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BeginAddExercise(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	custom, err := svc.AddExercise(ctx, testUser, "Болгарские выпады")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LogInput(ctx, testUser, "12 20"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UndoLastSet(ctx, testUser, custom.Exercise.ID); err != nil {
		t.Fatal(err)
	}

	// A second muscle group, to check the history summary.
	if err := svc.BackToGroups(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	backList, err := svc.SelectMuscleGroup(ctx, testUser, back.ID)
	if err != nil {
		t.Fatal(err)
	}
	row := backList.Rows[0].Exercise
	if _, err := svc.BeginExercise(ctx, testUser, row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LogInput(ctx, testUser, "40 5"); err != nil {
		t.Fatal(err)
	}

	detail, err := svc.EndWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Exercises) != 2 || detail.Exercises[0].ExerciseID != press.ID || detail.Exercises[1].ExerciseID != row.ID {
		t.Fatalf("exercises must come in the order they were done: %+v", detail.Exercises)
	}
	if _, active, err := svc.ActiveWorkout(ctx, testUser); err != nil || active {
		t.Fatalf("no workout must stay active: active=%v err=%v", active, err)
	}

	page, err := svc.History(ctx, testUser, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.HasNext {
		t.Fatalf("history must have exactly the finished workout: %+v", page)
	}
	item := page.Items[0]
	if item.Workout.ID != started.ID || item.ExerciseCount != 2 || !reflect.DeepEqual(item.MuscleGroups, []string{"ноги", "спина"}) {
		t.Fatalf("unexpected summary: %+v", item)
	}

	// The previous result shows up in the next workout.
	overview, err := svc.ExerciseOverview(ctx, testUser, legs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(overview.Rows[0].LastSets, want) || overview.Rows[0].LastAt.IsZero() {
		t.Fatalf("last result is missing: %+v", overview.Rows[0])
	}
	progress, err := svc.ExerciseProgress(ctx, testUser, press.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(progress.Points) != 1 || progress.Points[0].MaxWeight != 110 {
		t.Fatalf("unexpected progress: %+v", progress.Points)
	}
	groupProgress, err := svc.MuscleGroupProgress(ctx, testUser, legs.ID)
	if err != nil {
		t.Fatal(err)
	}
	if groupProgress.Group.ID != legs.ID || len(groupProgress.Series) != 1 {
		t.Fatalf("unexpected group progress: %+v", groupProgress)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	onDate, err := svc.WorkoutsOnDate(ctx, testUser, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(onDate) != 1 || onDate[0].Workout.ID != started.ID {
		t.Fatalf("today's workout is not found by date: %+v", onDate)
	}
	if onDate, _ = svc.WorkoutsOnDate(ctx, testUser, today.AddDate(0, 0, -1)); len(onDate) != 0 {
		t.Fatalf("unexpected workouts yesterday: %+v", onDate)
	}

	// Another user sees nothing of this.
	if _, err := svc.GetWorkout(ctx, testUser+1, started.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound for a foreign workout, got %v", err)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	pool := newPool(t)
	svc := newService(pool)
	ctx := context.Background()

	groups, err := svc.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartNewWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	list, err := svc.SelectMuscleGroup(ctx, testUser, groups[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BeginExercise(ctx, testUser, list.Rows[0].Exercise.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LogInput(ctx, testUser, "10 52,5"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EndWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}

	archive, _, err := svc.ExportUserData(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.ImportUserData(ctx, testUser, archive, domain.ImportModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkoutsAdded != 0 || res.WorkoutsSkipped != 1 {
		t.Fatalf("merge of own export must add nothing: %+v", res)
	}
	res, err = svc.ImportUserData(ctx, testUser, archive, domain.ImportModeReplace)
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkoutsAdded != 1 || res.ExercisesAdded != 1 {
		t.Fatalf("unexpected replace result: %+v", res)
	}
	page, err := svc.History(ctx, testUser, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("history after replace: %+v", page)
	}

	if err := svc.SetWeeklyBackup(ctx, testUser, true); err != nil {
		t.Fatal(err)
	}
	due, err := svc.ListUsersDueWeeklyBackup(ctx)
	if err != nil || !reflect.DeepEqual(due, []int64{testUser}) {
		t.Fatalf("user must be due for a backup: %v, %v", due, err)
	}
	if err := svc.MarkBackupSent(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	if due, _ = svc.ListUsersDueWeeklyBackup(ctx); len(due) != 0 {
		t.Fatalf("backup was just sent, nobody is due: %v", due)
	}
}

func loadsOf(t *testing.T, pool *pgxpool.Pool, name string) []string {
	t.Helper()
	var loads []string
	if err := pool.QueryRow(context.Background(), "SELECT loads FROM exercises WHERE name = $1", name).Scan(&loads); err != nil {
		t.Fatalf("%q: %v", name, err)
	}
	return loads
}

func scalar[T any](t *testing.T, pool *pgxpool.Pool, query string, args ...any) T {
	t.Helper()
	var v T
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return v
}

func exec(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// TestUpgradeOfExistingData starts from a database like the one of a user who has used the bot
// since the first version: the old exercise list (written in lower case) next to the same exercises
// written with a capital letter that later imports added. The new migrations and the tidying must
// keep every set.
func TestUpgradeOfExistingData(t *testing.T) {
	pool := newBarePool(t)
	ctx := context.Background()
	applyFiles(t, pool,
		"../../../migrations/000001_schema.up.sql",
		"testdata/old_seed.sql",
		"../../../migrations/000003_user_settings.up.sql",
	)
	for _, ex := range []struct{ upper, lower string }{{"Подтягивания", "подтягивания"}, {"Планка", "планка"}, {"Жим ногами", "жим ногами"}} {
		exec(t, pool, "INSERT INTO exercises (name, muscle_group_id) SELECT $1, muscle_group_id FROM exercises WHERE name = $2", ex.upper, ex.lower)
	}
	// The progress of an exercise covers the last half a year, so the workouts must be recent.
	ago := func(days int) string { return time.Now().AddDate(0, 0, -days).UTC().Format(time.RFC3339) }
	exerciseID := func(name string) int32 {
		return scalar[int32](t, pool, "SELECT id FROM exercises WHERE name = $1", name)
	}
	workout := func(day string, sets map[string]string) int32 {
		id := scalar[int32](t, pool, "INSERT INTO workouts (user_id, date, status) VALUES ($1, $2::timestamptz, 'completed') RETURNING id", testUser, day)
		for name, js := range sets {
			exec(t, pool, "INSERT INTO workout_exercises (workout_id, exercise_id, sets) VALUES ($1, $2, $3::jsonb)", id, exerciseID(name), js)
		}
		return id
	}
	workout(ago(30), map[string]string{
		"подтягивания": `[{"reps":8,"weight":20},{"reps":6,"weight":20}]`,
		"планка":       `[{"reps":60,"weight":0}]`,
		"жим ногами":   `[{"reps":10,"weight":100}]`,
	})
	workout(ago(23), map[string]string{
		"Подтягивания": `[{"reps":9,"weight":22}]`,
		"Планка":       `[{"reps":70,"weight":0},{"reps":50,"weight":5}]`,
		"Жим ногами":   `[{"reps":10,"weight":105}]`,
	})
	both := workout(ago(16), map[string]string{
		"подтягивания": `[{"reps":5,"weight":25}]`,
		"Подтягивания": `[{"reps":4,"weight":25}]`,
	})
	countSets := func() int {
		return scalar[int](t, pool, "SELECT COALESCE(SUM(jsonb_array_length(sets)), 0)::int FROM workout_exercises")
	}
	setsBefore := countSets()

	applyFiles(t, pool, "../../../migrations/000004_exercise_loads.up.sql", "../../../migrations/000005_workout_finished_at.up.sql")

	if got := countSets(); got != setsBefore {
		t.Fatalf("the migration changed the number of sets: %d -> %d", setsBefore, got)
	}
	want := []string{"weight", "band", "bodyweight"}
	for _, name := range []string{"подтягивания", "Подтягивания"} {
		if got := loadsOf(t, pool, name); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: loads %v, want %v", name, got, want)
		}
	}
	if got := loadsOf(t, pool, "Планка"); !reflect.DeepEqual(got, []string{"bodyweight", "weight"}) {
		t.Fatalf("plank loads: %v", got)
	}
	if got := loadsOf(t, pool, "Жим ногами"); !reflect.DeepEqual(got, []string{"weight"}) {
		t.Fatalf("a machine exercise stays weight only: %v", got)
	}
	if got := loadsOf(t, pool, "Отжимания"); !reflect.DeepEqual(got, []string{"bodyweight", "weight", "band"}) {
		t.Fatalf("push-ups are added for the body: %v", got)
	}
	// Bare "reps × 0 kg" sets of a bodyweight exercise become bodyweight sets; sets with a weight and
	// sets of other exercises are left as they were.
	plank := scalar[string](t, pool, "SELECT sets::text FROM workout_exercises WHERE exercise_id = $1 AND workout_id = (SELECT MIN(id) FROM workouts)", exerciseID("планка"))
	if plank != `[{"kind": "bodyweight", "reps": 60, "weight": 0}]` {
		t.Fatalf("plank sets: %s", plank)
	}
	plank = scalar[string](t, pool, "SELECT sets::text FROM workout_exercises WHERE exercise_id = $1", exerciseID("Планка"))
	if plank != `[{"kind": "bodyweight", "reps": 70, "weight": 0}, {"reps": 50, "weight": 5}]` {
		t.Fatalf("a set with a weight must stay as it was: %s", plank)
	}
	if got := scalar[string](t, pool, "SELECT sets::text FROM workout_exercises WHERE exercise_id = $1", exerciseID("Жим ногами")); got != `[{"reps": 10, "weight": 105}]` {
		t.Fatalf("machine sets must not change: %s", got)
	}
	if got := scalar[int](t, pool, "SELECT COUNT(*)::int FROM workouts WHERE finished_at IS NOT NULL"); got != 0 {
		t.Fatalf("old workouts have no finish time: %d", got)
	}

	// Now tidy the list through the same code the command uses.
	svc := newService(pool)
	report, err := svc.ExerciseCatalogReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Merges) != 3 {
		t.Fatalf("three exercises are written twice: %+v", report.Merges)
	}
	for _, m := range report.Merges {
		if m.Keep.Exercise.Name[0] >= 0x80 && m.Keep.Exercise.Name != "Подтягивания" && m.Keep.Exercise.Name != "Планка" && m.Keep.Exercise.Name != "Жим ногами" {
			t.Fatalf("the capitalized spelling must stay: %+v", m.Keep)
		}
	}
	if err := svc.ApplyMerges(ctx, report.Merges); err != nil {
		t.Fatal(err)
	}
	if got := countSets(); got != setsBefore {
		t.Fatalf("the tidying lost sets: %d -> %d", setsBefore, got)
	}
	if got := scalar[int](t, pool, "SELECT COUNT(*)::int FROM exercises WHERE name IN ('подтягивания', 'планка', 'жим ногами')"); got != 0 {
		t.Fatalf("the lower case spellings must be gone: %d", got)
	}
	detail, err := svc.GetWorkout(ctx, testUser, int(both))
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Exercises) != 1 || !reflect.DeepEqual(detail.Exercises[0].Sets, []domain.SetEntry{{Reps: 4, Weight: 25}, {Reps: 5, Weight: 25}}) {
		t.Fatalf("a workout that had both spellings must have one record with all sets: %+v", detail.Exercises)
	}
	progress, err := svc.ExerciseProgress(ctx, testUser, int(exerciseID("Подтягивания")))
	if err != nil || len(progress.Points) != 3 {
		t.Fatalf("the history of the kept exercise must be whole: %d points, %v", len(progress.Points), err)
	}
	again, err := svc.ExerciseCatalogReport(ctx)
	if err != nil || len(again.Merges) != 0 {
		t.Fatalf("nothing is left to merge: %+v, %v", again.Merges, err)
	}
	if err := svc.MergeExercises(ctx, int(exerciseID("Планка")), int(exerciseID("Планка"))); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("an exercise cannot be merged into itself: %v", err)
	}
}

func TestKindsAndReport(t *testing.T) {
	pool := newPool(t)
	svc := newService(pool)
	ctx := context.Background()

	groups, err := svc.ListMuscleGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	back := group(t, groups, "спина")
	if _, err := svc.StartNewWorkout(ctx, testUser); err != nil {
		t.Fatal(err)
	}
	list, err := svc.SelectMuscleGroup(ctx, testUser, back.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pull domain.Exercise
	for _, r := range list.Rows {
		if r.Exercise.Name == "Подтягивания" {
			pull = r.Exercise
		}
	}
	if want := []domain.LoadKind{domain.LoadWeight, domain.LoadBand, domain.LoadBodyweight}; !reflect.DeepEqual(pull.Loads, want) {
		t.Fatalf("a new database must give pull-ups their kinds: %v", pull.Loads)
	}

	if _, err := svc.BeginExercise(ctx, testUser, pull.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetLoadKind(ctx, testUser, pull.ID, domain.LoadBand); err != nil {
		t.Fatal(err)
	}
	log, err := svc.LogInput(ctx, testUser, "12 4, 10 3")
	if err != nil {
		t.Fatal(err)
	}
	if want := []domain.SetEntry{{Reps: 12, Band: 4, Kind: domain.LoadBand}, {Reps: 10, Band: 3, Kind: domain.LoadBand}}; !reflect.DeepEqual(log.Sets, want) {
		t.Fatalf("band sets must survive the database: %+v", log.Sets)
	}
	if _, err := svc.SetLoadKind(ctx, testUser, pull.ID, domain.LoadBodyweight); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LogInput(ctx, testUser, "6"); err != nil {
		t.Fatal(err)
	}
	// The group view lists exercises with their kinds too.
	loaded, err := svc.BackToExercises(ctx, testUser)
	if err != nil || len(loaded.Rows[0].Exercise.Loads) == 0 {
		t.Fatalf("exercises of a group must carry their loads: %v", err)
	}

	detail, err := svc.EndWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Workout.FinishedAt.IsZero() {
		t.Fatal("the end of a workout must be kept")
	}
	stored, err := svc.GetWorkout(ctx, testUser, detail.Workout.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d := stored.Workout.FinishedAt.Sub(detail.Workout.FinishedAt); d < -time.Second || d > time.Second {
		t.Fatalf("the stored finish time %v differs from %v", stored.Workout.FinishedAt, detail.Workout.FinishedAt)
	}
	r, err := svc.WorkoutReport(ctx, testUser, stored)
	if err != nil {
		t.Fatal(err)
	}
	if r.Sets != 3 || r.Reps != 28 || r.Volume != 0 || r.WeekWorkouts != 1 || len(r.FirstTime) != 1 || r.PrevDate.After(time.Now()) {
		t.Fatalf("unexpected report: %+v", r)
	}
	progress, err := svc.ExerciseProgress(ctx, testUser, pull.ID)
	if err != nil || len(progress.Points) != 1 || progress.Points[0].Kind != domain.LoadBand || progress.Points[0].TotalReps != 28 || progress.Points[0].WorkoutID != detail.Workout.ID {
		t.Fatalf("progress of mixed sets: %+v, %v", progress.Points, err)
	}

	// Kinds of load can be changed afterwards; the first one is the default.
	ex, err := svc.SetExerciseLoads(ctx, pull.ID, []domain.LoadKind{domain.LoadBand, domain.LoadWeight})
	if err != nil || !reflect.DeepEqual(ex.Loads, []domain.LoadKind{domain.LoadBand, domain.LoadWeight}) {
		t.Fatalf("loads: %v, %v", ex.Loads, err)
	}
}
