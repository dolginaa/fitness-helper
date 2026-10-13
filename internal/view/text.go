package view

import (
	"fmt"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
)

const (
	SetInputHint = "Не понял подход. Напишите повторы и вес через пробел, например <code>10 50</code> или <code>10 52,5</code>.\n" +
		"Несколько подходов — через запятую: <code>10 50, 8 55</code>."
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
			fmt.Fprintf(&sb, "▫️ %s — %s: %s (раб. вес %s кг)", esc(r.Exercise.Name), ShortDate(r.LastAt),
				Sets(r.LastSets), Weight(roundTo(domain.WorkingWeightFromSets(r.LastSets), 1)))
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
	if len(log.LastSets) > 0 {
		fmt.Fprintf(&sb, "\nПрошлый раз (%s): %s", ShortDate(log.LastAt), Sets(log.LastSets))
	} else {
		sb.WriteString("\nПрошлый раз: " + NoData)
	}
	if len(log.Sets) > 0 {
		sb.WriteString("\n\nСегодня:")
		for i, s := range log.Sets {
			fmt.Fprintf(&sb, "\n%d) %d × %s кг", i+1, s.Reps, Weight(s.Weight))
		}
	}
	sb.WriteString("\n\n")
	switch {
	case log.PendingReps > 0:
		fmt.Fprintf(&sb, "Повторов: <b>%d</b>. Теперь напишите вес в кг (0 — без веса).", log.PendingReps)
	case len(log.Sets) > 0:
		sb.WriteString("Следующий подход — повторы и вес: <code>10 50</code>")
	default:
		sb.WriteString("Напишите подход — повторы и вес: <code>10 50</code>\n" +
			"Можно несколько сразу: <code>10 50, 8 55</code>")
	}
	return sb.String()
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
	fmt.Fprintf(&sb, "\nУпражнений: %d · подходов: %d · тоннаж: %s кг", len(d.Exercises), setCount, thousands(volume))
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
	if first, last := p.Points[0], p.Points[len(p.Points)-1]; len(p.Points) > 1 {
		fmt.Fprintf(&sb, "\n\nМакс. вес за полгода: %s → %s кг", Weight(first.MaxWeight), Weight(last.MaxWeight))
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
