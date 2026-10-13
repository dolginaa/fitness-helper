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

func TestSetFormatsByKind(t *testing.T) {
	t.Parallel()
	sets := []domain.SetEntry{
		{Reps: 10, Weight: 52.5},
		{Reps: 15, Kind: domain.LoadBodyweight},
		{Reps: 8, Weight: 5, Kind: domain.LoadBodyweight},
		{Reps: 12, Band: 4, Kind: domain.LoadBand},
	}
	if got := Sets(sets); got != "10×52.5, 15×св.вес, 8×св.вес+5, 12×рез.4" {
		t.Fatalf("lists: %q", got)
	}
	want := []string{"10 × 52.5 кг", "15 × свой вес", "8 × свой вес +5 кг", "12 × резинка 4"}
	for i, s := range sets {
		if got := SetLine(s); got != want[i] {
			t.Fatalf("%+v: got %q want %q", s, got, want[i])
		}
	}
}

func TestSmallFormats(t *testing.T) {
	t.Parallel()
	for d, want := range map[time.Duration]string{
		52 * time.Minute:                "52 мин",
		time.Hour:                       "1 ч",
		72*time.Minute + 20*time.Second: "1 ч 12 мин",
	} {
		if got := Duration(d); got != want {
			t.Fatalf("%v: got %q want %q", d, got, want)
		}
	}
	for _, c := range []struct {
		before, now float64
		want        string
	}{{100, 108, "+8%"}, {100, 97, "−3%"}, {100, 100, "0%"}, {0, 50, ""}} {
		if got := Percent(c.before, c.now); got != c.want {
			t.Fatalf("%v→%v: got %q want %q", c.before, c.now, got, c.want)
		}
	}
	if DaysAgo(0) != "сегодня" || DaysAgo(1) != "вчера" || DaysAgo(5) != "5 дн. назад" {
		t.Fatal("days ago")
	}
}

func TestWorkoutReportText(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 10, 4, 12, 0, 0, 0, time.Local)
	press := []domain.SetEntry{{Reps: 10, Weight: 105}, {Reps: 8, Weight: 105}}
	r := usecase.WorkoutReport{
		Workout: domain.WorkoutDetail{
			Workout:   domain.Workout{Date: day},
			Exercises: []domain.WorkoutExerciseDetail{{ExerciseName: "Жим <ногами>", MuscleGroup: "ноги", Sets: press}},
		},
		Duration:   52 * time.Minute,
		Sets:       2,
		Reps:       18,
		Volume:     1890,
		Groups:     []string{"ноги"},
		PrevDate:   day.AddDate(0, 0, -3),
		PrevVolume: 1750,

		DaysSincePrev: 3,
		WeekWorkouts:  3,
		Records:       []usecase.Record{{Exercise: "Жим <ногами>", Kind: usecase.RecordWeight, Value: 105, Previous: 100}},
		Changes: []usecase.ExerciseChange{
			{Exercise: "Жим <ногами>", Load: domain.LoadWeight,
				Before: domain.ExerciseProgressPoint{MaxWeight: 100, Volume: 1750}, Now: domain.ExerciseProgressPoint{MaxWeight: 105, Volume: 1890}},
			{Exercise: "Икры", Load: domain.LoadWeight,
				Before: domain.ExerciseProgressPoint{MaxWeight: 40, Volume: 800}, Now: domain.ExerciseProgressPoint{MaxWeight: 40, Volume: 800}},
			{Exercise: "Подтягивания", Load: domain.LoadBand,
				Before: domain.ExerciseProgressPoint{Band: 4, TotalReps: 20}, Now: domain.ExerciseProgressPoint{Band: 3, TotalReps: 24}},
		},
		FirstTime: []string{"Присед"},
	}
	text := WorkoutReport(r)
	for _, want := range []string{
		"🏁 <b>Тренировка завершена",
		"вс 04.10.2026 · ⏱ 52 мин · ноги",
		"📊 Упражнений: 1 · подходов: 2 · повторов: 18 · тоннаж: 1 890 кг (+8% к прошлой)",
		"🏆 <b>Рекорды</b>\n• Жим &lt;ногами&gt; — вес 105 кг (было 100)",
		"• Жим &lt;ногами&gt;: 100 → 105 кг, объём +8%",
		"• Подтягивания: резинка 4 → 3, повторов 20 → 24",
		"🆕 Впервые: Присед",
		"📅 тренировок за 7 дней: 3 · прошлая — 3 дн. назад",
		"• Жим &lt;ногами&gt; — 10×105, 8×105",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Икры:") {
		t.Fatalf("an exercise that did not change is not listed:\n%s", text)
	}

	// A bare workout: no duration, no records, nothing to compare, no tonnage.
	bare := usecase.WorkoutReport{Workout: r.Workout, Sets: 2, Reps: 18}
	text = WorkoutReport(bare)
	for _, unwanted := range []string{"⏱", "🏆", "📈", "🆕", "📅", "тоннаж: 0"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("%q must not be there:\n%s", unwanted, text)
		}
	}
}

func TestExerciseLogPromptsByKind(t *testing.T) {
	t.Parallel()
	ex := domain.Exercise{Name: "Подтягивания", Loads: []domain.LoadKind{domain.LoadWeight, domain.LoadBand, domain.LoadBodyweight}}
	cases := map[domain.LoadKind][]string{
		domain.LoadWeight:     {"· вес", "<code>10 50</code>"},
		domain.LoadBand:       {"· резинка", "уровень резинки", "<code>12 4</code>"},
		domain.LoadBodyweight: {"· свой вес", "повторы: <code>15</code>", "<code>15, 12, 10</code>"},
	}
	for kind, wants := range cases {
		text := ExerciseLog(usecase.ExerciseLog{Exercise: ex, Kind: kind})
		for _, want := range wants {
			if !strings.Contains(text, want) {
				t.Fatalf("%s: missing %q in:\n%s", kind, want, text)
			}
		}
	}
	// With one kind of load there is nothing to name.
	if text := ExerciseLog(usecase.ExerciseLog{Exercise: domain.Exercise{Name: "Икры"}, Kind: domain.LoadWeight}); strings.Contains(text, "· вес") {
		t.Fatalf("a single kind is not named:\n%s", text)
	}
	if text := ExerciseLog(usecase.ExerciseLog{Exercise: ex, Kind: domain.LoadBand, PendingReps: 12}); !strings.Contains(text, "уровень резинки (1–10)") {
		t.Fatalf("pending reps of a band set ask for the level:\n%s", text)
	}
}
