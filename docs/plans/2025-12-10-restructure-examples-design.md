# Examples Restructuring Design

**Date:** 2025-12-10
**Status:** Approved
**Author:** Claude Code

## Overview

Restructure the ragfs project to move example code into a dedicated `examples/` directory, clarifying that the current `cmd/ragfs-mount` is an example implementation rather than a general-purpose command.

## Problem Statement

Currently, the project has:
- `example.go` - Standalone example file in root
- `examples_test.go` - Example test functions in root
- `cmd/ragfs-mount/` - FUSE mount implementation that appears to be a production tool

This structure creates confusion about what is library code versus examples. The `cmd/ragfs-mount` appears to be a general-purpose command when it's actually a demonstration of how to use the ragfs library with FUSE.

## Goals

1. Clearly separate library code from example code
2. Make it obvious that `ragfs-mount` is an example, not a production tool
3. Maintain all example functionality while improving organization
4. Follow Go conventions for example code and documentation

## Design

### Directory Structure

```
ragfs/
├── ragfs.go                    # Core library (unchanged)
├── ragfs_test.go              # Unit tests (unchanged)
├── fuse.go                    # FUSE bridge (unchanged)
├── examples_test.go           # Example test functions for godoc (enhanced)
├── examples/
│   └── mount/
│       └── main.go            # FUSE mount example (from cmd/ragfs-mount)
└── docs/
    └── plans/
        └── 2025-12-10-restructure-examples-design.md
```

### Key Changes

1. **Move cmd/ragfs-mount → examples/mount**
   - Path: `cmd/ragfs-mount/main.go` → `examples/mount/main.go`
   - Remains a runnable program (`package main`)
   - Clearly identified as an example

2. **Convert example.go → Example test functions**
   - Content from `example.go` becomes `ExampleFS()`, `ExampleMap()`, etc.
   - Added to `examples_test.go`
   - Automatically appears in godoc
   - Testable via `go test`

3. **Remove obsolete files**
   - Delete `example.go` (content migrated)
   - Delete `cmd/` directory (moved to examples)

4. **Enhance examples_test.go**
   - Keep existing JSON handler test example
   - Add new Example functions from `example.go`

### Example Test Functions

Example test functions follow Go's convention:
```go
func ExampleFS() {
    fsys := ragfs.New()
    // ... example code ...
    // Output:
    // expected output
}
```

Benefits:
- Appear in `go doc` output
- Tested automatically by `go test`
- Self-documenting with output verification

### Migration Path

1. Create `examples/mount/` directory
2. Move `cmd/ragfs-mount/main.go` to `examples/mount/main.go`
3. Extract key concepts from `example.go` into Example test functions
4. Add Example functions to `examples_test.go`
5. Remove `example.go`
6. Remove `cmd/` directory
7. Update documentation references

## Testing Strategy

1. **Verify all tests pass** before and after restructuring
2. **Build examples/mount** to ensure it still compiles
3. **Check godoc output** to verify Example functions appear correctly
4. **Run Example tests** to verify output matches expected

## Documentation Updates

Update references to examples in:
- README.md
- CLAUDE.md (if referenced)
- FUSE.md (update path to mount example)

## Non-Goals

- Changing functionality of existing code
- Modifying the core library (ragfs.go, fuse.go)
- Changing test behavior or coverage
- Adding new features

## Success Criteria

1. All existing tests continue to pass
2. `examples/mount/main.go` builds and runs successfully
3. Example functions appear in godoc output
4. Project structure clearly separates library from examples
5. No broken references in documentation
