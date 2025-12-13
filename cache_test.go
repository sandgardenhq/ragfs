package ragfs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"sync"
	"testing"
	"time"
)

// testEntry is a simple DirEntry implementation for testing
type testEntry struct {
	name    string
	content []byte
	isDir   bool
}

func (e *testEntry) Name() string               { return e.name }
func (e *testEntry) IsDir() bool                { return e.isDir }
func (e *testEntry) Type() fs.FileMode          { return 0 }
func (e *testEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *testEntry) Content() []byte            { return e.content }

// TestCacheDisabledByDefault verifies caching is opt-in
func TestCacheDisabledByDefault(t *testing.T) {
	fsys := New()

	if fsys.cache != nil {
		t.Error("Cache should be nil by default")
	}

	// Stats should return nil when cache is disabled
	stats := fsys.Stats()
	if stats != nil {
		t.Error("Stats should be nil when cache is disabled")
	}
}

// TestEnableCacheWithDefaults verifies default configuration
func TestEnableCacheWithDefaults(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{})

	if fsys.cache == nil {
		t.Fatal("Cache should be initialized after EnableCache")
	}

	if fsys.cache.handlerCache.maxEntries != 1000 {
		t.Errorf("Expected MaxEntries=1000, got %d", fsys.cache.handlerCache.maxEntries)
	}

	if fsys.cache.handlerCache.ttl != 30*time.Second {
		t.Errorf("Expected TTL=30s, got %v", fsys.cache.handlerCache.ttl)
	}
}

// TestEnableCacheWithCustomConfig verifies custom configuration
func TestEnableCacheWithCustomConfig(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 100,
		TTL:        5 * time.Second,
	})

	if fsys.cache.handlerCache.maxEntries != 100 {
		t.Errorf("Expected MaxEntries=100, got %d", fsys.cache.handlerCache.maxEntries)
	}

	if fsys.cache.handlerCache.ttl != 5*time.Second {
		t.Errorf("Expected TTL=5s, got %v", fsys.cache.handlerCache.ttl)
	}
}

// TestCacheHitAndMiss verifies basic cache hit/miss behavior
func TestCacheHitAndMiss(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        1 * time.Minute,
	})

	callCount := 0
	fsys.Map("/test", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{name: "file.txt", content: []byte("content")},
		}, nil
}))

	// First call - cache miss
	f1, err := fsys.Open("/test")
	if err != nil {
		t.Fatalf("First Open failed: %v", err)
	}
	f1.Close()

	if callCount != 1 {
		t.Errorf("Expected handler called once, got %d", callCount)
	}

	stats := fsys.Stats()
	// Two misses: one for handler cache, one for content cache
	if stats.Misses.Load() != 2 {
		t.Errorf("Expected 2 misses (both layers), got %d", stats.Misses.Load())
	}

	// Second call - cache hit
	f2, err := fsys.Open("/test")
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}
	f2.Close()

	if callCount != 1 {
		t.Errorf("Handler should not be called again, got %d calls", callCount)
	}

	stats = fsys.Stats()
	// Two hits: one for handler cache, one for content cache
	if stats.Hits.Load() != 2 {
		t.Errorf("Expected 2 hits (both layers), got %d", stats.Hits.Load())
	}
}

// TestCacheTTLExpiry verifies TTL expiration behavior
func TestCacheTTLExpiry(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        100 * time.Millisecond, // Short TTL for testing
})

	callCount := 0
	fsys.Map("/test", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{name: "file.txt", content: []byte(fmt.Sprintf("call-%d", callCount))},
		}, nil
}))

	// First call
	f1, _ := fsys.Open("/test")
	content1, _ := io.ReadAll(f1)
	f1.Close()

	if string(content1) != "call-1" {
		t.Errorf("Expected 'call-1', got '%s'", string(content1))
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Second call - should be cache miss due to TTL expiry
	f2, _ := fsys.Open("/test")
	content2, _ := io.ReadAll(f2)
	f2.Close()

	if string(content2) != "call-2" {
		t.Errorf("Expected 'call-2' after TTL expiry, got '%s'", string(content2))
	}

	if callCount != 2 {
		t.Errorf("Expected handler called twice due to TTL expiry, got %d", callCount)
	}
}

// TestCacheNoTTL verifies that NoTTL disables time-based expiration
func TestCacheNoTTL(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        NoTTL, // Disable TTL
})

	callCount := 0
	fsys.Map("/test", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{name: "file.txt", content: []byte(fmt.Sprintf("call-%d", callCount))},
		}, nil
}))

	// First call
	f1, _ := fsys.Open("/test")
	content1, _ := io.ReadAll(f1)
	f1.Close()

	if string(content1) != "call-1" {
		t.Errorf("Expected 'call-1', got '%s'", string(content1))
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Wait longer than a typical TTL would be
	time.Sleep(200 * time.Millisecond)

	// Second call should still use cache (NoTTL means never expire)
	f2, _ := fsys.Open("/test")
	content2, _ := io.ReadAll(f2)
	f2.Close()

	if string(content2) != "call-1" {
		t.Errorf("Expected 'call-1' (cached), got '%s'", string(content2))
	}

	if callCount != 1 {
		t.Errorf("Expected handler to still be called only 1 time (NoTTL should prevent expiry), got %d", callCount)
	}

	// Verify stats show hits
	stats := fsys.Stats()
	if stats.Hits.Load() < 2 {
		t.Errorf("Expected at least 2 cache hits with NoTTL, got %d", stats.Hits.Load())
	}
}

// TestCacheLRUEviction verifies LRU eviction behavior
func TestCacheLRUEviction(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 2, // Small cache for testing eviction
		TTL:        1 * time.Minute,
})

	callCounts := make(map[string]int)
	fsys.Map("/{name}", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		name := params["name"]
		callCounts[name]++
		return []fs.DirEntry{
			&testEntry{name: name, content: []byte(name)},
		}, nil
}))

	// Fill cache with 2 entries
	fsys.Open("/file1")
	fsys.Open("/file2")

	if callCounts["file1"] != 1 || callCounts["file2"] != 1 {
		t.Error("Expected both files called once")
	}

	// Access file1 again (should be cache hit)
	fsys.Open("/file1")
	if callCounts["file1"] != 1 {
		t.Error("file1 should be cache hit")
	}

	// Access file3 - should evict file2 (least recently used)
	fsys.Open("/file3")

	// Access file2 again - should be cache miss (evicted)
	fsys.Open("/file2")
	if callCounts["file2"] != 2 {
		t.Errorf("file2 should be called twice (evicted), got %d", callCounts["file2"])
	}

	// Access file1 again - may or may not be cached depending on eviction order
	// With MaxEntries=2 and 2 cache layers, evictions are complex
	// Just verify evictions occurred
	stats := fsys.Stats()
	if stats.Evictions.Load() < 1 {
		t.Errorf("Expected at least 1 eviction, got %d", stats.Evictions.Load())
	}
}

// TestInvalidate verifies single path invalidation
func TestInvalidate(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        1 * time.Minute,
})

	callCount := 0
	fsys.Map("/test", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{name: "file.txt", content: []byte(fmt.Sprintf("call-%d", callCount))},
		}, nil
}))

	// First call
	f1, _ := fsys.Open("/test")
	content1, _ := io.ReadAll(f1)
	f1.Close()

	// Second call - should be cache hit
	f2, _ := fsys.Open("/test")
	content2, _ := io.ReadAll(f2)
	f2.Close()

	if string(content1) != string(content2) {
		t.Error("Second call should return cached content")
	}

	// Invalidate the cache
	err := fsys.Invalidate("/test")
	if err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	// Third call - should be cache miss after invalidation
	f3, _ := fsys.Open("/test")
	content3, _ := io.ReadAll(f3)
	f3.Close()

	if string(content3) != "call-2" {
		t.Errorf("Expected 'call-2' after invalidation, got '%s'", string(content3))
	}
}

// TestInvalidatePrefix verifies prefix-based invalidation
func TestInvalidatePrefix(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        1 * time.Minute,
})

	callCounts := make(map[string]int)
	fsys.Map("/users/{id}", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		id := params["id"]
		callCounts[id]++
		return []fs.DirEntry{
			&testEntry{name: id, content: []byte(id)},
		}, nil
}))

	// Cache some entries
	fsys.Open("/users/1")
	fsys.Open("/users/2")
	fsys.Open("/users/3")

	if callCounts["1"] != 1 || callCounts["2"] != 1 || callCounts["3"] != 1 {
		t.Error("Expected all users called once")
	}

	// Verify they're cached
	fsys.Open("/users/1")
	if callCounts["1"] != 1 {
		t.Error("user 1 should be cached")
	}

	// Invalidate all users
	err := fsys.InvalidatePrefix("/users/")
	if err != nil {
		t.Fatalf("InvalidatePrefix failed: %v", err)
	}

	// All should be cache misses now
	fsys.Open("/users/1")
	fsys.Open("/users/2")
	fsys.Open("/users/3")

	if callCounts["1"] != 2 || callCounts["2"] != 2 || callCounts["3"] != 2 {
		t.Errorf("Expected all users called twice after invalidation, got %v", callCounts)
	}
}

// TestInvalidateErrors verifies error handling
func TestInvalidateErrors(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{})

	// Empty path should error
	err := fsys.Invalidate("")
	if err == nil {
		t.Error("Expected error for empty path")
	}

	err = fsys.InvalidatePrefix("")
	if err == nil {
		t.Error("Expected error for empty prefix")
	}
}

// TestConcurrentAccess verifies thread safety
func TestConcurrentAccess(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 100,
		TTL:        1 * time.Minute,
})

	var callCount int
	var mu sync.Mutex

	fsys.Map("/{id}", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		return []fs.DirEntry{
			&testEntry{name: params["id"], content: []byte(params["id"])},
		}, nil
}))

	// Concurrent reads
	const goroutines = 50
	const iterations = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			path := fmt.Sprintf("/%d", id%10) // 10 unique paths

			for j := 0; j < iterations; j++ {
				f, err := fsys.Open(path)
				if err != nil {
					t.Errorf("Open failed: %v", err)
					return
				}
				f.Close()
			}
		}(i)
	}

	wg.Wait()

	// With 10 unique paths and high concurrency, we should see:
	// - First access to each path calls handler (10 calls)
	// - Subsequent accesses are cache hits
	// - Due to concurrency, there may be some duplicate handler calls before caching completes
	// - Total calls should be well under the total iterations (500)
	mu.Lock()
	finalCallCount := callCount
	mu.Unlock()

	if finalCallCount > 50 {
		t.Errorf("Expected handler calls to be reduced by caching (max ~50), got %d", finalCallCount)
	}

	stats := fsys.Stats()
	if stats.Hits.Load() == 0 {
		t.Error("Expected some cache hits with concurrent access")
	}

	// Most accesses should be hits (500 total opens - initial misses)
	if stats.Hits.Load() < 400 {
		t.Errorf("Expected mostly cache hits, got %d hits vs %d misses",
			stats.Hits.Load(), stats.Misses.Load())
	}
}

// TestConcurrentInvalidation verifies thread safety of invalidation
func TestConcurrentInvalidation(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 100,
		TTL:        1 * time.Minute,
})

	fsys.Map("/{id}", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testEntry{name: params["id"], content: []byte(params["id"])},
		}, nil
}))

	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent reads
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				fsys.Open(fmt.Sprintf("/%d", id))
				time.Sleep(time.Millisecond)
			}
		}(i)
	}

	// Concurrent invalidations
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				fsys.Invalidate(fmt.Sprintf("/%d", id))
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()

	// If we get here without panicking, thread safety is working
}

// TestStatsAccuracy verifies stats are tracked correctly
func TestStatsAccuracy(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 3,
		TTL:        1 * time.Minute,
})

	fsys.Map("/{id}", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testEntry{name: params["id"], content: []byte(params["id"])},
		}, nil
}))

	// First access to /1 - miss in both layers
	fsys.Open("/1")
	stats := fsys.Stats()
	if stats.Misses.Load() != 2 { // handler + content
		t.Errorf("Expected 2 misses (both layers), got %d", stats.Misses.Load())
	}

	// Second access to /1 - hit in both layers
	fsys.Open("/1")
	stats = fsys.Stats()
	if stats.Misses.Load() != 2 || stats.Hits.Load() != 2 {
		t.Errorf("Expected 2 misses, 2 hits (both layers), got %d misses, %d hits",
			stats.Misses.Load(), stats.Hits.Load())
	}

	// Access /2 and /3 - 2 more misses per file (4 total new misses)
	fsys.Open("/2")
	fsys.Open("/3")
	stats = fsys.Stats()
	if stats.Misses.Load() != 6 { // 2 + 2 + 2
		t.Errorf("Expected 6 misses, got %d", stats.Misses.Load())
	}

	// Access /4 - should trigger evictions in both layers
	fsys.Open("/4")
	stats = fsys.Stats()
	if stats.Evictions.Load() < 2 { // At least 2 evictions (one per layer)
		t.Errorf("Expected at least 2 evictions, got %d", stats.Evictions.Load())
	}
}

// TestBothCacheLayers verifies both handler and content caches work independently
func TestBothCacheLayers(t *testing.T) {
	fsys := New()
	fsys.EnableCache(CacheConfig{
		MaxEntries: 10,
		TTL:        1 * time.Minute,
})

	handlerCallCount := 0
	fsys.Map("/test", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		handlerCallCount++
		return []fs.DirEntry{
			&testEntry{name: "file.txt", content: []byte("content")},
		}, nil
}))

	// First Open - both layers miss
	f1, _ := fsys.Open("/test")
	f1.Close()

	if handlerCallCount != 1 {
		t.Errorf("Expected 1 handler call, got %d", handlerCallCount)
	}

	stats := fsys.Stats()
	initialMisses := stats.Misses.Load()

	// Second Open - both layers hit
	f2, _ := fsys.Open("/test")
	content, _ := io.ReadAll(f2)
	f2.Close()

	if string(content) != "content" {
		t.Errorf("Expected 'content', got '%s'", string(content))
	}

	if handlerCallCount != 1 {
		t.Error("Handler should not be called again (cached)")
	}

	stats = fsys.Stats()
	// Should have 2 hits (one for handler layer, one for content layer)
	if stats.Hits.Load() != 2 {
		t.Errorf("Expected 2 hits (both layers), got %d", stats.Hits.Load())
	}

	// Should have same number of misses as before
	if stats.Misses.Load() != initialMisses {
		t.Errorf("Misses should not change on cache hit, got %d", stats.Misses.Load())
	}
}
