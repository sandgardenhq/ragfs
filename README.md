# ragfs

A Go library that maps filesystem operations to custom data handlers, implementing the standard `fs.FS` interface.

## Concept

ragfs leverages the fact that LLM models are well-trained on understanding and manipulating file systems. By mapping data access patterns (API calls, database queries, configuration lookups) to filesystem paths, we create an intuitive interface that both humans and LLMs can naturally work with.

## Features

- **Pattern-based routing**: Map path patterns like `/emails/{date}` to custom handlers
- **Parameter extraction**: Automatically extract and pass path parameters to handlers
- **Standard interface**: Implements Go's `fs.FS` for seamless integration
- **Read and write support**: Full CRUD operations via the Handler interface
- **FUSE support**: Mount as a real filesystem with read/write operations (Linux/macOS)
- **High-performance caching**: Optional two-layer in-memory LRU cache with TTL
- **BoltDB persistent cache**: Optional disk-backed cache that survives restarts
- **Built-in observability**: `/_metrics` virtual filesystem for runtime monitoring
- **Type-safe**: Leverages Go's type system for reliable filesystem operations

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
    fsys := ragfs.New()

    // Map a pattern to a read-only handler
    fsys.Map("/config/{key}", ragfs.NewReadOnlyHandler(
        func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
            value := getConfig(params["key"])
            return []fs.DirEntry{
                ragfs.NewFileEntry(params["key"]+".json", []byte(value)),
            }, nil
        },
    ))

    // Read from the filesystem
    f, _ := fsys.Open("config/database")
    content, _ := io.ReadAll(f)
    os.Stdout.Write(content)
}
```

## Handler Interface

Handler is an interface that defines all filesystem operations for a path pattern:

```go
type Handler interface {
    Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)
    Write(ctx context.Context, path string, data []byte, params map[string]string) error
    Remove(ctx context.Context, path string, params map[string]string) error
    Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error
    Mkdir(ctx context.Context, path string, params map[string]string) error
    Rmdir(ctx context.Context, path string, params map[string]string) error
    Truncate(ctx context.Context, path string, size int64, params map[string]string) error
}
```

Handlers return `[]fs.DirEntry` where each entry must implement a `Content() []byte` method. Use the built-in `NewFileEntry(name, content)` helper to create entries.

### Read-Only Handlers

For handlers that only need to read data, use `NewReadOnlyHandler`:

```go
fsys.Map("/emails/{date}", ragfs.NewReadOnlyHandler(
    func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
        emails := fetchEmails(params["date"])
        return []fs.DirEntry{ragfs.NewFileEntry("emails.json", emails)}, nil
    },
))
```

### Writable Handlers

For handlers that support write operations, embed `DefaultHandler` and override the methods you need:

```go
type MyHandler struct {
    ragfs.DefaultHandler
    data map[string][]byte
}

func (h *MyHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    content := h.data[path]
    return []fs.DirEntry{ragfs.NewFileEntry("data.txt", content)}, nil
}

func (h *MyHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
    h.data[path] = data
    return nil
}
```

### Write Operations

The FS type exposes these methods for write operations:

```go
fsys.WriteFile("path/to/file", []byte("content")) // Write data to a file
fsys.Remove("path/to/file")                        // Delete a file
fsys.Rename("old/path", "new/path")                // Rename/move a file
fsys.Mkdir("path/to/dir")                          // Create a directory
fsys.Rmdir("path/to/dir")                          // Remove an empty directory
fsys.Truncate("path/to/file", 0)                   // Change file size
```

Write operations automatically invalidate relevant cache entries.

## Caching

ragfs includes two caching backends, both opt-in and disabled by default.

### In-Memory LRU Cache

```go
fsys := ragfs.New()

// Enable with default settings (1000 entries, 30s TTL)
fsys.EnableCache(ragfs.CacheConfig{})

// Or customize
fsys.EnableCache(ragfs.CacheConfig{
    MaxEntries: 500,
    TTL:        60 * time.Second,
})
```

The in-memory cache uses a two-layer design:

1. **Layer 1 (Handler Cache)**: Caches handler results (`[]fs.DirEntry`)
2. **Layer 2 (Content Cache)**: Caches extracted file content (`[]byte`)

### BoltDB Persistent Cache

For caching that survives process restarts:

```go
err := fsys.EnableBoltDBCache(ragfs.BoltDBCacheConfig{
    DBPath:     "/tmp/ragfs-cache.db",
    MaxEntries: 1000,
    TTL:        60 * time.Second,
})
defer fsys.CloseBoltDBCache()
```

### Cache Invalidation

```go
fsys.Invalidate("/users/123")     // Invalidate a specific path
fsys.InvalidatePrefix("/users/")  // Invalidate all paths with a prefix
fsys.InvalidatePrefix("/")        // Clear everything
```

### Performance Monitoring

```go
stats := fsys.Stats()
hitRate := float64(stats.Hits.Load()) / float64(stats.Hits.Load() + stats.Misses.Load())
fmt.Printf("Cache hit rate: %.2f%%\n", hitRate*100)
fmt.Printf("Entries: %d, Evictions: %d\n", stats.Entries.Load(), stats.Evictions.Load())
```

### Performance

- **8000x faster** for cached reads vs uncached (1.27ms → 157ns)
- **~155ns** per cache hit operation
- **Thread-safe** with minimal overhead for concurrent access

See `cache_bench_test.go` for detailed benchmarks.

## Observability

ragfs automatically exposes runtime metrics via the `/_metrics` virtual filesystem:

```
/_metrics/
├── version.txt       # API version
├── summary.md        # Overall metrics summary
├── cache/            # Cache hit rates, entries, evictions
├── io/               # Bytes read/written, operation counts
│   ├── bytes_read
│   ├── bytes_written
│   ├── read_ops
│   └── write_ops
└── errors/           # Error counts and recent errors
```

Access metrics programmatically or via FUSE:

```go
// Programmatic access
snapshot := fsys.Collector().Snapshot()
fmt.Printf("Read ops: %d, Bytes read: %d\n", snapshot.ReadOps, snapshot.BytesRead)

// Via FUSE mount
// cat /tmp/ragfs/_metrics/summary.md
// cat /tmp/ragfs/_metrics/io/bytes_read
```

## FUSE Integration

Mount ragfs as a real filesystem with full read/write support:

```bash
# Build and mount the JSON example
go build -o ragfs-mount ./examples/json
./ragfs-mount -mount /tmp/ragfs -config config.json

# Read files
cat /tmp/ragfs/app/name
ls /tmp/ragfs/database/

# Write operations (with writable handlers)
echo "hello" > /tmp/writable/test.txt
mkdir /tmp/writable/mydir
rm /tmp/writable/test.txt
```

The FUSE bridge supports: file read/write, create, delete, rename, mkdir, rmdir, and truncate.

See [FUSE.md](FUSE.md) for complete installation and usage instructions.

## Examples

### JSON Configuration (`examples/json/`)

Maps a JSON config file to filesystem paths:

```bash
go run examples/json/main.go -mount /tmp/json -config config.json
cat /tmp/json/app/name        # Read config values
cat /tmp/json/_metrics/summary.md  # View metrics
```

### SQLite Database (`examples/sqlite/`)

Maps SQLite tables and rows to filesystem paths:

```bash
go run examples/sqlite/main.go -mount /tmp/sqlite -db example.db
ls /tmp/sqlite/users/          # List table rows
cat /tmp/sqlite/users/1.json   # Read a row as JSON
cat /tmp/sqlite/users/_metrics/row_count  # Table metrics
```

Supports optional BoltDB persistent caching with the `-cache` flag.

### Writable Filesystem (`examples/writable/`)

An in-memory writable filesystem demonstrating full CRUD:

```bash
go run examples/writable/main.go -mount /tmp/writable
echo "hello" > /tmp/writable/test.txt
cat /tmp/writable/test.txt
mkdir /tmp/writable/mydir
rm /tmp/writable/test.txt
```

### Cache Examples (`examples/lru-cache/`, `examples/boltdb-cache/`)

Programmatic demonstrations of the caching subsystems:

```bash
go run examples/lru-cache/main.go     # In-memory LRU cache demo
go run examples/boltdb-cache/main.go  # BoltDB persistent cache demo
```

### Test Examples

See `examples_test.go` for additional examples including path parameter extraction and nested JSON navigation.

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
- ✅ `fs.FS` interface implementation
- ✅ Handler interface with read/write operations
- ✅ `DefaultHandler` and `NewReadOnlyHandler` convenience types
- ✅ Write operations (WriteFile, Remove, Rename, Mkdir, Rmdir, Truncate)
- ✅ FUSE support for all read and write operations
- ✅ High-performance in-memory LRU cache with TTL
- ✅ BoltDB persistent cache
- ✅ Cache invalidation (single path and prefix-based)
- ✅ Cache statistics and monitoring
- ✅ `/_metrics` virtual filesystem for observability
- ✅ JSON, SQLite, and writable filesystem examples

## License

MIT - See [LICENSE](LICENSE) for details.
