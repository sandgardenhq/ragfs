// LRU Cache Example
//
// This example demonstrates using the in-memory LRU cache with ragfs.
// The LRU cache is fast but does NOT persist across restarts.
//
// Usage:
//   go run main.go
//
// The example demonstrates:
// - Cache hits and misses
// - TTL expiration
// - LRU eviction when max entries reached
// - Cache statistics tracking

package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"time"

	"github.com/brittcrawford/ragfs"
)

// fileEntry implements fs.DirEntry with content
type fileEntry struct {
	name    string
	content []byte
	isDir   bool
}

func (e *fileEntry) Name() string               { return e.name }
func (e *fileEntry) IsDir() bool                { return e.isDir }
func (e *fileEntry) Type() fs.FileMode          { return 0 }
func (e *fileEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *fileEntry) Content() []byte            { return e.content }

func main() {
	fmt.Println("=== LRU Cache Example ===\n")

	// Create filesystem
	fsys := ragfs.New()

	// Enable in-memory LRU cache
	fsys.EnableCache(ragfs.CacheConfig{
		MaxEntries: 3,             // Small limit to demonstrate eviction
		TTL:        5 * time.Second, // Short TTL for demo
	})

	fmt.Println("LRU cache enabled:")
	fmt.Println("  Max Entries: 3")
	fmt.Println("  TTL: 5 seconds\n")

	// Simulate fetching data
	callCount := 0
	fsys.Map("/items/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		id := params["id"]

		fmt.Printf("[Handler Called #%d] Fetching item: %s\n", callCount, id)

		data := fmt.Sprintf(`{"id": "%s", "data": "Item %s data", "timestamp": "%s"}`,
			id, id, time.Now().Format(time.RFC3339))

		return []fs.DirEntry{
			&fileEntry{
				name:    id + ".json",
				content: []byte(data),
				isDir:   false,
			},
		}, nil
	})

	// Test 1: Cache hits and misses
	fmt.Println("=== Test 1: Cache Hits and Misses ===")

	// First access - cache miss
	fmt.Println("\nAccess item-1 (1st time):")
	f1, _ := fsys.Open("/items/item-1")
	content1, _ := io.ReadAll(f1)
	f1.Close()
	fmt.Printf("  Result: %s\n", string(content1))
	fmt.Printf("  Handler calls: %d\n", callCount)

	// Second access to same item - cache hit
	fmt.Println("\nAccess item-1 (2nd time - cached):")
	f2, _ := fsys.Open("/items/item-1")
	content2, _ := io.ReadAll(f2)
	f2.Close()
	fmt.Printf("  Result: %s\n", string(content2))
	fmt.Printf("  Handler calls: %d (same - cache hit!)\n", callCount)

	stats := fsys.Stats()
	fmt.Printf("  Cache stats: %d hits, %d misses\n", stats.Hits.Load(), stats.Misses.Load())

	// Test 2: LRU Eviction
	fmt.Println("\n=== Test 2: LRU Eviction (MaxEntries=3) ===")

	// Access item-2 and item-3
	fmt.Println("\nAccess item-2:")
	f3, _ := fsys.Open("/items/item-2")
	io.ReadAll(f3)
	f3.Close()
	fmt.Printf("  Handler calls: %d\n", callCount)

	fmt.Println("\nAccess item-3:")
	f4, _ := fsys.Open("/items/item-3")
	io.ReadAll(f4)
	f4.Close()
	fmt.Printf("  Handler calls: %d\n", callCount)

	stats = fsys.Stats()
	fmt.Printf("  Cache entries: %d/3 (at max capacity)\n", stats.Entries.Load())

	// Access item-4 - this will evict item-1 (least recently used)
	fmt.Println("\nAccess item-4 (will evict item-1):")
	f5, _ := fsys.Open("/items/item-4")
	io.ReadAll(f5)
	f5.Close()
	fmt.Printf("  Handler calls: %d\n", callCount)

	stats = fsys.Stats()
	fmt.Printf("  Cache entries: %d/3\n", stats.Entries.Load())
	fmt.Printf("  Evictions: %d\n", stats.Evictions.Load())

	// Try to access item-1 again - it was evicted, so handler is called
	fmt.Println("\nAccess item-1 again (was evicted):")
	f6, _ := fsys.Open("/items/item-1")
	io.ReadAll(f6)
	f6.Close()
	fmt.Printf("  Handler calls: %d (increased - cache miss!)\n", callCount)

	// Test 3: TTL Expiration
	fmt.Println("\n=== Test 3: TTL Expiration (5 seconds) ===")

	// Access item-5
	fmt.Println("\nAccess item-5:")
	f7, _ := fsys.Open("/items/item-5")
	content7, _ := io.ReadAll(f7)
	f7.Close()
	timestamp1 := string(content7)
	fmt.Printf("  Timestamp: %s\n", timestamp1[len(timestamp1)-30:])

	// Immediate re-access - cached
	fmt.Println("\nAccess item-5 immediately (cached):")
	f8, _ := fsys.Open("/items/item-5")
	content8, _ := io.ReadAll(f8)
	f8.Close()
	timestamp2 := string(content8)
	fmt.Printf("  Timestamp: %s (same as before)\n", timestamp2[len(timestamp2)-30:])
	fmt.Printf("  Handler calls: %d (not called - cached)\n", callCount)

	// Wait for TTL to expire
	fmt.Println("\nWaiting 6 seconds for TTL to expire...")
	time.Sleep(6 * time.Second)

	// Access again - cache expired, refetch
	fmt.Println("\nAccess item-5 after TTL expired:")
	f9, _ := fsys.Open("/items/item-5")
	content9, _ := io.ReadAll(f9)
	f9.Close()
	timestamp3 := string(content9)
	fmt.Printf("  Timestamp: %s (NEW timestamp!)\n", timestamp3[len(timestamp3)-30:])
	fmt.Printf("  Handler calls: %d (called again - TTL expired!)\n", callCount)

	// Final statistics
	stats = fsys.Stats()
	fmt.Println("\n=== Final Cache Statistics ===")
	fmt.Printf("Total hits: %d\n", stats.Hits.Load())
	fmt.Printf("Total misses: %d\n", stats.Misses.Load())
	fmt.Printf("Total evictions: %d\n", stats.Evictions.Load())
	fmt.Printf("Current entries: %d\n", stats.Entries.Load())

	hitRate := float64(stats.Hits.Load()) / float64(stats.Hits.Load()+stats.Misses.Load()) * 100
	fmt.Printf("Hit rate: %.1f%%\n", hitRate)

	fmt.Println("\n=== Key Differences from BoltDB Cache ===")
	fmt.Println("LRU Cache:")
	fmt.Println("  ✓ Faster (in-memory)")
	fmt.Println("  ✓ Lower overhead")
	fmt.Println("  ✗ Lost on restart")
	fmt.Println("  ✗ Limited by memory")
	fmt.Println("\nBoltDB Cache:")
	fmt.Println("  ✓ Survives restarts")
	fmt.Println("  ✓ Larger capacity (disk)")
	fmt.Println("  ✗ Slightly slower (disk I/O)")
	fmt.Println("  ✗ Higher overhead")
}
