package statez

import (
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"

	"github.com/alexzulu/dayz-stats-tg-bot/internal/config"
)

// CachedClientPool caches one CachedClient per server (keyed by base URL + token) so repeated requests reuse the same
// client and its TTL cache. Safe for concurrent use.
type CachedClientPool struct {
	pool map[string]*CachedClient
	ttl  time.Duration
	opts []api.ClientOption
	mu   sync.Mutex
}

// NewCachedClientPool returns an empty CachedClientPool. ttl is the per-endpoint cache window applied to every
// client it creates; clientOpts are forwarded to each client.
func NewCachedClientPool(ttl time.Duration, clientOpts ...api.ClientOption) *CachedClientPool {
	return &CachedClientPool{
		pool: make(map[string]*CachedClient),
		ttl:  ttl,
		opts: clientOpts,
	}
}

// serverKey identifies a client - base URL and token together, so a changed token spins up a fresh client.
func (cp *CachedClientPool) serverKey(server config.ConfigServer) string {
	return server.BaseUrl + ":" + server.AuthToken
}

// Get returns the pooled client for the server, creating and caching one on the first call.
func (cp *CachedClientPool) Get(server config.ConfigServer) *CachedClient {
	key := cp.serverKey(server)

	cp.mu.Lock()
	defer cp.mu.Unlock()

	if client, exists := cp.pool[key]; exists {
		return client
	}

	// miss - build the client once and keep it for later requests
	client := NewCachedClient(server.BaseUrl, server.AuthToken, cp.ttl, cp.opts...)
	cp.pool[key] = client

	return client
}

// Forget drops the pooled client for the server, so its next Get rebuilds it. Call it when a server is
// removed or its credentials change.
func (cp *CachedClientPool) Forget(server config.ConfigServer) {
	cp.mu.Lock()
	delete(cp.pool, cp.serverKey(server))
	cp.mu.Unlock()
}
