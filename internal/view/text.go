package view

import (
	"fmt"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

const (
	ExerciseNameHint = "Название должно быть от 2 до 100 символов. Напишите его ещё раз или нажмите «Отмена»."
	WorkoutDateHint  = "Укажите дату: <code>/workout 15.03.2026</code>"
	ImportHint       = "Пришлите сюда архив .zip с файлом export.json внутри (например, из выгрузки)."
	ImportAsk        = "Файл получен. Как импортировать данные?"
	ImportReplaceAsk = "⚠️ Все ваши текущие тренировки будут <b>удалены</b> и заменены данными из архива. Продолжить?"
	ImportCancelled  = "Импорт отменён."
	ImportNoFile     = "Нет файла для импорта. Пришлите архив .zip ещё раз."
	ImportFailed     = "Не получилось прочитать архив. Нужен .zip с файлом export.json из выгрузки бота."

	NoActiveWorkout = "Сейчас нет активной тренировки."
	StaleButton     = "Эта кнопка уже не действует."
	NotFound        = "Не нашёл такую запись."
	Failure         = "Что-то пошло не так. Попробуйте ещё раз."
	UnknownCommand  = "Не знаю такую команду."

	EmptyHistory     = "Завершённых тренировок пока нет."
	NoWorkoutOnDate  = "В этот день тренировок не было."
	ProgressGroups   = "📈 <b>Прогресс</b>\nВыберите группу мышц — покажу последние результаты и графики."
	NoChartData      = "Для графика нужно хотя бы две тренировки с этим упражнением."
	NoGroupChartData = "Для графика нужно хотя бы две тренировки с упражнениями этой группы."
	ExportCaption    = "Архив ваших тренировок"
	WeeklyCaption    = "Еженедельный бекап ваших тренировок"
	EmptyWorkoutEnd  = "Тренировка без упражнений — не сохраняю её."
	WorkoutStarted   = "🏋️ <b>Тренировка начата</b>\nВыберите группу мышц:"
	WorkoutGroups    = "🏋️ <b>Тренировка</b>\nВыберите группу мышц:"
	// WebAppButton is the label of the chat menu button that opens the Mini App.
	WebAppButton = "Тренировки"
)

func MainMenu(active *domain.Workout) string {
	var sb strings.Builder
	sb.WriteString("🏋️ <b>Fitness Helper</b>\n")
	sb.WriteString("Записываю подходы и веса, показываю прошлые результаты и прогресс.")
	if active != nil {
		fmt.Fprintf(&sb, "\n\n▶️ Есть незавершённая тренировка от %s.", ShortDate(active.Date))
	}
	return sb.String()
}

func ActiveWorkoutPrompt(w domain.Workout) string {
	return fmt.Sprintf("У вас есть незавершённая тренировка от %s. Продолжить её или начать новую?\n\n"+
		"Если начать новую, текущая будет завершена и сохранена.", ShortDate(w.Date))
}

// ExerciseList is the exercise picker of a workout or, outside a workout, the overview of last results.
func ExerciseList(list usecase.ExerciseList, inWorkout bool) string {
	var sb strings.Builder
	sb.WriteString(bold(title(list.Group.Name)))
	if inWorkout {
		sb.WriteString(" · выберите упражнение\n")
	} else {
		sb.WriteString(" · последние результаты\n")
	}
	if len(list.Rows) == 0 {
		sb.WriteString("\nВ этой группе пока нет упражнений.")
		return sb.String()
	}
	for _, r := range list.Rows {
		sb.WriteString("\n")
		switch {
		case len(r.TodaySets) > 0:
			fmt.Fprintf(&sb, "✅ %s — сегодня %s", bold(r.Exercise.Name), Sets(r.TodaySets))
		case len(r.LastSets) > 0:
			fmt.Fprintf(&sb, "▫️ %s — %s: %s%s", esc(r.Exercise.Name), ShortDate(r.LastAt), Sets(r.LastSets), workingWeight(r.LastSets))
		default:
			fmt.Fprintf(&sb, "▫️ %s — %s", esc(r.Exercise.Name), NoData)
		}
	}
	return sb.String()
}

// ExerciseLog is the screen of the exercise being logged.
func ExerciseLog(log usecase.ExerciseLog) string {
	var sb strings.Builder
	sb.WriteString(bold(log.Exercise.Name))
	if len(log.Exercise.Offered()) > 1 {
		fmt.Fprintf(&sb, " · %s", esc(strings.ToLower(LoadName(log.Kind))))
	}
	if len(log.LastSets) > 0 {
		fmt.Fprintf(&sb, "\nПрошлый раз (%s): %s", ShortDate(log.LastAt), Sets(log.LastSets))
	} else {
		sb.WriteString("\nПрошлый раз: " + NoData)
	}
	if len(log.Sets) > 0 {
		sb.WriteString("\n\nСегодня:")
		for i, s := range log.Sets {
			fmt.Fprintf(&sb, "\n%d) %s", i+1, SetLine(s))
		}
	}
	sb.WriteString("\n\n")
	if log.PendingReps > 0 {
		fmt.Fprintf(&sb, "Повторов: <b>%d</b>. %s", log.PendingReps, pendingPrompt(log.Kind))
		return sb.String()
	}
	sb.WriteString(setPrompt(log.Kind, len(log.Sets) > 0))
	return sb.String()
}

func pendingPrompt(k domain.LoadKind) string {
	if k == domain.LoadBand {
		return fmt.Sprintf("Теперь уровень резинки (1–%d).", domain.MaxBand)
	}
	return "Теперь напишите вес в кг (0 — без веса)."
}

// setPrompt tells how to write the next set in the kind of load being logged.
func setPrompt(k domain.LoadKind, hasSets bool) string {
	var next, example, several string
	switch k {
	case domain.LoadBodyweight:
		next, example, several = "Следующий подход — повторы", "<code>15</code>", "<code>15, 12, 10</code>"
	case domain.LoadBand:
		next, example, several = "Следующий подход — повторы и уровень резинки", "<code>12 4</code>", "<code>12 4, 10 3</code>"
	default:
		next, example, several = "Следующий подход — повторы и вес", "<code>10 50</code>", "<code>10 50, 8 55</code>"
	}
	if hasSets {
		return next + ": " + example
	}
	return "Напишите подход" + strings.TrimPrefix(next, "Следующий подход") + ": " + example + "\nМожно несколько сразу: " + several
}

// LoadConfig is the screen where the kinds of load of an exercise are chosen.
func LoadConfig(ex domain.Exercise) string {
	return bold(ex.Name) + "\nС чем вы делаете это упражнение? Отметьте всё, что подходит: " +
		"если вариантов несколько, на экране подхода появится переключатель.\n" +
		"Это общая настройка упражнения, не только этой тренировки."
}

func NewExercisePrompt(group domain.MuscleGroup) string {
	return fmt.Sprintf("➕ Новое упражнение в группе %s.\nНапишите его название:", bold(title(group.Name)))
}

// WorkoutDetail lists the exercises of a workout grouped by muscle group.
func WorkoutDetail(d domain.WorkoutDetail) string {
	var sb strings.Builder
	when := d.Workout.Date.Local()
	fmt.Fprintf(&sb, "<b>Тренировка %s</b>, %s\n", DayLabel(when), when.Format("15:04"))
	if len(d.Exercises) == 0 {
		sb.WriteString("\nУпражнений нет.")
		return sb.String()
	}
	var groups []string
	byGroup := map[string][]domain.WorkoutExerciseDetail{}
	for _, ex := range d.Exercises {
		if _, ok := byGroup[ex.MuscleGroup]; !ok {
			groups = append(groups, ex.MuscleGroup)
		}
		byGroup[ex.MuscleGroup] = append(byGroup[ex.MuscleGroup], ex)
	}
	setCount := 0
	var volume float64
	for _, g := range groups {
		fmt.Fprintf(&sb, "\n%s\n", bold(title(g)))
		for _, ex := range byGroup[g] {
			fmt.Fprintf(&sb, "• %s — %s\n", esc(ex.ExerciseName), Sets(ex.Sets))
			setCount += len(ex.Sets)
			volume += domain.VolumeFromSets(ex.Sets)
		}
	}
	fmt.Fprintf(&sb, "\nУпражнений: %d · подходов: %d", len(d.Exercises), setCount)
	if volume > 0 {
		fmt.Fprintf(&sb, " · тоннаж: %s кг", thousands(volume))
	}
	return sb.String()
}

func EndWorkoutConfirm(d domain.WorkoutDetail) string {
	if len(d.Exercises) == 0 {
		return "В тренировке пока нет ни одного подхода. Завершить её? Пустая тренировка не сохранится."
	}
	return WorkoutDetail(d) + "\n\nЗавершить тренировку?"
}

func WorkoutFinished(d domain.WorkoutDetail) string {
	if len(d.Exercises) == 0 {
		return EmptyWorkoutEnd
	}
	return "🏁 <b>Тренировка завершена. Хорошая работа!</b>\n\n" + WorkoutDetail(d)
}

func History(page usecase.HistoryPage) string {
	if page.Page == 0 {
		return "📅 <b>История тренировок</b>\nНажмите на тренировку, чтобы посмотреть её."
	}
	return fmt.Sprintf("📅 <b>История тренировок</b> · стр. %d", page.Page+1)
}

// HistoryItem is a button label: "сб 08.02.2026 · ноги, спина".
func HistoryItem(s domain.WorkoutSummary) string {
	label := DayLabel(s.Workout.Date)
	if len(s.MuscleGroups) > 0 {
		label += " · " + strings.Join(s.MuscleGroups, ", ")
	}
	return label
}

// exerciseProgressRows is how many recent workouts the exercise card lists.
const exerciseProgressRows = 6

// ExerciseProgress is the caption of an exercise chart: recent results, newest first.
func ExerciseProgress(p usecase.ExerciseProgress) string {
	var sb strings.Builder
	sb.WriteString(bold(p.Exercise.Name))
	if len(p.Points) == 0 {
		sb.WriteString("\nЗа последние полгода записей нет.")
		return sb.String()
	}
	for i := len(p.Points) - 1; i >= 0 && i >= len(p.Points)-exerciseProgressRows; i-- {
		fmt.Fprintf(&sb, "\n%s: %s", ShortDate(p.Points[i].Date), Sets(p.Points[i].Sets))
	}
	kind, series := domain.ChartSeries(p.Points)
	if len(series) < 2 {
		return sb.String()
	}
	first, last := series[0], series[len(series)-1]
	if kind == domain.LoadWeight {
		fmt.Fprintf(&sb, "\n\nМакс. вес за полгода: %s → %s кг", Weight(first.MaxWeight), Weight(last.MaxWeight))
	} else {
		fmt.Fprintf(&sb, "\n\nМакс. повторов в подходе (%s): %d → %d", strings.ToLower(LoadName(kind)), first.MaxReps, last.MaxReps)
	}
	return sb.String()
}

func Backup(s domain.UserSettings) string {
	var sb strings.Builder
	sb.WriteString("💾 <b>Бекап</b>\n")
	if s.WeeklyBackupEnabled {
		sb.WriteString("Еженедельная выгрузка в этот чат: <b>включена</b>")
	} else {
		sb.WriteString("Еженедельная выгрузка в этот чат: <b>выключена</b>")
	}
	if s.LastBackupAt != nil {
		fmt.Fprintf(&sb, "\nПоследний автобекап: %s", Date(*s.LastBackupAt))
	}
	sb.WriteString("\n\nЧтобы восстановить данные, пришлите сюда .zip из выгрузки.")
	return sb.String()
}

func ImportResult(r domain.ImportResult) string {
	return fmt.Sprintf("Импорт завершён.\nДобавлено тренировок: %d\nУпражнений: %d\nПропущено (уже есть или пустые): %d",
		r.WorkoutsAdded, r.ExercisesAdded, r.WorkoutsSkipped)
}

// workingWeight is " (раб. вес 51.1 кг)" for sets with a weight and nothing for the rest.
func workingWeight(sets []domain.SetEntry) string {
	w := domain.WorkingWeightFromSets(sets)
	if w == 0 {
		return ""
	}
	return fmt.Sprintf(" (раб. вес %s кг)", Weight(roundTo(w, 1)))
}

// SetInputHint explains what was not understood and how to write a set in the kind of load being logged.
func SetInputHint(k domain.LoadKind) string {
	switch k {
	case domain.LoadBodyweight:
		return "Не понял подход. Напишите повторы, например <code>15</code>; с дополнительным весом — <code>8 5</code>.\n" +
			"Несколько подходов — через запятую: <code>15, 12, 10</code>."
	case domain.LoadBand:
		return fmt.Sprintf("Не понял подход. Напишите повторы и уровень резинки от 1 до %d, например <code>12 4</code>.\n", domain.MaxBand) +
			"Несколько подходов — через запятую: <code>12 4, 10 3</code>."
	}
	return "Не понял подход. Напишите повторы и вес через пробел, например <code>10 50</code> или <code>10 52,5</code>.\n" +
		"Несколько подходов — через запятую: <code>10 50, 8 55</code>."
}
