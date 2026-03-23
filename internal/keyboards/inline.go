package keyboards

import (
	"fmt"
	"strconv"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/usecase"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	CallbackNewWorkout = "n"
	cbNextGroup        = "next"
	cbEndWorkout       = "end"
	cbFinishEx         = "done"
	cbLastPrefix       = "lg:"
	cbExPrefix         = "ex:"
	cbMgPrefix         = "mg:"
)

func MainMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Новая тренировка", CallbackNewWorkout),
		),
	)
}

func MuscleGroups(groups []domain.MuscleGroup) tgbotapi.InlineKeyboardMarkup {
	row := make([]tgbotapi.InlineKeyboardButton, 0, len(groups))
	for _, g := range groups {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(g.Name, cbMgPrefix+strconv.Itoa(g.ID)))
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(row...))
}

func LastMuscleGroups(groups []domain.MuscleGroup) tgbotapi.InlineKeyboardMarkup {
	row := make([]tgbotapi.InlineKeyboardButton, 0, len(groups))
	for _, g := range groups {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(g.Name, cbLastPrefix+strconv.Itoa(g.ID)))
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(row...))
}

func Exercises(rows []usecase.ExerciseRow) tgbotapi.InlineKeyboardMarkup {
	kbRows := make([][]tgbotapi.InlineKeyboardButton, 0)
	var cur []tgbotapi.InlineKeyboardButton
	for i, r := range rows {
		label := fmt.Sprintf("%s (%s)", r.Exercise.Name, r.LastSummary)
		if len([]rune(label)) > 60 {
			label = r.Exercise.Name
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(label, cbExPrefix+strconv.Itoa(r.Exercise.ID))
		cur = append(cur, btn)
		if len(cur) == 2 || i == len(rows)-1 {
			kbRows = append(kbRows, cur)
			cur = nil
		}
	}
	return tgbotapi.NewInlineKeyboardMarkup(kbRows...)
}

func LastExercises(rows []usecase.ExerciseRow) tgbotapi.InlineKeyboardMarkup {
	kbRows := make([][]tgbotapi.InlineKeyboardButton, 0)
	var cur []tgbotapi.InlineKeyboardButton
	for i, r := range rows {
		label := r.Exercise.Name
		btn := tgbotapi.NewInlineKeyboardButtonData(label, "le:"+strconv.Itoa(r.Exercise.ID))
		cur = append(cur, btn)
		if len(cur) == 2 || i == len(rows)-1 {
			kbRows = append(kbRows, cur)
			cur = nil
		}
	}
	return tgbotapi.NewInlineKeyboardMarkup(kbRows...)
}

func WorkoutExerciseActions() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Завершить упражнение", cbFinishEx),
		),
	)
}

func AfterExerciseMenu() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Следующая группа мышц", cbNextGroup),
			tgbotapi.NewInlineKeyboardButtonData("Конец тренировки", cbEndWorkout),
		),
	)
}
