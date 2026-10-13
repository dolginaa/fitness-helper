// Package mtproto talks to Telegram over MTProto, the protocol of Telegram apps.
// The bot logs in with its token on behalf of an application registered at my.telegram.org.
// Unlike the HTTP Bot API, this connection can go through an MTProto proxy.
package mtproto

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/gotd/log/logslog"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/adolgina/fitness-helper/internal/chat"
)

type Config struct {
	// APIID and APIHash identify the application (my.telegram.org/apps).
	APIID   int
	APIHash string
	// Token is the bot token from @BotFather.
	Token string
	// SessionFile keeps the authorization key, so that the bot does not log in on every start.
	SessionFile string
	// Proxy is optional.
	Proxy *Proxy
}

type Client struct {
	cfg    Config
	log    *slog.Logger
	client *telegram.Client
	gaps   *updates.Manager
	api    *tg.Client
	sender *message.Sender

	// handle is set by Run; updates are passed to it one at a time.
	handle chat.Handler
	serial sync.Mutex
	hashes sync.Map // user id -> access hash
}

func New(cfg Config, log *slog.Logger) (*Client, error) {
	if cfg.APIID == 0 || cfg.APIHash == "" {
		return nil, errors.New("the MTProto transport needs TELEGRAM_API_ID and TELEGRAM_API_HASH from https://my.telegram.org/apps")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.SessionFile), 0o700); err != nil {
		return nil, fmt.Errorf("session directory: %w", err)
	}
	c := &Client{cfg: cfg, log: log}

	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnNewMessage(c.onMessage)
	dispatcher.OnBotCallbackQuery(c.onCallback)
	c.gaps = updates.New(updates.Config{Handler: dispatcher})

	opts := telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.SessionFile},
		// The library's messages go to the bot's log; they show up with LOG_LEVEL=debug.
		Logger:        logslog.New(log.With("component", "mtproto")),
		UpdateHandler: c.gaps,
		Middlewares:   []telegram.Middleware{updhook.UpdateHook(c.gaps.Handle)},
	}
	if cfg.Proxy != nil {
		resolver, err := dcs.MTProxy(cfg.Proxy.Addr, cfg.Proxy.Secret, dcs.MTProxyOptions{})
		if err != nil {
			return nil, fmt.Errorf("mtproto proxy: %w", err)
		}
		opts.Resolver = resolver
	}
	c.client = telegram.NewClient(cfg.APIID, cfg.APIHash, opts)
	c.api = c.client.API()
	c.sender = message.NewSender(c.api).WithUploader(uploader.NewUploader(c.api))
	return c, nil
}

// Run connects, logs the bot in and passes updates to handle until ctx is cancelled.
// ready is called once the bot can send messages.
func (c *Client) Run(ctx context.Context, handle chat.Handler, ready func(username string)) error {
	c.handle = handle
	err := c.client.Run(ctx, func(ctx context.Context) error {
		status, err := c.client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		if !status.Authorized {
			if _, err := c.client.Auth().Bot(ctx, c.cfg.Token); err != nil {
				return fmt.Errorf("bot login: %w", err)
			}
		}
		self, err := c.client.Self(ctx)
		if err != nil {
			return fmt.Errorf("self: %w", err)
		}
		return c.gaps.Run(ctx, c.api, self.ID, updates.AuthOptions{
			IsBot: true,
			OnStart: func(context.Context) {
				go ready(self.Username)
			},
		})
	})
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (c *Client) onMessage(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
	msg, ok := u.Message.(*tg.Message)
	if !ok || msg.Out {
		return nil
	}
	// The bot is a personal tracker: only private chats, where the chat is the user.
	peer, ok := msg.PeerID.(*tg.PeerUser)
	if !ok {
		return nil
	}
	c.remember(e)
	m := &chat.Message{ChatID: peer.UserID, UserID: peer.UserID, Text: msg.Message}
	if media, ok := msg.Media.(*tg.MessageMediaDocument); ok {
		if doc, ok := media.Document.(*tg.Document); ok {
			m.Document = &chat.Document{FileName: fileName(doc), Ref: doc}
		}
	}
	c.dispatch(ctx, chat.Update{Message: m})
	return nil
}

func (c *Client) onCallback(ctx context.Context, e tg.Entities, u *tg.UpdateBotCallbackQuery) error {
	peer, ok := u.Peer.(*tg.PeerUser)
	if !ok {
		return nil
	}
	c.remember(e)
	c.dispatch(ctx, chat.Update{Callback: &chat.Callback{
		ID:        strconv.FormatInt(u.QueryID, 10),
		ChatID:    peer.UserID,
		UserID:    u.UserID,
		MessageID: u.MsgID,
		Data:      string(u.Data),
	}})
	return nil
}

func (c *Client) dispatch(ctx context.Context, u chat.Update) {
	c.serial.Lock()
	defer c.serial.Unlock()
	c.handle(ctx, u)
}

func (c *Client) remember(e tg.Entities) {
	for id, user := range e.Users {
		if hash, ok := user.GetAccessHash(); ok {
			c.hashes.Store(id, hash)
		}
	}
}

func fileName(doc *tg.Document) string {
	for _, attr := range doc.Attributes {
		if name, ok := attr.(*tg.DocumentAttributeFilename); ok {
			return name.FileName
		}
	}
	return ""
}

// peer addresses a private chat. Telegram lets bots pass a zero access hash for users
// who have talked to them, which covers a user not seen since the restart.
func (c *Client) peer(chatID int64) *tg.InputPeerUser {
	p := &tg.InputPeerUser{UserID: chatID}
	if hash, ok := c.hashes.Load(chatID); ok {
		p.AccessHash = hash.(int64)
	}
	return p
}

func markup(kb chat.Keyboard) tg.ReplyMarkupClass {
	rows := make([]tg.KeyboardInlineButtonRow, 0, len(kb))
	for _, row := range kb {
		buttons := make([]tg.KeyboardInlineButton, 0, len(row))
		for _, b := range row {
			var kind tg.InlineButtonTypeClass = &tg.InlineButtonTypeCallback{Data: []byte(b.Data)}
			if b.WebAppURL != "" {
				kind = &tg.InlineButtonTypeWebView{URL: b.WebAppURL}
			}
			buttons = append(buttons, tg.KeyboardInlineButton{Text: b.Text, Type: kind})
		}
		rows = append(rows, tg.KeyboardInlineButtonRow{Buttons: buttons})
	}
	return &tg.ReplyInlineMarkup{Rows: rows}
}

func styled(text string) styling.StyledTextOption {
	return html.String(nil, text)
}

// to starts a message to the chat, with buttons when there are any.
func (c *Client) to(chatID int64, kb chat.Keyboard) *message.Builder {
	b := &c.sender.To(c.peer(chatID)).Builder
	if len(kb) > 0 {
		b = b.Markup(markup(kb))
	}
	return b
}

func (c *Client) Send(ctx context.Context, chatID int64, text string, kb chat.Keyboard) error {
	_, err := c.to(chatID, kb).StyledText(ctx, styled(text))
	return err
}

func (c *Client) Edit(ctx context.Context, chatID int64, messageID int, text string, kb chat.Keyboard) error {
	_, err := c.to(chatID, kb).Edit(messageID).StyledText(ctx, styled(text))
	if tgerr.Is(err, "MESSAGE_NOT_MODIFIED") {
		return nil
	}
	return err
}

func (c *Client) SendPhotos(ctx context.Context, chatID int64, photos []chat.Photo) error {
	up := uploader.NewUploader(c.api)
	for len(photos) > 0 {
		n := min(len(photos), chat.MaxAlbumSize)
		album := make([]message.MultiMediaOption, 0, n)
		for i, p := range photos[:n] {
			file, err := up.FromBytes(ctx, fmt.Sprintf("progress%d.png", i), p.PNG)
			if err != nil {
				return fmt.Errorf("upload photo: %w", err)
			}
			var caption []styling.StyledTextOption
			if p.Caption != "" {
				caption = append(caption, styled(p.Caption))
			}
			album = append(album, message.UploadedPhoto(file, caption...))
		}
		photos = photos[n:]
		if _, err := c.to(chatID, nil).Album(ctx, album[0], album[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) SendDocument(ctx context.Context, chatID int64, fileName string, data []byte, caption string) error {
	file, err := uploader.NewUploader(c.api).FromBytes(ctx, fileName, data)
	if err != nil {
		return fmt.Errorf("upload document: %w", err)
	}
	doc := message.UploadedDocument(file, styling.Plain(caption)).Filename(fileName).ForceFile(true)
	_, err = c.to(chatID, nil).Media(ctx, doc)
	return err
}

func (c *Client) AnswerCallback(ctx context.Context, cb chat.Callback) error {
	id, err := strconv.ParseInt(cb.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("callback id: %w", err)
	}
	_, err = c.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: id})
	return err
}

func (c *Client) Download(ctx context.Context, doc chat.Document) ([]byte, error) {
	d, ok := doc.Ref.(*tg.Document)
	if !ok {
		return nil, errors.New("document does not belong to the MTProto transport")
	}
	var buf bytes.Buffer
	if _, err := downloader.NewDownloader().Download(c.api, d.AsInputDocumentFileLocation("")).Stream(ctx, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Client) SetCommands(ctx context.Context, commands []chat.Command) error {
	list := make([]tg.BotCommand, 0, len(commands))
	for _, cmd := range commands {
		list = append(list, tg.BotCommand{Command: cmd.Name, Description: cmd.Description})
	}
	_, err := c.api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		Commands: list,
	})
	return err
}

func (c *Client) SetMenuWebApp(ctx context.Context, title, url string) error {
	_, err := c.api.BotsSetBotMenuButton(ctx, &tg.BotsSetBotMenuButtonRequest{
		UserID: &tg.InputUserEmpty{},
		Button: &tg.BotMenuButton{Text: title, URL: url},
	})
	return err
}

var _ chat.Messenger = (*Client)(nil)
