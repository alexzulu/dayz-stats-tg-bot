package maps_test

import (
	"fmt"
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/map/maps"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/map/maps/chernarusplus"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestMaps(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name          string
		m             maps.Map
		wantMapMeters int
	}{
		{"chernarusplus", chernarusplus.Map, 15360},
		// {"livonia", livonia.Map, 12800},
		// {"nasdara", nasdara.Map, 16384},
		// {"sakhal", sakhal.Map, 15360},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, tt.m.Name())
			assert.Equal(t, 256, tt.m.TileSizePX())
			assert.Equal(t, 8192, tt.m.MapSizePX())
			assert.Equal(t, tt.wantMapMeters, tt.m.MapSize())
			assert.Equal(t, 31, tt.m.MaxXTileIdx())
			assert.Equal(t, 31, tt.m.MaxYTileIdx())

			tile, err := tt.m.Tile(0, 0)
			assert.Nil(t, err)
			assert.NotNil(t, tile)
			assert.Equal(t, 256, tile.Bounds().Dx())
			assert.Equal(t, 256, tile.Bounds().Dy())

			_, err = tt.m.Tile(-1, 0)
			assert.Error(t, err)

			assert.NotEmpty(t, len(tt.m.Locations()))

			for _, l := range tt.m.Locations() {
				assert.NotEmpty(t, l.Category())
				assert.NotEmpty(t, l.PosX())
				assert.NotEmpty(t, l.PosY())
				assert.NotEmpty(t, l.Name())
			}
		})
	}

	t.Run("chernarusplus InAreas", func(t *testing.T) {
		t.Parallel()

		m := chernarusplus.Map

		for _, tt := range []struct {
			giveX, giveY  float64
			wantAreaNames []maps.AreaName
		}{
			// NWAF
			// https://dayz.xam.nu/#location=4478.91;10464.84;8
			{4478.91, 10464.84, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=5388.52;9665.86;8
			{5388.52, 9665.86, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=5073.52;10288.59;8
			{5073.52, 10288.59, nil},
			// https://dayz.xam.nu/#location=4958.38;10386.91;10
			{4958.38, 10386.91, nil},
			// https://dayz.xam.nu/#location=4406.25;11817.89;8
			{4406.25, 11817.89, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=3558.05;11452.97;8
			{3558.05, 11452.97, nil},
			// https://dayz.xam.nu/#location=3568.13;11454.38;8
			{3568.13, 11454.38, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=3563.88;10918.47;8
			{3563.88, 10918.47, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=4367.96;9538.62;8
			{4367.96, 9538.62, nil},
			// https://dayz.xam.nu/#location=4382.34;9539.30;8
			{4382.34, 9539.30, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=5083.15;9010.05;8
			{5083.15, 9010.05, []maps.AreaName{chernarusplus.NWAFAreaName}},
			// https://dayz.xam.nu/#location=5084.53;8995.55;8
			{5084.53, 8995.55, nil},

			// Tisy MB
			// https://dayz.xam.nu/#location=1722.42;13906.17;7
			{1722.42, 13906.17, []maps.AreaName{chernarusplus.TMBAreaName}},
			// https://dayz.xam.nu/#location=1112.34;14565.47;7
			{1112.34, 14565.47, []maps.AreaName{chernarusplus.TMBAreaName}},
			// https://dayz.xam.nu/#location=1522.50;13584.84;7
			{1522.50, 13584.84, []maps.AreaName{chernarusplus.TMBAreaName}},
			// https://dayz.xam.nu/#location=1545.00;13491.09;7
			{1545.00, 13491.09, nil},
			// https://dayz.xam.nu/#location=2025.47;13883.91;7
			{2025.47, 13883.91, nil},
			// https://dayz.xam.nu/#location=638.44;14936.25;7
			{638.44, 14936.25, nil},

			// Kamensk MB
			// https://dayz.xam.nu/#location=8072.34;14578.13;7
			{8072.34, 14578.13, []maps.AreaName{chernarusplus.KMBAreaName}},
			// https://dayz.xam.nu/#location=7910.63;14773.59;7
			{7910.63, 14773.59, []maps.AreaName{chernarusplus.KMBAreaName}},
			// https://dayz.xam.nu/#location=7730.16;14838.28;7
			{7730.16, 14838.28, []maps.AreaName{chernarusplus.KMBAreaName}},
			// https://dayz.xam.nu/#location=7696.41;14662.03;7
			{7696.41, 14662.03, nil},
			// https://dayz.xam.nu/#location=8168.44;14618.91;7
			{8168.44, 14618.91, nil},
			// https://dayz.xam.nu/#location=7677.66;14919.84;7
			{7677.66, 14919.84, nil},
		} {
			t.Run(fmt.Sprintf("x=%.2f,y=%.2f", tt.giveX, tt.giveY), func(t *testing.T) {
				assert.DeepEqual(t, tt.wantAreaNames, m.InAreas(tt.giveX, tt.giveY))
			})
		}
	})
}
