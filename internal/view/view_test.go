package view

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

func TestSets(t *testing.T) {
	t.Parallel()
	got := Sets([]domain.SetEntry{{Reps: 10, Weight: 50}, {Reps: 8, Weight: 52.5}, {Reps: 60, Weight: 0}})
	if got != "10×50, 8×52.5, 60×0" {
		t.Fatalf("unexpected format: %q", got)
	}
	if Sets(nil) != NoData {
		t.Fatal("nil sets")
	}
}

func TestExerciseList(t *testing.T) {
	t.Parallel()
	at := time.Date(2020, 2, 8, 15, 0, 0, 0, time.Local)
	list := usecase.ExerciseList{
		Group: domain.MuscleGroup{ID: 1, Name: "ноги"},
		Rows: []usecase.ExerciseRow{{
			Exercise: domain.Exercise{ID: 1, Name: "Жим ногами"},
			LastAt:   at,
			LastSets: []domain.SetEntry{{Reps: 10, Weight: 50}, {Reps: 8, Weight: 52.5}},
		}, {
			Exercise: domain.Exercise{ID: 2, Name: "Икры"},
		}, {
			Exercise:  domain.Exercise{ID: 3, Name: "Присед <со штангой>"},
			TodaySets: []domain.SetEntry{{Reps: 5, Weight: 60}},
		}},
	}
	text := ExerciseList(list, true)
	for _, want := range []string{
		"<b>Ноги</b>",
		"Жим ногами — 08.02.2020: 10×50, 8×52.5 (раб. вес 51.1 кг)",
		"Икры — нет данных",
		"✅ <b>Присед &lt;со штангой&gt;</b> — сегодня 5×60",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
}

func TestExerciseLog(t *testing.T) {
	t.Parallel()
	log := usecase.ExerciseLog{
		Exercise: domain.Exercise{Name: "Жим ногами"},
		Sets:     []domain.SetEntry{{Reps: 10, Weight: 100}, {Reps: 8, Weight: 102.5}},
	}
	text := ExerciseLog(log)
	if !strings.Contains(text, "1) 10 × 100 кг\n2) 8 × 102.5 кг") || !strings.Contains(text, "Прошлый раз: нет данных") {
		t.Fatalf("unexpected text:\n%s", text)
	}
	log.PendingReps = 12
	if text = ExerciseLog(log); !strings.Contains(text, "Повторов: <b>12</b>") {
		t.Fatalf("pending reps are not shown:\n%s", text)
	}
}

func TestWorkoutDetail(t *testing.T) {
	t.Parallel()
	d := domain.WorkoutDetail{
		Workout: domain.Workout{Date: time.Date(2026, 2, 7, 15, 20, 0, 0, time.Local)},
		Exercises: []domain.WorkoutExerciseDetail{
			{ExerciseName: "Жим ногами", MuscleGroup: "ноги", Sets: []domain.SetEntry{{Reps: 10, Weight: 100}, {Reps: 10, Weight: 100}}},
			{ExerciseName: "Гиперэкстензия", MuscleGroup: "спина", Sets: []domain.SetEntry{{Reps: 40, Weight: 5}}},
			{ExerciseName: "Икры", MuscleGroup: "ноги", Sets: []domain.SetEntry{{Reps: 20, Weight: 40}}},
		},
	}
	want := "<b>Тренировка сб 07.02.2026</b>, 15:20\n" +
		"\n<b>Ноги</b>\n• Жим ногами — 10×100, 10×100\n• Икры — 20×40\n" +
		"\n<b>Спина</b>\n• Гиперэкстензия — 40×5\n" +
		"\nУпражнений: 3 · подходов: 4 · тоннаж: 3 000 кг"
	if got := WorkoutDetail(d); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestHistoryItem(t *testing.T) {
	t.Parallel()
	s := domain.WorkoutSummary{
		Workout:      domain.Workout{Date: time.Date(2026, 2, 8, 15, 20, 0, 0, time.Local)},
		MuscleGroups: []string{"ноги", "спина"},
	}
	if got := HistoryItem(s); got != "вс 08.02.2026 · ноги, спина" {
		t.Fatalf("unexpected label %q", got)
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()
	if got := Split("короткий текст", 100); len(got) != 1 || got[0] != "короткий текст" {
		t.Fatalf("short text must stay intact: %q", got)
	}
	text := strings.Repeat("строка из кириллицы\n", 50)
	parts := Split(text, 100)
	if len(parts) < 2 {
		t.Fatalf("expected several parts, got %d", len(parts))
	}
	for _, p := range parts {
		if !utf8.ValidString(p) || utf8.RuneCountInString(p) > 100 {
			t.Fatalf("bad part %q", p)
		}
		if !strings.HasPrefix(p, "строка") || !strings.HasSuffix(p, "кириллицы") {
			t.Fatalf("part is not cut at line ends: %q", p)
		}
	}
	if got := strings.Join(parts, "\n") + "\n"; got != text {
		t.Fatal("text is not preserved")
	}
}

func TestThousands(t *testing.T) {
	t.Parallel()
	for v, want := range map[float64]string{0: "0", 920: "920", 3000: "3 000", 1234567.4: "1 234 567"} {
		if got := thousands(v); got != want {
			t.Fatalf("%v: got %q want %q", v, got, want)
		}
	}
}
