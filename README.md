# ragfs

A Go library that maps filesystem operations to custom data handlers, implementing the standard `fs.FS` interface.

## Concept

ragfs leverages the fact that LLM models are well-trained on understanding and manipulating file systems. By mapping data access patterns (API calls, database queries, configuration lookups) to filesystem paths, we create an intuitive interface that both humans and LLMs can naturally work with.

## Features

- **Pattern-based routing**: Map path patterns like `/emails/{date}` to custom handlers
- **Parameter extraction**: Automatically extract and pass path parameters to handlers
- **Standard interface**: Implements Go's `fs.FS` for seamless integration
- **FUSE support**: Mount as a real filesystem using FUSE (Linux/macOS)
- **Flexible handlers**: User-defined functions that fetch data from any source
- **Type-safe**: Leverages Go's type system for reliable filesystem operations
- **High-performance caching**: Optional in-memory LRU cache with TTL support

## Quick Start

```go
package main

import (
    "context"
    "io"
    "io/fs"
    "os"

    "github.com/brittcrawford/ragfs"
)

func main() {
    // Create a new filesystem
    fsys := ragfs.New()

    // Map a pattern to a handler
    fsys.Map("/config/{key}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
        // Fetch data based on the key parameter
        value := getConfig(params["key"])

        // Return as a file entry
        return []fs.DirEntry{
            &FileEntry{
                name:    params["key"] + ".json",
                content: []byte(value),
            },
        }, nil
    })

    // Read from the filesystem
    f, _ := fsys.Open("/config/database")
    content, _ := io.ReadAll(f)
    os.Stdout.Write(content)
}
```

## Example Use Cases

### JSON Configuration Access

Map filesystem paths to JSON object queries:

```go
// Reading /config/database/host queries config.database.host
fsys.Map("/config/{section}/{key}", jsonHandler)
```

### Email Retrieval by Date

Fetch emails using date-based paths:

```go
// Reading /emails/2025-10-07 fetches emails from that date
fsys.Map("/emails/{date}", emailHandler)
```

### User Profile Access

Access user data via filesystem paths:

```go
// Reading /users/alice/profile fetches alice's profile
fsys.Map("/users/{id}/profile", profileHandler)
```

## Handler Function

Handlers are functions that fetch data for a given path:

```go
type Handler func(
    ctx context.Context,      // Request context
    path string,              // Full path (e.g., "/emails/2025-10-07")
    params map[string]string, // Extracted params (e.g., {"date": "2025-10-07"})
) ([]fs.DirEntry, error)
```

Your handler must return directory entries that implement a `Content() []byte` method to provide file contents.

## Caching

ragfs includes an optional high-performance in-memory LRU cache to accelerate repeated filesystem operations. Caching is **opt-in** and disabled by default.

### Enabling Cache

```go
fsys := ragfs.New()

// Enable with default settings (1000 entries, 30s TTL)
fsys.EnableCache(ragfs.CacheConfig{})

// Or customize the configuration
fsys.EnableCache(ragfs.CacheConfig{
    MaxEntries: 500,              // Maximum entries per cache layer
    TTL:        60 * time.Second, // Time-to-live for cached entries
})
```

### Two-Layer Architecture

The cache uses a two-layer design for maximum efficiency:

1. **Layer 1 (Handler Cache)**: Caches the results of expensive handler calls (`[]fs.DirEntry`)
2. **Layer 2 (Content Cache)**: Caches extracted file content (`[]byte`)

This architecture ensures both expensive operations (database queries, API calls) and content extraction are cached independently.

### Cache Invalidation

Manually invalidate cached entries when your data changes:

```go
// Invalidate a specific path
fsys.Invalidate("/users/123")

// Invalidate all paths with a prefix
fsys.InvalidatePrefix("/users/")

// Clear everything
fsys.InvalidatePrefix("/")
```

### Performance Monitoring

Track cache performance with built-in statistics:

```go
stats := fsys.Stats()
hitRate := float64(stats.Hits.Load()) / float64(stats.Hits.Load() + stats.Misses.Load())
fmt.Printf("Cache hit rate: %.2f%%\n", hitRate*100)
fmt.Printf("Entries: %d, Evictions: %d\n", stats.Entries.Load(), stats.Evictions.Load())
```

### Performance

Benchmarks show significant performance improvements with caching enabled:

- **8000x faster** for cached reads vs uncached (1.27ms → 157ns)
- **~155ns** per cache hit operation
- **Thread-safe** with minimal overhead for concurrent access
- **Negligible TTL overhead** (~1-2ns per access)

See `cache_bench_test.go` for detailed benchmarks.

### Configuration Guidelines

- **MaxEntries**: Set based on your working set size. Default 1000 is suitable for most applications.
- **TTL**: Balance between data freshness and cache effectiveness. 0 means no expiry.
- **Memory**: Each cache layer stores entries separately. Monitor with `Stats()`.

## FUSE Integration

Mount ragfs as a real filesystem:

```bash
# Build the mount program
go build -o ragfs-mount ./examples/json

# Create mount point
mkdir /tmp/ragfs

# Mount the filesystem
./ragfs-mount -mount /tmp/ragfs -config config.json

# Access your data as files
cat /tmp/ragfs/app/name
cat /tmp/ragfs/database/host
```

See [FUSE.md](FUSE.md) for complete installation and usage instructions.

## Examples

See `examples_test.go` for complete examples including:
- Mapping paths to JSON data
- Email retrieval with date parameters
- Nested JSON structure navigation

Run tests to see examples in action:

```bash
go test -v
```

## Development

This project follows Test-Driven Development (TDD):

```bash
# Run all tests
go test -v

# Run with coverage
go test -v -cover
```

See `CLAUDE.md` for detailed development guidelines.

## Status

**Active Development** - Core functionality implemented and tested:
- ✅ Pattern-based routing with parameter extraction
- ✅ fs.FS interface implementation
- ✅ File content reading
- ✅ JSON mapping example
- ✅ High-performance in-memory LRU cache with TTL
- ✅ Cache invalidation (single path and prefix-based)
- ✅ Cache statistics and monitoring
- 🚧 Directory listing (ReadDir) - planned
- 🚧 Most-specific route matching - planned
- 🚧 Wildcard patterns - planned
- 🚧 BoltDB cache adapter - planned

## License

MIT - See [LICENSE](LICENSE) for details.
