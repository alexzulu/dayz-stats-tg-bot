package statez

import (
	"context"
	"sync"
	"time"

	"gh.tarampamp.am/dayz-statez/server/pkg/api"
)

type (
	// CachedClient is an [api.Client] with a TTL cache per endpoint. Safe for concurrent use.
	CachedClient struct {
		c   *api.Client
		ttl time.Duration

		cache struct {
			animals     cacheSlot[api.Animals]
			areas       cacheSlot[api.Areas]
			events      cacheSlot[api.Events]
			hits        cacheSlot[api.Hits]
			players     cacheSlot[api.Players]
			territories cacheSlot[api.Territories]
			vehicles    cacheSlot[api.Vehicles]
			weapons     cacheSlot[api.Weapons]
			world       cacheSlot[api.World]
			zombies     cacheSlot[api.Zombies]
			status      cacheSlot[api.Status]
		}
	}

	// cacheSlot holds the last value fetched for one endpoint and the time it was stored.
	cacheSlot[T any] struct {
		mu   sync.Mutex
		data *T
		at   time.Time
	}
)

// Get returns the cached value, or nil if the slot is empty or older than ttl.
func (cs *cacheSlot[T]) Get(ttl time.Duration) *T {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.data == nil || time.Since(cs.at) > ttl {
		cs.data, cs.at = nil, time.Time{} // drop the stale value so it can be collected

		return nil
	}

	return cs.data
}

// Set stores v as the current value, stamped with the current time.
func (cs *cacheSlot[T]) Set(v *T) {
	cs.mu.Lock()
	cs.data, cs.at = v, time.Now()
	cs.mu.Unlock()
}

// NewCachedClient builds a client against baseURL authenticated with authToken. A non-positive cacheTTL falls back
// to defaultCacheTTL.
func NewCachedClient(baseURL, authToken string, cacheTTL time.Duration) *CachedClient {
	const defaultCacheTTL = 2 * time.Second

	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}

	return &CachedClient{
		c:   NewClient(baseURL, authToken),
		ttl: cacheTTL,
	}
}

// Animals returns the live wildlife census.
func (c *CachedClient) Animals(ctx context.Context) (*api.Animals, error) {
	if v := c.cache.animals.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Animals(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.animals.Set(data)

	return data, nil
}

// Areas returns the active effect areas.
func (c *CachedClient) Areas(ctx context.Context) (*api.Areas, error) {
	if v := c.cache.areas.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Areas(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.areas.Set(data)

	return data, nil
}

// Events returns the cumulative engine event counters.
func (c *CachedClient) Events(ctx context.Context) (*api.Events, error) {
	if v := c.cache.events.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Events(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.events.Set(data)

	return data, nil
}

// Hits returns the cumulative qualifying hit counts.
func (c *CachedClient) Hits(ctx context.Context) (*api.Hits, error) {
	if v := c.cache.hits.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Hits(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.hits.Set(data)

	return data, nil
}

// Players returns the snapshot of every online player.
func (c *CachedClient) Players(ctx context.Context) (*api.Players, error) {
	if v := c.cache.players.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Players(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.players.Set(data)

	return data, nil
}

// Territories returns the base territory flags.
func (c *CachedClient) Territories(ctx context.Context) (*api.Territories, error) {
	if v := c.cache.territories.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Territories(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.territories.Set(data)

	return data, nil
}

// Vehicles returns every tracked vehicle.
func (c *CachedClient) Vehicles(ctx context.Context) (*api.Vehicles, error) {
	if v := c.cache.vehicles.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Vehicles(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.vehicles.Set(data)

	return data, nil
}

// Weapons returns the weapon usage statistics.
func (c *CachedClient) Weapons(ctx context.Context) (*api.Weapons, error) {
	if v := c.cache.weapons.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Weapons(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.weapons.Set(data)

	return data, nil
}

// World returns the current world snapshot.
func (c *CachedClient) World(ctx context.Context) (*api.World, error) {
	if v := c.cache.world.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.World(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.world.Set(data)

	return data, nil
}

// Zombies returns the live infected census.
func (c *CachedClient) Zombies(ctx context.Context) (*api.Zombies, error) {
	if v := c.cache.zombies.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Zombies(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.zombies.Set(data)

	return data, nil
}

// Status returns the public server/world status summary.
func (c *CachedClient) Status(ctx context.Context) (*api.Status, error) {
	if v := c.cache.status.Get(c.ttl); v != nil {
		return v, nil
	}

	data, err := c.c.Status(ctx)
	if err != nil {
		return nil, err
	}

	c.cache.status.Set(data)

	return data, nil
}
