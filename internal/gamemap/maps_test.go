package gamemap_test

import (
	"testing"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/gamemap"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestMapByName(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		mapName string
		wantErr bool
	}{
		"chernarusplus is supported": {mapName: "chernarusplus"},
		"nasdara is supported":       {mapName: "nasdara"},
		"unknown map is an error":    {mapName: "narnia", wantErr: true},
		"empty name is an error":     {mapName: "", wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m, err := gamemap.MapByName(tt.mapName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, m)

				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, m)
			assert.Equal(t, tt.mapName, m.Name()) // resolved map reports the requested name
		})
	}
}
