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
- ✅ Directory listing (ReadDir) - planned
- 🚧 Most-specific route matching - planned
- 🚧 Wildcard patterns - planned

## License

MIT - See [LICENSE](LICENSE) for details.
