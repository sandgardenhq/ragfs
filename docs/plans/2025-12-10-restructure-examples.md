# Examples Restructuring Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Restructure ragfs project to move examples and cmd into dedicated examples/ directory

**Architecture:** Move cmd/ragfs-mount to examples/mount, convert example.go functions to Go Example test functions in examples_test.go

**Tech Stack:** Go 1.x, io/fs, testing package

---

## Task 1: Create examples directory structure

**Files:**
- Create: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/examples/mount/main.go`

**Step 1: Create examples/mount directory**

Run: `mkdir -p examples/mount`
Expected: Directory created successfully

**Step 2: Copy cmd/ragfs-mount/main.go to examples/mount/main.go**

Run: `cp cmd/ragfs-mount/main.go examples/mount/main.go`
Expected: File copied successfully

**Step 3: Verify the file was copied correctly**

Run: `ls -la examples/mount/main.go`
Expected: File exists at new location

**Step 4: Build the example to verify it compiles**

Run: `go build -o /tmp/ragfs-mount-test ./examples/mount/main.go`
Expected: Binary builds without errors

**Step 5: Clean up test binary**

Run: `rm /tmp/ragfs-mount-test`
Expected: Test binary removed

**Step 6: Commit**

```bash
git add examples/
git commit -m "Add examples/mount directory with FUSE mount example

Moved from cmd/ragfs-mount to clarify this is an example

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 2: Convert example.go to Example test functions

**Files:**
- Modify: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/examples_test.go`
- Reference: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/example.go`

**Step 1: Read example.go to understand what needs to be converted**

The file contains three example functions:
- `Example()` - Basic config mapping
- `EmailExample()` - Email fetching by date
- `JSONPathExample()` - Nested JSON navigation

**Step 2: Add ExampleFS test function to examples_test.go**

Add this after the existing TestJSONHandler:

```go
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
```

**Step 3: Add ExampleFS_emails test function to examples_test.go**

Add this after ExampleFS:

```go
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
```

**Step 4: Add ExampleFS_jsonPath test function to examples_test.go**

Add this after ExampleFS_emails:

```go
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
```

**Step 5: Add fmt import to examples_test.go**

Add `"fmt"` to the import block if not already present:

```go
import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	"github.com/brittcrawford/ragfs"
)
```

**Step 6: Run tests to verify Example functions work**

Run: `go test -v`
Expected: All tests pass including the new Example functions

**Step 7: Commit**

```bash
git add examples_test.go
git commit -m "Add Example test functions for godoc

Converted example.go content to testable Example functions

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 3: Remove obsolete files

**Files:**
- Delete: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/example.go`
- Delete: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/cmd/` (directory)

**Step 1: Remove example.go**

Run: `git rm example.go`
Expected: File staged for deletion

**Step 2: Remove cmd directory**

Run: `git rm -r cmd/`
Expected: Directory and contents staged for deletion

**Step 3: Run tests to ensure nothing breaks**

Run: `go test -v`
Expected: All tests still pass

**Step 4: Build examples/mount to verify it still works**

Run: `go build -o /tmp/ragfs-mount-test ./examples/mount/main.go`
Expected: Binary builds successfully

**Step 5: Clean up test binary**

Run: `rm /tmp/ragfs-mount-test`
Expected: Test binary removed

**Step 6: Commit**

```bash
git add -A
git commit -m "Remove example.go and cmd/ directory

Migrated content to examples/ and Example test functions

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 4: Update documentation references

**Files:**
- Modify: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/README.md`
- Modify: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/CLAUDE.md`
- Modify: `/Users/brittcrawford/workspace/ragfs/.worktrees/restructure-examples/FUSE.md`

**Step 1: Read README.md to find references to examples**

Run: `grep -n "example\|cmd" README.md`
Expected: Shows lines that reference examples or cmd

**Step 2: Update README.md references**

Update any references to:
- `cmd/ragfs-mount` → `examples/mount`
- `example.go` → `examples_test.go` (Example functions)

**Step 3: Read CLAUDE.md to find references**

Run: `grep -n "example\|cmd" CLAUDE.md`
Expected: Shows lines that reference examples or cmd

**Step 4: Update CLAUDE.md references**

Update the "Code Organization" section:

```markdown
### Code Organization

- `ragfs.go` - Core library implementation
- `fuse.go` - FUSE bridge implementation
- `ragfs_test.go` - Unit tests for core functionality
- `examples_test.go` - Example test functions and integration tests
- `examples/mount/` - FUSE mount example program
- `FUSE.md` - Complete FUSE integration guide
```

**Step 5: Read FUSE.md to find references**

Run: `grep -n "cmd/ragfs-mount" FUSE.md`
Expected: Shows lines that reference cmd/ragfs-mount

**Step 6: Update FUSE.md references**

Replace all instances of `cmd/ragfs-mount` with `examples/mount`

**Step 7: Run tests to ensure everything still works**

Run: `go test -v`
Expected: All tests pass

**Step 8: Commit**

```bash
git add README.md CLAUDE.md FUSE.md
git commit -m "Update documentation to reflect examples restructuring

Updated paths: cmd/ragfs-mount → examples/mount

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 5: Final verification

**Files:**
- All modified files

**Step 1: Run all tests with coverage**

Run: `go test -v -cover`
Expected: All tests pass with coverage report

**Step 2: Build examples/mount program**

Run: `go build -o ragfs-mount ./examples/mount/main.go`
Expected: Binary builds successfully

**Step 3: Verify Example functions appear in godoc**

Run: `go doc -all | grep -A 5 "^func Example"`
Expected: Shows all Example functions with their documentation

**Step 4: Check project structure**

Run: `ls -la`
Expected: Shows new structure without example.go or cmd/

**Step 5: Run git status to verify clean state**

Run: `git status`
Expected: Working directory clean, all changes committed

**Step 6: View commit log**

Run: `git log --oneline -5`
Expected: Shows 4-5 commits for the restructuring

---

## Notes

- Follow TDD as specified in CLAUDE.md
- Ensure all tests pass before committing
- Use atomic commits (one logical change per commit)
- Keep Example test functions simple and focused
- Ensure godoc output is clean and readable
