package charts

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// linear maps a value from the domain [d0, d1] to pixels [r0, r1].
type linear struct {
	d0, d1 float64
	r0, r1 int
}

func (s linear) at(v float64) int {
	if s.d1 == s.d0 {
		return (s.r0 + s.r1) / 2
	}
	return s.r0 + int(math.Round((v-s.d0)/(s.d1-s.d0)*float64(s.r1-s.r0)))
}

// timeScale maps dates to pixels proportionally to the time between them.
func timeScale(first, last time.Time, r0, r1 int) func(time.Time) int {
	s := linear{d0: 0, d1: last.Sub(first).Seconds(), r0: r0, r1: r1}
	return func(t time.Time) int {
		return s.at(t.Sub(first).Seconds())
	}
}

// niceStep returns a round step (1, 2, 2.5 or 5 times a power of ten)
// that splits the span into at most maxTicks intervals.
func niceStep(span float64, maxTicks int) float64 {
	if span <= 0 {
		return 1
	}
	raw := span / float64(maxTicks)
	pow := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*pow >= raw {
			return m * pow
		}
	}
	return 10 * pow
}

// ticks returns the round values from lo to hi (both multiples of step).
func ticks(lo, hi, step float64) []float64 {
	var out []float64
	for i := 0; ; i++ {
		v := lo + float64(i)*step
		if v > hi+step/1000 {
			return out
		}
		out = append(out, v)
	}
}

// lineDomain picks round axis bounds around the values of a line.
// It leaves room above the highest point for its label.
func lineDomain(values []float64) (lo, hi, step float64) {
	lo, hi = bounds(values)
	span := hi - lo
	if span == 0 {
		// The same value every time: still give the axis some height.
		span = math.Max(hi*0.2, 2)
	}
	step = niceStep(span, 3)
	axisLo := math.Floor(lo/step) * step
	axisHi := math.Ceil(hi/step) * step
	if axisHi-hi < step*0.5 {
		axisHi += step
	}
	if axisLo == lo && axisLo >= step {
		axisLo -= step
	}
	return axisLo, axisHi, step
}

// barDomain is a zero-based axis: bars are compared by length.
func barDomain(values []float64) (hi, step float64) {
	_, top := bounds(values)
	if top <= 0 {
		return 1, 1
	}
	// A quarter of the height stays free for the label over the highest bar.
	step = niceStep(top*1.25, 3)
	return math.Ceil(top*1.25/step) * step, step
}

func bounds(values []float64) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, v := range values {
		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
	}
	return lo, hi
}

func maxIndex(values []float64) int {
	best := 0
	for i, v := range values {
		if v > values[best] {
			best = i
		}
	}
	return best
}

// number prints 52.5 as "52.5" and 3900 as "3 900".
func number(v float64) string {
	v = math.Round(v*10) / 10
	whole, frac := math.Modf(math.Abs(v))
	digits := strconv.FormatInt(int64(whole), 10)
	var sb strings.Builder
	if v < 0 {
		sb.WriteString("−")
	}
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			sb.WriteRune(' ')
		}
		sb.WriteRune(d)
	}
	if frac > 0.04 {
		sb.WriteString(strconv.FormatFloat(frac, 'f', 1, 64)[1:])
	}
	return sb.String()
}

func shortDate(t time.Time) string {
	return t.Local().Format("02.01")
}

// period prints the dates covered by the chart: "08.02 – 13.05.2026".
func period(first, last time.Time) string {
	return shortDate(first) + " – " + last.Local().Format("02.01.2006")
}

// capitalize upper-cases the first letter: muscle groups are stored in lower case.
func capitalize(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
