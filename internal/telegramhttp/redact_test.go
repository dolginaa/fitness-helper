package telegramhttp

import (
	"strings"
	"testing"
)

func TestRedactToken(t *testing.T) {
	tok := "123456789:AAHxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
	in := "Post \"https://api.telegram.org/bot" + tok + "/getMe\": error"
	out := RedactToken(in, tok)
	if !strings.Contains(out, "<BOT_TOKEN>") || strings.Contains(out, tok) {
		t.Fatalf("expected redaction, got %q", out)
	}
}
