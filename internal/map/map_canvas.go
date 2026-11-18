package game_map

import (
	"image"
	"image/draw"
	"math"
	"sync"
	"sync/atomic"

	xdraw "golang.org/x/image/draw"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/map/maps"
)

// mapCanvas is the rendered crop of a map plus the knowledge of how in-game coordinates land on it.
// It owns the pixels and the game<->pixel projection.
//
// The whole math, start to finish:
//   - Game coords are in METERS (origin at the map's SW corner, Y grows north / up). The image is in PIXELS (origin
//     top-left, Y grows down; different units, and Y points the opposite way)
//   - scale = pixels per meter (MapSizePX / MapSize). It is the unit bridge - multiply any meter value by scale to
//     get its pixel offset on the FULL map. Without it meters and pixels can't be compared
//   - The canvas is only a CROP of the full map, so a point's canvas pixel is its full-map pixel minus the crop's
//     corner. originX and topY are that corner, pre-computed in full-map pixels
//   - X needs no flip (both axes grow east), so it is a plain offset - `canvasX = x*scale - originX`
//     (originX = full-map pixel of the crop's LEFT edge)
//   - Y is flipped (game up vs image down), so instead of subtracting an origin we count DOWN from the top edge -
//     `canvasY = topY - y*scale` (topY = full-map pixel of the crop's TOP edge)
//
// That asymmetry is exactly why the two anchors are named differently: originX is a start we subtract
// (an offset along X), topY is a baseline we subtract FROM (the Y-flip). Both are integer pixels, so
// LocToPoint is one multiply per axis and no division.
type mapCanvas struct {
	img *image.RGBA // the rendered crop

	scale   float64 // pixels per in-game meter of the OUTPUT canvas ((MapSizePX/downscale) / MapSize)
	originX int     // full-map pixel of the crop's left edge; canvasX = x*scale - originX
	topY    int     // full-map pixel of the crop's top edge (highest game Y); canvasY = topY - y*scale
}

// newMapCanvas renders the crop of provided map, centered on the in-game point (locX east, locY north, meters) and
// returns it ready to be drawn on.
//
// The crop is a window of 2*marginXpx by 2*marginYpx pixels, near a map edge it slides fully inside the map instead
// of padding with empty space. If a margin made the window exceed the map, that axis is clamped to the map size, so
// the output can come out smaller than requested (a margin >= the map yields the whole map).
//
// downscale (>=1; 0 is treated as 1, and it may be fractional-equivalent like 6) shrinks the output: each tile is
// scaled down as it is blitted, so the full-resolution crop is never materialized, and scale is reduced to match so
// all the downstream projection math keeps working. Neighbouring tiles are placed on abutting rounded rectangles,
// so there are no gaps regardless of the factor. downscale == 1 is an exact 1:1 blit (no resampling).
func newMapCanvas(m maps.Map, locX, locY float64, marginXpx, marginYpx, downscale int) (*mapCanvas, error) {
	if downscale < 1 {
		downscale = 1
	}

	mapPX := m.MapSizePX()
	fullScale := float64(mapPX) / float64(m.MapSize())
	factor := 1.0 / float64(downscale) // output pixels per full-map pixel

	// viewport is the margin on each side, clamped so it never exceeds the map
	vw, vh := min(2*marginXpx, mapPX), min(2*marginYpx, mapPX)

	// center in whole-map pixels (Y flipped - the top pixel row is the highest game Y), then slide the window fully
	// inside the map so we never address tiles outside the grid
	cx := locX * fullScale
	cy := float64(mapPX) - locY*fullScale
	ox := int(math.Round(max(0, min(cx-float64(vw)/2, float64(mapPX-vw)))))
	oy := int(math.Round(max(0, min(cy-float64(vh)/2, float64(mapPX-vh)))))

	img := image.NewRGBA(image.Rect(0, 0, int(math.Round(float64(vw)*factor)), int(math.Round(float64(vh)*factor))))

	// blit every tile overlapping the crop, scaling it into its slot when downscale > 1. Rounding both edges of
	// each tile's destination (rather than scaling a fixed tile size) keeps neighbouring tiles abutting with no gap.
	ts := m.TileSizePX()

	var (
		wg      sync.WaitGroup
		lastErr atomic.Pointer[error]
	)

	for ty := oy / ts; ty <= (oy+vh-1)/ts; ty++ {
		wg.Go(func() {
			for tx := ox / ts; tx <= (ox+vw-1)/ts; tx++ {
				if lastErr.Load() != nil {
					return
				}

				tile, err := m.Tile(tx, ty)
				if err != nil {
					lastErr.Store(&err)

					return
				}

				sx, sy := tx*ts-ox, ty*ts-oy // tile's offset inside the crop, in full-map pixels
				dr := image.Rect(
					int(math.Round(float64(sx)*factor)), int(math.Round(float64(sy)*factor)),
					int(math.Round(float64(sx+ts)*factor)), int(math.Round(float64(sy+ts)*factor)),
				)

				if downscale == 1 {
					draw.Draw(img, dr, tile, image.Point{}, draw.Src)
				} else {
					xdraw.CatmullRom.Scale(img, dr, tile, tile.Bounds(), xdraw.Src, nil)
				}
			}
		})
	}

	wg.Wait()

	if err := lastErr.Load(); err != nil {
		return nil, *err
	}

	return &mapCanvas{
		img:     img,
		scale:   fullScale * factor,
		originX: int(math.Round(float64(ox) * factor)),
		topY:    int(math.Round(float64(mapPX-oy) * factor)), // reduced whole-map pixel of the crop's top == highest game Y
	}, nil
}

// Image returns the underlying canvas to draw on, encode, etc.
func (c *mapCanvas) Image() *image.RGBA { return c.img }

// Clone returns a fresh, drawable copy of the canvas - same projection, copied pixels.
func (c *mapCanvas) Clone() *mapCanvas {
	cp := *c
	cp.img = image.NewRGBA(c.img.Rect)
	copy(cp.img.Pix, c.img.Pix)

	return &cp
}

// LocToPoint projects an in-game position (x east, y north, meters) onto a pixel of the canvas. The point is always
// returned, even when it falls outside the image.
func (c *mapCanvas) LocToPoint(x, y float64) (image.Point, bool) {
	pt := image.Pt(
		int(math.Round(x*c.scale))-c.originX,
		c.topY-int(math.Round(y*c.scale)),
	)

	return pt, pt.In(c.img.Bounds())
}
