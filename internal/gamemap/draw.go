package gamemap

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
)

// Point is a single annotated position on the map: a marker at (PosX, PosY) in game meters (PosX east,
// PosY north), with an optional label above and below it.
type Point struct {
	PosX, PosY  float64
	TopLabel    string
	BottomLabel string
	Color       color.Color // optional marker fill; nil falls back to the DrawOption default
}

// colorOr returns the point's Color if set, or the provided fallback color otherwise.
func (p Point) colorOr(fallback color.Color) color.Color {
	if p.Color != nil {
		return p.Color
	}

	return fallback
}

type (
	// drawOptions holds the resolved settings for a DrawPoint/DrawPoints call.
	drawOptions struct {
		marginXpx, marginYpx int // fo DrawPoint only (half-size of the viewport on each side of the point, px)

		drawPointsOfInterest bool // draw the points of interest as faint watermarks?

		markerDiameterPx int         // marker diameter, px
		markerColor      color.Color // marker fill color

		topFontPt    float64 // top label size, points
		bottomFontPt float64 // bottom label size, points
		poiFontPt    float64 // POI watermark size, points

		topOffsetPx    int // distance between the point and the top label (drawn above it), px
		bottomOffsetPx int // distance between the point and the bottom label (drawn below it), px
	}

	// DrawOption is a functional option.
	DrawOption func(*drawOptions)
)

// WithMarginPixels sets the half-size of the viewport kept on each side of the point, in pixels (the output is
// 2*x by 2*y, clamped to the map). Each axis is capped at 960x540.
func WithMarginPixels(x, y int) DrawOption {
	return func(o *drawOptions) {
		o.marginXpx, o.marginYpx = min(x, 960), min(y, 540) //nolint:mnd // max = 1920x1080
	}
}

// WithPointsOfInterest toggles drawing the points of interest as faint watermarks.
func WithPointsOfInterest(draw bool) DrawOption {
	return func(o *drawOptions) { o.drawPointsOfInterest = draw }
}

// WithPointsOfInterestFontSize sets the POI watermark size, in points.
func WithPointsOfInterestFontSize(pt float64) DrawOption {
	return func(o *drawOptions) { o.poiFontPt = pt }
}

// WithMarker sets the marker diameter (px) and fill color.
func WithMarker(diameterPx int, fill color.Color) DrawOption {
	return func(o *drawOptions) { o.markerDiameterPx, o.markerColor = diameterPx, fill }
}

// WithTopLabelFontSize sets the top label size, in points.
func WithTopLabelFontSize(pt float64) DrawOption {
	return func(o *drawOptions) { o.topFontPt = pt }
}

// WithBottomLabelFontSize sets the bottom label size, in points.
func WithBottomLabelFontSize(pt float64) DrawOption {
	return func(o *drawOptions) { o.bottomFontPt = pt }
}

// WithTopLabelOffset sets the distance between the point and the top label (drawn above it), in pixels.
func WithTopLabelOffset(px int) DrawOption {
	return func(o *drawOptions) { o.topOffsetPx = px }
}

// WithBottomLabelOffset sets the distance between the point and the bottom label (drawn below it), in pixels.
func WithBottomLabelOffset(px int) DrawOption {
	return func(o *drawOptions) { o.bottomOffsetPx = px }
}

// DrawPoint renders a crop of the map centered on the point's in-game coordinates (PosX east, PosY north,
// meters) and overlays the marker with its top label above it and its bottom label below. It returns an
// in-memory canvas ready to be encoded into any format.
func DrawPoint(m maps.Map, p Point, opts ...DrawOption) (*image.RGBA, error) {
	opt := drawOptions{
		marginXpx:            640,                                            //nolint:mnd
		marginYpx:            360,                                            //nolint:mnd
		markerDiameterPx:     14,                                             //nolint:mnd
		markerColor:          color.RGBA{R: 0xEE, G: 0x22, B: 0x22, A: 0xFF}, //nolint:mnd
		topFontPt:            18,                                             //nolint:mnd
		bottomFontPt:         14,                                             //nolint:mnd
		poiFontPt:            40,                                             //nolint:mnd
		topOffsetPx:          21,                                             //nolint:mnd
		bottomOffsetPx:       19,                                             //nolint:mnd
		drawPointsOfInterest: false,
	}
	for _, o := range opts {
		o(&opt)
	}

	canvas, cErr := newMapCanvas(m, p.PosX, p.PosY, opt.marginXpx, opt.marginYpx, 1)
	if cErr != nil {
		return nil, cErr
	}

	// POI watermarks first (background) -every location that falls inside the crop
	if opt.drawPointsOfInterest {
		poiFace, err := newFontFace(goBoldFont, opt.poiFontPt)
		if err != nil {
			return nil, err
		}
		defer func() { _ = poiFace.Close() }()

		for _, l := range m.Locations() {
			if _, inView := canvas.LocToPoint(l.PosX(), l.PosY()); !inView {
				continue // not in the viewport, skip it
			}

			drawBackgroundText(canvas, l.PosX(), l.PosY(), titleCase(l.Name()), poiFace)
		}
	}

	drawPoint(canvas, p.PosX, p.PosY, opt.markerDiameterPx, p.colorOr(opt.markerColor))

	// top label above the point (negative Y), bottom label below it (positive Y)
	topFace, fErr := newFontFace(goRegularFont, opt.topFontPt)
	if fErr != nil {
		return nil, fErr
	}
	defer func() { _ = topFace.Close() }()

	bottomFace, fErr := newFontFace(goRegularFont, opt.bottomFontPt)
	if fErr != nil {
		return nil, fErr
	}
	defer func() { _ = bottomFace.Close() }()

	if p.TopLabel != "" {
		drawForegroundText(canvas, p.PosX, p.PosY, p.TopLabel, topFace, withDrawOffsetPixels(0, -opt.topOffsetPx))
	}

	if p.BottomLabel != "" {
		drawForegroundText(canvas, p.PosX, p.PosY, p.BottomLabel, bottomFace, withDrawOffsetPixels(0, opt.bottomOffsetPx))
	}

	return canvas.Image(), nil
}

// overviewMapCache pools the pristine downscaled overview (map imagery only) per map, so repeat DrawPoints
// calls skip decoding every tile. Keyed by map name - the base canvas (pixels AND projection) is map-specific,
// so one shared pool would serve one map's background for another. [sync.Pool] lets the GC reclaim it.
var overviewMapCache sync.Map //nolint:gochecknoglobals // map[string]*sync.Pool, each holding *mapCanvas

// DrawPoints renders the whole map, downscaled for a quick "where is everything" overview, and overlays every
// point's marker with its top label above it and its bottom label below.
func DrawPoints(m maps.Map, points []Point, opts ...DrawOption) (*image.RGBA, error) {
	opt := drawOptions{
		markerDiameterPx:     10,                                             //nolint:mnd
		markerColor:          color.RGBA{R: 0xEE, G: 0x22, B: 0x22, A: 0xFF}, //nolint:mnd
		topFontPt:            16,                                             //nolint:mnd
		bottomFontPt:         11,                                             //nolint:mnd
		poiFontPt:            24,                                             //nolint:mnd
		topOffsetPx:          16,                                             //nolint:mnd
		bottomOffsetPx:       16,                                             //nolint:mnd
		drawPointsOfInterest: true,
	}
	for _, o := range opts {
		o(&opt)
	}

	// fixed factor the whole map is shrunk by for the overview
	const downscale = 6

	// reuse a pooled bare overview for THIS map, or build it once (return it to the pool for the next call)
	poolAny, _ := overviewMapCache.LoadOrStore(m.Name(), &sync.Pool{})
	pool, _ := poolAny.(*sync.Pool) //nolint:errcheck

	base, _ := pool.Get().(*mapCanvas) //nolint:errcheck
	if base == nil {
		var err error
		if base, err = newMapCanvas(m,
			float64(m.MapSize())/2, float64(m.MapSize())/2, //nolint:mnd // center (clamp makes the exact value irrelevant)
			m.MapSizePX(), m.MapSizePX(), // margins >= map -> the whole map
			downscale,
		); err != nil {
			return nil, err
		}
	}
	defer pool.Put(base)

	canvas := base.Clone() // overlays go on a copy so the pooled base stays pristine

	// POI watermarks first (background)
	if opt.drawPointsOfInterest {
		poiFace, err := newFontFace(goBoldFont, opt.poiFontPt)
		if err != nil {
			return nil, err
		}
		defer func() { _ = poiFace.Close() }()

		for _, l := range m.Locations() {
			if l.Category() != "capital" && l.Category() != "city" {
				continue // skip camps, hills, ruins, etc. - too many for an overview
			}

			drawBackgroundText(canvas, l.PosX(), l.PosY(), titleCase(l.Name()), poiFace)
		}
	}

	// all markers before any label, so a neighboring marker never lands on top of a label
	for _, p := range points {
		drawPoint(canvas, p.PosX, p.PosY, opt.markerDiameterPx, p.colorOr(opt.markerColor))
	}

	// one regular face per label row, shared across every point
	topFace, err := newFontFace(goRegularFont, opt.topFontPt)
	if err != nil {
		return nil, err
	}
	defer func() { _ = topFace.Close() }()

	bottomFace, err := newFontFace(goRegularFont, opt.bottomFontPt)
	if err != nil {
		return nil, err
	}
	defer func() { _ = bottomFace.Close() }()

	for _, p := range points {
		if p.TopLabel != "" {
			drawForegroundText(canvas, p.PosX, p.PosY, p.TopLabel, topFace, withDrawOffsetPixels(0, -opt.topOffsetPx))
		}

		if p.BottomLabel != "" {
			drawForegroundText(canvas, p.PosX, p.PosY, p.BottomLabel, bottomFace, withDrawOffsetPixels(0, opt.bottomOffsetPx))
		}
	}

	return canvas.Image(), nil
}

type (
	// drawTextOptions holds the tunable knobs shared by the draw* helpers.
	drawTextOptions struct {
		// xOffsetPx, yOffsetPx shift the drawing away from its anchor point, in canvas pixels. Positive x
		// moves it right, positive y moves it down (image Y grows downward); negatives move left/up.
		xOffsetPx, yOffsetPx int
	}

	// drawTextOption customizes a draw* call.
	drawTextOption func(*drawTextOptions)
)

// withDrawOffsetPixels shifts the drawing away from its anchor point by (x, y) canvas pixels (see drawTextOptions).
func withDrawOffsetPixels(x, y int) drawTextOption { //nolint:unparam
	return func(o *drawTextOptions) { o.xOffsetPx, o.yOffsetPx = x, y }
}

var (
	goBoldFont    = mustFont(gobold.TTF)    //nolint:gochecknoglobals
	goRegularFont = mustFont(goregular.TTF) //nolint:gochecknoglobals
)

func mustFont(b []byte) *opentype.Font {
	f, err := opentype.Parse(b)
	if err != nil {
		panic(err)
	}

	return f
}

// newFontFace creates a new font face.
func newFontFace(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull}) //nolint:mnd
}

// drawBackgroundText stamps a big label centered (both axes) on the in-game point (x east, y north, meters)
// using the provided font face. An optional pixel offset (withDrawOffsetPixels) nudges it away from that point.
func drawBackgroundText(c *mapCanvas, x, y float64, text string, face font.Face, opts ...drawTextOption) {
	if text == "" {
		return
	}

	var opt drawTextOptions
	for _, o := range opts {
		o(&opt)
	}

	metrics := face.Metrics()
	boxH := (metrics.Ascent + metrics.Descent).Ceil() // rendered glyph-box height, px
	shadow := max(1, boxH/14)                         //nolint:mnd // drop-shadow thickness scales with the glyph size
	lw := font.MeasureString(face, text).Ceil() + shadow
	lh := boxH + shadow

	// render the text opaque onto a scratch layer (black shadow first, white body back over it)
	layer := image.NewRGBA(image.Rect(0, 0, lw, lh))
	base := fixed.Point26_6{X: 0, Y: metrics.Ascent}

	drawer := &font.Drawer{Dst: layer, Face: face}

	drawer.Src = image.NewUniform(color.Black)
	drawer.Dot = fixed.Point26_6{X: base.X + fixed.I(shadow), Y: base.Y + fixed.I(shadow)}
	drawer.DrawString(text)

	drawer.Src = image.NewUniform(color.White)
	drawer.Dot = base
	drawer.DrawString(text)

	// place the layer centered on the projected point (plus the optional offset) and blend it in at 20%
	center, _ := c.LocToPoint(x, y) // ignore the in-view flag (clipping handles off-canvas)
	center = center.Add(image.Pt(opt.xOffsetPx, opt.yOffsetPx))
	dstX, dstY := center.X-lw/2, center.Y-lh/2 //nolint:mnd

	draw.DrawMask(
		c.Image(),
		image.Rect(dstX, dstY, dstX+lw, dstY+lh),
		layer,
		image.Point{},
		image.NewUniform(color.Alpha{A: 60}), //nolint:mnd // `A` is opacity 0-255
		image.Point{},
		draw.Over,
	)
}

// drawForegroundText stamps a label centered (both axes) on the in-game point (x east, y north, meters)
// using the provided font face. An optional pixel offset (withDrawOffsetPixels) nudges it away from that point.
func drawForegroundText(c *mapCanvas, x, y float64, text string, face font.Face, opts ...drawTextOption) {
	if text == "" {
		return
	}

	var opt drawTextOptions
	for _, o := range opts {
		o(&opt)
	}

	metrics := face.Metrics()
	shadow := max(1, (metrics.Ascent+metrics.Descent).Ceil()/18) //nolint:mnd
	width := font.MeasureString(face, text)

	// center the glyph box on the projected point (plus the optional offset)
	center, _ := c.LocToPoint(x, y) // ignore the in-view flag (clipping handles off-canvas)
	center = center.Add(image.Pt(opt.xOffsetPx, opt.yOffsetPx))
	base := fixed.Point26_6{
		X: fixed.I(center.X) - width/2,
		Y: fixed.I(center.Y) - (metrics.Ascent+metrics.Descent)/2 + metrics.Ascent,
	}

	// opaque text draws straight onto the canvas: black shadow first, white body back over it
	drawer := &font.Drawer{Dst: c.Image(), Face: face}

	drawer.Src = image.NewUniform(color.Black)
	drawer.Dot = fixed.Point26_6{X: base.X + fixed.I(shadow), Y: base.Y + fixed.I(shadow)}
	drawer.DrawString(text)

	drawer.Src = image.NewUniform(color.White)
	drawer.Dot = base
	drawer.DrawString(text)
}

// drawPoint stamps a filled dot of the given diameter (pixels) and fill color on the in-game point (x east, y north,
// meters), ringed by a white outline. The outline sits outside the dot and its width is 10% of the diameter (at least
// 1px), so the total size is diameter + 2*outline.
func drawPoint(c *mapCanvas, x, y float64, diameter int, fill color.Color, opts ...drawTextOption) {
	var opt drawTextOptions
	for _, o := range opts {
		o(&opt)
	}

	center, _ := c.LocToPoint(x, y) // ignore the in-view flag (clipping handles off-canvas)
	center = center.Add(image.Pt(opt.xOffsetPx, opt.yOffsetPx))

	var (
		core  = float64(diameter) / 2
		outer = core + math.Max(1, float64(diameter)/10) //nolint:mnd // + white ring, 10% of the diameter (min 1px)
		img   = c.Image()
		bound = int(outer) + 1
	)

	fr, fg, fb, _ := fill.RGBA()

	// blend a 16-bit channel from white toward the fill by t, returning an 8-bit value
	toFill := func(ch uint32, t float64) float64 { return (float64(ch)*t + 0xFFFF*(1-t)) / 257 } //nolint:mnd

	for dy := -bound; dy <= bound; dy++ {
		for dx := -bound; dx <= bound; dx++ {
			d := math.Sqrt(float64(dx*dx + dy*dy))

			shape := math.Min(1, math.Max(0, outer+0.5-d)) // anti-aliased outer edge (dot vs map)
			if shape == 0 {
				continue
			}

			fillT := math.Min(1, math.Max(0, core+0.5-d)) // anti-aliased inner edge (core vs white ring)

			// color = white ring leaped to the fill across the core edge, composited over the map by shape
			px, py := center.X+dx, center.Y+dy
			bg := img.RGBAAt(px, py)
			img.SetRGBA(px, py, color.RGBA{
				R: uint8(toFill(fr, fillT)*shape + float64(bg.R)*(1-shape)),
				G: uint8(toFill(fg, fillT)*shape + float64(bg.G)*(1-shape)),
				B: uint8(toFill(fb, fillT)*shape + float64(bg.B)*(1-shape)),
				A: 0xFF, //nolint:mnd
			})
		}
	}
}

// titleCase upper-cases the first letter of each word ("stary sobor" -> "Stary Sobor").
func titleCase(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	wordStart := true

	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			wordStart = true
		case wordStart:
			r = unicode.ToUpper(r)
			wordStart = false
		}

		b.WriteRune(r)
	}

	return b.String()
}
