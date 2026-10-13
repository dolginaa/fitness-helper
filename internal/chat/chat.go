// Package chat describes a conversation with the user independently of how
// the bot talks to Telegram (HTTP Bot API or MTProto).
package chat

import "context"

// Update is something the user did in the chat.
type Update struct {
	Message  *Message
	Callback *Callback
}

type Message struct {
	ChatID   int64
	UserID   int64
	Text     string
	Document *Document
}

type Document struct {
	FileName string
	// Ref identifies the file for Messenger.Download; its content belongs to the transport.
	Ref any
}

// Callback is a press of an inline button.
type Callback struct {
	ID        string
	ChatID    int64
	UserID    int64
	MessageID int
	Data      string
}

// Button either sends Data back to the bot or, with WebAppURL set, opens the Mini App.
type Button struct {
	Text      string
	Data      string
	WebAppURL string
}

// Keyboard is the rows of inline buttons under a message.
type Keyboard [][]Button

type Photo struct {
	PNG     []byte
	Caption string
}

type Command struct {
	Name        string
	Description string
}

// Messenger sends things to the chat. Texts and captions are Telegram HTML.
type Messenger interface {
	Send(ctx context.Context, chatID int64, text string, kb Keyboard) error
	// Edit replaces the text and the buttons of a message; editing to the same content is not an error.
	Edit(ctx context.Context, chatID int64, messageID int, text string, kb Keyboard) error
	// SendPhotos sends one photo as a message and several as albums.
	SendPhotos(ctx context.Context, chatID int64, photos []Photo) error
	SendDocument(ctx context.Context, chatID int64, fileName string, data []byte, caption string) error
	AnswerCallback(ctx context.Context, cb Callback) error
	Download(ctx context.Context, doc Document) ([]byte, error)
	SetCommands(ctx context.Context, commands []Command) error
	// SetMenuWebApp makes the menu button of the chat open the Mini App.
	SetMenuWebApp(ctx context.Context, title, url string) error
}

// Handler processes updates one at a time.
type Handler func(ctx context.Context, u Update)

// MaxAlbumSize is the Telegram limit of photos in one album.
const MaxAlbumSize = 10
