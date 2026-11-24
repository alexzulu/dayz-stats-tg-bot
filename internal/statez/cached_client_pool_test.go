package statez_test

import (
	"testing"
	"time"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/testutil/assert"
)

func TestCachedClientPoolGet(t *testing.T) {
	t.Parallel()

	pool := statez.NewCachedClientPool(time.Second)
	srv := config.ConfigServer{BaseUrl: "https://host", AuthToken: "tok"}

	assert.Same(t, pool.Get(srv), pool.Get(srv)) // same key -> one shared client

	for name, other := range map[string]config.ConfigServer{
		"different token is a different client": {BaseUrl: "https://host", AuthToken: "other"},
		"different url is a different client":   {BaseUrl: "https://host2", AuthToken: "tok"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.NotSame(t, pool.Get(srv), pool.Get(other))
		})
	}
}

func TestCachedClientPoolForget(t *testing.T) {
	t.Parallel()

	pool := statez.NewCachedClientPool(time.Second)
	srv := config.ConfigServer{BaseUrl: "https://host", AuthToken: "tok"}

	first := pool.Get(srv)
	pool.Forget(srv)

	assert.NotSame(t, first, pool.Get(srv)) // Forget drops the client, next Get rebuilds
}
