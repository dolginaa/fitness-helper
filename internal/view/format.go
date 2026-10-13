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

// Set is a set in a list: 10×50, 15×св.вес, 15×св.вес+5, 12×рез.4.
func Set(s domain.SetEntry) string {
	reps := strconv.Itoa(s.Reps)
	switch s.Load() {
	case domain.LoadBodyweight:
		if s.Weight > 0 {
			return reps + "×св.вес+" + Weight(s.Weight)
		}
		return reps + "×св.вес"
	case domain.LoadBand:
		return reps + "×рез." + strconv.Itoa(s.Band)
	}
	return reps + "×" + Weight(s.Weight)
}

// SetLine is a set written out for the screen of an exercise: "10 × 50 кг", "15 × свой вес".
func SetLine(s domain.SetEntry) string {
	reps := strconv.Itoa(s.Reps) + " × "
	switch s.Load() {
	case domain.LoadBodyweight:
		if s.Weight > 0 {
			return reps + "свой вес +" + Weight(s.Weight) + " кг"
		}
		return reps + "свой вес"
	case domain.LoadBand:
		return reps + "резинка " + strconv.Itoa(s.Band)
	}
	return reps + Weight(s.Weight) + " кг"
}

// LoadName is the name of a kind of load for buttons and headings.
func LoadName(k domain.LoadKind) string {
	switch k {
	case domain.LoadBodyweight:
		return "Свой вес"
	case domain.LoadBand:
		return "Резинка"
	}
	return "Вес"
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

// Duration prints 52 minutes as "52 мин" and 72 as "1 ч 12 мин".
func Duration(d time.Duration) string {
	minutes := int(d.Round(time.Minute).Minutes())
	if minutes < 60 {
		return strconv.Itoa(minutes) + " мин"
	}
	if minutes%60 == 0 {
		return strconv.Itoa(minutes/60) + " ч"
	}
	return strconv.Itoa(minutes/60) + " ч " + strconv.Itoa(minutes%60) + " мин"
}

// Percent prints the change from before to now: "+8%", "−3%", "0%".
func Percent(before, now float64) string {
	if before == 0 {
		return ""
	}
	p := int(math.Round((now - before) / before * 100))
	switch {
	case p > 0:
		return "+" + strconv.Itoa(p) + "%"
	case p < 0:
		return "−" + strconv.Itoa(-p) + "%"
	}
	return "0%"
}

// DaysAgo says how long ago a day was, counted in calendar days.
func DaysAgo(days int) string {
	switch {
	case days <= 0:
		return "сегодня"
	case days == 1:
		return "вчера"
	}
	return strconv.Itoa(days) + " дн. назад"
}
