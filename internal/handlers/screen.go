package handlers

import (
	"context"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/view"
)

// maxMessageLen is a bit below the Telegram limit of 4096 characters.
const maxMessageLen = 4000

// target is where a screen is rendered: a new message or an edit of an existing one.
type target struct {
	chatID    int64
	messageID int
}

func newMessage(chatID int64) target {
	return target{chatID: chatID}
}

// show renders a screen: edits the target message or, if that is impossible, sends a new one.
func (b *Bot) show(ctx context.Context, t target, text string, kb chat.Keyboard) {
	parts := view.Split(text, maxMessageLen)
	if len(parts) == 0 {
		return
	}
	// Only the last part carries the keyboard; a long text cannot be an edit of one message.
	if len(parts) > 1 {
		for _, part := range parts[:len(parts)-1] {
			b.say(ctx, t.chatID, part)
		}
		t = newMessage(t.chatID)
	}
	text = parts[len(parts)-1]
	if t.messageID != 0 {
		err := b.tg.Edit(ctx, t.chatID, t.messageID, text, kb)
		if err == nil {
			return
		}
		b.log.Debug("edit message, sending a new one instead", "err", err)
	}
	if err := b.tg.Send(ctx, t.chatID, text, kb); err != nil {
		b.log.Error("send message", "err", err)
	}
}

// say sends a plain message without buttons.
func (b *Bot) say(ctx context.Context, chatID int64, text string) {
	for _, part := range view.Split(text, maxMessageLen) {
		if err := b.tg.Send(ctx, chatID, part, nil); err != nil {
			b.log.Error("send message", "err", err)
		}
	}
}

func (b *Bot) sendPhotos(ctx context.Context, chatID int64, photos ...chat.Photo) {
	if err := b.tg.SendPhotos(ctx, chatID, photos); err != nil {
		b.log.Error("send photos", "err", err)
	}
}
