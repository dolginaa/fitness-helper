package botapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/adolgina/fitness-helper/internal/chat"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const testToken = "123456:TEST-TOKEN-for-unit-tests"

// call is one request the client made to the fake Bot API.
type call struct {
	method string
	form   map[string]string
	files  map[string]string // form field -> file name
}

type fakeAPI struct {
	mu    sync.Mutex
	calls []call
	// fail makes a method answer with this description.
	fail map[string]string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "/bot" + testToken + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	c := call{method: strings.TrimPrefix(r.URL.Path, prefix), form: map[string]string{}, files: map[string]string{}}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(1 << 20)
		for k, files := range r.MultipartForm.File {
			c.files[k] = files[0].Filename
		}
	} else {
		_ = r.ParseForm()
	}
	for k := range r.Form {
		c.form[k] = r.Form.Get(k)
	}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	desc := f.fail[c.method]
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case desc != "":
		fmt.Fprintf(w, `{"ok":false,"error_code":400,"description":%q}`, desc)
	case c.method == "getMe":
		io.WriteString(w, `{"ok":true,"result":{"id":1,"is_bot":true,"username":"fit_test_bot"}}`)
	default:
		io.WriteString(w, `{"ok":true,"result":true}`)
	}
}

func (f *fakeAPI) last(t *testing.T, method string) call {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls[len(f.calls)-1]
	if c.method != method {
		t.Fatalf("last call is %s, want %s", c.method, method)
	}
	return c
}

func newClient(t *testing.T) (*Client, *fakeAPI) {
	t.Helper()
	api := &fakeAPI{fail: map[string]string{}}
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	c, err := newWithEndpoint(testToken, srv.URL+"/bot%s/%s", srv.Client(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return c, api
}

func TestSendAndEdit(t *testing.T) {
	t.Parallel()
	c, api := newClient(t)
	ctx := context.Background()
	kb := chat.Keyboard{
		{{Text: "Жим ногами", Data: "ex:16"}},
		{{Text: "Приложение", WebAppURL: "https://fit.example.com/"}},
	}
	if err := c.Send(ctx, 42, "<b>Ноги</b>", kb); err != nil {
		t.Fatal(err)
	}
	sent := api.last(t, "sendMessage")
	if sent.form["chat_id"] != "42" || sent.form["text"] != "<b>Ноги</b>" || sent.form["parse_mode"] != "HTML" {
		t.Fatalf("unexpected message: %v", sent.form)
	}
	var markup struct {
		InlineKeyboard [][]map[string]any `json:"inline_keyboard"`
	}
	if err := json.Unmarshal([]byte(sent.form["reply_markup"]), &markup); err != nil {
		t.Fatal(err)
	}
	callback, web := markup.InlineKeyboard[0][0], markup.InlineKeyboard[1][0]
	if callback["callback_data"] != "ex:16" || callback["web_app"] != nil {
		t.Fatalf("unexpected callback button: %v", callback)
	}
	if web["callback_data"] != nil || web["web_app"].(map[string]any)["url"] != "https://fit.example.com/" {
		t.Fatalf("unexpected Mini App button: %v", web)
	}

	if err := c.Send(ctx, 42, "без кнопок", nil); err != nil {
		t.Fatal(err)
	}
	if _, has := api.last(t, "sendMessage").form["reply_markup"]; has {
		t.Fatal("a message without buttons must not carry a keyboard")
	}

	if err := c.Edit(ctx, 42, 7, "новый текст", kb); err != nil {
		t.Fatal(err)
	}
	if edit := api.last(t, "editMessageText"); edit.form["message_id"] != "7" || edit.form["text"] != "новый текст" {
		t.Fatalf("unexpected edit: %v", edit.form)
	}

	// Pressing a button that leads to the same screen is not a failure.
	api.fail["editMessageText"] = "Bad Request: message is not modified"
	if err := c.Edit(ctx, 42, 7, "новый текст", kb); err != nil {
		t.Fatalf("an unchanged message must not be an error: %v", err)
	}
	api.fail["editMessageText"] = "Bad Request: message to edit not found"
	if err := c.Edit(ctx, 42, 7, "новый текст", kb); err == nil {
		t.Fatal("a real edit failure must be reported")
	}
}

func TestPhotosAndDocuments(t *testing.T) {
	t.Parallel()
	c, api := newClient(t)
	ctx := context.Background()
	png := []byte("\x89PNG fake")

	if err := c.SendPhotos(ctx, 42, []chat.Photo{{PNG: png, Caption: "<b>Жим</b>"}}); err != nil {
		t.Fatal(err)
	}
	one := api.last(t, "sendPhoto")
	if one.files["photo"] == "" || one.form["caption"] != "<b>Жим</b>" || one.form["parse_mode"] != "HTML" {
		t.Fatalf("unexpected photo: %+v", one)
	}

	photos := make([]chat.Photo, chat.MaxAlbumSize+2)
	for i := range photos {
		photos[i] = chat.Photo{PNG: png}
	}
	before := len(api.calls)
	if err := c.SendPhotos(ctx, 42, photos); err != nil {
		t.Fatal(err)
	}
	albums := api.calls[before:]
	if len(albums) != 2 || albums[0].method != "sendMediaGroup" || len(albums[0].files) != chat.MaxAlbumSize || len(albums[1].files) != 2 {
		t.Fatalf("twelve photos are two albums, got %d calls", len(albums))
	}
	var media []map[string]string
	if err := json.Unmarshal([]byte(albums[1].form["media"]), &media); err != nil {
		t.Fatal(err)
	}
	for _, m := range media {
		if m["type"] != "photo" || albums[1].files[strings.TrimPrefix(m["media"], "attach://")] == "" {
			t.Fatalf("album item does not point at an uploaded file: %v", m)
		}
	}

	if err := c.SendDocument(ctx, 42, "backup.zip", []byte("PK"), "Архив"); err != nil {
		t.Fatal(err)
	}
	if doc := api.last(t, "sendDocument"); doc.files["document"] != "backup.zip" || doc.form["caption"] != "Архив" {
		t.Fatalf("unexpected document: %+v", doc)
	}
}

func TestSetupCalls(t *testing.T) {
	t.Parallel()
	c, api := newClient(t)
	ctx := context.Background()

	if err := c.SetCommands(ctx, []chat.Command{{Name: "startnew", Description: "Новая тренировка"}}); err != nil {
		t.Fatal(err)
	}
	if got := api.last(t, "setMyCommands").form["commands"]; got != `[{"command":"startnew","description":"Новая тренировка"}]` {
		t.Fatalf("unexpected commands: %s", got)
	}
	if err := c.SetMenuWebApp(ctx, "Тренировки", "https://fit.example.com/"); err != nil {
		t.Fatal(err)
	}
	if got := api.last(t, "setChatMenuButton").form["menu_button"]; got != `{"type":"web_app","text":"Тренировки","web_app":{"url":"https://fit.example.com/"}}` {
		t.Fatalf("unexpected menu button: %s", got)
	}
	if err := c.AnswerCallback(ctx, chat.Callback{ID: "777"}); err != nil {
		t.Fatal(err)
	}
	if got := api.last(t, "answerCallbackQuery").form["callback_query_id"]; got != "777" {
		t.Fatalf("unexpected callback answer: %s", got)
	}
}

func TestConvert(t *testing.T) {
	t.Parallel()
	msg, ok := convert(tgbotapi.Update{Message: &tgbotapi.Message{
		From:     &tgbotapi.User{ID: 42},
		Chat:     &tgbotapi.Chat{ID: 42},
		Text:     "10 50",
		Document: &tgbotapi.Document{FileID: "file-1", FileName: "backup.zip"},
	}})
	if !ok || msg.Message.UserID != 42 || msg.Message.Text != "10 50" || msg.Message.Document.Ref != "file-1" {
		t.Fatalf("unexpected message: %+v", msg.Message)
	}
	cb, ok := convert(tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{
		ID:      "cb",
		From:    &tgbotapi.User{ID: 42},
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 42}},
		Data:    "ex:16",
	}})
	if !ok || cb.Callback.MessageID != 7 || cb.Callback.Data != "ex:16" || cb.Callback.ChatID != 42 {
		t.Fatalf("unexpected callback: %+v", cb.Callback)
	}
	// A callback of a message too old for Telegram to send has nothing to edit.
	if _, ok := convert(tgbotapi.Update{CallbackQuery: &tgbotapi.CallbackQuery{ID: "cb", From: &tgbotapi.User{ID: 42}}}); ok {
		t.Fatal("a callback without a message must be skipped")
	}
}

func TestErrorsDoNotLeakToken(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens: the request fails with a network error that quotes the URL
	_, err := newWithEndpoint(testToken, srv.URL+"/bot%s/%s", http.DefaultClient, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("the error leaks the bot token: %v", err)
	}
}
