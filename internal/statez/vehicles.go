package statez

import "gh.tarampamp.am/dayz-statez/server/pkg/api"

// IsVehicleOccupied returns true if the vehicle looks claimed/in-use, false otherwise.
func IsVehicleOccupied(v api.Vehicle) bool {
	if v.Hash < 0 {
		return false
	}

	if v.State.Health >= 1 && v.State.CoolantFraction <= 0 && v.State.CargoItems == 0 {
		return false
	}

	return true
}
