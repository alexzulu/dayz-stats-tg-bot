package maps_test

import (
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestAreaContains(t *testing.T) {
	t.Parallel()

	var (
		// 10x10 square, open contour (first vertex not repeated)
		squareOpen = maps.Area{Name: "square", Polygon: [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}
		// same square, explicitly closed (first vertex repeated) - must behave identically to the open one
		squareClosed = maps.Area{Name: "square-closed", Polygon: [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}}}
		// non-square rectangle (10 wide, 4 tall) - an x<->y swap of an inside point lands outside
		rect = maps.Area{Name: "rect", Polygon: [][2]float64{{0, 0}, {10, 0}, {10, 4}, {0, 4}}}
		// concave "L" - the 4x4 box minus its top-right 2x2 quadrant (x>2 && y>2 is the notch, outside the shape)
		lShape = maps.Area{Name: "L", Polygon: [][2]float64{{0, 0}, {4, 0}, {4, 2}, {2, 2}, {2, 4}, {0, 4}}}
		// diamond with left/right vertices sharing y=0 - a horizontal ray through the center grazes both
		diamond = maps.Area{Name: "diamond", Polygon: [][2]float64{{0, 0}, {5, 5}, {10, 0}, {5, -5}}}
	)

	for _, tt := range []struct {
		name string
		area maps.Area
		x, y float64
		want bool
	}{
		{"nil polygon", maps.Area{Name: "empty"}, 1, 1, false},
		{"single vertex", maps.Area{Polygon: [][2]float64{{0, 0}}}, 0, 0, false},
		{"two vertices", maps.Area{Polygon: [][2]float64{{0, 0}, {10, 10}}}, 5, 5, false},

		{"square center inside", squareOpen, 5, 5, true},
		{"square outside right", squareOpen, 15, 5, false},
		{"square outside below", squareOpen, 5, -1, false},
		{"square far outside", squareOpen, 100, 100, false},

		{"closed square center inside", squareClosed, 5, 5, true},
		{"closed square outside", squareClosed, 15, 5, false},

		{"rect inside", rect, 8, 2, true},
		{"rect swapped coords outside", rect, 2, 8, false},

		{"L lower stem inside", lShape, 1, 1, true},
		{"L bottom arm inside", lShape, 3, 1, true},
		{"L left arm inside", lShape, 1, 3, true},
		{"L notch outside", lShape, 3, 3, false},

		{"diamond center through grazed vertices inside", diamond, 5, 0, true},
		{"diamond left of shape same y outside", diamond, -1, 0, false},
		{"diamond above apex outside", diamond, 5, 6, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.area.Contains(tt.x, tt.y))
		})
	}
}

func TestInAreas(t *testing.T) {
	t.Parallel()

	// two overlapping squares - A spans [0..10], B spans [5..15]; their overlap is [5..10] on both axes
	a := maps.Area{Name: "A", Polygon: [][2]float64{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}
	b := maps.Area{Name: "B", Polygon: [][2]float64{{5, 5}, {15, 5}, {15, 15}, {5, 15}}}
	degenerate := maps.Area{Name: "degenerate", Polygon: [][2]float64{{0, 0}, {1, 1}}}

	for _, tt := range []struct {
		name  string
		areas []maps.Area
		x, y  float64
		want  []maps.AreaName
	}{
		{"nil areas", nil, 5, 5, nil},
		{"point in none", []maps.Area{a, b}, 20, 20, nil},
		{"point in A only", []maps.Area{a, b}, 2, 2, []maps.AreaName{"A"}},
		{"point in B only", []maps.Area{a, b}, 12, 12, []maps.AreaName{"B"}},
		{"point in overlap keeps slice order", []maps.Area{a, b}, 7, 7, []maps.AreaName{"A", "B"}},
		{"degenerate area is skipped", []maps.Area{degenerate, a}, 2, 2, []maps.AreaName{"A"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.DeepEqual(t, tt.want, maps.InAreas(tt.areas, tt.x, tt.y))
		})
	}
}
