package ragfs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/brittcrawford/ragfs"
)

// TestJSONHandler demonstrates mapping filesystem paths to JSON queries.
// Path /config/name reads from data.config.name in the JSON
func TestJSONHandler(t *testing.T) {
	// Sample JSON data
	jsonData := `{
		"config": {
			"name": "MyApp",
			"version": "1.0.0",
			"database": {
				"host": "localhost",
				"port": 5432
			}
		},
		"users": ["alice", "bob", "charlie"]
	}`

	var data map[string]any
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	fsys := ragfs.New()

	// Create a handler that maps filesystem paths to JSON paths
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		// Convert /config/name to ["config", "name"]
		parts := strings.Split(strings.Trim(path, "/"), "/")

		// Navigate through JSON
		var current any = data
		for _, part := range parts {
			switch v := current.(type) {
			case map[string]any:
				val, exists := v[part]
				if !exists {
					return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
				}
				current = val
			default:
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
		}

		// Convert the value to a file entry
		var content []byte
		switch v := current.(type) {
		case string:
			content = []byte(v)
		default:
			// For all other types (numbers, objects, arrays), return JSON
			var err error
			content, err = json.MarshalIndent(v, "", "  ")
			if err != nil {
				return nil, err
			}
		}

		// Return as a single file entry
		return []fs.DirEntry{
			&jsonFileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil
	}

	// Map a wildcard pattern (for now, we'll use specific paths)
	fsys.Map("/config/name", handler)
	fsys.Map("/config/version", handler)
	fsys.Map("/config/database/host", handler)
	fsys.Map("/config/database", handler)

	// Test reading /config/name
	t.Run("read config name", func(t *testing.T) {
		f, err := fsys.Open("/config/name")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if string(content) != "MyApp" {
			t.Errorf("expected 'MyApp', got %q", content)
		}
	})

	// Test reading nested object
	t.Run("read database config", func(t *testing.T) {
		f, err := fsys.Open("/config/database")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer f.Close()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		var db map[string]any
		if err := json.Unmarshal(content, &db); err != nil {
			t.Fatalf("failed to parse JSON response: %v", err)
		}

		if db["host"] != "localhost" {
			t.Errorf("expected host 'localhost', got %v", db["host"])
		}
	})
}

type jsonFileEntry struct {
	name    string
	content []byte
}

func (e *jsonFileEntry) Name() string               { return e.name }
func (e *jsonFileEntry) IsDir() bool                { return false }
func (e *jsonFileEntry) Type() fs.FileMode          { return 0 }
func (e *jsonFileEntry) Info() (fs.FileInfo, error) { return nil, nil }
func (e *jsonFileEntry) Content() []byte            { return e.content }

// ExampleFS demonstrates how to create a filesystem that maps paths to JSON data.
//
// This example shows:
//   - Creating a ragfs filesystem
//   - Mapping path patterns to handlers
//   - Extracting path parameters
//   - Converting data to filesystem entries
func ExampleFS() {
	// Sample configuration data
	config := map[string]any{
		"app": map[string]any{
			"name":    "MyApp",
			"version": "1.0.0",
		},
		"database": map[string]any{
			"host": "localhost",
			"port": 5432,
		},
	}

	// Create a new filesystem
	fsys := ragfs.New()

	// Map a handler for /config/{key} pattern
	fsys.Map("/config/{key}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		key := params["key"]

		// Look up the value in our config
		value, exists := config[key]
		if !exists {
			return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
		}

		// Convert to JSON
		content, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return nil, err
		}

		// Return as a file entry
		return []fs.DirEntry{
			&jsonFileEntry{
				name:    key + ".json",
				content: content,
			},
		}, nil
	})

	// Now you can read from the filesystem
	f, _ := fsys.Open("/config/database")
	content, _ := io.ReadAll(f)
	fmt.Println(string(content))
	// Output:
	// {
	//   "host": "localhost",
	//   "port": 5432
	// }
}

// ExampleFS_emails demonstrates fetching data with date parameters.
func ExampleFS_emails() {
	fsys := ragfs.New()

	// Map emails by date
	fsys.Map("/emails/{date}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		date := params["date"]

		// In a real implementation, this would query a database or API
		// For this example, we'll return mock data
		emails := []struct {
			ID   string
			Body string
		}{
			{ID: "msg1", Body: "Email content for " + date},
			{ID: "msg2", Body: "Another email for " + date},
		}

		// Return each email as a separate file entry
		var entries []fs.DirEntry
		for _, email := range emails {
			entries = append(entries, &jsonFileEntry{
				name:    email.ID + ".txt",
				content: []byte(email.Body),
			})
		}

		return entries, nil
	})

	// This demonstrates the pattern - in practice you would:
	// entries, _ := fs.ReadDir(fsys, "/emails/2025-10-07")
	// for _, entry := range entries { ... }
	fmt.Println("Email handler registered")
	// Output:
	// Email handler registered
}

// ExampleFS_jsonPath shows navigating nested JSON structures.
func ExampleFS_jsonPath() {
	data := map[string]any{
		"users": map[string]any{
			"alice": map[string]any{
				"email": "alice@example.com",
				"role":  "admin",
			},
		},
	}

	fsys := ragfs.New()

	// Generic handler that navigates JSON by path
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		// Split path into components
		parts := strings.Split(strings.Trim(path, "/"), "/")

		// Navigate through the JSON structure
		var current any = data
		for _, part := range parts {
			m, ok := current.(map[string]any)
			if !ok {
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
			current, ok = m[part]
			if !ok {
				return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
			}
		}

		// Serialize the result
		content, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			return nil, err
		}

		return []fs.DirEntry{
			&jsonFileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil
	}

	fsys.Map("/users/alice/email", handler)

	// Reading /users/alice/email returns just the email
	f, _ := fsys.Open("/users/alice/email")
	content, _ := io.ReadAll(f)
	fmt.Println(string(content))
	// Output:
	// "alice@example.com"
}

// ExampleFS_caching demonstrates using the built-in cache to accelerate
// repeated filesystem operations.
func ExampleFS_caching() {
	fsys := ragfs.New()

	// Enable caching with custom configuration
	fsys.EnableCache(ragfs.CacheConfig{
		MaxEntries: 100,              // Cache up to 100 entries per layer
		TTL:        60 * time.Second, // Entries expire after 60 seconds
	})

	// Simulate an expensive database query
	callCount := 0
	fsys.Map("/users/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++ // Track how many times handler is called

		// Simulate expensive operation (e.g., database query)
		user := map[string]any{
			"id":    params["id"],
			"name":  "User " + params["id"],
			"email": params["id"] + "@example.com",
		}

		content, _ := json.MarshalIndent(user, "", "  ")
		return []fs.DirEntry{
			&jsonFileEntry{
				name:    params["id"] + ".json",
				content: content,
			},
		}, nil
	})

	// First read - calls the handler (cache miss)
	f1, _ := fsys.Open("/users/123")
	io.ReadAll(f1)
	f1.Close()

	// Second read - uses cached result (cache hit)
	f2, _ := fsys.Open("/users/123")
	io.ReadAll(f2)
	f2.Close()

	// Third read - still cached
	f3, _ := fsys.Open("/users/123")
	io.ReadAll(f3)
	f3.Close()

	// Check cache statistics
	stats := fsys.Stats()
	fmt.Printf("Handler called: %d times\n", callCount)
	fmt.Printf("Cache hits: %d\n", stats.Hits.Load())
	fmt.Printf("Cache misses: %d\n", stats.Misses.Load())

	// Invalidate the cache
	fsys.Invalidate("/users/123")

	// Next read will call handler again
	f4, _ := fsys.Open("/users/123")
	io.ReadAll(f4)
	f4.Close()

	fmt.Printf("Handler called after invalidation: %d times\n", callCount)

	// Output:
	// Handler called: 1 times
	// Cache hits: 4
	// Cache misses: 2
	// Handler called after invalidation: 2 times
}

// ExampleFS_cachingWithPrefix demonstrates prefix-based cache invalidation
// for efficiently clearing related entries.
func ExampleFS_cachingWithPrefix() {
	fsys := ragfs.New()

	// Enable caching with default config
	fsys.EnableCache(ragfs.CacheConfig{})

	callCount := 0
	fsys.Map("/data/{category}/{id}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		callCount++
		content := []byte(fmt.Sprintf("%s-%s", params["category"], params["id"]))
		return []fs.DirEntry{
			&jsonFileEntry{
				name:    params["id"],
				content: content,
			},
		}, nil
	})

	// Access multiple paths
	paths := []string{
		"/data/users/1",
		"/data/users/2",
		"/data/posts/1",
		"/data/posts/2",
	}

	for _, path := range paths {
		f, _ := fsys.Open(path)
		io.ReadAll(f)
		f.Close()
	}

	fmt.Printf("Initial calls: %d\n", callCount)

	// Access again - all from cache
	for _, path := range paths {
		f, _ := fsys.Open(path)
		io.ReadAll(f)
		f.Close()
	}

	fmt.Printf("After cached reads: %d\n", callCount)

	// Invalidate all users
	fsys.InvalidatePrefix("/data/users/")

	// Re-access users - these will call the handler
	// Posts remain cached
	for _, path := range paths {
		f, _ := fsys.Open(path)
		io.ReadAll(f)
		f.Close()
	}

	stats := fsys.Stats()
	fmt.Printf("After prefix invalidation: %d\n", callCount)
	fmt.Printf("Total cache hits: %d\n", stats.Hits.Load())
	fmt.Printf("Total cache misses: %d\n", stats.Misses.Load())

	// Output:
	// Initial calls: 4
	// After cached reads: 4
	// After prefix invalidation: 6
	// Total cache hits: 12
	// Total cache misses: 12
}
