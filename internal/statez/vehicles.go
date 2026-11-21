package statez

import "gh.tarampamp.am/dayz-statez/server/pkg/api"

// OccupiedHashes returns the Hash of every vehicle that looks claimed/in-use, deduplicated.
// Vehicles without a persistent id (Hash < 0) are skipped, as they cannot be tracked.
func OccupiedHashes(vehicles api.Vehicles) []int {
	var (
		out  []int
		seen = make(map[int]struct{}, len(vehicles))
	)

	for i := range vehicles {
		v := vehicles[i]

		if v.Hash < 0 {
			continue
		}

		if v.State.Health >= 1 && v.State.CoolantFraction <= 0 && v.State.CargoItems == 0 {
			continue
		}

		if _, dup := seen[v.Hash]; dup {
			continue
		}

		seen[v.Hash] = struct{}{}
		out = append(out, v.Hash)
	}

	return out
}
