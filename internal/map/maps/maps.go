package maps

import "image"

// Map is the read-only view of a single embedded game map - metadata, image tiles and points of interest.
// The map is a square grid of square tiles; it owns its on-disk format and hands back decoded images.
//
// Mind the two coordinate systems when placing entities on the map image. Players, vehicles and locations all
// arrive in the first one (in-game meters); the image you draw on uses the second.
//
// In-game meters - origin at the south-west corner, Y points north (up):
//
//	  Y
//	  ^
//	  |
//	  |
//	--+-----> X
//	0 |
//
// Image canvas (tiles/output) - origin in the top-left corner, Y points down:
//
//	0 |
//	--+-----> X
//	  |
//	  |
//	  v
//	  Y
//
// X is identical, only Y is inverted. Convert an in-game point (gx, gy) to a pixel once, the same way for every
// entity (no per-source special-casing):
//
//	px = gx / MapSize() * MapSizePX()
//	py = (MapSize() - gy) / MapSize() * MapSizePX()
type Map interface {
	// Name is the map's identifier (e.g. "chernarusplus").
	Name() string

	// TileSizePX is the side length of one (square) tile, in pixels.
	TileSizePX() int

	// MapSizePX is the side length of the whole (square) map, in pixels.
	MapSizePX() int

	// MapSize is the side length of the whole (square) map, in in-game meters.
	MapSize() int

	// MaxXTileIdx is the highest valid column index (0-based, inclusive).
	MaxXTileIdx() int

	// MaxYTileIdx is the highest valid row index (0-based, inclusive).
	MaxYTileIdx() int

	// Tile decodes and returns the image tile at the given column/row indices. The map owns its storage
	// format, so callers get a ready-to-use image. Out-of-range indices return an error.
	Tile(xIdx, yIdx int) (image.Image, error)

	// Locations returns the well-known in-game points of interest on this map (towns, camps, etc.).
	Locations() []Loc

	// InAreas returns the names of every area containing the point (in-game meters). Nil if none.
	InAreas(x, y float64) []AreaName
}

// Loc is a well-known point of interest on the map (town, camp, etc.).
type Loc interface {
	// Category is the kind of place (capital, city, village, local, camp, hill, ruin, marine, etc.).
	Category() string

	// Name is the display name.
	Name() string

	// PosX is the east-west position, in in-game meters (grows east).
	PosX() float64

	// PosY is the north-south position, in in-game meters (grows north).
	PosY() float64
}
