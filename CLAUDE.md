# CLAUDE.md - ragfs Development Guide

## Project Overview

**ragfs** is a Go library that maps filesystem operations to custom data handlers. It implements the standard `fs.FS` interface, enabling LLM agents and applications to interact with data sources (APIs, databases, JSON files) using familiar filesystem operations.

### Core Concept

LLM models are well-trained on filesystem manipulation. By mapping data access patterns to filesystem paths, we create an intuitive interface that leverages this training. For example:
- Reading `/emails/2025-10-07` triggers an API call to fetch emails from that date
- Reading `/config/database/host` queries a JSON configuration file
- Reading `/users/{id}/profile` fetches user data from a database

## Architecture

### Key Components

1. **FS struct**: Implements `fs.FS` interface with route registration
2. **Handler function**: User-defined function that fetches data for a path
3. **Pattern matching**: Extracts parameters from paths like `/emails/{date}`
4. **Route matching**: Finds the appropriate handler for a given path
5. **FUSE bridge**: Integrates with go-fuse for mounting as a real filesystem

### Handler Interface

The Handler interface defines all filesystem operations. For read-only handlers, use `NewReadOnlyHandler()` which wraps a simple read function and provides default implementations for write operations.

```go
type Handler interface {
    // Read returns directory entries for the given path.
    Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)

    // Write writes data to the file at path. Returns fs.ErrPermission if not supported.
    Write(ctx context.Context, path string, data []byte, params map[string]string) error

    // Remove deletes the file at path. Returns fs.ErrPermission if not supported.
    Remove(ctx context.Context, path string, params map[string]string) error

    // Rename moves a file from oldPath to newPath. Returns fs.ErrPermission if not supported.
    Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error

    // Mkdir creates a directory at path. Returns fs.ErrPermission if not supported.
    Mkdir(ctx context.Context, path string, params map[string]string) error

    // Rmdir removes an empty directory at path. Returns fs.ErrPermission if not supported.
    Rmdir(ctx context.Context, path string, params map[string]string) error

    // Truncate changes the size of the file at path. Returns fs.ErrPermission if not supported.
    Truncate(ctx context.Context, path string, size int64, params map[string]string) error
}
```

**For read-only handlers**, use the convenience wrapper:
```go
fsys.Map("/data", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    return []fs.DirEntry{ragfs.NewFileEntry("data.txt", []byte("content"))}, nil
}))
```

**For writable handlers**, embed `DefaultHandler` and override the methods you need:
```go
type MyHandler struct {
    ragfs.DefaultHandler
    data map[string][]byte
}

func (h *MyHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    // Custom read implementation
}

func (h *MyHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
    // Custom write implementation
}
```

Handlers return `[]fs.DirEntry` where each entry must implement a `Content() []byte` method to provide file contents.

## Development Practices

### Test-Driven Development (TDD)

This project follows strict TDD with RED-GREEN-REFACTOR:

1. **RED**: Write a failing test first
2. **GREEN**: Write minimal code to pass the test
3. **REFACTOR**: Clean up the code while keeping tests passing

**Never write implementation code before writing a test.**

### Running Tests

```bash
# Run all tests
go test -v

# Run specific test
go test -v -run TestName

# Run with coverage
go test -v -cover
```

### Code Organization

- `ragfs.go` - Core library implementation
- `fuse.go` - FUSE bridge implementation
- `ragfs_test.go` - Unit tests for core functionality
- `examples_test.go` - Example test functions and integration tests
- `examples/json/` - JSON FUSE mount example program
- `FUSE.md` - Complete FUSE integration guide

## Common Development Tasks

### Adding a New Feature

1. **Write a failing test** in `ragfs_test.go` or `examples_test.go`
2. **Run tests** to verify it fails: `go test -v`
3. **Implement** the minimal code to pass the test
4. **Run tests** to verify it passes: `go test -v`
5. **Refactor** if needed, ensuring tests still pass
6. **Add Godoc comments** for all exported types and functions
7. **Commit** with descriptive message

### Pattern Matching

Current implementation:
- `{param}` syntax extracts path segments as parameters
- Static segments must match exactly
- First matching route wins (consider implementing most-specific matching)

### Caching

The library includes a high-performance two-layer LRU cache:

**Layer 1**: Caches handler results (`[]fs.DirEntry`) - expensive handler calls
**Layer 2**: Caches file content (`[]byte`) - content extraction from DirEntry

**Key Guidelines**:
- Caching is **opt-in** via `fsys.EnableCache(config)`
- Default config: 1000 entries per layer, 30 second TTL
- Thread-safe with minimal overhead (~1-2ns per access)
- Provides ~8000x speedup for cached reads (1.27ms → 157ns)
- Stats tracking: hits, misses, evictions, entries

**When to enable caching**:
- Handlers make expensive API calls or database queries
- Same paths are accessed repeatedly
- File content extraction is expensive (large payloads)
- FUSE mounts with concurrent access patterns

**Cache invalidation**:
- Use `Invalidate(path)` after updating specific data
- Use `InvalidatePrefix(prefix)` to clear entire trees
- Monitor stats with `Stats()` to track cache effectiveness

**Testing caching**:
- Test handlers with and without cache enabled
- Verify cache hits/misses with `Stats()`
- Test TTL expiry by waiting and re-accessing
- Test invalidation clears correct entries
- Test concurrent access with goroutines

**Example**:
```go
fsys := ragfs.New()
fsys.EnableCache(ragfs.CacheConfig{
    MaxEntries: 500,
    TTL:        60 * time.Second,
})

// ... later, when data changes
fsys.Invalidate("/users/123")
fsys.InvalidatePrefix("/users/")  // Clear all users
```

### Write Operations

ragfs supports full write operations through the Handler interface:

**FS Methods:**
- `WriteFile(name string, data []byte) error` - Write data to a file
- `Remove(name string) error` - Delete a file
- `Rename(oldPath, newPath string) error` - Rename/move a file
- `Mkdir(name string) error` - Create a directory
- `Rmdir(name string) error` - Remove an empty directory
- `Truncate(name string, size int64) error` - Change file size

**FUSE Support:**
All write operations are supported via FUSE when mounting with go-fuse:
- File creation (NodeCreater)
- File deletion (NodeUnlinker)
- File rename (NodeRenamer)
- Directory creation (NodeMkdirer)
- Directory removal (NodeRmdirer)
- Truncation (NodeSetattrer)

See `examples/writable/` for a complete in-memory writable filesystem example.

### Future Enhancements

1. **Symbolic links**: Support Symlink/Readlink operations
2. **Extended attributes**: Setxattr/Getxattr/Removexattr
3. **fs.StatFS interface**: Stat without opening files
4. **fs.ReadFileFS interface**: Read entire files efficiently
5. **Hard links**: Link operation support

## Example Patterns

### Static File Mapping

```go
fsys.Map("/config.json", func(ctx, path, params) ([]fs.DirEntry, error) {
    content := []byte(`{"version": "1.0"}`)
    return []fs.DirEntry{&fileEntry{name: "config.json", content: content}}, nil
})
```

### Dynamic Path Parameters

```go
fsys.Map("/users/{id}", func(ctx, path, params) ([]fs.DirEntry, error) {
    user := fetchUser(params["id"])
    content := user.ToJSON()
    return []fs.DirEntry{&fileEntry{name: params["id"], content: content}}, nil
})
```

### JSON Data Mapping

See `examples_test.go` for a complete example of mapping filesystem paths to JSON object queries.

## Guidelines for Claude Code

When working on this project:

1. **Always write tests first** - Follow TDD religiously
2. **Run tests frequently** - After every change
3. **Keep commits atomic** - One logical change per commit
4. **Document exported symbols** - All public types/functions need Godoc
5. **Maintain backward compatibility** - Don't break existing APIs
6. **Prefer simplicity** - Minimal code to solve the problem
7. **Handle errors explicitly** - Return appropriate fs.PathError values

### Verification Requirements

**MANDATORY RULE**: NO TASK IS COMPLETE until ALL example filesystems have been tested with real builds using foreground shell commands and Unix filesystem tools.

**CRITICAL**: Before claiming any work is complete, you MUST verify:

1. **Code compiles**: `go build ./...` must succeed
2. **Tests pass**: `go test -v ./...` must pass with no failures
3. **ALL examples verified in FOREGROUND**: Build and test EVERY example filesystem with real commands

**For EVERY task, you MUST test ALL examples:**

1. **JSON Example**:
   ```bash
   # Build
   go build -o json-mount examples/json/main.go

   # Mount and verify
   ./json-mount -mount /tmp/json-test -config config.json &
   MOUNT_PID=$!
   sleep 2

   # Test with Unix tools
   ls /tmp/json-test/
   cat /tmp/json-test/app/name
   ls /tmp/json-test/app/

   # Cleanup
   kill $MOUNT_PID
   ```

2. **SQLite Example**:
   ```bash
   # Build
   go build -o sqlite-mount examples/sqlite/main.go

   # Mount and verify
   ./sqlite-mount -mount /tmp/sqlite-test -db examples/sqlite/example.db &
   MOUNT_PID=$!
   sleep 2

   # Test with Unix tools
   ls /tmp/sqlite-test/
   ls /tmp/sqlite-test/users/
   cat /tmp/sqlite-test/users/1.json
   cat /tmp/sqlite-test/users/_metrics/row_count

   # Cleanup
   kill $MOUNT_PID
   ```

3. **Writable Example**:
   ```bash
   # Build
   go build -o writable-mount examples/writable/main.go

   # Mount and verify
   ./writable-mount -mount /tmp/writable-test &
   MOUNT_PID=$!
   sleep 2

   # Test write operations
   echo "hello" > /tmp/writable-test/test.txt
   cat /tmp/writable-test/test.txt
   mkdir /tmp/writable-test/mydir
   ls /tmp/writable-test/
   rmdir /tmp/writable-test/mydir
   rm /tmp/writable-test/test.txt

   # Cleanup
   kill $MOUNT_PID
   ```

**ABSOLUTE REQUIREMENTS:**
- ❌ **NEVER** use background bash processes for verification
- ❌ **NEVER** trust output from worktrees when on main branch
- ❌ **NEVER** claim completion without testing ALL examples
- ✅ **ALWAYS** run commands in FOREGROUND
- ✅ **ALWAYS** test with `ls` and `cat` to prove it works
- ✅ **ALWAYS** show actual command output

**Never claim success based on:**
- Code that compiles but hasn't been tested with ALL examples
- Tests that pass but haven't been verified in real filesystem usage
- Output from background processes
- Assumptions about what "should" work
- Testing only ONE example when multiple exist

**Evidence required:**
- Show compilation output for ALL examples
- Show test output: `go test -v ./...`
- Show FOREGROUND execution of EVERY example with Unix tool verification

## Testing Strategy

### Unit Tests
- Test individual functions in isolation
- Mock external dependencies
- Focus on edge cases and error conditions

### Integration Tests
- Test complete workflows (example handlers)
- Verify fs.FS interface compliance
- Demonstrate real-world usage patterns

### Test Helpers
- `testFileEntry`: Implements fs.DirEntry with Content() method
- `jsonFileEntry`: For JSON-based examples

## Godoc Standards

All exported symbols must have Godoc comments:
- Start with the name of the symbol
- Use complete sentences
- Provide examples for complex functionality
- Reference related functions/types when relevant

Example:
```go
// New creates a new ragfs filesystem.
// The returned FS implements the fs.FS interface and can be used
// with any function that accepts an fs.FS.
func New() *FS
```
