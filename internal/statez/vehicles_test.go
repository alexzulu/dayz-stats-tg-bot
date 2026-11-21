package statez_test

import (
	"testing"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

// veh builds a one-element api.Vehicles with the fields OccupiedHashes inspects (nested anonymous structs make
// a composite literal impractical, so fields are set by index).
func veh(hash int, health, speed, coolant float64, cargo int) api.Vehicles {
	v := make(api.Vehicles, 1)
	v[0].Hash = hash
	v[0].State.Health = health
	v[0].State.SpeedMS = speed
	v[0].State.CoolantFraction = coolant
	v[0].State.CargoItems = cargo

	return v
}

func concat(parts ...api.Vehicles) api.Vehicles {
	var out api.Vehicles
	for _, p := range parts {
		out = append(out, p...)
	}

	return out
}

func TestOccupiedHashes(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in   api.Vehicles
		want []int
	}{
		"pristine car is free":             {in: veh(1, 1.0, 0, 0, 0), want: nil},
		"pristine boat (coolant -1) free":  {in: veh(2, 1.0, 0, -1, 0), want: nil},
		"slightly damaged is occupied":     {in: veh(3, 0.999, 0, 0, 0), want: []int{3}},
		"moving but pristine is free":      {in: veh(4, 1.0, 16, -1, 0), want: nil}, // speed is ignored
		"coolant added is occupied":        {in: veh(5, 1.0, 0, 0.5, 0), want: []int{5}},
		"cargo present is occupied":        {in: veh(6, 1.0, 0, 0, 3), want: []int{6}},
		"hash -1 is skipped":               {in: veh(-1, 0.5, 5, 0.5, 9), want: nil},
		"duplicate hash is deduplicated":   {in: concat(veh(7, 0.5, 0, 0, 0), veh(7, 0.5, 0, 0, 0)), want: []int{7}},
		"mix keeps only occupied in order": {in: concat(veh(1, 1, 0, 0, 0), veh(8, 0.8, 0, 0, 0), veh(9, 1, 0, 0, 2)), want: []int{8, 9}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.DeepEqual(t, tc.want, statez.OccupiedHashes(tc.in))
		})
	}
}
