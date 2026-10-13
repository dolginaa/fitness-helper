// Package charts renders progress charts as PNG images sized for a Telegram chat.
package charts

import (
	"fmt"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

const minPoints = 2

// Layout of the exercise chart.
const (
	exerciseHeight = 880

	titleY    = 92
	subtitleY = 138

	plotLeft  = 150
	plotRight = canvasWidth - margin
	// plotInset keeps the first and the last mark away from the plot edges.
	plotInset = 44

	panelLabelY = 204
	panelTop    = 232
	weightBot   = 520
	volumeLabel = 594
	volumeTop   = 622
	panelBottom = 790
	dateY       = 836
)

type Renderer struct{}

func New() Renderer {
	return Renderer{}
}

// panels are the two charts an exercise gets: a line on top and bars under it.
type panels struct {
	topTitle, bottomTitle string
	top, bottom           []float64
}

// exercisePanels chooses what to draw for the kind of load. Weights, reps and band levels
// cannot share an axis, so a chart is made of one kind (see domain.ChartSeries).
func exercisePanels(kind domain.LoadKind, points []domain.ExerciseProgressPoint) panels {
	var pn panels
	weighted := false
	for _, p := range points {
		pn.top = append(pn.top, 0)
		pn.bottom = append(pn.bottom, float64(p.TotalReps))
		weighted = weighted || p.MaxWeight > 0
	}
	for i, p := range points {
		switch kind {
		case domain.LoadBand:
			pn.top[i] = float64(p.Band)
		case domain.LoadBodyweight:
			pn.top[i] = float64(p.MaxReps)
		default:
			pn.top[i] = p.MaxWeight
			if weighted {
				pn.bottom[i] = p.Volume
			}
		}
	}
	switch {
	case kind == domain.LoadBand:
		pn.topTitle = fmt.Sprintf("Резинка, уровень (1 — тонкая, %d — толстая)", domain.MaxBand)
		pn.bottomTitle = "Всего повторов за тренировку"
	case kind == domain.LoadBodyweight:
		pn.topTitle = "Макс. повторов в подходе"
		pn.bottomTitle = "Всего повторов за тренировку"
	case weighted:
		pn.topTitle = "Макс. вес, кг"
		pn.bottomTitle = "Объём за тренировку (вес × повторы), кг"
	default:
		// An exercise done without weight: one chart of reps.
		pn.topTitle, pn.top, pn.bottom = "Повторов за тренировку", pn.bottom, nil
	}
	return pn
}

// loadLabel names the kind in a subtitle; plain weight needs no note.
func loadLabel(kind domain.LoadKind) string {
	switch kind {
	case domain.LoadBodyweight:
		return "свой вес"
	case domain.LoadBand:
		return "с резинкой"
	}
	return ""
}

// ExerciseProgress draws how the exercise changed from workout to workout: the top weight
// (or reps, or band level) as a line and the volume (or total reps) as bars under it.
func (Renderer) ExerciseProgress(name string, points []domain.ExerciseProgressPoint) ([]byte, error) {
	kind, points := domain.ChartSeries(points)
	if len(points) < minPoints {
		return nil, domain.ErrNotEnoughData
	}
	c, err := newCanvas(canvasWidth, exerciseHeight)
	if err != nil {
		return nil, err
	}
	first, last := points[0].Date, points[len(points)-1].Date
	subtitle := fmt.Sprintf("%s · тренировок: %d", period(first, last), len(points))
	if l := loadLabel(kind); l != "" {
		subtitle += " · " + l
	}
	c.text(c.fit(name, sizeTitle, canvasWidth-2*margin), margin, titleY, sizeTitle, colorInk, alignLeft)
	c.text(subtitle, margin, subtitleY, sizeLabel, colorMuted, alignLeft)

	x := timeScale(first, last, plotLeft+plotInset, plotRight-plotInset)
	xs := make([]int, len(points))
	for i, p := range points {
		xs[i] = x(p.Date)
	}
	pn := exercisePanels(kind, points)
	c.text(pn.topTitle, margin, panelLabelY, sizeLabel, colorSecondary, alignLeft)
	if pn.bottom == nil {
		c.linePanel(xs, pn.top, panelTop, panelBottom)
	} else {
		c.linePanel(xs, pn.top, panelTop, weightBot)
		c.text(pn.bottomTitle, margin, volumeLabel, sizeLabel, colorSecondary, alignLeft)
		c.barPanel(xs, pn.bottom, volumeTop, panelBottom)
	}
	c.dateAxis(xs, points, plotLeft, plotRight, dateY)
	return c.png()
}

// linePanel draws one series as a line with a light fill under it.
// Only the last, the highest and the first value are labelled; the axis carries the rest.
func (c *canvas) linePanel(xs []int, values []float64, top, bottom int) {
	lo, hi, step := lineDomain(values)
	y := linear{d0: lo, d1: hi, r0: bottom, r1: top}
	pts := make([]point, len(values))
	for i, v := range values {
		pts[i] = point{xs[i], y.at(v)}
	}

	area := append([]point{}, pts...)
	area = append(area, point{pts[len(pts)-1].x, bottom}, point{pts[0].x, bottom})
	c.polygon(area, colorWash)
	c.yAxis(y, ticks(lo, hi, step), lo)
	c.polyline(pts, lineWidth, colorSeries)
	for _, p := range pts {
		c.dot(p, dotRadius)
	}

	l := labels{c: c, size: sizeValue, color: colorInk}
	for _, i := range []int{len(values) - 1, maxIndex(values), 0} {
		l.put(number(values[i]), pts[i].x, pts[i].y-dotRadius-dotRing-12, plotLeft, plotRight)
	}
}

// barPanel draws one series as columns on a zero-based axis.
func (c *canvas) barPanel(xs []int, values []float64, top, bottom int) {
	hi, step := barDomain(values)
	y := linear{d0: 0, d1: hi, r0: bottom, r1: top}
	c.yAxis(y, ticks(0, hi, step), 0)

	const maxBarWidth, minBarWidth, barGap = 56, 10, 6
	width := maxBarWidth
	for i := 1; i < len(xs); i++ {
		width = min(width, xs[i]-xs[i-1]-barGap)
	}
	width = max(width, minBarWidth)
	for i, v := range values {
		c.bar(xs[i]-width/2, xs[i]+width/2, y.at(v), bottom, colorSeries)
	}

	l := labels{c: c, size: sizeValue, color: colorInk}
	for _, i := range []int{len(values) - 1, maxIndex(values)} {
		l.put(number(values[i]), xs[i], y.at(values[i])-12, plotLeft, plotRight)
	}
}

// yAxis draws hairline gridlines with round values on the left; the line at base is the baseline.
func (c *canvas) yAxis(y linear, values []float64, base float64) {
	for _, v := range values {
		color := colorGrid
		if v == base {
			color = colorBaseline
		}
		c.hline(plotLeft, plotRight, y.at(v), color)
		c.text(number(v), plotLeft-18, y.at(v)+sizeAxis/3, sizeAxis, colorMuted, alignRight)
	}
}

// dateAxis writes workout dates under their marks, as many as fit without overlapping.
func (c *canvas) dateAxis(xs []int, points []domain.ExerciseProgressPoint, left, right, y int) {
	l := labels{c: c, size: sizeAxis, color: colorMuted}
	last := len(points) - 1
	l.put(shortDate(points[last].Date), xs[last], y, left, right)
	for i := 0; i < last; i++ {
		l.put(shortDate(points[i].Date), xs[i], y, left, right)
	}
}

// Layout of the muscle group chart: a grid of small charts, one per exercise.
const (
	groupColumns = 2
	groupTop     = 190
	cellHeight   = 300
	cellGap      = 64

	cellNameY  = 30
	cellStatY  = 86
	sparkTop   = 122
	sparkBot   = 216
	cellDateY  = 254
	sparkInset = 16
	// sparkPad keeps the end marker clear of the baseline and of the text above.
	sparkPad = 18

	sizeStat     = 44
	sparkDot     = 6
	sizeCellDate = 24
)

// MuscleGroupProgress draws the top weight of every exercise that has enough data
// (the most reps in a set for exercises without weight).
// Each exercise gets its own small chart with its own scale: exercises of one group
// differ in weight several times, and on a common axis every line would look flat.
// The time axis is shared, so the charts can be compared by date.
func (Renderer) MuscleGroupProgress(groupName string, series []domain.ExerciseProgressSeries) ([]byte, error) {
	var shown []sparkSeries
	var first, last time.Time
	for _, s := range series {
		kind, points := domain.ChartSeries(s.Points)
		if len(points) < minPoints {
			continue
		}
		shown = append(shown, newSparkSeries(s.ExerciseName, kind, points))
		if from := points[0].Date; first.IsZero() || from.Before(first) {
			first = from
		}
		if to := points[len(points)-1].Date; to.After(last) {
			last = to
		}
	}
	if len(shown) == 0 {
		return nil, domain.ErrNotEnoughData
	}

	columns := min(groupColumns, len(shown))
	rows := (len(shown) + columns - 1) / columns
	c, err := newCanvas(canvasWidth, groupTop+rows*cellHeight)
	if err != nil {
		return nil, err
	}
	c.text(c.fit(capitalize(groupName), sizeTitle, canvasWidth-2*margin), margin, titleY, sizeTitle, colorInk, alignLeft)
	c.text("Лучший подход по упражнениям · "+period(first, last), margin, subtitleY, sizeLabel, colorMuted, alignLeft)

	cellWidth := (canvasWidth - 2*margin - (columns-1)*cellGap) / columns
	for i, s := range shown {
		left := margin + (i%columns)*(cellWidth+cellGap)
		top := groupTop + (i/columns)*cellHeight
		c.sparkCell(s, left, left+cellWidth, top, timeScale(first, last, left+sparkInset, left+cellWidth-sparkInset))
	}
	return c.png()
}

// sparkSeries is what a small chart shows: one number per workout and what the number is.
type sparkSeries struct {
	name   string
	unit   string
	values []float64
	dates  []time.Time
}

func newSparkSeries(name string, kind domain.LoadKind, points []domain.ExerciseProgressPoint) sparkSeries {
	s := sparkSeries{name: name, unit: "кг"}
	weighted := false
	for _, p := range points {
		weighted = weighted || p.MaxWeight > 0
	}
	if l := loadLabel(kind); l != "" {
		s.name += " · " + l
	}
	if kind != domain.LoadWeight || !weighted {
		s.unit = "повт."
	}
	for _, p := range points {
		v := p.MaxWeight
		if s.unit == "повт." {
			v = float64(p.MaxReps)
		}
		s.values = append(s.values, v)
		s.dates = append(s.dates, p.Date)
	}
	return s
}

// sparkCell draws one exercise: its name, the current value with the change
// since the first workout of the period, and a small line.
func (c *canvas) sparkCell(s sparkSeries, left, right, top int, x func(time.Time) int) {
	values := s.values
	current := values[len(values)-1]

	c.text(c.fit(s.name, sizeLabel, right-left), left, top+cellNameY, sizeLabel, colorSecondary, alignLeft)
	stat := number(current) + " " + s.unit
	c.text(stat, left, top+cellStatY, sizeStat, colorInk, alignLeft)
	delta, deltaColor := "без изменений", colorMuted
	switch change := current - values[0]; {
	case change > 0:
		delta, deltaColor = "+"+number(change)+" "+s.unit, colorGood
	case change < 0:
		delta, deltaColor = number(change)+" "+s.unit, colorSecondary
	}
	c.text(delta, left+c.textWidth(stat, sizeStat)+18, top+cellStatY, sizeLabel, deltaColor, alignLeft)

	lo, hi := bounds(values)
	y := linear{d0: lo, d1: hi, r0: top + sparkBot - sparkPad, r1: top + sparkTop + sparkPad}
	pts := make([]point, len(values))
	for i, v := range values {
		pts[i] = point{x(s.dates[i]), y.at(v)}
	}
	area := append([]point{}, pts...)
	area = append(area, point{pts[len(pts)-1].x, top + sparkBot}, point{pts[0].x, top + sparkBot})
	c.polygon(area, colorWash)
	c.hline(left, right, top+sparkBot, colorGrid)
	c.polyline(pts, lineWidth, colorSeries)
	for _, p := range pts[:len(pts)-1] {
		c.dot(p, sparkDot)
	}
	c.dot(pts[len(pts)-1], dotRadius)

	l := labels{c: c, size: sizeCellDate, color: colorMuted}
	l.put(shortDate(s.dates[len(s.dates)-1]), pts[len(pts)-1].x, top+cellDateY, left, right)
	l.put(shortDate(s.dates[0]), pts[0].x, top+cellDateY, left, right)
}
