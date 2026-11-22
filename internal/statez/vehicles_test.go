package statez_test

import (
	"testing"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

// veh builds a single api.Vehicle with the fields IsVehicleOccupied inspects (nested anonymous structs make a
// composite literal impractical, so fields are set by assignment).
func veh(hash int, health, speed, coolant float64, cargo int) api.Vehicle {
	var v api.Vehicle

	v.Hash = hash
	v.State.Health = health
	v.State.SpeedMS = speed
	v.State.CoolantFraction = coolant
	v.State.CargoItems = cargo

	return v
}

func TestIsVehicleOccupied(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in   api.Vehicle
		want bool
	}{
		"pristine car is free":            {in: veh(1, 1.0, 0, 0, 0), want: false},
		"pristine boat (coolant -1) free": {in: veh(2, 1.0, 0, -1, 0), want: false},
		"slightly damaged is occupied":    {in: veh(3, 0.999, 0, 0, 0), want: true},
		"moving but pristine is free":     {in: veh(4, 1.0, 16, -1, 0), want: false}, // speed is ignored
		"coolant added is occupied":       {in: veh(5, 1.0, 0, 0.5, 0), want: true},
		"cargo present is occupied":       {in: veh(6, 1.0, 0, 0, 3), want: true},
		"hash -1 is free":                 {in: veh(-1, 0.5, 5, 0.5, 9), want: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, statez.IsVehicleOccupied(tc.in))
		})
	}
}
