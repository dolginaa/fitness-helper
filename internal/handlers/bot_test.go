package handlers

import (
	"context"
	"log/slog"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestHandleUpdate_NilCallbackMessage_NoPanic(t *testing.T) {
	t.Parallel()
	b := &Bot{Log: slog.Default()}
	u := tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "1",
			From: &tgbotapi.User{ID: 1},
			Data: "x",
		},
	}
	// Svc is nil — only testing early return before Svc use
	b.HandleUpdate(context.Background(), nil, u)
}
