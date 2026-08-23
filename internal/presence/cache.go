package presence

import (
	"context"
	"sync"
	"time"
)

// Cache keeps short-lived presence results in memory. Presence is deliberately
// ephemeral: the device registry and audit history belong in the repository,
// while a probe result only prevents duplicate network work for a short time.
type Cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	entries  map[string]cacheEntry
	inFlight map[string]*cacheCall
}

type cacheEntry struct {
	result    Result
	expiresAt time.Time
}

type cacheCall struct {
	done   chan struct{}
	result Result
	err    error
}

func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Cache{
		ttl:      ttl,
		entries:  make(map[string]cacheEntry),
		inFlight: make(map[string]*cacheCall),
	}
}

func (c *Cache) TTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.ttl
}

// Get returns a fresh result. force bypasses the cache but does not cancel a
// probe already running for the same target; concurrent callers still share
// one network operation.
func (c *Cache) Get(key string, force bool) (Result, bool) {
	if c == nil || key == "" || force {
		return Result{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok || !time.Now().Before(entry.expiresAt) {
		if ok {
			delete(c.entries, key)
		}
		return Result{}, false
	}
	result := entry.result
	result.Cached = true
	result.ExpiresAt = entry.expiresAt.UTC().Format(time.RFC3339Nano)
	return result, true
}

func (c *Cache) Set(key string, result Result) Result {
	if c == nil || key == "" {
		return result
	}
	expiresAt := time.Now().Add(c.ttl)
	result.Cached = false
	result.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	c.mu.Lock()
	c.entries[key] = cacheEntry{result: result, expiresAt: expiresAt}
	c.mu.Unlock()
	return result
}

func (c *Cache) Invalidate(key string) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Resolve returns a fresh cached result or runs probe once for all concurrent
// callers requesting the same key.
func (c *Cache) Resolve(ctx context.Context, key string, force bool, probe func(context.Context) Result) (Result, error) {
	if result, ok := c.Get(key, force); ok {
		return result, nil
	}

	c.mu.Lock()
	if call, ok := c.inFlight[key]; ok {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.result, call.err
		case <-ctx.Done():
			return Result{}, ctx.Err()
		}
	}
	call := &cacheCall{done: make(chan struct{})}
	c.inFlight[key] = call
	c.mu.Unlock()

	result := probe(ctx)
	result = c.Set(key, result)

	c.mu.Lock()
	call.result = result
	call.err = nil
	delete(c.inFlight, key)
	close(call.done)
	c.mu.Unlock()
	return result, nil
}
