package maps

// AreaName is the stable marker identifying an area.
type AreaName string

// Area is a named polygon on the map, in in-game meters (vertices as [x, y], same grid as live entity positions).
// The contour is open - the closing edge (last vertex -> first) is implicit.
type Area struct {
	Name    AreaName
	Polygon [][2]float64
}

// Contains reports whether the point (in-game meters, [x, y]) lies inside the area's polygon.
//
// Even-odd ray casting (PNPOLY) - the rightward ray from the point crosses an odd number of edges iff inside.
// Robust on concave and self-intersecting contours. The contour is closed implicitly via the last->first edge
// (j starts at the last vertex). The strict ">" on exactly one endpoint makes each edge's vertical span
// half-open, so a ray grazing a shared vertex is counted once, not twice.
//
// Points exactly on an edge are reported inconsistently - acceptable given the ~1-2m contour tolerance.
func (a Area) Contains(x, y float64) bool {
	if len(a.Polygon) < 3 {
		return false
	}

	in := false
	for i, j := 0, len(a.Polygon)-1; i < len(a.Polygon); j, i = i, i+1 {
		xi, yi := a.Polygon[i][0], a.Polygon[i][1]
		xj, yj := a.Polygon[j][0], a.Polygon[j][1]

		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}

	return in
}

// InAreas returns the names of every area containing the point (in-game meters). Nil if none.
// A point may fall into several areas (overlaps allowed); callers compare the result against name constants.
func InAreas(areas []Area, x, y float64) []AreaName {
	var names []AreaName
	for i := range areas {
		if areas[i].Contains(x, y) {
			names = append(names, areas[i].Name)
		}
	}

	return names
}
