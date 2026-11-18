package game_map_test

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	gamemap "github.com/alexzulu/dayz-stats-tg-bot/internal/map"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/map/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/map/maps/chernarusplus"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/random"
)

func TestDrawPointDimensions(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		opts         []gamemap.DrawOption
		wantW, wantH int
	}{
		// defaults are 640x360 half-sizes -> 1280x720, and the map is big enough not to clamp.
		"default margins": {nil, 1280, 720},
		// 2*margin on each axis.
		"custom margins": {[]gamemap.DrawOption{gamemap.WithMarginPixels(100, 80)}, 200, 160},
		// margins past the option cap (960x540) clamp to it -> 1920x1080, still inside this map.
		"margins exceed cap": {[]gamemap.DrawOption{gamemap.WithMarginPixels(5000, 5000)}, 1920, 1080},
		// axes are independent.
		"asymmetric margins": {[]gamemap.DrawOption{gamemap.WithMarginPixels(300, 50)}, 600, 100},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			img, err := gamemap.DrawPoint(newStubMap(), gamemap.Point{PosX: stubCenter, PosY: stubCenter, TopLabel: "hero"}, tt.opts...)
			assert.NoError(t, err)
			assert.NotNil(t, img)

			assert.Equal(t, tt.wantW, img.Bounds().Dx())
			assert.Equal(t, tt.wantH, img.Bounds().Dy())
		})
	}
}

func TestDrawPointDrawsMarker(t *testing.T) {
	t.Parallel()

	// With margin (100,80) the centered point lands on pixel (100,80). The marker core is pure fill at its
	// center, so that exact pixel must come out as the fill color.
	fill := color.RGBA{R: 0x00, G: 0x00, B: 0xFF, A: 0xFF}

	img, err := gamemap.DrawPoint(newStubMap(), gamemap.Point{PosX: stubCenter, PosY: stubCenter},
		gamemap.WithMarginPixels(100, 80),
		gamemap.WithMarker(40, fill),
	)
	assert.NoError(t, err)
	assert.NotNil(t, img)

	assert.Equal(t, fill, img.RGBAAt(100, 80))
}

func TestDrawPointPointsOfInterest(t *testing.T) {
	t.Parallel()

	inView := stubLoc{name: "Town", x: 1000, y: 1050} // inside the (100,80) crop around the center
	outOfView := stubLoc{name: "Far", x: 100, y: 100} // nowhere near the crop

	for name, tt := range map[string]struct {
		locs      []maps.Loc
		wantEqual bool // true => toggling the POI watermark must NOT change the output
	}{
		"in-view POI changes output": {[]maps.Loc{inView}, false},
		"out-of-view POI is culled":  {[]maps.Loc{outOfView}, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := newStubMap()
			m.locations = tt.locs

			base := gamemap.WithMarginPixels(100, 80)
			p := gamemap.Point{PosX: stubCenter, PosY: stubCenter, TopLabel: "hero"}

			on, err := gamemap.DrawPoint(m, p, base, gamemap.WithPointsOfInterest(true))
			assert.NoError(t, err)

			off, err := gamemap.DrawPoint(m, p, base, gamemap.WithPointsOfInterest(false))
			assert.NoError(t, err)

			assert.Equal(t, tt.wantEqual, bytes.Equal(on.Pix, off.Pix))
		})
	}
}

func TestDrawPointLabels(t *testing.T) {
	t.Parallel()

	base := gamemap.WithMarginPixels(100, 80)
	center := gamemap.Point{PosX: stubCenter, PosY: stubCenter}

	// no labels is a no-op (not an error), and both labels are optional and independent
	blank, err := gamemap.DrawPoint(newStubMap(), center, base)
	assert.NoError(t, err)
	assert.NotNil(t, blank)

	withTop := center
	withTop.TopLabel = "hero"
	top, err := gamemap.DrawPoint(newStubMap(), withTop, base)
	assert.NoError(t, err)
	assert.False(t, bytes.Equal(blank.Pix, top.Pix)) // a top label leaves a visible mark

	withBottom := center
	withBottom.BottomLabel = "[1, 2]"
	bottom, err := gamemap.DrawPoint(newStubMap(), withBottom, base)
	assert.NoError(t, err)
	assert.False(t, bytes.Equal(blank.Pix, bottom.Pix)) // a bottom label leaves a visible mark
}

func TestDrawPointTileError(t *testing.T) {
	t.Parallel()

	m := newStubMap()
	m.tileErr = errTileBoom

	img, err := gamemap.DrawPoint(m, gamemap.Point{PosX: stubCenter, PosY: stubCenter, TopLabel: "hero"}, gamemap.WithMarginPixels(100, 80))

	assert.ErrorIs(t, err, errTileBoom)
	assert.Nil(t, img)
}

func TestDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping demo render in short mode")
	}

	t.Parallel()

	outDir := filepath.Join("testdata", "demo")

	assert.NoError(t, os.RemoveAll(outDir)) // fresh run, drop previous renders

	allOpts := []gamemap.DrawOption{
		gamemap.WithMarginPixels(700, 500),
		gamemap.WithPointsOfInterest(true),
		gamemap.WithPointsOfInterestFontSize(30),
		gamemap.WithMarker(24, color.RGBA{R: 0x22, G: 0x88, B: 0xEE, A: 0xFF}),
		gamemap.WithTopLabelFontSize(22),
		gamemap.WithBottomLabelFontSize(16),
		gamemap.WithTopLabelOffset(18),
		gamemap.WithBottomLabelOffset(16),
	}

	// save writes img as a PNG under ./testdata/demo/<map>/ and checks it is a non-empty, cleanly-encoded image.
	save := func(t *testing.T, m maps.Map, name string, img *image.RGBA, err error) {
		t.Helper()
		assert.NoError(t, err)
		assert.NotNil(t, img)
		assert.True(t, img.Bounds().Dx() > 0 && img.Bounds().Dy() > 0)

		dir := filepath.Join(outDir, m.Name())
		assert.NoError(t, os.MkdirAll(dir, 0o755))

		f, cErr := os.Create(filepath.Join(dir, name+".png"))
		assert.NoError(t, cErr)
		defer func() { assert.NoError(t, f.Close()) }()
		assert.NoError(t, png.Encode(f, img))
	}

	// points scatters n annotated points (random top label, coordinates as the bottom label) across the map.
	points := func(m maps.Map, n int) []gamemap.Point {
		ps := make([]gamemap.Point, n)
		for i := range ps {
			x, y := rand.Float64()*float64(m.MapSize()), rand.Float64()*float64(m.MapSize())
			ps[i] = gamemap.Point{
				PosX:        x,
				PosY:        y,
				TopLabel:    random.String(8), //nolint:mnd
				BottomLabel: fmt.Sprintf("[%d, %d]", int(x), int(y)),
			}
		}

		return ps
	}

	for _, m := range []maps.Map{chernarusplus.Map} {
		t.Run("DrawPoint", func(t *testing.T) {
			center := float64(m.MapSize()) / 2
			p := gamemap.Point{
				PosX:        center,
				PosY:        center,
				TopLabel:    random.String(8), //nolint:mnd
				BottomLabel: fmt.Sprintf("[%d, %d]", int(center), int(center)),
			}

			for name, opts := range map[string][]gamemap.DrawOption{
				"default":     nil,
				"all_options": allOpts,
			} {
				img, err := gamemap.DrawPoint(m, p, opts...)
				save(t, m, "point_"+name, img, err)
			}
		})

		t.Run("DrawPoints", func(t *testing.T) {
			for name, tt := range map[string]struct {
				count int
				opts  []gamemap.DrawOption
			}{
				"default_1":      {1, nil},
				"default_10":     {10, nil},
				"default_30":     {30, nil},
				"default_60":     {60, nil},
				"all_options_1":  {1, allOpts},
				"all_options_10": {10, allOpts},
				"all_options_30": {30, allOpts},
				"all_options_60": {60, allOpts},
				"empty":          {0, nil}, // edge: no points -> bare overview
			} {
				img, err := gamemap.DrawPoints(m, points(m, tt.count), tt.opts...)
				save(t, m, "points_"+name, img, err)
			}
		})
	}
}

func BenchmarkDrawPoint(b *testing.B) {
	m := chernarusplus.Map
	center := float64(m.MapSize()) / 2

	b.ReportAllocs()

	for b.Loop() {
		if _, err := gamemap.DrawPoint(m, gamemap.Point{PosX: center, PosY: center, TopLabel: "survivor"}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDrawPoints(b *testing.B) {
	m := chernarusplus.Map

	points := make([]gamemap.Point, 30)
	for i := range points {
		points[i] = gamemap.Point{
			PosX:     rand.Float64() * float64(m.MapSize()),
			PosY:     rand.Float64() * float64(m.MapSize()),
			TopLabel: "survivor_" + strconv.Itoa(i),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := gamemap.DrawPoints(m, points); err != nil {
			b.Fatal(err)
		}
	}
}

// --------------------------------------------------------------------------------------------------------------------

// The stub map is a flat 2048px / 2048m square (scale 1) of uniform gray tiles. Scale 1 keeps the math trivial -
// a point at (1024,1024) with margin (mx,my) and far from every edge projects to pixel (mx,my).
const stubCenter = 1024.0 // point position used across the tests (map center)

var errTileBoom = errors.New("tile boom")

// stubLoc is a fixed point of interest.
type stubLoc struct {
	name string
	x, y float64
}

func (l stubLoc) Category() string { return "city" }
func (l stubLoc) Name() string     { return l.name }
func (l stubLoc) PosX() float64    { return l.x }
func (l stubLoc) PosY() float64    { return l.y }

// stubMap is a test double for maps.Map - a flat square of uniform gray tiles, with optional points of interest
// and an optional tile error for the failure path.
type stubMap struct {
	tileSizePX, mapSizePX, mapSize int
	locations                      []maps.Loc
	tileErr                        error // if set, every Tile call fails with it
}

func newStubMap() *stubMap {
	return &stubMap{tileSizePX: 64, mapSizePX: 2048, mapSize: 2048}
}

func (m *stubMap) Name() string                         { return "stub" }
func (m *stubMap) TileSizePX() int                      { return m.tileSizePX }
func (m *stubMap) MapSizePX() int                       { return m.mapSizePX }
func (m *stubMap) MapSize() int                         { return m.mapSize }
func (m *stubMap) MaxXTileIdx() int                     { return m.mapSizePX/m.tileSizePX - 1 }
func (m *stubMap) MaxYTileIdx() int                     { return m.mapSizePX/m.tileSizePX - 1 }
func (m *stubMap) Locations() []maps.Loc                { return m.locations }
func (m *stubMap) InAreas(_, _ float64) []maps.AreaName { return nil }

func (m *stubMap) Tile(_, _ int) (image.Image, error) {
	if m.tileErr != nil {
		return nil, m.tileErr
	}

	tile := image.NewRGBA(image.Rect(0, 0, m.tileSizePX, m.tileSizePX))
	draw.Draw(tile, tile.Bounds(), image.NewUniform(color.RGBA{R: 40, G: 40, B: 40, A: 0xFF}), image.Point{}, draw.Src)

	return tile, nil
}
