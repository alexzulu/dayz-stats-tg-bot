package vehicles

import (
	"image/color"
	"strings"
)

// catalog maps a base vehicle class to its presentation, covering the vanilla DayZ set.
//
//nolint:mnd,lll
var catalog = map[string]struct { //nolint:gochecknoglobals
	name string
	fill color.Color
}{
	"CivilianSedan":    {"Olga 24", color.RGBA{R: 0xE0, G: 0x30, B: 0x30, A: 0xFF}},   // CivilianSedan, CivilianSedan_Black, CivilianSedan_Wine
	"Hatchback_02":     {"Gunter 2", color.RGBA{R: 0xE8, G: 0x80, B: 0x20, A: 0xFF}},  // Hatchback_02, Hatchback_02_Black, Hatchback_02_Blue
	"Sedan_02":         {"Sarka 120", color.RGBA{R: 0xF0, G: 0xD0, B: 0x20, A: 0xFF}}, // Sedan_02, Sedan_02_Grey, Sedan_02_Red
	"Offroad_02":       {"M1025", color.RGBA{R: 0x30, G: 0xB0, B: 0x40, A: 0xFF}},     // Offroad_02, Offroad_02_Beige
	"OffroadHatchback": {"Ada 4x4", color.RGBA{R: 0x30, G: 0x70, B: 0xE0, A: 0xFF}},   // OffroadHatchback, OffroadHatchback_Blue, OffroadHatchback_White
	"Truck_01_Covered": {"M3S", color.RGBA{R: 0x90, G: 0x40, B: 0xC0, A: 0xFF}},       // Truck_01_Covered, Truck_01_Covered_Blue, Truck_01_Covered_Orange
	"Boat_01":          {"Boat", color.RGBA{R: 0x8B, G: 0x5A, B: 0x2B, A: 0xFF}},      // Boat_01_Black, Boat_01_Blue, Boat_01_Orange
	"Motorbike_01":     {"Moped", color.RGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}},     // Motorbike_01, Motorbike_01_Blue
	"Motorbike_02":     {"Enduro", color.RGBA{R: 0xF0, G: 0xF0, B: 0xF0, A: 0xFF}},    // Motorbike_02, Motorbike_02_Blue
}

// friendlyNameAndColor returns a human-friendly name and color for a vehicle class, if known. If the class is not
// in the catalog, it returns the original class name, nil color, and false.
func friendlyNameAndColor(className string) (string, color.Color, bool) {
	for base, v := range catalog {
		if strings.HasPrefix(className, base) {
			return v.name, v.fill, true
		}
	}

	return className, nil, false
}
