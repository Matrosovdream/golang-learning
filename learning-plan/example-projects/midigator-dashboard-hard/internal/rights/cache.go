// Package rights caches each user's effective right-set so the auth middleware
// doesn't hit the database on every request. It mirrors the Laravel app's
// Cache::rememberForever("user:{id}:rights") with a TTL and explicit flush.
package rights

import (
	"context"
	"sync"
	"time"
)

// Loader fetches a user's right slugs from storage (the RoleRepository).
type Loader func(ctx context.Context, userID int64) ([]string, error)

type entry struct {
	rights  map[string]struct{}
	expires time.Time
}

// Cache is a process-local, TTL'd map of user id -> right set.
type Cache struct {
	mu      sync.RWMutex
	entries map[int64]entry
	load    Loader
	ttl     time.Duration
	now     func() time.Time
}

// New builds a cache backed by load, with the given TTL.
func New(load Loader, ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[int64]entry),
		load:    load,
		ttl:     ttl,
		now:     time.Now,
	}
}

// Get returns the user's right set, loading and caching it on a miss.
func (c *Cache) Get(ctx context.Context, userID int64) (map[string]struct{}, error) {
	c.mu.RLock()
	e, ok := c.entries[userID]
	c.mu.RUnlock()
	if ok && c.now().Before(e.expires) {
		return e.rights, nil
	}

	slugs, err := c.load(ctx, userID)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(slugs))
	for _, s := range slugs {
		set[s] = struct{}{}
	}

	c.mu.Lock()
	c.entries[userID] = entry{rights: set, expires: c.now().Add(c.ttl)}
	c.mu.Unlock()
	return set, nil
}

// Flush drops a user's cached rights (call after changing their roles/rights).
func (c *Cache) Flush(userID int64) {
	c.mu.Lock()
	delete(c.entries, userID)
	c.mu.Unlock()
}
