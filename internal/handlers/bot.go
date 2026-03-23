package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/usecase"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	Svc *usecase.Service
	Log *slog.Logger
}

func (b *Bot) HandleUpdate(ctx context.Context, api *tgbotapi.BotAPI, u tgbotapi.Update) {
	if u.Message != nil {
		b.handleMessage(ctx, api, u.Message)
		return
	}
	if u.CallbackQuery != nil {
		b.handleCallback(ctx, api, u.CallbackQuery)
	}
}

func (b *Bot) handleMessage(ctx context.Context, api *tgbotapi.BotAPI, m *tgbotapi.Message) {
	userID := m.From.ID
	chatID := m.Chat.ID
	if m.IsCommand() {
		b.handleCommand(ctx, api, m)
		return
	}
	_, st, err := b.Svc.CurrentState(ctx, userID)
	if err != nil {
		b.replyErr(api, chatID, err)
		return
	}
	if st.Phase == domain.PhaseLoggingSets {
		_, err := b.Svc.ProcessLogInput(ctx, userID, m.Text)
		if err != nil {
			b.sendText(api, chatID, "Введите целое число повторений или вес (кг), например: 10 или 52.5")
			return
		}
		_, st2, _ := b.Svc.CurrentState(ctx, userID)
		if st2.Awaiting == "weight" {
			b.sendText(api, chatID, "Теперь вес (кг) для этого подхода.")
		} else {
			b.sendText(api, chatID, "Подход записан. Введите повторения для следующего подхода или нажмите «Завершить упражнение».")
		}
		return
	}
	b.sendText(api, chatID, "Используйте кнопки меню или команды /start, /startnew, /history, /last")
}

func (b *Bot) handleCommand(ctx context.Context, api *tgbotapi.BotAPI, m *tgbotapi.Message) {
	userID := m.From.ID
	chatID := m.Chat.ID
	cmd := m.Command()
	switch cmd {
	case "start":
		msg := "Привет! Я помогаю вести силовые тренировки.\n\nКоманды:\n/startnew — новая тренировка\n/history — история\n/last — последние веса по упражнению"
		cfg := tgbotapi.NewMessage(chatID, msg)
		cfg.ReplyMarkup = keyboards.MainMenu()
		api.Send(cfg)
	case "startnew":
		b.startNewWorkout(ctx, api, chatID, userID)
	case "history":
		ws, err := b.Svc.History(ctx, userID, 15)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		if len(ws) == 0 {
			b.sendText(api, chatID, "Завершённых тренировок пока нет.")
			return
		}
		var sb strings.Builder
		sb.WriteString("Последние тренировки:\n")
		for _, w := range ws {
			sb.WriteString(fmt.Sprintf("— %s (id %d)\n", w.Date.Format("2006-01-02 15:04"), w.ID))
		}
		b.sendText(api, chatID, sb.String())
	case "last":
		if err := b.Svc.BeginLastCommand(ctx, userID); err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		groups, err := b.Svc.Catalog.ListMuscleGroups(ctx)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		cfg := tgbotapi.NewMessage(chatID, "Выберите группу мышц для просмотра последних весов:")
		cfg.ReplyMarkup = keyboards.LastMuscleGroups(groups)
		api.Send(cfg)
	default:
		b.sendText(api, chatID, "Неизвестная команда. /start")
	}
}

func (b *Bot) startNewWorkout(ctx context.Context, api *tgbotapi.BotAPI, chatID int64, userID int64) {
	if _, err := b.Svc.StartNewWorkout(ctx, userID); err != nil {
		b.replyErr(api, chatID, err)
		return
	}
	groups, err := b.Svc.Catalog.ListMuscleGroups(ctx)
	if err != nil {
		b.replyErr(api, chatID, err)
		return
	}
	cfg := tgbotapi.NewMessage(chatID, "Тренировка начата. Выберите группу мышц:")
	cfg.ReplyMarkup = keyboards.MuscleGroups(groups)
	api.Send(cfg)
}

func (b *Bot) handleCallback(ctx context.Context, api *tgbotapi.BotAPI, cq *tgbotapi.CallbackQuery) {
	if cq.Message == nil {
		return
	}
	userID := cq.From.ID
	chatID := cq.Message.Chat.ID
	data := cq.Data
	if _, err := api.Request(tgbotapi.NewCallback(cq.ID, "")); err != nil {
		b.Log.Debug("callback answer", "err", err)
	}

	switch {
	case data == keyboards.CallbackNewWorkout:
		b.startNewWorkout(ctx, api, chatID, userID)
	case strings.HasPrefix(data, "mg:"):
		id, err := strconv.Atoi(strings.TrimPrefix(data, "mg:"))
		if err != nil {
			return
		}
		if err := b.Svc.SelectMuscleGroup(ctx, userID, id); err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		rows, err := b.Svc.ListExercisesForCurrentGroup(ctx, userID)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID,
			"Упражнения (последние данные в скобках):", keyboards.Exercises(rows))
		api.Send(msg)
	case strings.HasPrefix(data, "ex:"):
		id, err := strconv.Atoi(strings.TrimPrefix(data, "ex:"))
		if err != nil {
			return
		}
		ex, last, err := b.Svc.BeginExercise(ctx, userID, id)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		lastLine := usecase.FormatSets(last)
		text := fmt.Sprintf("%s\n\nПрошлый раз: %s\n\nВведите повторения для первого подхода (целое число).", ex.Name, lastLine)
		msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID, text, keyboards.WorkoutExerciseActions())
		api.Send(msg)
	case data == "done":
		if err := b.Svc.FinishExercise(ctx, userID); err != nil {
			b.sendText(api, chatID, "Нельзя завершить: нет подходов или не та фаза. Сначала введите хотя бы один подход.")
			return
		}
		rows, err := b.Svc.ListExercisesForCurrentGroup(ctx, userID)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID,
			"Упражнение сохранено. Выберите следующее или действие:", keyboards.Exercises(rows))
		api.Send(msg)
		extra := tgbotapi.NewMessage(chatID, "Действия:")
		extra.ReplyMarkup = keyboards.AfterExerciseMenu()
		api.Send(extra)
	case data == "next":
		if err := b.Svc.NextMuscleGroup(ctx, userID); err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		groups, err := b.Svc.Catalog.ListMuscleGroups(ctx)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		msg := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID,
			"Выберите группу мышц:", keyboards.MuscleGroups(groups))
		api.Send(msg)
	case data == "end":
		if err := b.Svc.EndWorkout(ctx, userID); err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		msg := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, "Тренировка завершена. Хорошей работы!")
		api.Send(msg)
	case strings.HasPrefix(data, "lg:"):
		id, err := strconv.Atoi(strings.TrimPrefix(data, "lg:"))
		if err != nil {
			return
		}
		if err := b.Svc.LastSelectGroup(ctx, userID, id); err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		rows, err := b.Svc.ListExercisesWithLast(ctx, userID, id)
		if err != nil {
			b.replyErr(api, chatID, err)
			return
		}
		edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, cq.Message.MessageID,
			"Выберите упражнение:", keyboards.LastExercises(rows))
		api.Send(edit)
	case strings.HasPrefix(data, "le:"):
		id, err := strconv.Atoi(strings.TrimPrefix(data, "le:"))
		if err != nil {
			return
		}
		ex, sets, at, err := b.Svc.LastSelectExercise(ctx, userID, id)
		if err != nil {
			if errors.Is(err, domain.ErrNoLastExerciseData) {
				b.sendText(api, chatID, "Нет сохранённых данных по этому упражнению.")
			} else {
				b.replyErr(api, chatID, err)
			}
			_ = b.Svc.ResetLastFlow(ctx, userID)
			return
		}
		text := fmt.Sprintf("%s\nПоследняя тренировка: %s\n%s", ex.Name, at.Format("2006-01-02 15:04"), usecase.FormatSets(sets))
		edit := tgbotapi.NewEditMessageText(chatID, cq.Message.MessageID, text)
		api.Send(edit)
		_ = b.Svc.ResetLastFlow(ctx, userID)
	default:
		b.sendText(api, chatID, "Неизвестное действие")
	}
}

func (b *Bot) sendText(api *tgbotapi.BotAPI, chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	api.Send(msg)
}

func (b *Bot) replyErr(api *tgbotapi.BotAPI, chatID int64, err error) {
	b.Log.Error("handler error", "err", err)
	b.sendText(api, chatID, "Ошибка: "+err.Error())
}
