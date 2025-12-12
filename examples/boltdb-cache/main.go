// BoltDB Cache Example
//
// This example demonstrates using the BoltDB persistent cache with ragfs.
// The cache survives process restarts, making it ideal for expensive API calls.
//
// Usage:
//   go run main.go
//
// The example simulates an expensive API call that fetches GitHub user data.
// Run it multiple times to see the cache in action - the second run will be instant.

package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
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
	fmt.Println("=== BoltDB Cache Example ===")

	// Create filesystem
	fsys := ragfs.New()

	// Enable BoltDB cache with 5 minute TTL
	dbPath := "/tmp/ragfs-cache.db"
	err := fsys.EnableBoltDBCache(ragfs.BoltDBCacheConfig{
		DBPath:     dbPath,
		MaxEntries: 100,
		TTL:        5 * time.Minute,
	})
	if err != nil {
		log.Fatalf("Failed to enable BoltDB cache: %v", err)
	}
	defer fsys.CloseBoltDBCache()

	fmt.Printf("BoltDB cache enabled: %s\n", dbPath)
	fmt.Printf("TTL: 5 minutes\n\n")

	// Simulate an expensive API call
	callCount := 0
	fsys.Map("/users/{username}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		username := params["username"]

		fmt.Printf("[Handler Called #%d] Fetching user: %s\n", callCount, username)
		fmt.Println("  Simulating expensive API call...")
		time.Sleep(2 * time.Second) // Simulate 2 second API call

		// Simulate GitHub API response
		data := fmt.Sprintf(`{
  "login": "%s",
  "id": 12345,
  "name": "Test User",
  "company": "Acme Corp",
  "blog": "https://example.com",
  "location": "San Francisco, CA",
  "bio": "Full-stack developer",
  "public_repos": 42,
  "followers": 100,
  "following": 50,
  "created_at": "2020-01-01T00:00:00Z"
}`, username)

		return []fs.DirEntry{
			&fileEntry{
				name:    username + ".json",
				content: []byte(data),
				isDir:   false,
			},
		}, nil
	})

	// Test 1: First access (cache miss)
	fmt.Println("--- Test 1: First Access (Cache Miss) ---")
	start := time.Now()
	f1, err := fsys.Open("/users/octocat")
	if err != nil {
		log.Fatalf("Open failed: %v", err)
	}
	content1, _ := io.ReadAll(f1)
	f1.Close()
	elapsed1 := time.Since(start)

	fmt.Printf("\nResponse:\n%s\n", string(content1))
	fmt.Printf("\nTime taken: %v\n", elapsed1)
	fmt.Printf("Handler calls: %d\n\n", callCount)

	// Test 2: Second access (cache hit)
	fmt.Println("--- Test 2: Second Access (Cache Hit) ---")
	start = time.Now()
	f2, err := fsys.Open("/users/octocat")
	if err != nil {
		log.Fatalf("Open failed: %v", err)
	}
	content2, _ := io.ReadAll(f2)
	f2.Close()
	elapsed2 := time.Since(start)

	fmt.Printf("Response:\n%s\n", string(content2))
	fmt.Printf("\nTime taken: %v (instant!)\n", elapsed2)
	fmt.Printf("Handler calls: %d (same as before - cached!)\n\n", callCount)

	// Show cache statistics
	stats := fsys.Stats()
	fmt.Println("--- Cache Statistics ---")
	fmt.Printf("Cache hits: %d\n", stats.Hits.Load())
	fmt.Printf("Cache misses: %d\n", stats.Misses.Load())
	fmt.Printf("Total entries: %d\n\n", stats.Entries.Load())

	// Test 3: Different user (cache miss)
	fmt.Println("--- Test 3: Different User (Cache Miss) ---")
	start = time.Now()
	f3, err := fsys.Open("/users/torvalds")
	if err != nil {
		log.Fatalf("Open failed: %v", err)
	}
	content3, _ := io.ReadAll(f3)
	f3.Close()
	elapsed3 := time.Since(start)

	fmt.Printf("Response:\n%s\n", string(content3))
	fmt.Printf("\nTime taken: %v\n", elapsed3)
	fmt.Printf("Handler calls: %d\n\n", callCount)

	// Final statistics
	stats = fsys.Stats()
	fmt.Println("--- Final Cache Statistics ---")
	fmt.Printf("Cache hits: %d\n", stats.Hits.Load())
	fmt.Printf("Cache misses: %d\n", stats.Misses.Load())
	fmt.Printf("Total entries: %d\n\n", stats.Entries.Load())

	fmt.Println("=== Try running this again! ===")
	fmt.Println("The cache persists, so the second run will be instant.")
	fmt.Printf("Cache file: %s\n", dbPath)
}
