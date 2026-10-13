package keyboards

import (
	"strconv"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/adolgina/fitness-helper/internal/view"
)

// Callback data is an action optionally followed by numeric arguments: "ex:12", "wid:5:1".
const (
	CallbackMenu = "menu"

	CallbackNewWorkout      = "n"
	CallbackNewWorkoutForce = "n!"
	CallbackResume          = "res"
	CallbackMuscleGroup     = "mg"
	CallbackExercise        = "ex"
	CallbackRepeatSet       = "rep"
	CallbackUndoSet         = "undo"
	CallbackExercises       = "done"
	CallbackGroups          = "next"
	CallbackAddExercise     = "addex"
	CallbackEndWorkout      = "end"
	CallbackEndWorkoutForce = "end!"

	CallbackHistory   = "hist"
	CallbackWorkoutID = "wid"

	CallbackProgress         = "prog"
	CallbackProgressExercise = "pex"
	CallbackChartMg          = "chart_mg"
	CallbackChartsAll        = "pall"

	CallbackBackup          = "bkp"
	CallbackExport          = "export"
	CallbackBackupOn        = "bkp_on"
	CallbackBackupOff       = "bkp_off"
	CallbackImpMerge        = "imp_merge"
	CallbackImpReplace      = "imp_replace"
	CallbackImpReplaceForce = "imp_replace!"
	CallbackImpCancel       = "imp_cancel"
)

func data(action string, args ...int) string {
	for _, a := range args {
		action += ":" + strconv.Itoa(a)
	}
	return action
}

func button(text, action string, args ...int) chat.Button {
	return chat.Button{Text: text, Data: data(action, args...)}
}

func row(buttons ...chat.Button) []chat.Button {
	return buttons
}

// grid lays buttons out in rows of perRow.
func grid(buttons []chat.Button, perRow int) [][]chat.Button {
	var rows [][]chat.Button
	for len(buttons) > perRow {
		rows = append(rows, buttons[:perRow])
		buttons = buttons[perRow:]
	}
	if len(buttons) > 0 {
		rows = append(rows, buttons)
	}
	return rows
}

func markup(rows ...[]chat.Button) chat.Keyboard {
	return rows
}

func menuButton() chat.Button {
	return button("🏠 Меню", CallbackMenu)
}

// MainMenu is the home screen. With webAppURL set it also offers to open the Mini App.
func MainMenu(hasActiveWorkout bool, webAppURL string) chat.Keyboard {
	var rows [][]chat.Button
	if hasActiveWorkout {
		rows = append(rows, row(button("▶️ Продолжить тренировку", CallbackResume)))
	}
	rows = append(rows,
		row(button("🏋️ Новая тренировка", CallbackNewWorkout)),
		row(button("📅 История", CallbackHistory, 0), button("📈 Прогресс", CallbackProgress)),
		row(button("💾 Бекап", CallbackBackup)),
	)
	if webAppURL != "" {
		rows = append(rows, row(chat.Button{Text: "📱 Открыть приложение", WebAppURL: webAppURL}))
	}
	return markup(rows...)
}

func Menu() chat.Keyboard {
	return markup(row(menuButton()))
}

// NoWorkout is shown when an action needs an active workout and there is none.
func NoWorkout() chat.Keyboard {
	return markup(row(button("🏋️ Новая тренировка", CallbackNewWorkout), menuButton()))
}

// Stale is shown when a button of an old message no longer applies.
func Stale() chat.Keyboard {
	return markup(row(button("▶️ К тренировке", CallbackResume), menuButton()))
}

func ActiveWorkoutPrompt() chat.Keyboard {
	return markup(
		row(button("▶️ Продолжить", CallbackResume)),
		row(button("🆕 Завершить и начать новую", CallbackNewWorkoutForce)),
	)
}

func groupButtons(groups []domain.MuscleGroup, action string) []chat.Button {
	buttons := make([]chat.Button, 0, len(groups))
	for _, g := range groups {
		buttons = append(buttons, button(g.Name, action, g.ID))
	}
	return buttons
}

func WorkoutGroups(groups []domain.MuscleGroup) chat.Keyboard {
	rows := grid(groupButtons(groups, CallbackMuscleGroup), 2)
	rows = append(rows, row(button("🏁 Завершить тренировку", CallbackEndWorkout)))
	return markup(rows...)
}

func WorkoutExercises(list usecase.ExerciseList) chat.Keyboard {
	rows := make([][]chat.Button, 0, len(list.Rows)+3)
	for _, r := range list.Rows {
		label := r.Exercise.Name
		if len(r.TodaySets) > 0 {
			label = "✅ " + label
		}
		rows = append(rows, row(button(label, CallbackExercise, r.Exercise.ID)))
	}
	rows = append(rows,
		row(button("➕ Своё упражнение", CallbackAddExercise), button("📊 График группы", CallbackChartMg, list.Group.ID)),
		row(button("◀️ Группы мышц", CallbackGroups), button("🏁 Завершить", CallbackEndWorkout)),
	)
	return markup(rows...)
}

func ExerciseLog(log usecase.ExerciseLog) chat.Keyboard {
	var rows [][]chat.Button
	id := log.Exercise.ID
	switch {
	case log.PendingReps > 0:
		rows = append(rows, row(button("↩️ Отменить повторы", CallbackUndoSet, id)))
	case len(log.Sets) > 0:
		var actions []chat.Button
		if log.Suggestion != nil {
			label := "➕ Ещё " + view.Set(*log.Suggestion)
			if *log.Suggestion != log.Sets[len(log.Sets)-1] {
				// Not a repeat of the previous set but the next step of the last workout.
				label = "➕ Дальше " + view.Set(*log.Suggestion)
			}
			actions = append(actions, button(label, CallbackRepeatSet, id))
		}
		rows = append(rows, append(actions, button("↩️ Убрать последний", CallbackUndoSet, id)))
	case log.Suggestion != nil:
		rows = append(rows, row(button("🔁 "+view.Set(*log.Suggestion)+" — как в прошлый раз", CallbackRepeatSet, id)))
	}
	if len(log.Sets) > 0 {
		rows = append(rows, row(button("✅ Готово, к упражнениям", CallbackExercises)))
	} else {
		rows = append(rows, row(button("◀️ К упражнениям", CallbackExercises)))
	}
	return markup(rows...)
}

func NewExercisePrompt() chat.Keyboard {
	return markup(row(button("◀️ Отмена", CallbackExercises)))
}

func EndWorkoutConfirm() chat.Keyboard {
	return markup(row(button("◀️ Назад", CallbackResume), button("🏁 Завершить", CallbackEndWorkoutForce)))
}

func WorkoutFinished() chat.Keyboard {
	return markup(row(button("📅 История", CallbackHistory, 0), menuButton()))
}

func History(page usecase.HistoryPage) chat.Keyboard {
	rows := make([][]chat.Button, 0, len(page.Items)+2)
	for _, item := range page.Items {
		rows = append(rows, row(button(view.HistoryItem(item), CallbackWorkoutID, item.Workout.ID, page.Page)))
	}
	var nav []chat.Button
	if page.Page > 0 {
		nav = append(nav, button("◀️ Новее", CallbackHistory, page.Page-1))
	}
	if page.HasNext {
		nav = append(nav, button("Раньше ▶️", CallbackHistory, page.Page+1))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, row(menuButton()))
	return markup(rows...)
}

// HistoryWorkout is shown under a workout opened from the history page.
func HistoryWorkout(page int) chat.Keyboard {
	return markup(row(button("◀️ К списку", CallbackHistory, page), menuButton()))
}

func ProgressGroups(groups []domain.MuscleGroup) chat.Keyboard {
	rows := grid(groupButtons(groups, CallbackProgress), 2)
	rows = append(rows, row(menuButton()))
	return markup(rows...)
}

func ProgressExercises(list usecase.ExerciseList) chat.Keyboard {
	rows := make([][]chat.Button, 0, len(list.Rows)+2)
	for _, r := range list.Rows {
		if len(r.LastSets) == 0 {
			continue
		}
		rows = append(rows, row(button("📈 "+r.Exercise.Name, CallbackProgressExercise, r.Exercise.ID)))
	}
	if len(rows) > 0 {
		rows = append(rows, row(
			button("📊 График группы", CallbackChartMg, list.Group.ID),
			button("🖼 Все графики", CallbackChartsAll, list.Group.ID),
		))
	}
	rows = append(rows, row(button("◀️ Группы мышц", CallbackProgress), menuButton()))
	return markup(rows...)
}

func Backup(s domain.UserSettings) chat.Keyboard {
	toggle := button("🔔 Включить еженедельный бекап", CallbackBackupOn)
	if s.WeeklyBackupEnabled {
		toggle = button("🔕 Выключить еженедельный бекап", CallbackBackupOff)
	}
	return markup(
		row(button("📤 Выгрузить архив", CallbackExport)),
		row(toggle),
		row(menuButton()),
	)
}

func ImportConfirm() chat.Keyboard {
	return markup(
		row(button("➕ Добавить к данным", CallbackImpMerge)),
		row(button("♻️ Заменить всё", CallbackImpReplace)),
		row(button("Отмена", CallbackImpCancel)),
	)
}

func ImportReplaceConfirm() chat.Keyboard {
	return markup(row(button("Отмена", CallbackImpCancel), button("♻️ Да, заменить", CallbackImpReplaceForce)))
}
