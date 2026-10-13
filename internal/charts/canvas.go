package charts

import (
	"bytes"

	"github.com/golang/freetype/truetype"
	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

// The image is shown in a chat bubble about three times smaller than its pixel size,
// so every size here is roughly 3x of what it should look like on the screen.
const (
	canvasWidth = 1200
	margin      = 56

	sizeTitle = 44
	sizeLabel = 28
	sizeValue = 30
	sizeAxis  = 26

	lineWidth = 6
	gridWidth = 2
	dotRadius = 9
	dotRing   = 4
	barRadius = 10
)

var (
	colorSurface   = drawing.ColorFromHex("fcfcfb")
	colorInk       = drawing.ColorFromHex("0b0b0b")
	colorSecondary = drawing.ColorFromHex("52514e")
	colorMuted     = drawing.ColorFromHex("898781")
	colorGrid      = drawing.ColorFromHex("e1e0d9")
	colorBaseline  = drawing.ColorFromHex("c3c2b7")
	colorSeries    = drawing.ColorFromHex("2a78d6")
	// colorWash is the series color at 10% over the surface.
	colorWash = drawing.ColorFromHex("e7eff7")
	colorGood = drawing.ColorFromHex("006300")
)

type point struct{ x, y int }

type align int

const (
	alignLeft align = iota
	alignCenter
	alignRight
)

// canvas is a thin drawing layer over the go-chart raster renderer.
type canvas struct {
	r             chart.Renderer
	width, height int
}

func newCanvas(width, height int) (*canvas, error) {
	r, err := chart.PNG(width, height)
	if err != nil {
		return nil, err
	}
	font, err := chart.GetDefaultFont()
	if err != nil {
		return nil, err
	}
	// 72 DPI makes font sizes equal to pixels.
	r.SetDPI(72)
	c := &canvas{r: r, width: width, height: height}
	c.setFont(font)
	c.rect(0, 0, width, height, colorSurface)
	return c, nil
}

func (c *canvas) setFont(f *truetype.Font) {
	c.r.SetFont(f)
}

func (c *canvas) png() ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := c.r.Save(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *canvas) rect(x0, y0, x1, y1 int, fill drawing.Color) {
	c.polygon([]point{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}, fill)
}

func (c *canvas) polygon(pts []point, fill drawing.Color) {
	if len(pts) < 3 {
		return
	}
	c.r.SetFillColor(fill)
	c.r.MoveTo(pts[0].x, pts[0].y)
	for _, p := range pts[1:] {
		c.r.LineTo(p.x, p.y)
	}
	c.r.Close()
	c.r.Fill()
}

func (c *canvas) polyline(pts []point, width float64, color drawing.Color) {
	if len(pts) < 2 {
		return
	}
	c.r.SetStrokeColor(color)
	c.r.SetStrokeWidth(width)
	c.r.MoveTo(pts[0].x, pts[0].y)
	for _, p := range pts[1:] {
		c.r.LineTo(p.x, p.y)
	}
	c.r.Stroke()
}

func (c *canvas) hline(x0, x1, y int, color drawing.Color) {
	c.polyline([]point{{x0, y}, {x1, y}}, gridWidth, color)
}

// dot draws a marker with a ring in the surface color, so it stays readable on top of a line.
func (c *canvas) dot(p point, radius float64) {
	c.r.SetFillColor(colorSurface)
	c.r.Circle(radius+dotRing, p.x, p.y)
	c.r.Fill()
	c.r.SetFillColor(colorSeries)
	c.r.Circle(radius, p.x, p.y)
	c.r.Fill()
}

// bar draws a column that is rounded at the data end and square at the baseline.
func (c *canvas) bar(x0, x1, top, base int, fill drawing.Color) {
	if base-top < 1 {
		return
	}
	radius := min(barRadius, base-top, (x1-x0)/2)
	c.r.SetFillColor(fill)
	c.r.MoveTo(x0, base)
	c.r.LineTo(x0, top+radius)
	c.r.QuadCurveTo(x0, top, x0+radius, top)
	c.r.LineTo(x1-radius, top)
	c.r.QuadCurveTo(x1, top, x1, top+radius)
	c.r.LineTo(x1, base)
	c.r.Close()
	c.r.Fill()
}

func (c *canvas) textWidth(s string, size float64) int {
	c.r.SetFontSize(size)
	return c.r.MeasureText(s).Width()
}

// text draws s with its baseline at y.
func (c *canvas) text(s string, x, y int, size float64, color drawing.Color, a align) {
	switch a {
	case alignCenter:
		x -= c.textWidth(s, size) / 2
	case alignRight:
		x -= c.textWidth(s, size)
	}
	c.r.SetFontSize(size)
	c.r.SetFontColor(color)
	c.r.Text(s, x, y)
}

// fit shortens s with an ellipsis until it is at most maxWidth wide.
func (c *canvas) fit(s string, size float64, maxWidth int) string {
	if c.textWidth(s, size) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 1 {
		runes = runes[:len(runes)-1]
		if short := string(runes) + "…"; c.textWidth(short, size) <= maxWidth {
			return short
		}
	}
	return s
}

// span is a horizontal extent already taken by a label.
type span struct{ left, right int }

// labels places texts on one line and skips those that would overlap an earlier one.
type labels struct {
	c     *canvas
	size  float64
	color drawing.Color
	taken []span
}

// put draws s centered at x (kept inside [left, right]) unless the place is taken.
func (l *labels) put(s string, x, y, left, right int) bool {
	const gap = 14
	w := l.c.textWidth(s, l.size)
	x0 := min(max(x-w/2, left), right-w)
	for _, t := range l.taken {
		if x0 < t.right+gap && t.left < x0+w+gap {
			return false
		}
	}
	l.taken = append(l.taken, span{x0, x0 + w})
	l.c.text(s, x0, y, l.size, l.color, alignLeft)
	return true
}
