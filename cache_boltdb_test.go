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

// TestBoltDBCacheBasicOperations tests basic get/set operations with BoltDB cache
func TestBoltDBCacheBasicOperations(t *testing.T) {
	// Create temporary database file
	tmpFile, err := os.CreateTemp("", "ragfs-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	dbPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(dbPath)

	// Create filesystem with BoltDB cache
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

	// Register handler that tracks call count
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

	// First call - should call handler (cache miss)
	f1, err := fsys.Open("/data")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	content1, err := io.ReadAll(f1)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	f1.Close()

	if string(content1) != "call-1" {
		t.Errorf("Expected 'call-1', got '%s'", string(content1))
	}

	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}

	// Second call - should use cache (cache hit)
	f2, err := fsys.Open("/data")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	content2, err := io.ReadAll(f2)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	f2.Close()

	if string(content2) != "call-1" {
		t.Errorf("Expected 'call-1' (cached), got '%s'", string(content2))
	}

	if callCount != 1 {
		t.Errorf("Expected handler to still be called only 1 time (cache hit), got %d", callCount)
	}

	// Verify stats
	stats := fsys.Stats()
	if stats.Hits.Load() < 2 {
		t.Errorf("Expected at least 2 cache hits, got %d", stats.Hits.Load())
	}
}
