# Write Operations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add write operations (WriteFile, Remove, Rename) to ragfs filesystem.

**Architecture:** Convert Handler from function type to interface with separate methods for read/write operations. Provide default implementation that returns fs.ErrPermission for unimplemented write methods. Auto-invalidate cache on successful writes.

**Tech Stack:** Go 1.21+, io/fs package, existing ragfs architecture

---

## Task 1: Convert Handler to Interface

**Files:**
- Modify: `ragfs.go:28-30`
- Modify: `ragfs.go:226-239` (Map method)
- Test: `ragfs_test.go`

### Step 1: Write failing test for new Handler interface

Create test file section in `ragfs_test.go`:

```go
func TestHandlerInterface(t *testing.T) {
	fsys := New()

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{NewFileEntry("test.txt", []byte("hello"))}, nil
		},
	}

	fsys.Map("/test", handler)

	f, err := fsys.Open("/test")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got %q", string(data))
	}
}

type testHandler struct {
	readFunc   func(context.Context, string, map[string]string) ([]fs.DirEntry, error)
	writeFunc  func(context.Context, string, []byte, map[string]string) error
	removeFunc func(context.Context, string, map[string]string) error
	renameFunc func(context.Context, string, string, map[string]string) error
}

func (h *testHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	if h.readFunc != nil {
		return h.readFunc(ctx, path, params)
	}
	return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrPermission}
}

func (h *testHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
	if h.writeFunc != nil {
		return h.writeFunc(ctx, path, data, params)
	}
	return &fs.PathError{Op: "write", Path: path, Err: fs.ErrPermission}
}

func (h *testHandler) Remove(ctx context.Context, path string, params map[string]string) error {
	if h.removeFunc != nil {
		return h.removeFunc(ctx, path, params)
	}
	return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrPermission}
}

func (h *testHandler) Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error {
	if h.renameFunc != nil {
		return h.renameFunc(ctx, oldPath, newPath, params)
	}
	return &fs.PathError{Op: "rename", Path: oldPath, Err: fs.ErrPermission}
}
```

### Step 2: Run test to verify it fails

Run: `go test -v -run TestHandlerInterface`
Expected: FAIL with compilation error "cannot use handler (type *testHandler) as type Handler"

### Step 3: Convert Handler to interface

In `ragfs.go:28-30`, replace:

```go
// Handler is a function that handles filesystem operations for a given path.
// It receives the full path, extracted parameters, and returns directory entries.
type Handler func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)
```

With:

```go
// Handler is an interface that handles filesystem operations for a given path.
// Implementations should use DefaultHandler as a base to only implement needed methods.
type Handler interface {
	// Read handles read operations, returning directory entries for the path.
	Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)

	// Write handles write operations, creating or updating a file with the given data.
	// Returns fs.ErrPermission if writes are not supported.
	Write(ctx context.Context, path string, data []byte, params map[string]string) error

	// Remove handles delete operations, removing the file or directory at path.
	// Returns fs.ErrPermission if removal is not supported.
	Remove(ctx context.Context, path string, params map[string]string) error

	// Rename handles rename/move operations from oldPath to newPath.
	// Returns fs.ErrPermission if rename is not supported.
	Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error
}
```

### Step 4: Add DefaultHandler implementation

Add after the Handler interface in `ragfs.go`:

```go
// DefaultHandler provides default implementations that return fs.ErrPermission.
// Embed this in your handler to only implement the methods you need.
type DefaultHandler struct{}

func (h *DefaultHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrPermission}
}

func (h *DefaultHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
	return &fs.PathError{Op: "write", Path: path, Err: fs.ErrPermission}
}

func (h *DefaultHandler) Remove(ctx context.Context, path string, params map[string]string) error {
	return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrPermission}
}

func (h *DefaultHandler) Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error {
	return &fs.PathError{Op: "rename", Path: oldPath, Err: fs.ErrPermission}
}
```

### Step 5: Update registerMetricsHandlers to use new interface

This will require creating adapter handlers. Add helper type before registerMetricsHandlers:

```go
// readOnlyHandler wraps a read function to implement the Handler interface.
type readOnlyHandler struct {
	DefaultHandler
	readFunc func(context.Context, string, map[string]string) ([]fs.DirEntry, error)
}

func (h *readOnlyHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	return h.readFunc(ctx, path, params)
}
```

Then in registerMetricsHandlers, wrap each function. For example, line 63-72:

```go
f.Map("/_metrics", &readOnlyHandler{
	readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return NewDirectoryListing([]fs.DirEntry{
			NewFileEntry("version.txt", []byte("v1")),
			NewFileEntry("summary.md", []byte(GenerateSummaryMarkdown(f.collector.Snapshot()))),
			NewDirEntry("cache", true),
			NewDirEntry("io", true),
			NewDirEntry("system", true),
			NewDirEntry("errors", true),
		}), nil
	},
})
```

Repeat for all metric handlers (this is tedious but necessary).

### Step 6: Update withMetricsPath to work with new interface

In `ragfs.go:210-221`, replace:

```go
func withMetricsPath(handler Handler) Handler {
	return func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		entries, err := handler(ctx, path, params)
		if err != nil {
			return nil, err
		}
		if path == "/" {
			entries = append(entries, NewDirEntry("_metrics", true))
		}
		return entries, nil
	}
}
```

With:

```go
func withMetricsPath(handler Handler) Handler {
	return &metricsPathHandler{handler: handler}
}

type metricsPathHandler struct {
	handler Handler
}

func (h *metricsPathHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	entries, err := h.handler.Read(ctx, path, params)
	if err != nil {
		return nil, err
	}
	if path == "/" {
		entries = append(entries, NewDirEntry("_metrics", true))
	}
	return entries, nil
}

func (h *metricsPathHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
	return h.handler.Write(ctx, path, data, params)
}

func (h *metricsPathHandler) Remove(ctx context.Context, path string, params map[string]string) error {
	return h.handler.Remove(ctx, path, params)
}

func (h *metricsPathHandler) Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error {
	return h.handler.Rename(ctx, oldPath, newPath, params)
}
```

### Step 7: Update ReadDir to call handler.Read

In `ragfs.go:273-284`, change line 279:

```go
entries, err := matched.route.handler(context.Background(), name, matched.params)
```

To:

```go
entries, err := matched.route.handler.Read(context.Background(), name, matched.params)
```

### Step 8: Update Open to call handler.Read

In `ragfs.go:292-380`, change line 311:

```go
entries, err = matched.route.handler(context.Background(), name, matched.params)
```

To:

```go
entries, err = matched.route.handler.Read(context.Background(), name, matched.params)
```

### Step 9: Run test to verify it passes

Run: `go test -v -run TestHandlerInterface`
Expected: PASS

### Step 10: Commit

```bash
git add ragfs.go ragfs_test.go
git commit -m "feat: convert Handler from function type to interface

- Handler is now an interface with Read/Write/Remove/Rename methods
- DefaultHandler provides base implementation returning fs.ErrPermission
- Updated all internal handlers to use new interface
- Backward compatibility broken intentionally per requirements"
```

---

## Task 2: Convert JSON Example to New Interface

**Files:**
- Modify: `examples/json/main.go:46-153`

### Step 1: Wrap existing handler logic

In `examples/json/main.go`, replace the handler function (lines 47-153) with:

```go
// Create handler that implements ragfs.Handler interface
	type jsonHandler struct {
		ragfs.DefaultHandler
		config map[string]any
	}

	handler := &jsonHandler{config: config}

	// Map all possible paths
	fsys.Map("/*", handler)
```

### Step 2: Implement Read method

Add after the handler creation:

```go
func (h *jsonHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	// Convert /config/name to ["config", "name"]
	trimmed := strings.Trim(path, "/")
	var parts []string
	if trimmed != "" {
		parts = strings.Split(trimmed, "/")
	}

	// Navigate through JSON
	var current any = h.config
	for _, part := range parts {
		if part == "" {
			continue
		}
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

	// Handle based on type
	switch v := current.(type) {
	case map[string]any:
		var entries []fs.DirEntry
		for key, value := range v {
			isDir := false
			switch value.(type) {
			case map[string]any, []any:
				isDir = true
			}
			entries = append(entries, &dirEntry{
				name:  key,
				isDir: isDir,
			})
		}
		return entries, nil

	case []any:
		var entries []fs.DirEntry
		for i, value := range v {
			isDir := false
			switch value.(type) {
			case map[string]any, []any:
				isDir = true
			}
			entries = append(entries, &dirEntry{
				name:  fmt.Sprintf("%d", i),
				isDir: isDir,
			})
		}
		return entries, nil

	case string:
		content := []byte(v)
		return []fs.DirEntry{
			&fileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil

	case float64:
		content := []byte(fmt.Sprintf("%v", v))
		return []fs.DirEntry{
			&fileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil

	case bool:
		content := []byte(fmt.Sprintf("%v", v))
		return []fs.DirEntry{
			&fileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil

	default:
		content, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, err
		}
		return []fs.DirEntry{
			&fileEntry{
				name:    parts[len(parts)-1],
				content: content,
			},
		}, nil
	}
}
```

### Step 3: Test JSON example compiles

Run: `go build -o examples/json/ragfs-mount ./examples/json`
Expected: Success

### Step 4: Test JSON example works

Run:
```bash
./examples/json/ragfs-mount -mount /tmp/json-test -config config.json &
MOUNT_PID=$!
sleep 2
ls /tmp/json-test/
cat /tmp/json-test/app/name
kill $MOUNT_PID
```
Expected: Lists directories, reads "MyApp"

### Step 5: Commit

```bash
git add examples/json/main.go
git commit -m "refactor: convert JSON example to use Handler interface"
```

---

## Task 3: Convert SQLite Example to New Interface

**Files:**
- Modify: `examples/sqlite/main.go`

### Step 1: Find and wrap SQLite handlers

In `examples/sqlite/main.go`, locate all `fsys.Map()` calls and wrap the handler functions with the readOnlyHandler pattern similar to the JSON example.

Create a helper type:

```go
type sqliteHandler struct {
	ragfs.DefaultHandler
	readFunc func(context.Context, string, map[string]string) ([]fs.DirEntry, error)
}

func (h *sqliteHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	return h.readFunc(ctx, path, params)
}
```

Then wrap each handler, for example:

```go
fsys.Map("/", &sqliteHandler{
	readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		// ... existing logic
	},
})
```

### Step 2: Test SQLite example compiles

Run: `go build -o examples/sqlite/sqlite-mount ./examples/sqlite`
Expected: Success

### Step 3: Test SQLite example works

Run:
```bash
./examples/sqlite/sqlite-mount -mount /tmp/sqlite-test -db examples/sqlite/example.db &
MOUNT_PID=$!
sleep 2
ls /tmp/sqlite-test/
cat /tmp/sqlite-test/users/1.json
kill $MOUNT_PID
```
Expected: Lists tables, reads user data

### Step 4: Commit

```bash
git add examples/sqlite/main.go
git commit -m "refactor: convert SQLite example to use Handler interface"
```

---

## Task 4: Add WriteFile Operation

**Files:**
- Modify: `ragfs.go` (add WriteFile method to FS)
- Test: `ragfs_test.go`

### Step 1: Write failing test for WriteFile

Add to `ragfs_test.go`:

```go
func TestWriteFile(t *testing.T) {
	fsys := New()

	// In-memory storage for writes
	storage := make(map[string][]byte)

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			data, exists := storage[path]
			if !exists {
				return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
			}
			return []fs.DirEntry{NewFileEntry("test.txt", data)}, nil
		},
		writeFunc: func(ctx context.Context, path string, data []byte, params map[string]string) error {
			storage[path] = data
			return nil
		},
	}

	fsys.Map("/test", handler)

	// Write data
	err := fsys.WriteFile("/test", []byte("hello world"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read back
	f, err := fsys.Open("/test")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(data) != "hello world" {
		t.Errorf("Expected 'hello world', got %q", string(data))
	}
}
```

### Step 2: Run test to verify it fails

Run: `go test -v -run TestWriteFile`
Expected: FAIL with "fsys.WriteFile undefined"

### Step 3: Implement WriteFile method

Add to `ragfs.go` after the Open method:

```go
// WriteFile writes data to the named file.
// It matches the path against registered patterns and calls the handler's Write method.
// If caching is enabled, the cache is invalidated for the written path.
// Returns fs.ErrNotExist if no pattern matches, or fs.ErrPermission if the handler
// doesn't support writes.
func (f *FS) WriteFile(name string, data []byte) error {
	matched := f.findLongestMatch(name)
	if matched == nil {
		return &fs.PathError{Op: "write", Path: name, Err: fs.ErrNotExist}
	}

	// Call the handler's Write method
	err := matched.route.handler.Write(context.Background(), name, data, matched.params)
	if err != nil {
		return err
	}

	// Invalidate cache on successful write
	if f.cache != nil {
		f.cache.invalidate(name)
	}
	if f.boltDBCache != nil {
		f.boltDBCache.invalidate(name)
	}

	// Record write metrics
	f.collector.RecordWrite(int64(len(data)))

	return nil
}
```

### Step 4: Run test to verify it passes

Run: `go test -v -run TestWriteFile`
Expected: PASS

### Step 5: Test write operation doesn't work with read-only handler

Add test:

```go
func TestWriteFilePermissionDenied(t *testing.T) {
	fsys := New()

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{NewFileEntry("test.txt", []byte("readonly"))}, nil
		},
		// No writeFunc - will use default that returns ErrPermission
	}

	fsys.Map("/test", handler)

	err := fsys.WriteFile("/test", []byte("data"))
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected fs.ErrPermission, got %v", err)
	}
}
```

Run: `go test -v -run TestWriteFilePermissionDenied`
Expected: PASS

### Step 6: Commit

```bash
git add ragfs.go ragfs_test.go
git commit -m "feat: add WriteFile operation with cache invalidation

- WriteFile method finds matching handler and calls Write
- Auto-invalidates cache on successful write
- Returns fs.ErrPermission if handler doesn't support writes
- Records write metrics"
```

---

## Task 5: Add Remove Operation

**Files:**
- Modify: `ragfs.go`
- Test: `ragfs_test.go`

### Step 1: Write failing test for Remove

Add to `ragfs_test.go`:

```go
func TestRemove(t *testing.T) {
	fsys := New()

	storage := map[string][]byte{
		"/test": []byte("data"),
	}

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			data, exists := storage[path]
			if !exists {
				return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
			}
			return []fs.DirEntry{NewFileEntry("test", data)}, nil
		},
		removeFunc: func(ctx context.Context, path string, params map[string]string) error {
			if _, exists := storage[path]; !exists {
				return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrNotExist}
			}
			delete(storage, path)
			return nil
		},
	}

	fsys.Map("/test", handler)

	// Verify file exists
	_, err := fsys.Open("/test")
	if err != nil {
		t.Fatalf("File should exist: %v", err)
	}

	// Remove file
	err = fsys.Remove("/test")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify file is gone
	_, err = fsys.Open("/test")
	if err == nil {
		t.Fatal("File should not exist after Remove")
	}
}
```

### Step 2: Run test to verify it fails

Run: `go test -v -run TestRemove`
Expected: FAIL with "fsys.Remove undefined"

### Step 3: Implement Remove method

Add to `ragfs.go` after WriteFile:

```go
// Remove removes the named file or directory.
// It matches the path against registered patterns and calls the handler's Remove method.
// If caching is enabled, the cache is invalidated for the removed path.
// Returns fs.ErrNotExist if no pattern matches, or fs.ErrPermission if the handler
// doesn't support removal.
func (f *FS) Remove(name string) error {
	matched := f.findLongestMatch(name)
	if matched == nil {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
	}

	// Call the handler's Remove method
	err := matched.route.handler.Remove(context.Background(), name, matched.params)
	if err != nil {
		return err
	}

	// Invalidate cache on successful removal
	if f.cache != nil {
		f.cache.invalidate(name)
	}
	if f.boltDBCache != nil {
		f.boltDBCache.invalidate(name)
	}

	return nil
}
```

### Step 4: Run test to verify it passes

Run: `go test -v -run TestRemove`
Expected: PASS

### Step 5: Test Remove with permission denied

Add test:

```go
func TestRemovePermissionDenied(t *testing.T) {
	fsys := New()

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{NewFileEntry("test", []byte("readonly"))}, nil
		},
		// No removeFunc
	}

	fsys.Map("/test", handler)

	err := fsys.Remove("/test")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected fs.ErrPermission, got %v", err)
	}
}
```

Run: `go test -v -run TestRemovePermissionDenied`
Expected: PASS

### Step 6: Commit

```bash
git add ragfs.go ragfs_test.go
git commit -m "feat: add Remove operation with cache invalidation

- Remove method finds matching handler and calls Remove
- Auto-invalidates cache on successful removal
- Returns fs.ErrPermission if handler doesn't support removal"
```

---

## Task 6: Add Rename Operation

**Files:**
- Modify: `ragfs.go`
- Test: `ragfs_test.go`

### Step 1: Write failing test for Rename

Add to `ragfs_test.go`:

```go
func TestRename(t *testing.T) {
	fsys := New()

	storage := map[string][]byte{
		"/old": []byte("data"),
	}

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			data, exists := storage[path]
			if !exists {
				return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
			}
			name := strings.TrimPrefix(path, "/")
			return []fs.DirEntry{NewFileEntry(name, data)}, nil
		},
		renameFunc: func(ctx context.Context, oldPath, newPath string, params map[string]string) error {
			data, exists := storage[oldPath]
			if !exists {
				return &fs.PathError{Op: "rename", Path: oldPath, Err: fs.ErrNotExist}
			}
			storage[newPath] = data
			delete(storage, oldPath)
			return nil
		},
	}

	fsys.Map("/*", handler)

	// Verify old file exists
	f, err := fsys.Open("/old")
	if err != nil {
		t.Fatalf("Old file should exist: %v", err)
	}
	f.Close()

	// Rename file
	err = fsys.Rename("/old", "/new")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Verify old file is gone
	_, err = fsys.Open("/old")
	if err == nil {
		t.Fatal("Old file should not exist after Rename")
	}

	// Verify new file exists
	f, err = fsys.Open("/new")
	if err != nil {
		t.Fatalf("New file should exist: %v", err)
	}
	defer f.Close()

	data, _ := io.ReadAll(f)
	if string(data) != "data" {
		t.Errorf("Expected 'data', got %q", string(data))
	}
}
```

### Step 2: Run test to verify it fails

Run: `go test -v -run TestRename`
Expected: FAIL with "fsys.Rename undefined"

### Step 3: Implement Rename method

Add to `ragfs.go` after Remove:

```go
// Rename renames (moves) oldPath to newPath.
// It matches oldPath against registered patterns and calls the handler's Rename method.
// If caching is enabled, both old and new paths are invalidated.
// Returns fs.ErrNotExist if no pattern matches, or fs.ErrPermission if the handler
// doesn't support rename.
func (f *FS) Rename(oldPath, newPath string) error {
	matched := f.findLongestMatch(oldPath)
	if matched == nil {
		return &fs.PathError{Op: "rename", Path: oldPath, Err: fs.ErrNotExist}
	}

	// Call the handler's Rename method
	err := matched.route.handler.Rename(context.Background(), oldPath, newPath, matched.params)
	if err != nil {
		return err
	}

	// Invalidate cache for both paths on successful rename
	if f.cache != nil {
		f.cache.invalidate(oldPath)
		f.cache.invalidate(newPath)
	}
	if f.boltDBCache != nil {
		f.boltDBCache.invalidate(oldPath)
		f.boltDBCache.invalidate(newPath)
	}

	return nil
}
```

### Step 4: Run test to verify it passes

Run: `go test -v -run TestRename`
Expected: PASS

### Step 5: Test Rename with permission denied

Add test:

```go
func TestRenamePermissionDenied(t *testing.T) {
	fsys := New()

	handler := &testHandler{
		readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
			return []fs.DirEntry{NewFileEntry("old", []byte("readonly"))}, nil
		},
		// No renameFunc
	}

	fsys.Map("/old", handler)

	err := fsys.Rename("/old", "/new")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var pathErr *fs.PathError
	if !errors.As(err, &pathErr) || pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected fs.ErrPermission, got %v", err)
	}
}
```

Run: `go test -v -run TestRenamePermissionDenied`
Expected: PASS

### Step 6: Commit

```bash
git add ragfs.go ragfs_test.go
git commit -m "feat: add Rename operation with cache invalidation

- Rename method finds matching handler and calls Rename
- Auto-invalidates cache for both old and new paths
- Returns fs.ErrPermission if handler doesn't support rename"
```

---

## Task 7: Update Documentation

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md` (if exists)

### Step 1: Update CLAUDE.md Handler section

In `CLAUDE.md`, find the "Handler Signature" section and update it:

```markdown
### Handler Interface

```go
type Handler interface {
    // Read handles read operations, returning directory entries for the path.
    Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)

    // Write handles write operations, creating or updating a file.
    Write(ctx context.Context, path string, data []byte, params map[string]string) error

    // Remove handles delete operations.
    Remove(ctx context.Context, path string, params map[string]string) error

    // Rename handles rename/move operations.
    Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error
}
```

Handlers implement this interface. Use `ragfs.DefaultHandler` as a base to only implement needed methods - unimplemented methods return `fs.ErrPermission`.
```

### Step 2: Add write operations examples

Add after the existing examples:

```markdown
### Read-Only Handler

```go
type MyHandler struct {
    ragfs.DefaultHandler  // Provides Write/Remove/Rename that return ErrPermission
}

func (h *MyHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    // Your read logic
}
```

### Read-Write Handler

```go
type MyWritableHandler struct {
    ragfs.DefaultHandler
    storage map[string][]byte
}

func (h *MyWritableHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
    data := h.storage[path]
    return []fs.DirEntry{ragfs.NewFileEntry("file", data)}, nil
}

func (h *MyWritableHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
    h.storage[path] = data
    return nil
}

func (h *MyWritableHandler) Remove(ctx context.Context, path string, params map[string]string) error {
    delete(h.storage, path)
    return nil
}
```
```

### Step 3: Update write operations section

In the "Future Enhancements" section, remove the items that are now implemented:

Remove:
- "Write operations: Create/update files, delete, rename"

### Step 4: Commit

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with Handler interface and write operations"
```

---

## Task 8: Final Verification

**Files:**
- All modified files

### Step 1: Run all tests

Run: `go test -v ./...`
Expected: All tests PASS

### Step 2: Build all examples

Run:
```bash
go build -o examples/json/ragfs-mount ./examples/json
go build -o examples/sqlite/sqlite-mount ./examples/sqlite
```
Expected: Both build successfully

### Step 3: Test JSON example in foreground

Run:
```bash
./examples/json/ragfs-mount -mount /tmp/json-test -config config.json &
MOUNT_PID=$!
sleep 2
ls /tmp/json-test/
cat /tmp/json-test/app/name
kill $MOUNT_PID
wait $MOUNT_PID
```
Expected: Lists directories, reads files correctly

### Step 4: Test SQLite example in foreground

Run:
```bash
./examples/sqlite/sqlite-mount -mount /tmp/sqlite-test -db examples/sqlite/example.db &
MOUNT_PID=$!
sleep 2
ls /tmp/sqlite-test/
cat /tmp/sqlite-test/users/1.json
kill $MOUNT_PID
wait $MOUNT_PID
```
Expected: Lists tables, reads user data correctly

### Step 5: Verify go fmt

Run: `go fmt ./...`
Expected: No changes needed

### Step 6: Final commit if needed

If any cleanup needed:
```bash
git add .
git commit -m "chore: final cleanup and formatting"
```

---

## Execution Handoff

Plan complete and saved to `docs/plans/2025-12-12-write-operations.md`. Two execution options:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** - Open new session with executing-plans, batch execution with checkpoints

**Which approach?**
