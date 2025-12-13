package ragfs

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"
)

// TestBoltDBCachePersistence verifies that the cache survives process restarts
func TestBoltDBCachePersistence(t *testing.T) {
	// Create temporary database file
	tmpFile, err := os.CreateTemp("", "ragfs-persist-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	callCount := 0

	// Phase 1: Create filesystem, enable cache, and populate it
	t.Run("populate cache", func(t *testing.T) {
		fsys := New()
		err = fsys.EnableBoltDBCache(BoltDBCacheConfig{
			DBPath:     dbPath,
			MaxEntries: 10,
			TTL:        60 * time.Second,
		})
		if err != nil {
			t.Fatalf("failed to enable BoltDB cache: %v", err)
		}

		// Register handler that tracks call count
		fsys.Map("/data", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			callCount++
			return []fs.DirEntry{
				&testEntry{
					name:    "file.txt",
					content: []byte("persistent-data"),
					isDir:   false,
				},
			}, nil
		}))

		// First access - should call handler
		f1, err := fsys.Open("/data")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		content1, err := io.ReadAll(f1)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		f1.Close()

		if string(content1) != "persistent-data" {
			t.Errorf("Expected 'persistent-data', got '%s'", string(content1))
		}

		if callCount != 1 {
			t.Errorf("Expected 1 call, got %d", callCount)
		}

		// Close the database
		if err := fsys.CloseBoltDBCache(); err != nil {
			t.Fatalf("failed to close BoltDB cache: %v", err)
		}
	})

	// Phase 2: Create NEW filesystem instance with same DB path
	// This simulates a process restart
	t.Run("verify persistence after restart", func(t *testing.T) {
		// Reset call count to track new handler
		callCount = 0

		fsys := New()
		err = fsys.EnableBoltDBCache(BoltDBCacheConfig{
			DBPath:     dbPath,
			MaxEntries: 10,
			TTL:        60 * time.Second,
		})
		if err != nil {
			t.Fatalf("failed to enable BoltDB cache: %v", err)
		}

		// Register the SAME handler
		fsys.Map("/data", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			callCount++
			return []fs.DirEntry{
				&testEntry{
					name:    "file.txt",
					content: []byte("persistent-data"),
					isDir:   false,
				},
			}, nil
		}))

		// Access the same path - should use cached data, NOT call handler
		f2, err := fsys.Open("/data")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		content2, err := io.ReadAll(f2)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		f2.Close()

		if string(content2) != "persistent-data" {
			t.Errorf("Expected 'persistent-data' (from cache), got '%s'", string(content2))
		}

		// CRITICAL: Handler should NOT have been called (cache hit)
		if callCount != 0 {
			t.Errorf("Expected 0 calls (cache hit after restart), got %d", callCount)
		}

		// Verify stats show cache hit
		stats := fsys.Stats()
		if stats.Hits.Load() < 2 {
			t.Errorf("Expected at least 2 cache hits (handler + content), got %d", stats.Hits.Load())
		}

		// Close the database
		if err := fsys.CloseBoltDBCache(); err != nil {
			t.Fatalf("failed to close BoltDB cache: %v", err)
		}
	})
}

// TestBoltDBCacheTTLExpiration verifies that TTL-based expiration works
func TestBoltDBCacheTTLExpiration(t *testing.T) {
	// Create temporary database file
	tmpFile, err := os.CreateTemp("", "ragfs-ttl-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	fsys := New()
	// Use very short TTL for testing
	err = fsys.EnableBoltDBCache(BoltDBCacheConfig{
		DBPath:     dbPath,
		MaxEntries: 10,
		TTL:        100 * time.Millisecond, // 100ms TTL
	})
	if err != nil {
		t.Fatalf("failed to enable BoltDB cache: %v", err)
	}
	defer fsys.CloseBoltDBCache()

	callCount := 0
	fsys.Map("/data", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{
				name:    "file.txt",
				content: []byte(fmt.Sprintf("call-%d", callCount)),
				isDir:   false,
			},
		}, nil
	}))

	// First access - calls handler
	f1, _ := fsys.Open("/data")
	content1, _ := io.ReadAll(f1)
	f1.Close()

	if string(content1) != "call-1" {
		t.Errorf("Expected 'call-1', got '%s'", string(content1))
	}

	// Immediate second access - uses cache
	f2, _ := fsys.Open("/data")
	content2, _ := io.ReadAll(f2)
	f2.Close()

	if string(content2) != "call-1" {
		t.Errorf("Expected 'call-1' (cached), got '%s'", string(content2))
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call (cache hit), got %d", callCount)
	}

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Third access - cache expired, calls handler again
	f3, _ := fsys.Open("/data")
	content3, _ := io.ReadAll(f3)
	f3.Close()

	if string(content3) != "call-2" {
		t.Errorf("Expected 'call-2' (expired, refetched), got '%s'", string(content3))
	}

	if callCount != 2 {
		t.Errorf("Expected 2 calls (TTL expired), got %d", callCount)
	}
}

// TestBoltDBCacheInvalidation verifies cache invalidation works
func TestBoltDBCacheInvalidation(t *testing.T) {
	// Create temporary database file
	tmpFile, err := os.CreateTemp("", "ragfs-invalidate-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	fsys := New()
	err = fsys.EnableBoltDBCache(BoltDBCacheConfig{
		DBPath:     dbPath,
		MaxEntries: 10,
		TTL:        60 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to enable BoltDB cache: %v", err)
	}
	defer fsys.CloseBoltDBCache()

	callCount := 0
	fsys.Map("/users/{id}", NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		return []fs.DirEntry{
			&testEntry{
				name:    params["id"] + ".json",
				content: []byte(fmt.Sprintf(`{"id": "%s", "call": %d}`, params["id"], callCount)),
				isDir:   false,
			},
		}, nil
	}))

	// Access user 1
	f1, _ := fsys.Open("/users/1")
	io.ReadAll(f1)
	f1.Close()

	// Access user 2
	f2, _ := fsys.Open("/users/2")
	io.ReadAll(f2)
	f2.Close()

	if callCount != 2 {
		t.Errorf("Expected 2 calls, got %d", callCount)
	}

	// Access again - both cached
	f3, _ := fsys.Open("/users/1")
	io.ReadAll(f3)
	f3.Close()

	f4, _ := fsys.Open("/users/2")
	io.ReadAll(f4)
	f4.Close()

	if callCount != 2 {
		t.Errorf("Expected still 2 calls (cache hits), got %d", callCount)
	}

	// Invalidate user 1
	fsys.boltDBCache.invalidate("/users/1")

	// Access user 1 - refetches
	f5, _ := fsys.Open("/users/1")
	content5, _ := io.ReadAll(f5)
	f5.Close()

	if string(content5) != `{"id": "1", "call": 3}` {
		t.Errorf("Expected call 3 (invalidated), got '%s'", string(content5))
	}

	// Access user 2 - still cached
	f6, _ := fsys.Open("/users/2")
	content6, _ := io.ReadAll(f6)
	f6.Close()

	if string(content6) != `{"id": "2", "call": 2}` {
		t.Errorf("Expected call 2 (still cached), got '%s'", string(content6))
	}

	if callCount != 3 {
		t.Errorf("Expected 3 calls (user 1 refetched), got %d", callCount)
	}
}
