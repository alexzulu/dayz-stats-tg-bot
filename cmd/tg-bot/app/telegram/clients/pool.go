package clients

import (
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

type Pool struct {
	pool map[string]*statez.CachedClient
	ttl  time.Duration
	opts []api.ClientOption
	mu   sync.Mutex
}

func NewPool(ttl time.Duration, clientOpts ...api.ClientOption) *Pool {
	return &Pool{
		pool: make(map[string]*statez.CachedClient),
		ttl:  ttl,
		opts: clientOpts,
	}
}

func (cp *Pool) serverKey(server config.ConfigServer) string {
	return server.BaseUrl + ":" + server.AuthToken
}

func (cp *Pool) Get(server config.ConfigServer) *statez.CachedClient {
	key := cp.serverKey(server)

	cp.mu.Lock()
	defer cp.mu.Unlock()

	if client, exists := cp.pool[key]; exists {
		return client
	}

	client := statez.NewCachedClient(server.BaseUrl, server.AuthToken, cp.ttl, cp.opts...)
	cp.pool[key] = client

	return client
}

func (cp *Pool) Forget(server config.ConfigServer) {
	cp.mu.Lock()
	delete(cp.pool, cp.serverKey(server))
	cp.mu.Unlock()
}
