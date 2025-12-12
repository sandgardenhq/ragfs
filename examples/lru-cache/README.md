# LRU Cache Example

This example demonstrates using the in-memory LRU (Least Recently Used) cache with ragfs. The LRU cache is fast but does NOT persist across process restarts.

## What This Example Does

Demonstrates all key features of the LRU cache:
1. **Cache hits and misses**: Shows performance improvement from caching
2. **LRU eviction**: Demonstrates what happens when max entries is reached
3. **TTL expiration**: Shows time-based cache expiration

## Features Demonstrated

- **Cache hits/misses**: Instant response vs handler call
- **LRU eviction**: Oldest entries evicted when limit reached
- **TTL support**: Automatic expiration after configured time
- **Cache statistics**: Real-time monitoring of cache performance

## Running the Example

```bash
cd examples/lru-cache
go run main.go
```

## Expected Output

The example runs three tests:

### Test 1: Cache Hits and Misses
```
Access item-1 (1st time):
  Handler calls: 1

Access item-1 (2nd time - cached):
  Handler calls: 1 (same - cache hit!)
  Cache stats: 2 hits, 2 misses
```

### Test 2: LRU Eviction (MaxEntries=3)
```
Access item-2:
  Handler calls: 2

Access item-3:
  Handler calls: 3
  Cache entries: 3/3 (at max capacity)

Access item-4 (will evict item-1):
  Handler calls: 4
  Evictions: 1

Access item-1 again (was evicted):
  Handler calls: 5 (increased - cache miss!)
```

### Test 3: TTL Expiration (5 seconds)
```
Access item-5:
  Timestamp: 2025-12-11T...

Access item-5 immediately (cached):
  Timestamp: 2025-12-11T... (same as before)
  Handler calls: 6 (not called - cached)

Waiting 6 seconds for TTL to expire...

Access item-5 after TTL expired:
  Timestamp: 2025-12-11T... (NEW timestamp!)
  Handler calls: 7 (called again - TTL expired!)
```

## Configuration

The example uses:
- **MaxEntries**: 3 (small limit to demonstrate eviction)
- **TTL**: 5 seconds (short TTL for quick demonstration)

Adjust these in `main.go` for production use:
```go
fsys.EnableCache(ragfs.CacheConfig{
    MaxEntries: 1000,            // More realistic capacity
    TTL:        30 * time.Second, // Longer TTL
})
```

## LRU vs BoltDB Cache

| Feature | LRU Cache | BoltDB Cache |
|---------|-----------|--------------|
| Speed | ✓ Faster (in-memory) | Slightly slower (disk I/O) |
| Persistence | ✗ Lost on restart | ✓ Survives restarts |
| Capacity | Limited by RAM | ✓ Limited by disk |
| Overhead | ✓ Lower | Higher |

## Use Cases

**LRU Cache is perfect for:**
- Short-lived processes
- Small to medium datasets
- Maximum performance requirements
- Development and testing

**BoltDB Cache is better for:**
- Long-running services
- Large datasets
- When cache warmup is expensive
- Production deployments

## TTL Configuration

```go
// Default TTL (30 seconds)
fsys.EnableCache(ragfs.CacheConfig{})

// Custom TTL
fsys.EnableCache(ragfs.CacheConfig{
    TTL: 5 * time.Minute,
})

// Disable TTL (never expire based on time)
fsys.EnableCache(ragfs.CacheConfig{
    TTL: ragfs.NoTTL,
})
```
