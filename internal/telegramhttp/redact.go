package telegramhttp

import "strings"

// RedactToken removes the bot token from strings (e.g. error messages, URLs) before logging.
func RedactToken(s, token string) string {
	if token == "" || len(token) < 12 {
		return s
	}
	return strings.ReplaceAll(s, token, "<BOT_TOKEN>")
}
