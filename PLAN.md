# Caching Subsystem Implementation Plan

**Issue**: [#1 - Caching subsystem: MVP in-memory LRU + BoltDB adapter](https://github.com/sandgardenhq/ragfs/issues/1)

**Branch**: `add-caching-subsystem`

**Scope**: MVP - In-memory LRU cache only (BoltDB deferred to future PR)

---

## Requirements Summary

### What Gets Cached
- **Layer 1**: Handler results (`[]fs.DirEntry`) - expensive handler calls
- **Layer 2**: File content (`[]byte`) - content extraction from DirEntry

### Cache Key
- Path string only (no normalization)

### Configuration
- **Opt-in**: Call `fs.EnableCache(config)` to enable caching
- **Defaults**:
  - `MaxEntries`: 1000
  - `TTL`: 30 seconds

### Statistics
- Global stats across all cached routes
- Thread-safe using atomic operations
- Metrics: Hits, Misses, Evictions, Entries

### Thread Safety
- All cache operations protected by `sync.RWMutex`
- Stats use `atomic.Int64` for lock-free updates

### Invalidation API
- `fs.Invalidate(path string) error` - invalidate specific path
- `fs.InvalidatePrefix(prefix string) error` - invalidate all paths with prefix
- Both clear from both cache layers

### Out of Scope for MVP
- `Warm(path)` API - defer to future PR
- Handler opt-out mechanisms - handlers control caching internally
- Key normalization - trust paths as-is
- BoltDB adapter - defer to future PR
- Redis/Memcached adapters - defer to future PR

---

## Architecture

```go
// Two-layer cache architecture
type FS struct {
    routes []route
    cache  *Cache  // nil if caching disabled
}

type Cache struct {
    handlerCache *lruCache       // Layer 1: []fs.DirEntry results
    contentCache *lruCache       // Layer 2: []byte content
    stats        CacheStats      // Global statistics
    mu           sync.RWMutex    // Protects cache operations
}

type CacheConfig struct {
    MaxEntries int           // Max entries per layer (0 = unlimited)
    TTL        time.Duration // Time-to-live (0 = no expiry)
}

type CacheStats struct {
    Hits      atomic.Int64  // Cache hits
    Misses    atomic.Int64  // Cache misses
    Evictions atomic.Int64  // LRU evictions
    Entries   atomic.Int64  // Current entry count
}

type lruCache struct {
    maxEntries int
    ttl        time.Duration
    entries    map[string]*lruEntry
    lruList    *list.List
}

type lruEntry struct {
    key       string
    value     interface{}  // []fs.DirEntry or []byte
    timestamp time.Time
    element   *list.Element
}
```

---

## Implementation Tasks

### 1. Create `cache.go` ✅
- [x] Define `Cache`, `CacheConfig`, `CacheStats` structs
- [x] Define `lruCache` and `lruEntry` internal types
- [x] Implement LRU cache logic:
  - `get(key)` - retrieve and update LRU order, check TTL
  - `set(key, value)` - add/update, evict if needed
  - `delete(key)` - remove specific key
  - `deletePrefix(prefix)` - remove all keys with prefix
  - `clear()` - remove all entries
- [x] Implement `newCache(config)` constructor
- [x] Add Godoc comments for all exported types

### 2. Extend `ragfs.go` ✅
- [x] Add `cache *Cache` field to `FS` struct
- [x] Implement `EnableCache(config CacheConfig)` method
- [x] Implement `Invalidate(path string) error` method
- [x] Implement `InvalidatePrefix(prefix string) error` method
- [x] Implement `Stats() CacheStats` method
- [x] Modify `Open(name)` to use cache:
  - Check handler cache first (Layer 1)
  - On miss, call handler and cache result
  - Check content cache for file content (Layer 2)
  - On miss, extract content and cache it
  - Update stats appropriately
- [x] Add Godoc comments for all new methods

### 3. Write Unit Tests - `cache_test.go` ✅
- [x] Test TTL expiry behavior
- [x] Test LRU eviction order
- [x] Test `Invalidate(path)` single path
- [x] Test `InvalidatePrefix(prefix)` multiple paths
- [x] Test thread safety (concurrent reads/writes)
- [x] Test stats accuracy (hits, misses, evictions, entries)
- [x] Test cache hit/miss scenarios
- [x] Test both cache layers work independently
- [x] Test cache disabled by default

### 4. Write Benchmarks - `cache_bench_test.go` ✅
- [x] Benchmark cached vs non-cached reads
- [x] Benchmark concurrent access patterns
- [x] Benchmark different cache sizes
- [x] Benchmark invalidation operations

### 5. Update Documentation ✅
- [x] Add caching section to README.md
- [x] Add caching example to examples_test.go
- [x] Update CLAUDE.md with caching guidelines
- [x] Ensure all Godoc comments are complete

### 6. Verification ✅
- [x] All tests pass: `go test -v`
- [x] Code compiles: `go build ./...`
- [x] Benchmarks run: `go test -bench=.`
- [x] Examples work: verify example in examples_test.go
- [x] Coverage check: `go test -cover`

---

## Design Decisions

### Why Two Cache Layers?
- **Layer 1** caches expensive handler calls (database queries, API calls)
- **Layer 2** caches file content extraction (DirEntry.Content() calls)
- Both can be expensive, both benefit from caching independently

### Why Path-Only Keys?
- Simple and predictable
- Handlers already receive normalized paths from fs.FS interface
- No need for complex key collision handling

### Why Global Stats?
- Simpler API (one `Stats()` call)
- Easier to monitor overall cache performance
- Per-route stats can be added later if needed

### Why Thread-Safe by Default?
- FUSE mounts are inherently concurrent (multiple file ops)
- Better safe by default than forcing users to add locks
- Performance impact is minimal with RWMutex

### Why Handlers Control Caching?
- Handlers know their data better than the library
- Avoids complex opt-out mechanisms
- Keeps the API simple and predictable

---

## Testing Strategy

### Unit Tests
- Test individual functions in isolation
- Mock expensive operations
- Focus on edge cases (TTL boundaries, capacity limits, concurrent access)

### Integration Tests
- Test complete workflows with real handlers
- Verify both cache layers work together
- Demonstrate real-world usage patterns

### Benchmarks
- Compare performance with/without cache
- Measure overhead of cache operations
- Test scalability with different cache sizes

---

## Example Usage

```go
// Create filesystem
fsys := ragfs.New()

// Enable caching with custom config
fsys.EnableCache(ragfs.CacheConfig{
    MaxEntries: 500,
    TTL:        60 * time.Second,
})

// Map handlers as usual
fsys.Map("/users/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    // Expensive database query - result will be cached
    user := fetchUserFromDB(params["id"])
    return []fs.DirEntry{&userEntry{user}}, nil
})

// Read file - first call hits database, second call hits cache
f1, _ := fsys.Open("/users/123")
f2, _ := fsys.Open("/users/123") // Cache hit!

// Invalidate when data changes
fsys.Invalidate("/users/123")

// Invalidate all users
fsys.InvalidatePrefix("/users/")

// Check cache performance
stats := fsys.Stats()
fmt.Printf("Hits: %d, Misses: %d, Hit Rate: %.2f%%\n",
    stats.Hits, stats.Misses, float64(stats.Hits)/float64(stats.Hits+stats.Misses)*100)
```

---

## Success Criteria

- ✅ Cache interface designed and documented
- ✅ In-memory LRU adapter implemented with TTL and MaxEntries
- ✅ `EnableCache()` API implemented
- ✅ Runtime invalidation APIs implemented (`Invalidate`, `InvalidatePrefix`, `Stats`)
- ✅ Basic metrics exposed: hits, misses, evictions, entries
- ✅ Unit tests for TTL, eviction, invalidation, thread-safety
- ✅ Benchmarks comparing cached vs non-cached reads
- ✅ Documentation and README entries
- ✅ All tests pass, code compiles, examples work

---

## Future Work (Out of Scope for This PR)

- BoltDB adapter for durable local caching
- `Warm(path)` API for cache pre-population
- Redis/Memcached adapters
- Per-route statistics
- Cache interface for pluggable backends
- Background TTL cleanup goroutine (currently lazy cleanup on access)
- Cache size limits by memory (currently by entry count)
