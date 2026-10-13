// Package view builds the texts the bot shows to the user (Telegram HTML).
package view

import (
	"html"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/adolgina/fitness-helper/internal/domain"
)

const NoData = "нет данных"

var weekdays = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}

// Weight prints 50 as "50" and 52.5 as "52.5".
func Weight(w float64) string {
	return strconv.FormatFloat(math.Round(w*100)/100, 'f', -1, 64)
}

func Set(s domain.SetEntry) string {
	return strconv.Itoa(s.Reps) + "×" + Weight(s.Weight)
}

func Sets(sets []domain.SetEntry) string {
	if len(sets) == 0 {
		return NoData
	}
	parts := make([]string, 0, len(sets))
	for _, s := range sets {
		parts = append(parts, Set(s))
	}
	return strings.Join(parts, ", ")
}

func Date(t time.Time) string {
	return t.Local().Format("02.01.2006")
}

// ShortDate drops the year while it is the current one.
func ShortDate(t time.Time) string {
	t = t.Local()
	if t.Year() == time.Now().Year() {
		return t.Format("02.01")
	}
	return t.Format("02.01.2006")
}

// DayLabel is a date with the day of week: "сб 08.02.2026".
func DayLabel(t time.Time) string {
	t = t.Local()
	return weekdays[t.Weekday()] + " " + t.Format("02.01.2006")
}

func esc(s string) string {
	return html.EscapeString(s)
}

func bold(s string) string {
	return "<b>" + esc(s) + "</b>"
}

// title upper-cases the first letter: muscle groups are stored in lower case.
func title(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// thousands prints 12500 as "12 500".
func thousands(v float64) string {
	digits := strconv.FormatInt(int64(math.Round(v)), 10)
	var sb strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			sb.WriteRune(' ')
		}
		sb.WriteRune(d)
	}
	return sb.String()
}

// Split cuts a long text into messages of at most limit characters, breaking at line ends.
func Split(text string, limit int) []string {
	var parts []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if curLen > 0 {
			parts = append(parts, strings.TrimRight(cur.String(), "\n"))
			cur.Reset()
			curLen = 0
		}
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		runes := []rune(line)
		for len(runes) > limit {
			flush()
			parts = append(parts, string(runes[:limit]))
			runes = runes[limit:]
		}
		if curLen+len(runes) > limit {
			flush()
		}
		cur.WriteString(string(runes))
		curLen += len(runes)
	}
	flush()
	return parts
}

func roundTo(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	return math.Round(v*p) / p
}
