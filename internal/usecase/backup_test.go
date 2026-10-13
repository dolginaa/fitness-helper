package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/memory"
)

func TestParseExportZIP(t *testing.T) {
	t.Parallel()
	bundle := domain.ExportBundle{
		Version:    domain.ExportVersion,
		ExportedAt: time.Now(),
		Workouts: []domain.ExportWorkout{{
			Date:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			Status: "completed",
			Exercises: []domain.ExportWorkoutExercise{{
				ExerciseName: "жим лёжа",
				MuscleGroup:  "грудь",
				Sets:         []domain.SetEntry{{Reps: 10, Weight: 50}},
			}},
		}},
	}
	raw, _ := json.Marshal(bundle)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("export.json")
	_, _ = w.Write(raw)
	_ = zw.Close()
	parsed, err := parseExportZIP(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Workouts) != 1 || len(parsed.Workouts[0].Exercises) != 1 {
		t.Fatalf("unexpected bundle: %+v", parsed)
	}
}

func TestImportUserDataCreatesMissingExercises(t *testing.T) {
	t.Parallel()
	store := memory.NewStore()
	svc := NewService(store, store, store, store, store, store)
	bundle := domain.ExportBundle{
		Version: domain.ExportVersion,
		Workouts: []domain.ExportWorkout{{
			Date:   time.Date(2026, 2, 8, 15, 20, 0, 0, time.UTC),
			Status: "completed",
			Exercises: []domain.ExportWorkoutExercise{{
				ExerciseName: "Гиперэкстензия",
				MuscleGroup:  "спина",
				Sets:         []domain.SetEntry{{Reps: 10, Weight: 5}},
			}},
		}},
	}
	raw, _ := json.Marshal(bundle)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("export.json")
	_, _ = w.Write(raw)
	_ = zw.Close()

	res, err := svc.ImportUserData(context.Background(), 1, buf.Bytes(), domain.ImportModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkoutsAdded != 1 || res.ExercisesAdded != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	groups, _ := store.ListMuscleGroups(context.Background())
	if len(groups) != 1 || groups[0].Name != "спина" {
		t.Fatalf("expected created muscle group, got %+v", groups)
	}
	exercises, _ := store.ListExercisesByGroup(context.Background(), groups[0].ID)
	if len(exercises) != 1 || exercises[0].Name != "Гиперэкстензия" {
		t.Fatalf("expected created exercise, got %+v", exercises)
	}

	// Importing the same archive again in merge mode adds nothing.
	res, err = svc.ImportUserData(context.Background(), 1, buf.Bytes(), domain.ImportModeMerge)
	if err != nil {
		t.Fatal(err)
	}
	if res.WorkoutsAdded != 0 || res.WorkoutsSkipped != 1 {
		t.Fatalf("unexpected repeated import result: %+v", res)
	}
}

func TestExportUserDataZIP(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.store.AddCompletedWorkout(testUser, time.Now(), f.press.ID, sets(10, 100)...)
	data, name, err := f.svc.ExportUserData(context.Background(), testUser)
	if err != nil {
		t.Fatal(err)
	}
	if name == "" {
		t.Fatal("empty file name")
	}
	bundle, err := parseExportZIP(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Workouts) != 1 || bundle.Workouts[0].Exercises[0].ExerciseName != "Жим ногами" {
		t.Fatalf("bad export: %+v", bundle)
	}
}
