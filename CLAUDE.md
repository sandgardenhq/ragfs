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

### Handler Signature

```go
type Handler func(
    ctx context.Context,    // For cancellation and request-scoped values
    path string,            // Full path being accessed (e.g., "/emails/2025-10-07")
    params map[string]string // Extracted parameters (e.g., {"date": "2025-10-07"})
) ([]fs.DirEntry, error)
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
- `examples/mount/` - FUSE mount example program
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

### Future Enhancements

1. **ReadDir support**: Enable directory listing operations
2. **Most-specific matching**: Choose most specific route when multiple match
3. **Wildcard patterns**: Support `/**` for capturing remaining path segments
4. **Caching helpers**: Provide utilities for common caching patterns
5. **Middleware**: Support for logging, metrics, authentication
6. **Directory entries**: Better support for handlers that return directories

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

**CRITICAL**: Before claiming any work is complete, you MUST verify three things:

1. **Code compiles**: `go build ./...` must succeed
2. **Tests pass**: `go test -v ./...` must pass with no failures
3. **Real-world verification**: For FUSE mounts and examples, actually run the program and verify it works as expected

**For FUSE mount changes specifically:**
- Build the mount example: `go build -o ragfs-mount examples/mount/main.go`
- Run with timeout to verify it mounts quickly: Mount should complete in <3 seconds
- Verify mount point is accessible: `ls /tmp/ragfs-mount` should work
- Verify files are readable: `cat /tmp/ragfs-mount/app/name` should return content
- Verify directories list correctly: `ls /tmp/ragfs-mount/app/` should show entries

**Never claim success based on:**
- Code that compiles but hasn't been tested
- Tests that pass but haven't been verified in real use
- Output from background processes or worktrees when working on main branch
- Assumptions about what "should" work

**Evidence required:**
- Show compilation output
- Show test output
- Show real-world usage with actual command output demonstrating the feature works

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
