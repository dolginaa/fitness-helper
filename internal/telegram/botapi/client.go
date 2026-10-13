// Package botapi talks to Telegram over the HTTP Bot API.
package botapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/telegramhttp"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const pollTimeoutSeconds = 60

type Client struct {
	api   *tgbotapi.BotAPI
	token string
	log   *slog.Logger
}

// New checks the token against Telegram. All requests go through httpClient.
func New(token string, httpClient *http.Client, log *slog.Logger) (*Client, error) {
	return newWithEndpoint(token, tgbotapi.APIEndpoint, httpClient, log)
}

func newWithEndpoint(token, endpoint string, httpClient *http.Client, log *slog.Logger) (*Client, error) {
	c := &Client{token: token, log: log}
	api, err := tgbotapi.NewBotAPIWithClient(token, endpoint, httpClient)
	if err != nil {
		return nil, c.redact(err)
	}
	c.api = api
	return c, nil
}

// Run long-polls updates and passes them to handle one by one until ctx is cancelled.
// The token is already checked by New, so ready is called at once.
func (c *Client) Run(ctx context.Context, handle chat.Handler, ready func(username string)) error {
	go ready(c.api.Self.UserName)
	cfg := tgbotapi.NewUpdate(0)
	cfg.Timeout = pollTimeoutSeconds
	updates := c.api.GetUpdatesChan(cfg)
	defer c.api.StopReceivingUpdates()
	for {
		select {
		case <-ctx.Done():
			return nil
		case u, ok := <-updates:
			if !ok {
				return nil
			}
			if upd, ok := convert(u); ok {
				handle(ctx, upd)
			}
		}
	}
}

func convert(u tgbotapi.Update) (chat.Update, bool) {
	switch {
	case u.Message != nil && u.Message.From != nil:
		m := &chat.Message{
			ChatID: u.Message.Chat.ID,
			UserID: u.Message.From.ID,
			Text:   u.Message.Text,
		}
		if d := u.Message.Document; d != nil {
			m.Document = &chat.Document{FileName: d.FileName, Ref: d.FileID}
		}
		return chat.Update{Message: m}, true
	case u.CallbackQuery != nil && u.CallbackQuery.Message != nil && u.CallbackQuery.From != nil:
		cq := u.CallbackQuery
		return chat.Update{Callback: &chat.Callback{
			ID:        cq.ID,
			ChatID:    cq.Message.Chat.ID,
			UserID:    cq.From.ID,
			MessageID: cq.Message.MessageID,
			Data:      cq.Data,
		}}, true
	}
	return chat.Update{}, false
}

// The library has no types for Mini App buttons, so the markup is built here.
type webAppInfo struct {
	URL string `json:"url"`
}

type inlineButton struct {
	Text         string      `json:"text"`
	CallbackData string      `json:"callback_data,omitempty"`
	WebApp       *webAppInfo `json:"web_app,omitempty"`
}

type inlineMarkup struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

func markup(kb chat.Keyboard) inlineMarkup {
	rows := make([][]inlineButton, 0, len(kb))
	for _, row := range kb {
		buttons := make([]inlineButton, 0, len(row))
		for _, b := range row {
			btn := inlineButton{Text: b.Text, CallbackData: b.Data}
			if b.WebAppURL != "" {
				btn = inlineButton{Text: b.Text, WebApp: &webAppInfo{URL: b.WebAppURL}}
			}
			buttons = append(buttons, btn)
		}
		rows = append(rows, buttons)
	}
	return inlineMarkup{InlineKeyboard: rows}
}

func textParams(chatID int64, text string, kb chat.Keyboard) (tgbotapi.Params, error) {
	params := tgbotapi.Params{
		"chat_id":    strconv.FormatInt(chatID, 10),
		"text":       text,
		"parse_mode": tgbotapi.ModeHTML,
	}
	if len(kb) > 0 {
		if err := params.AddInterface("reply_markup", markup(kb)); err != nil {
			return nil, err
		}
	}
	return params, nil
}

func (c *Client) call(method string, params tgbotapi.Params) error {
	_, err := c.api.MakeRequest(method, params)
	return c.redact(err)
}

func (c *Client) Send(ctx context.Context, chatID int64, text string, kb chat.Keyboard) error {
	params, err := textParams(chatID, text, kb)
	if err != nil {
		return err
	}
	return c.call("sendMessage", params)
}

func (c *Client) Edit(ctx context.Context, chatID int64, messageID int, text string, kb chat.Keyboard) error {
	params, err := textParams(chatID, text, kb)
	if err != nil {
		return err
	}
	params["message_id"] = strconv.Itoa(messageID)
	err = c.call("editMessageText", params)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

type inputMediaPhoto struct {
	Type      string `json:"type"`
	Media     string `json:"media"`
	Caption   string `json:"caption,omitempty"`
	ParseMode string `json:"parse_mode,omitempty"`
}

func (c *Client) SendPhotos(ctx context.Context, chatID int64, photos []chat.Photo) error {
	for len(photos) > 0 {
		n := min(len(photos), chat.MaxAlbumSize)
		album := photos[:n]
		photos = photos[n:]
		var err error
		if len(album) == 1 {
			err = c.sendPhoto(chatID, album[0])
		} else {
			err = c.sendAlbum(chatID, album)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) sendPhoto(chatID int64, p chat.Photo) error {
	params := tgbotapi.Params{"chat_id": strconv.FormatInt(chatID, 10)}
	if p.Caption != "" {
		params["caption"] = p.Caption
		params["parse_mode"] = tgbotapi.ModeHTML
	}
	_, err := c.api.UploadFiles("sendPhoto", params, []tgbotapi.RequestFile{
		{Name: "photo", Data: tgbotapi.FileBytes{Name: "progress.png", Bytes: p.PNG}},
	})
	return c.redact(err)
}

func (c *Client) sendAlbum(chatID int64, album []chat.Photo) error {
	media := make([]inputMediaPhoto, 0, len(album))
	files := make([]tgbotapi.RequestFile, 0, len(album))
	for i, p := range album {
		name := fmt.Sprintf("photo%d", i)
		item := inputMediaPhoto{Type: "photo", Media: "attach://" + name, Caption: p.Caption}
		if p.Caption != "" {
			item.ParseMode = tgbotapi.ModeHTML
		}
		media = append(media, item)
		files = append(files, tgbotapi.RequestFile{Name: name, Data: tgbotapi.FileBytes{Name: name + ".png", Bytes: p.PNG}})
	}
	params := tgbotapi.Params{"chat_id": strconv.FormatInt(chatID, 10)}
	if err := params.AddInterface("media", media); err != nil {
		return err
	}
	_, err := c.api.UploadFiles("sendMediaGroup", params, files)
	return c.redact(err)
}

func (c *Client) SendDocument(ctx context.Context, chatID int64, fileName string, data []byte, caption string) error {
	params := tgbotapi.Params{"chat_id": strconv.FormatInt(chatID, 10)}
	params.AddNonEmpty("caption", caption)
	_, err := c.api.UploadFiles("sendDocument", params, []tgbotapi.RequestFile{
		{Name: "document", Data: tgbotapi.FileBytes{Name: fileName, Bytes: data}},
	})
	return c.redact(err)
}

func (c *Client) AnswerCallback(ctx context.Context, cb chat.Callback) error {
	return c.call("answerCallbackQuery", tgbotapi.Params{"callback_query_id": cb.ID})
}

func (c *Client) Download(ctx context.Context, doc chat.Document) ([]byte, error) {
	fileID, ok := doc.Ref.(string)
	if !ok {
		return nil, errors.New("document does not belong to the Bot API transport")
	}
	file, err := c.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, c.redact(err)
	}
	if file.FilePath == "" {
		return nil, errors.New("telegram file path is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.Link(c.token), nil)
	if err != nil {
		return nil, c.redact(err)
	}
	resp, err := c.api.Client.Do(req)
	if err != nil {
		return nil, c.redact(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download file: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type botCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

func (c *Client) SetCommands(ctx context.Context, commands []chat.Command) error {
	list := make([]botCommand, 0, len(commands))
	for _, cmd := range commands {
		list = append(list, botCommand{Command: cmd.Name, Description: cmd.Description})
	}
	params := tgbotapi.Params{}
	if err := params.AddInterface("commands", list); err != nil {
		return err
	}
	return c.call("setMyCommands", params)
}

type menuButton struct {
	Type   string     `json:"type"`
	Text   string     `json:"text"`
	WebApp webAppInfo `json:"web_app"`
}

func (c *Client) SetMenuWebApp(ctx context.Context, title, url string) error {
	params := tgbotapi.Params{}
	if err := params.AddInterface("menu_button", menuButton{Type: "web_app", Text: title, WebApp: webAppInfo{URL: url}}); err != nil {
		return err
	}
	return c.call("setChatMenuButton", params)
}

// redact removes the bot token from an error: the library puts request URLs into network errors.
func (c *Client) redact(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(telegramhttp.RedactToken(err.Error(), c.token))
}

var _ chat.Messenger = (*Client)(nil)
