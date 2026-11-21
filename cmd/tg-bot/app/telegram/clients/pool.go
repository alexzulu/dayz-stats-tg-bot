package clients

import (
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
	"github.com/alexzulu/dayz-stats-tg-bot/internal/statez"
)

// Pool caches one CachedClient per server (keyed by base URL + token) so repeated requests reuse the same
// client and its TTL cache. Safe for concurrent use.
type Pool struct {
	pool map[string]*statez.CachedClient
	ttl  time.Duration
	opts []api.ClientOption
	mu   sync.Mutex
}

// NewPool returns an empty Pool. ttl is the per-endpoint cache window applied to every client it creates (0
// means the client's own default); clientOpts are forwarded to each client.
func NewPool(ttl time.Duration, clientOpts ...api.ClientOption) *Pool {
	return &Pool{
		pool: make(map[string]*statez.CachedClient),
		ttl:  ttl,
		opts: clientOpts,
	}
}

// serverKey identifies a client - base URL and token together, so a changed token spins up a fresh client.
func (cp *Pool) serverKey(server config.ConfigServer) string {
	return server.BaseUrl + ":" + server.AuthToken
}

// Get returns the pooled client for the server, creating and caching one on the first call.
func (cp *Pool) Get(server config.ConfigServer) *statez.CachedClient {
	key := cp.serverKey(server)

	cp.mu.Lock()
	defer cp.mu.Unlock()

	if client, exists := cp.pool[key]; exists {
		return client
	}

	// miss - build the client once and keep it for later requests
	client := statez.NewCachedClient(server.BaseUrl, server.AuthToken, cp.ttl, cp.opts...)
	cp.pool[key] = client

	return client
}

// Forget drops the pooled client for the server, so its next Get rebuilds it. Call it when a server is
// removed or its credentials change.
func (cp *Pool) Forget(server config.ConfigServer) {
	cp.mu.Lock()
	delete(cp.pool, cp.serverKey(server))
	cp.mu.Unlock()
}
