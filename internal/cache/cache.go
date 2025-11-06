package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

type item struct {
	value      any
	expiration int64
}

// Cache provides a lightweight in-memory cache with TTL support.
type Cache struct {
	mu              sync.RWMutex
	items           map[string]item
	cleanupInterval time.Duration
	stopCh          chan struct{}
	stopped         atomic.Bool
}

// New creates a new cache instance. cleanupInterval defines how often the cache
// removes expired entries. A zero cleanupInterval disables background cleanup.
func New(cleanupInterval time.Duration) *Cache {
	c := &Cache{
		items:           make(map[string]item),
		cleanupInterval: cleanupInterval,
		stopCh:          make(chan struct{}),
	}

	if cleanupInterval > 0 {
		go c.cleanupLoop()
	}

	return c
}

// Set stores a value for the given key until the TTL expires. A non-positive TTL keeps the entry indefinitely.
func (c *Cache) Set(key string, value any, ttl time.Duration) {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = item{
		value:      value,
		expiration: expiresAt,
	}
}

// Get returns the stored value for the key if it exists and has not expired.
func (c *Cache) Get(key string) (any, bool) {
	c.mu.RLock()
	it, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if it.expiration > 0 && time.Now().UnixNano() > it.expiration {
		c.Delete(key)
		return nil, false
	}

	return it.value, true
}

// Delete removes the key from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Close stops the cleanup goroutine and clears cache entries.
func (c *Cache) Close() {
	if c.stopped.CompareAndSwap(false, true) {
		close(c.stopCh)
		c.mu.Lock()
		c.items = make(map[string]item)
		c.mu.Unlock()
	}
}

func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.removeExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cache) removeExpired() {
	now := time.Now().UnixNano()

	c.mu.Lock()
	for key, it := range c.items {
		if it.expiration > 0 && now > it.expiration {
			delete(c.items, key)
		}
	}
	c.mu.Unlock()
}
