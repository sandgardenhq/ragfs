package ragfs

import (
	"container/list"
	"io/fs"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// NoTTL is a special value for CacheConfig.TTL that disables TTL-based expiration.
	// Entries will never expire based on time and will only be evicted when MaxEntries is reached.
	// Use this for data that never changes or when you want to manually control cache invalidation.
	NoTTL time.Duration = -1
)

// CacheConfig configures the caching behavior for a ragfs filesystem.
// Caching is opt-in and disabled by default.
type CacheConfig struct {
	// MaxEntries is the maximum number of entries per cache layer.
	// When the limit is reached, least recently used entries are evicted.
	// 0 means unlimited (no eviction based on count).
	MaxEntries int

	// TTL is the time-to-live for cached entries.
	// Entries older than TTL are considered expired and will be refetched.
	// Special values:
	//   0: Default TTL (30 seconds) is used
	//   NoTTL (-1): TTL is disabled, entries never expire based on time
	//   > 0: Custom TTL duration
	TTL time.Duration
}

// CacheStats provides statistics about cache performance.
// All fields use atomic operations for thread-safe updates.
type CacheStats struct {
	// Hits is the number of successful cache lookups.
	Hits atomic.Int64

	// Misses is the number of cache lookups that required fetching data.
	Misses atomic.Int64

	// Evictions is the number of entries evicted due to capacity limits.
	Evictions atomic.Int64

	// Entries is the current number of entries in both cache layers.
	Entries atomic.Int64
}

// Cache manages a two-layer LRU cache for ragfs.
// Layer 1 caches handler results ([]fs.DirEntry).
// Layer 2 caches file content ([]byte).
type Cache struct {
	handlerCache *lruCache
	contentCache *lruCache
	stats        *CacheStats
	mu           sync.RWMutex
}

// newCache creates a new cache with the given configuration.
func newCache(config CacheConfig) *Cache {
	return &Cache{
		handlerCache: newLRUCache(config.MaxEntries, config.TTL),
		contentCache: newLRUCache(config.MaxEntries, config.TTL),
		stats:        &CacheStats{},
	}
}

// getHandler retrieves cached handler results for a path.
// Returns nil if not found or expired.
func (c *Cache) getHandler(path string) []fs.DirEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if val := c.handlerCache.get(path); val != nil {
		c.stats.Hits.Add(1)
		return val.([]fs.DirEntry)
	}

	c.stats.Misses.Add(1)
	return nil
}

// setHandler stores handler results in the cache.
func (c *Cache) setHandler(path string, entries []fs.DirEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	evicted := c.handlerCache.set(path, entries)
	if evicted {
		c.stats.Evictions.Add(1)
		c.stats.Entries.Add(-1)
	} else {
		c.stats.Entries.Add(1)
	}
}

// getContent retrieves cached file content for a path.
// Returns nil if not found or expired.
func (c *Cache) getContent(path string) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if val := c.contentCache.get(path); val != nil {
		c.stats.Hits.Add(1)
		return val.([]byte)
	}

	c.stats.Misses.Add(1)
	return nil
}

// setContent stores file content in the cache.
func (c *Cache) setContent(path string, content []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	evicted := c.contentCache.set(path, content)
	if evicted {
		c.stats.Evictions.Add(1)
		c.stats.Entries.Add(-1)
	} else {
		c.stats.Entries.Add(1)
	}
}

// invalidate removes a specific path from both cache layers.
func (c *Cache) invalidate(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.handlerCache.delete(path) {
		c.stats.Entries.Add(-1)
	}
	if c.contentCache.delete(path) {
		c.stats.Entries.Add(-1)
	}
}

// invalidatePrefix removes all paths starting with the given prefix from both cache layers.
func (c *Cache) invalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	handlerCount := c.handlerCache.deletePrefix(prefix)
	contentCount := c.contentCache.deletePrefix(prefix)
	c.stats.Entries.Add(-int64(handlerCount + contentCount))
}

// lruCache is a thread-unsafe LRU cache implementation.
// Callers must handle synchronization.
type lruCache struct {
	maxEntries int
	ttl        time.Duration
	entries    map[string]*lruEntry
	lruList    *list.List
}

// lruEntry represents a cached entry with metadata.
type lruEntry struct {
	key       string
	value     interface{}
	timestamp time.Time
	element   *list.Element
}

// newLRUCache creates a new LRU cache.
func newLRUCache(maxEntries int, ttl time.Duration) *lruCache {
	return &lruCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		entries:    make(map[string]*lruEntry),
		lruList:    list.New(),
	}
}

// get retrieves a value from the cache.
// Returns nil if not found or expired.
func (c *lruCache) get(key string) interface{} {
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	// Check TTL expiry (skip if NoTTL is set)
	if c.ttl > 0 && time.Since(entry.timestamp) > c.ttl {
		c.delete(key)
		return nil
	}

	// Move to front (most recently used)
	c.lruList.MoveToFront(entry.element)
	return entry.value
}

// set adds or updates a value in the cache.
// Returns true if an entry was evicted to make room.
func (c *lruCache) set(key string, value interface{}) bool {
	evicted := false

	// Update existing entry
	if entry, ok := c.entries[key]; ok {
		entry.value = value
		entry.timestamp = time.Now()
		c.lruList.MoveToFront(entry.element)
		return false
	}

	// Evict if at capacity
	if c.maxEntries > 0 && len(c.entries) >= c.maxEntries {
		c.evictOldest()
		evicted = true
	}

	// Add new entry
	entry := &lruEntry{
		key:       key,
		value:     value,
		timestamp: time.Now(),
	}
	entry.element = c.lruList.PushFront(entry)
	c.entries[key] = entry

	return evicted
}

// delete removes a specific key from the cache.
// Returns true if the key existed.
func (c *lruCache) delete(key string) bool {
	entry, ok := c.entries[key]
	if !ok {
		return false
	}

	c.lruList.Remove(entry.element)
	delete(c.entries, key)
	return true
}

// deletePrefix removes all keys starting with the given prefix.
// Returns the number of entries removed.
func (c *lruCache) deletePrefix(prefix string) int {
	count := 0
	for key, entry := range c.entries {
		if strings.HasPrefix(key, prefix) {
			c.lruList.Remove(entry.element)
			delete(c.entries, key)
			count++
		}
	}
	return count
}

// evictOldest removes the least recently used entry.
func (c *lruCache) evictOldest() {
	element := c.lruList.Back()
	if element == nil {
		return
	}

	entry := element.Value.(*lruEntry)
	c.lruList.Remove(element)
	delete(c.entries, entry.key)
}

// clear removes all entries from the cache.
func (c *lruCache) clear() {
	c.entries = make(map[string]*lruEntry)
	c.lruList = list.New()
}
