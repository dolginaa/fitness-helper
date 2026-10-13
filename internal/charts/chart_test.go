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
		points = append(points, domain.NewProgressPoint(i+1, now.AddDate(0, 0, -7*(len(weights)-1-i)), sets))
	}
	return points
}

// bandProgress builds weekly workouts on a band, with the level going down and the reps up.
func bandProgress(levels ...int) []domain.ExerciseProgressPoint {
	now := time.Now()
	points := make([]domain.ExerciseProgressPoint, 0, len(levels))
	for i, level := range levels {
		sets := []domain.SetEntry{
			{Reps: 8 + i, Kind: domain.LoadBand, Band: level},
			{Reps: 7 + i, Kind: domain.LoadBand, Band: level},
		}
		points = append(points, domain.NewProgressPoint(i+1, now.AddDate(0, 0, -7*(len(levels)-1-i)), sets))
	}
	return points
}

// bodyweightProgress builds weekly workouts of the body only.
func bodyweightProgress(reps ...int) []domain.ExerciseProgressPoint {
	now := time.Now()
	points := make([]domain.ExerciseProgressPoint, 0, len(reps))
	for i, r := range reps {
		sets := []domain.SetEntry{{Reps: r, Kind: domain.LoadBodyweight}, {Reps: r - 2, Kind: domain.LoadBodyweight}}
		points = append(points, domain.NewProgressPoint(i+1, now.AddDate(0, 0, -7*(len(reps)-1-i)), sets))
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

func TestExerciseProgressOfBandAndBody(t *testing.T) {
	t.Parallel()
	data, err := New().ExerciseProgress("Подтягивания", bandProgress(5, 5, 4, 3))
	if err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
	if data, err = New().ExerciseProgress("Отжимания", bodyweightProgress(10, 12, 15)); err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
}

func TestExerciseProgressMixedKinds(t *testing.T) {
	t.Parallel()
	// Weight and band workouts are not drawn on one axis: the latest kind with enough points wins.
	points := append(progress(40, 45), bandProgress(5, 4)...)
	for i := range points {
		points[i].WorkoutID = i + 1
		points[i].Date = time.Now().AddDate(0, 0, -(len(points)-i)*7)
	}
	data, err := New().ExerciseProgress("Подтягивания", points)
	if err != nil {
		t.Fatal(err)
	}
	assertPNG(t, data, exerciseHeight)
	// One band workout is not a chart, even though the weight ones are.
	if _, err := New().ExerciseProgress("Подтягивания", append(progress(40, 45), bandProgress(5)...)); err != nil {
		t.Fatalf("weight workouts must still be drawn: %v", err)
	}
	if _, err := New().ExerciseProgress("Подтягивания", append(bodyweightProgress(10), bandProgress(5)...)); !errors.Is(err, domain.ErrNotEnoughData) {
		t.Fatalf("want ErrNotEnoughData, got %v", err)
	}
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
		{ExerciseID: 5, ExerciseName: "Подтягивания", Points: bandProgress(5, 4, 3)},
		{ExerciseID: 6, ExerciseName: "Отжимания", Points: bodyweightProgress(10, 12)},
		{ExerciseID: 4, ExerciseName: "Икры", Points: progress(40)},
	}
	data, err := New().MuscleGroupProgress("ноги", series)
	if err != nil {
		t.Fatal(err)
	}
	// Five exercises have enough data: three rows of the grid.
	assertPNG(t, data, groupTop+3*cellHeight)

	if _, err := New().MuscleGroupProgress("ноги", series[len(series)-1:]); !errors.Is(err, domain.ErrNotEnoughData) {
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
