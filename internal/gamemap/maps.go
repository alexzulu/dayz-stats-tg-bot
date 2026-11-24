package gamemap

import (
	"fmt"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/chernarusplus"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap/maps/nasdara"
)

// MapByName returns a ready-to-use embedded game map by its identifier (e.g. "chernarusplus").
func MapByName(mapName string) (maps.Map, error) {
	switch mapName {
	case chernarusplus.Map.Name():
		return chernarusplus.Map, nil
	case nasdara.Map.Name():
		return nasdara.Map, nil
	default:
		return nil, fmt.Errorf("unsupported map %s", mapName)
	}
}
