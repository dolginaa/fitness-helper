package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/repository/memory"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/adolgina/fitness-helper/internal/view"
)

const (
	testUser int64 = 42
	testChat int64 = 42
)

// screen is what the user sees after an update: the text and the buttons under it.
type screen struct {
	text    string
	buttons []chat.Button
	edited  bool
}

func (s screen) String() string {
	labels := make([]string, 0, len(s.buttons))
	for _, b := range s.buttons {
		labels = append(labels, "["+b.Text+"]")
	}
	return s.text + "\n" + strings.Join(labels, " ")
}

// data returns the callback data of the button whose label contains the text.
func (s screen) data(t *testing.T, label string) string {
	t.Helper()
	for _, b := range s.buttons {
		if strings.Contains(b.Text, label) {
			return b.Data
		}
	}
	t.Fatalf("no button %q on screen:\n%s", label, s)
	return ""
}

// fakeMessenger records what the bot sends instead of talking to Telegram.
type fakeMessenger struct {
	screens   []screen
	photos    int
	documents int
	answered  int
	commands  []chat.Command
	menuURL   string
}

func flatten(kb chat.Keyboard) []chat.Button {
	var out []chat.Button
	for _, row := range kb {
		out = append(out, row...)
	}
	return out
}

func (f *fakeMessenger) Send(ctx context.Context, chatID int64, text string, kb chat.Keyboard) error {
	f.screens = append(f.screens, screen{text: text, buttons: flatten(kb)})
	return nil
}

func (f *fakeMessenger) Edit(ctx context.Context, chatID int64, messageID int, text string, kb chat.Keyboard) error {
	f.screens = append(f.screens, screen{text: text, buttons: flatten(kb), edited: true})
	return nil
}

func (f *fakeMessenger) SendPhotos(ctx context.Context, chatID int64, photos []chat.Photo) error {
	f.photos += len(photos)
	return nil
}

func (f *fakeMessenger) SendDocument(ctx context.Context, chatID int64, fileName string, data []byte, caption string) error {
	f.documents++
	return nil
}

func (f *fakeMessenger) AnswerCallback(ctx context.Context, cb chat.Callback) error {
	f.answered++
	return nil
}

func (f *fakeMessenger) Download(ctx context.Context, doc chat.Document) ([]byte, error) {
	data, _ := doc.Ref.([]byte)
	return data, nil
}

func (f *fakeMessenger) SetCommands(ctx context.Context, commands []chat.Command) error {
	f.commands = commands
	return nil
}

func (f *fakeMessenger) SetMenuWebApp(ctx context.Context, title, url string) error {
	f.menuURL = url
	return nil
}

type fakeCharts struct{}

func (fakeCharts) ExerciseProgress(name string, points []domain.ExerciseProgressPoint) ([]byte, error) {
	if len(points) < 2 {
		return nil, domain.ErrNotEnoughData
	}
	return []byte("png"), nil
}

func (fakeCharts) MuscleGroupProgress(groupName string, series []domain.ExerciseProgressSeries) ([]byte, error) {
	for _, s := range series {
		if len(s.Points) >= 2 {
			return []byte("png"), nil
		}
	}
	return nil, domain.ErrNotEnoughData
}

type harness struct {
	t     *testing.T
	bot   *Bot
	api   *fakeMessenger
	store *memory.Store
	legs  domain.MuscleGroup
	press domain.Exercise
	calf  domain.Exercise
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	store := memory.NewStore()
	legs := store.AddGroup("ноги")
	store.AddGroup("спина")
	press, _ := store.CreateExercise(ctx, "Жим ногами", legs.ID)
	calf, _ := store.CreateExercise(ctx, "Икры", legs.ID)
	api := &fakeMessenger{}
	return &harness{
		t:     t,
		api:   api,
		store: store,
		legs:  legs,
		press: press,
		calf:  calf,
		bot: &Bot{
			svc:           usecase.NewService(store, store, store, store, store, store),
			log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
			tg:            api,
			charts:        fakeCharts{},
			pendingImport: map[int64][]byte{},
		},
	}
}

// last returns the screen produced by the latest update.
func (h *harness) last() screen {
	h.t.Helper()
	if len(h.api.screens) == 0 {
		h.t.Fatal("the bot has sent nothing")
	}
	s := h.api.screens[len(h.api.screens)-1]
	h.t.Logf("\n%s\n", s)
	return s
}

func (h *harness) text(text string) screen {
	h.t.Helper()
	h.bot.HandleUpdate(context.Background(), chat.Update{Message: &chat.Message{
		ChatID: testChat,
		UserID: testUser,
		Text:   text,
	}})
	return h.last()
}

// command sends a bot command; in the chat it is an ordinary message.
func (h *harness) command(cmd string) screen {
	h.t.Helper()
	return h.text(cmd)
}

func (h *harness) tap(data string) screen {
	h.t.Helper()
	answered := h.api.answered
	h.bot.HandleUpdate(context.Background(), chat.Update{Callback: &chat.Callback{
		ID:        "cb",
		ChatID:    testChat,
		UserID:    testUser,
		MessageID: 7,
		Data:      data,
	}})
	if h.api.answered != answered+1 {
		h.t.Fatalf("callback %q is not answered: the button would keep spinning", data)
	}
	return h.last()
}

func mustContain(t *testing.T, s screen, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s.String(), p) {
			t.Fatalf("missing %q on screen:\n%s", p, s)
		}
	}
}

func TestWorkoutDialog(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.AddCompletedWorkout(testUser, time.Now().AddDate(0, 0, -7), h.press.ID,
		domain.SetEntry{Reps: 10, Weight: 90}, domain.SetEntry{Reps: 8, Weight: 95})

	s := h.command("/start")
	mustContain(t, s, "Fitness Helper", "[🏋️ Новая тренировка]", "[📅 История]", "[📈 Прогресс]")

	s = h.tap(s.data(t, "Новая тренировка"))
	mustContain(t, s, "Тренировка начата", "[ноги]", "[спина]")
	if !s.edited {
		t.Fatal("a button must update its message instead of sending a new one")
	}

	s = h.tap(s.data(t, "ноги"))
	mustContain(t, s, "<b>Ноги</b>", "Жим ногами — ", ": 10×90, 8×95 (раб. вес 92.2 кг)", "Икры — нет данных",
		"[Жим ногами]", "[◀️ Группы мышц]", "[➕ Своё упражнение]")

	s = h.tap(s.data(t, "Жим ногами"))
	mustContain(t, s, "<b>Жим ногами</b>", "): 10×90, 8×95", "[🔁 10×90 — как в прошлый раз]", "[◀️ К упражнениям]")

	s = h.tap(s.data(t, "как в прошлый раз"))
	// The last workout went 10×90, 8×95: after repeating its first set the next step is offered.
	mustContain(t, s, "1) 10 × 90 кг", "[➕ Дальше 8×95]", "[↩️ Убрать последний]")

	s = h.text("9 92,5")
	mustContain(t, s, "1) 10 × 90 кг\n2) 9 × 92.5 кг", "[➕ Ещё 9×92.5]")
	if s.edited {
		t.Fatal("the answer to a text message must be a new message")
	}

	s = h.text("много")
	mustContain(t, s, "Не понял подход")

	s = h.text("8")
	mustContain(t, s, "Повторов: <b>8</b>", "[↩️ Отменить повторы]")
	s = h.text("95")
	mustContain(t, s, "3) 8 × 95 кг")

	s = h.tap(s.data(t, "Ещё 8×95"))
	mustContain(t, s, "4) 8 × 95 кг")
	s = h.tap(s.data(t, "Убрать последний"))
	if strings.Contains(s.text, "4) ") {
		t.Fatalf("the last set must be removed:\n%s", s)
	}

	s = h.tap(s.data(t, "Готово"))
	mustContain(t, s, "✅ <b>Жим ногами</b> — сегодня 10×90, 9×92.5, 8×95", "[✅ Жим ногами]")

	s = h.tap(s.data(t, "Завершить"))
	mustContain(t, s, "Завершить тренировку?", "• Жим ногами — 10×90, 9×92.5, 8×95", "[◀️ Назад]")
	back := h.tap(s.data(t, "Назад"))
	mustContain(t, back, "выберите упражнение")

	s = h.tap(s.data(t, "Завершить"))
	mustContain(t, s, "Тренировка завершена", "Упражнений: 1 · подходов: 3 · тоннаж: 2 493 кг")

	s = h.tap(s.data(t, "История"))
	mustContain(t, s, "История тренировок")
	if len(s.buttons) != 3 {
		t.Fatalf("expected two workouts and the menu button:\n%s", s)
	}
	s = h.tap(s.buttons[0].Data)
	mustContain(t, s, "• Жим ногами — 10×90, 9×92.5, 8×95", "[◀️ К списку]")
}

func TestNewWorkoutAsksAboutUnfinishedOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.command("/startnew")
	h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackMuscleGroup, h.legs.ID))
	h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackExercise, h.press.ID))
	h.text("10 100")

	s := h.command("/startnew")
	mustContain(t, s, "незавершённая тренировка", "[▶️ Продолжить]", "[🆕 Завершить и начать новую]")

	s = h.tap(s.data(t, "Продолжить"))
	mustContain(t, s, "<b>Жим ногами</b>", "1) 10 × 100 кг")

	s = h.command("/start")
	mustContain(t, s, "Есть незавершённая тренировка", "[▶️ Продолжить тренировку]")
}

func TestProgressDoesNotBreakWorkout(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	now := time.Now()
	h.store.AddCompletedWorkout(testUser, now.AddDate(0, 0, -14), h.press.ID, domain.SetEntry{Reps: 10, Weight: 80})
	h.store.AddCompletedWorkout(testUser, now.AddDate(0, 0, -7), h.press.ID, domain.SetEntry{Reps: 10, Weight: 90})

	h.command("/startnew")
	h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackMuscleGroup, h.legs.ID))
	h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackExercise, h.press.ID))
	if h.api.photos != 1 {
		t.Fatalf("expected the exercise chart, got %d photos", h.api.photos)
	}

	// Looking at the progress in the middle of an exercise...
	s := h.command("/last")
	mustContain(t, s, "Прогресс", "[ноги]")
	s = h.tap(s.data(t, "ноги"))
	mustContain(t, s, "последние результаты", "[📈 Жим ногами]", "[🖼 Все графики]")
	h.tap(s.data(t, "Жим ногами"))
	h.tap(s.data(t, "Все графики"))
	h.tap(s.data(t, "График группы"))
	if h.api.photos != 4 {
		t.Fatalf("expected 4 photos, got %d", h.api.photos)
	}

	// ...must not interrupt logging.
	s = h.text("10 95")
	mustContain(t, s, "<b>Жим ногами</b>", "1) 10 × 95 кг")
}

func TestAddExerciseDialog(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.command("/startnew")
	s := h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackMuscleGroup, h.legs.ID))

	s = h.tap(s.data(t, "Своё упражнение"))
	mustContain(t, s, "Новое упражнение в группе <b>Ноги</b>", "[◀️ Отмена]")
	s = h.text("Болгарские выпады")
	mustContain(t, s, "<b>Болгарские выпады</b>", "Напишите подход")
	s = h.text("12 20")
	s = h.tap(s.data(t, "Готово"))
	mustContain(t, s, "✅ <b>Болгарские выпады</b> — сегодня 12×20")
}

func TestStaleAndIdle(t *testing.T) {
	t.Parallel()
	h := newHarness(t)

	s := h.text("привет")
	mustContain(t, s, "Fitness Helper", "[🏋️ Новая тренировка]")

	s = h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackExercise, h.press.ID))
	mustContain(t, s, view.NoActiveWorkout, "[🏋️ Новая тренировка]")

	h.command("/startnew")
	s = h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackRepeatSet, h.press.ID))
	mustContain(t, s, view.StaleButton, "[▶️ К тренировке]")

	s = h.command("/history")
	mustContain(t, s, view.EmptyHistory)

	s = h.tap(keyboards.CallbackEndWorkout)
	mustContain(t, s, "Пустая тренировка не сохранится")
	s = h.tap(s.data(t, "Завершить"))
	mustContain(t, s, view.EmptyWorkoutEnd)
}

func TestWorkoutByDate(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	at := time.Date(2026, 3, 15, 0, 30, 0, 0, time.Local)
	h.store.AddCompletedWorkout(testUser, at, h.calf.ID, domain.SetEntry{Reps: 20, Weight: 40})

	s := h.command("/workout 15.03.2026")
	mustContain(t, s, "Тренировка вс 15.03.2026", "• Икры — 20×40")
	s = h.command("/workout 14.03.2026")
	mustContain(t, s, view.NoWorkoutOnDate)
	s = h.command("/workout вчера")
	mustContain(t, s, "Не понял дату")
}

func TestCallbackDataFitsTelegramLimit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.command("/start")
	h.command("/startnew")
	h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackMuscleGroup, h.legs.ID))
	h.tap(fmt.Sprintf("%s:%d", keyboards.CallbackExercise, h.press.ID))
	h.text("10 100")
	h.command("/backup")
	for _, s := range h.api.screens {
		for _, b := range s.buttons {
			if len(b.Data) == 0 || len(b.Data) > 64 {
				t.Fatalf("bad callback data on button %q", b.Text)
			}
		}
	}
}

func TestParseWorkoutDate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.Local)
	want := time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local)
	for _, in := range []string{"05.03.2026", "5.3.2026", "2026-03-05", "5.3", " 05.03 "} {
		got, err := parseWorkoutDate(in, now)
		if err != nil || !got.Equal(want) {
			t.Fatalf("%q: got %v, %v", in, got, err)
		}
	}
	if _, err := parseWorkoutDate("завтра", now); err == nil {
		t.Fatal("expected an error")
	}
}

func TestCommandWithBotName(t *testing.T) {
	t.Parallel()
	cmd, args, ok := parseCommand("/workout@my_fitness_bot  15.03.2026 ")
	if !ok || cmd != "workout" || args != "15.03.2026" {
		t.Fatalf("got %q %q %v", cmd, args, ok)
	}
	if _, _, ok := parseCommand("10 50"); ok {
		t.Fatal("a set is not a command")
	}
}

func TestWebAppButtons(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.bot.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(h.api.commands) == 0 || h.api.menuURL != "" {
		t.Fatalf("without the Mini App only commands are registered: %+v, %q", h.api.commands, h.api.menuURL)
	}
	for _, b := range h.command("/start").buttons {
		if b.WebAppURL != "" {
			t.Fatalf("unexpected Mini App button %q", b.Text)
		}
	}

	h.bot.webAppURL = "https://fit.example.com/"
	if err := h.bot.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.api.menuURL != h.bot.webAppURL {
		t.Fatalf("menu button must open the Mini App, got %q", h.api.menuURL)
	}
	s := h.command("/start")
	if last := s.buttons[len(s.buttons)-1]; last.WebAppURL != h.bot.webAppURL || last.Data != "" {
		t.Fatalf("the menu must offer the Mini App: %+v", last)
	}
}

func TestImportDialog(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.store.AddCompletedWorkout(testUser, time.Now().AddDate(0, 0, -1), h.press.ID, domain.SetEntry{Reps: 10, Weight: 90})

	h.bot.HandleUpdate(context.Background(), chat.Update{Message: &chat.Message{ChatID: testChat, UserID: testUser, Text: "/export"}})
	if h.api.documents != 1 {
		t.Fatalf("expected the archive, got %d documents", h.api.documents)
	}
	archive, _, err := h.bot.svc.ExportUserData(context.Background(), testUser)
	if err != nil {
		t.Fatal(err)
	}
	h.bot.HandleUpdate(context.Background(), chat.Update{Message: &chat.Message{
		ChatID:   testChat,
		UserID:   testUser,
		Document: &chat.Document{FileName: "Backup.ZIP", Ref: archive},
	}})
	s := h.last()
	mustContain(t, s, view.ImportAsk, "[➕ Добавить к данным]", "[♻️ Заменить всё]")

	s = h.tap(s.data(t, "Заменить всё"))
	mustContain(t, s, "будут <b>удалены</b>", "[♻️ Да, заменить]")
	s = h.tap(s.data(t, "Да, заменить"))
	mustContain(t, s, "Импорт завершён", "Добавлено тренировок: 1")
}
