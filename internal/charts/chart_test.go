package charts

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

// progress builds weekly workouts of three sets of ten with the given weights.
func progress(weights ...float64) []domain.ExerciseProgressPoint {
	now := time.Now()
	points := make([]domain.ExerciseProgressPoint, 0, len(weights))
	for i, w := range weights {
		sets := []domain.SetEntry{{Reps: 10, Weight: w}, {Reps: 10, Weight: w}, {Reps: 8 + i, Weight: w}}
		points = append(points, domain.ExerciseProgressPoint{
			Date:          now.AddDate(0, 0, -7*(len(weights)-1-i)),
			MaxWeight:     domain.MaxWeightFromSets(sets),
			WorkingWeight: domain.WorkingWeightFromSets(sets),
			Sets:          sets,
		})
	}
	return points
}

func assertPNG(t *testing.T, data []byte, wantHeight int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a png: %v", err)
	}
	if b := img.Bounds(); b.Dx() != canvasWidth || b.Dy() != wantHeight {
		t.Fatalf("unexpected size %dx%d", b.Dx(), b.Dy())
	}
	save(t, t.Name(), data)
}

// save keeps the rendered chart when CHART_OUT_DIR is set:
// CHART_OUT_DIR=bin/charts go test ./internal/charts
func save(t *testing.T, name string, data []byte) {
	t.Helper()
	dir := os.Getenv("CHART_OUT_DIR")
	if dir == "" {
		return
	}
	name = strings.NewReplacer("/", "_", " ", "_").Replace(name)
	if err := os.WriteFile(filepath.Join(dir, name+".png"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExerciseProgress(t *testing.T) {
	t.Parallel()
	data, err := New().ExerciseProgress("Жим ногами", progress(40, 45, 45, 50, 52.5))
	if err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
}

func TestExerciseProgressFlatLine(t *testing.T) {
	t.Parallel()
	data, err := New().ExerciseProgress("Бабочка", progress(22, 22, 22))
	if err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
}

func TestExerciseProgressWithoutWeight(t *testing.T) {
	t.Parallel()
	data, err := New().ExerciseProgress("Планка", progress(0, 0, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
}

func TestExerciseProgressManyWorkouts(t *testing.T) {
	t.Parallel()
	weights := make([]float64, 26)
	for i := range weights {
		weights[i] = 40 + float64(i/3)*2.5
	}
	data, err := New().ExerciseProgress("Жим на грудь под 45 в тренажере", progress(weights...))
	if err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
}

func TestExerciseProgressNotEnoughData(t *testing.T) {
	t.Parallel()
	if _, err := New().ExerciseProgress("Жим ногами", progress(40)); !errors.Is(err, domain.ErrNotEnoughData) {
		t.Fatalf("want ErrNotEnoughData, got %v", err)
	}
}

func TestMuscleGroupProgress(t *testing.T) {
	t.Parallel()
	series := []domain.ExerciseProgressSeries{
		{ExerciseID: 1, ExerciseName: "Жим ногами", Points: progress(100, 110, 120)},
		{ExerciseID: 2, ExerciseName: "Румынская тяга", Points: progress(40, 45, 50, 50)},
		{ExerciseID: 3, ExerciseName: "Разгибания ног сидя", Points: progress(35, 30)},
		{ExerciseID: 4, ExerciseName: "Икры", Points: progress(40)},
	}
	data, err := New().MuscleGroupProgress("ноги", series)
	if err != nil {
		t.Fatal(err)
	}
	// Three exercises have enough data: two rows of the grid.
	assertPNG(t, data, groupTop+2*cellHeight)

	if _, err := New().MuscleGroupProgress("ноги", series[3:]); !errors.Is(err, domain.ErrNotEnoughData) {
		t.Fatalf("want ErrNotEnoughData, got %v", err)
	}
}

func TestNumber(t *testing.T) {
	t.Parallel()
	for v, want := range map[float64]string{0: "0", 52.5: "52.5", 50: "50", 3900: "3 900", 1234567.04: "1 234 567", -2.5: "−2.5"} {
		if got := number(v); got != want {
			t.Fatalf("%v: got %q want %q", v, got, want)
		}
	}
}

func TestAxisDomains(t *testing.T) {
	t.Parallel()
	lo, hi, step := lineDomain([]float64{22, 25, 20, 22})
	if lo > 20 || hi < 25 || step <= 0 {
		t.Fatalf("line domain [%v, %v] step %v does not cover the values", lo, hi, step)
	}
	if lo, hi, _ := lineDomain([]float64{22, 22}); lo >= 22 || hi <= 22 {
		t.Fatalf("a flat line needs room around it, got [%v, %v]", lo, hi)
	}
	if hi, step := barDomain([]float64{570, 528, 594}); hi < 594 || step <= 0 {
		t.Fatalf("bar domain %v step %v does not cover the values", hi, step)
	}
	if hi, _ := barDomain([]float64{0, 0}); hi <= 0 {
		t.Fatalf("empty bars still need an axis, got %v", hi)
	}
}

// TestPreviewFromExport renders charts from a real export file to look at them:
// CHART_EXPORT=export.json CHART_OUT_DIR=bin/charts go test ./internal/charts -run Preview
func TestPreviewFromExport(t *testing.T) {
	path := os.Getenv("CHART_EXPORT")
	if path == "" || os.Getenv("CHART_OUT_DIR") == "" {
		t.Skip("CHART_EXPORT and CHART_OUT_DIR are not set")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var bundle domain.ExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatal(err)
	}
	sort.Slice(bundle.Workouts, func(i, j int) bool { return bundle.Workouts[i].Date.Before(bundle.Workouts[j].Date) })

	byGroup := map[string]map[string]*domain.ExerciseProgressSeries{}
	for _, w := range bundle.Workouts {
		for _, ex := range w.Exercises {
			if byGroup[ex.MuscleGroup] == nil {
				byGroup[ex.MuscleGroup] = map[string]*domain.ExerciseProgressSeries{}
			}
			s := byGroup[ex.MuscleGroup][ex.ExerciseName]
			if s == nil {
				s = &domain.ExerciseProgressSeries{ExerciseName: ex.ExerciseName}
				byGroup[ex.MuscleGroup][ex.ExerciseName] = s
			}
			s.Points = append(s.Points, domain.ExerciseProgressPoint{
				Date:          w.Date,
				MaxWeight:     domain.MaxWeightFromSets(ex.Sets),
				WorkingWeight: domain.WorkingWeightFromSets(ex.Sets),
				Sets:          ex.Sets,
			})
		}
	}
	for group, exercises := range byGroup {
		var series []domain.ExerciseProgressSeries
		for _, s := range exercises {
			series = append(series, *s)
			if data, err := New().ExerciseProgress(s.ExerciseName, s.Points); err == nil {
				save(t, group+"_"+s.ExerciseName, data)
			}
		}
		sort.Slice(series, func(i, j int) bool { return series[i].ExerciseName < series[j].ExerciseName })
		if data, err := New().MuscleGroupProgress(group, series); err == nil {
			save(t, "группа_"+group, data)
		}
	}
}
