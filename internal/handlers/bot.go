package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/adolgina/fitness-helper/internal/view"
)

// ChartRenderer turns progress data into an image.
type ChartRenderer interface {
	ExerciseProgress(name string, points []domain.ExerciseProgressPoint) ([]byte, error)
	MuscleGroupProgress(groupName string, series []domain.ExerciseProgressSeries) ([]byte, error)
}

// Bot is the chat dialog. It does not know how messages reach Telegram: that is the Messenger's job.
type Bot struct {
	svc    *usecase.Service
	log    *slog.Logger
	tg     chat.Messenger
	charts ChartRenderer
	// webAppURL is the public address of the Mini App; empty when it is not served.
	webAppURL string

	mu            sync.Mutex
	pendingImport map[int64][]byte
}

func NewBot(svc *usecase.Service, log *slog.Logger, tg chat.Messenger, charts ChartRenderer, webAppURL string) *Bot {
	return &Bot{
		svc:           svc,
		log:           log,
		tg:            tg,
		charts:        charts,
		webAppURL:     webAppURL,
		pendingImport: make(map[int64][]byte),
	}
}

// Setup fills the command menu of the bot and, if the Mini App is served, its menu button.
func (b *Bot) Setup(ctx context.Context) error {
	err := b.tg.SetCommands(ctx, []chat.Command{
		{Name: "startnew", Description: "Новая тренировка"},
		{Name: "history", Description: "История тренировок"},
		{Name: "progress", Description: "Прогресс и последние веса"},
		{Name: "backup", Description: "Бекап и импорт"},
		{Name: "start", Description: "Главное меню"},
	})
	if err != nil {
		return err
	}
	if b.webAppURL == "" {
		return nil
	}
	return b.tg.SetMenuWebApp(ctx, view.WebAppButton, b.webAppURL)
}

func (b *Bot) HandleUpdate(ctx context.Context, u chat.Update) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("handler panic", "panic", r)
		}
	}()
	switch {
	case u.Message != nil:
		b.handleMessage(ctx, u.Message)
	case u.Callback != nil:
		b.handleCallback(ctx, u.Callback)
	}
}

func (b *Bot) handleMessage(ctx context.Context, m *chat.Message) {
	userID := m.UserID
	t := newMessage(m.ChatID)
	if m.Document != nil {
		b.onDocument(ctx, m)
		return
	}
	if cmd, args, ok := parseCommand(m.Text); ok {
		b.handleCommand(ctx, t, userID, cmd, args)
		return
	}
	st, err := b.svc.SessionState(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	switch st.Phase {
	case domain.PhaseLoggingSets:
		b.onSetInput(ctx, t, userID, m.Text)
	case domain.PhaseAddingExercise:
		b.onExerciseName(ctx, t, userID, m.Text)
	default:
		b.showMenu(ctx, t, userID)
	}
}

// parseCommand splits "/workout@my_bot 15.03" into "workout" and "15.03".
func parseCommand(text string) (cmd, args string, ok bool) {
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	cmd, args, _ = strings.Cut(text[1:], " ")
	cmd, _, _ = strings.Cut(cmd, "@")
	return strings.ToLower(cmd), strings.TrimSpace(args), cmd != ""
}

func (b *Bot) handleCommand(ctx context.Context, t target, userID int64, cmd, args string) {
	switch cmd {
	case "start", "menu", "help":
		b.showMenu(ctx, t, userID)
	case "startnew", "new":
		b.onNewWorkout(ctx, t, userID, false)
	case "history", "history6m":
		b.showHistory(ctx, t, userID, 0)
	case "workout":
		b.onWorkoutByDate(ctx, t, userID, args)
	case "progress", "last":
		b.showProgressGroups(ctx, t)
	case "backup":
		b.showBackup(ctx, t, userID)
	case "export":
		b.onExport(ctx, t, userID)
	case "import":
		b.say(ctx, t.chatID, view.ImportHint)
	case "backup_on":
		b.onWeeklyBackup(ctx, t, userID, true)
	case "backup_off":
		b.onWeeklyBackup(ctx, t, userID, false)
	default:
		b.show(ctx, t, view.UnknownCommand, keyboards.Menu())
	}
}

func (b *Bot) handleCallback(ctx context.Context, cq *chat.Callback) {
	userID := cq.UserID
	t := target{chatID: cq.ChatID, messageID: cq.MessageID}
	action, args := parseCallback(cq.Data)
	// arg returns the i-th numeric argument of the callback (0 if absent).
	arg := func(i int) int {
		if i < len(args) {
			return args[i]
		}
		return 0
	}

	switch action {
	case keyboards.CallbackMenu:
		b.showMenu(ctx, t, userID)

	case keyboards.CallbackNewWorkout:
		b.onNewWorkout(ctx, t, userID, false)
	case keyboards.CallbackNewWorkoutForce:
		b.onNewWorkout(ctx, t, userID, true)
	case keyboards.CallbackResume:
		b.showWorkout(ctx, t, userID)
	case keyboards.CallbackMuscleGroup:
		b.onMuscleGroup(ctx, t, userID, arg(0))
	case keyboards.CallbackExercise:
		b.onExercise(ctx, t, userID, arg(0))
	case keyboards.CallbackRepeatSet:
		b.onRepeatSet(ctx, t, userID, arg(0))
	case keyboards.CallbackUndoSet:
		b.onUndoSet(ctx, t, userID, arg(0))
	case keyboards.CallbackExercises:
		b.onBackToExercises(ctx, t, userID)
	case keyboards.CallbackGroups:
		b.onBackToGroups(ctx, t, userID)
	case keyboards.CallbackAddExercise:
		b.onAddExercise(ctx, t, userID)
	case keyboards.CallbackEndWorkout:
		b.onEndWorkout(ctx, t, userID, false)
	case keyboards.CallbackEndWorkoutForce:
		b.onEndWorkout(ctx, t, userID, true)

	case keyboards.CallbackHistory:
		b.showHistory(ctx, t, userID, arg(0))
	case keyboards.CallbackWorkoutID:
		b.showHistoryWorkout(ctx, t, userID, arg(0), arg(1))

	// "lg" and "le" are buttons of the former /last dialog.
	case keyboards.CallbackProgress, "lg":
		if len(args) == 0 {
			b.showProgressGroups(ctx, t)
		} else {
			b.showProgressGroup(ctx, t, userID, arg(0))
		}
	case keyboards.CallbackProgressExercise, "le":
		b.sendExerciseProgress(ctx, t.chatID, userID, arg(0))
	case keyboards.CallbackChartMg:
		b.sendGroupChart(ctx, t.chatID, userID, arg(0))
	case keyboards.CallbackChartsAll:
		b.sendGroupExerciseCharts(ctx, t.chatID, userID, arg(0))

	case keyboards.CallbackBackup:
		b.showBackup(ctx, t, userID)
	case keyboards.CallbackExport:
		b.onExport(ctx, t, userID)
	case keyboards.CallbackBackupOn:
		b.onWeeklyBackup(ctx, t, userID, true)
	case keyboards.CallbackBackupOff:
		b.onWeeklyBackup(ctx, t, userID, false)
	case keyboards.CallbackImpMerge:
		b.onImport(ctx, t, userID, domain.ImportModeMerge)
	case keyboards.CallbackImpReplace:
		b.show(ctx, t, view.ImportReplaceAsk, keyboards.ImportReplaceConfirm())
	case keyboards.CallbackImpReplaceForce:
		b.onImport(ctx, t, userID, domain.ImportModeReplace)
	case keyboards.CallbackImpCancel:
		b.takePendingImport(userID)
		b.show(ctx, t, view.ImportCancelled, keyboards.Menu())

	default:
		b.show(ctx, newMessage(t.chatID), view.StaleButton, keyboards.Menu())
	}

	if err := b.tg.AnswerCallback(ctx, *cq); err != nil {
		b.log.Debug("callback answer", "err", err)
	}
}

// parseCallback splits "wid:5:1" into the action "wid" and the arguments [5 1].
func parseCallback(data string) (string, []int) {
	parts := strings.Split(data, ":")
	args := make([]int, 0, len(parts)-1)
	for _, p := range parts[1:] {
		n, err := strconv.Atoi(p)
		if err != nil {
			return "", nil
		}
		args = append(args, n)
	}
	return parts[0], args
}

func (b *Bot) showMenu(ctx context.Context, t target, userID int64) {
	w, active, err := b.svc.ActiveWorkout(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	if active {
		b.show(ctx, t, view.MainMenu(&w), keyboards.MainMenu(true, b.webAppURL))
		return
	}
	b.show(ctx, t, view.MainMenu(nil), keyboards.MainMenu(false, b.webAppURL))
}

// fail tells the user what went wrong in words they can act on.
func (b *Bot) fail(ctx context.Context, t target, err error) {
	t = newMessage(t.chatID)
	switch {
	case errors.Is(err, domain.ErrNoActiveWorkout):
		b.show(ctx, t, view.NoActiveWorkout, keyboards.NoWorkout())
	case errors.Is(err, domain.ErrInvalidState):
		b.show(ctx, t, view.StaleButton, keyboards.Stale())
	case errors.Is(err, domain.ErrNotFound):
		b.show(ctx, t, view.NotFound, keyboards.Menu())
	default:
		b.log.Error("handler error", "err", err)
		b.show(ctx, t, view.Failure, keyboards.Menu())
	}
}
