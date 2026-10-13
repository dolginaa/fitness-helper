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

// newPool connects to TEST_DATABASE_URL and applies the migrations into a throwaway schema,
// so the test never touches existing data. Without the variable the test is skipped.
func newPool(t *testing.T) *pgxpool.Pool {
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
	migrations, err := filepath.Glob("../../../migrations/*.up.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("migrations not found: %v", err)
	}
	sort.Strings(migrations)
	for _, path := range migrations {
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	return pool
}

func newService(pool *pgxpool.Pool) *usecase.Service {
	q := sqlcgen.New(pool)
	return usecase.NewService(
		postgres.NewCatalog(q),
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
