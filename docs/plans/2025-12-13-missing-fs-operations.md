# Missing Filesystem Operations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement missing filesystem operations to make ragfs more complete and POSIX-compliant.

**Current State:** ragfs implements basic read/write operations but lacks directory creation, truncation, and standard Go fs interfaces.

---

## Analysis: Current vs Missing Operations

### Currently Implemented

| Operation | ragfs Method | FUSE Interface | Go fs Interface |
|-----------|--------------|----------------|-----------------|
| Open file | `Open()` | NodeOpener | fs.FS |
| Read directory | `ReadDir()` | NodeReaddirer | fs.ReadDirFS |
| Get attributes | - | NodeGetattrer | - |
| Lookup child | - | NodeLookuper | - |
| Create file | `WriteFile()` | NodeCreater | - |
| Delete file | `Remove()` | NodeUnlinker | - |
| Rename | `Rename()` | NodeRenamer | - |

### Missing Operations (Prioritized)

#### Priority 1: Essential Directory Operations
| Operation | Description | FUSE Interface |
|-----------|-------------|----------------|
| **Mkdir** | Create directory | NodeMkdirer |
| **Rmdir** | Remove empty directory | NodeRmdirer |

#### Priority 2: File Modification Operations
| Operation | Description | FUSE Interface |
|-----------|-------------|----------------|
| **Truncate** | Change file size | NodeSetattrer |
| **Chmod** | Change permissions | NodeSetattrer |
| **Chown** | Change ownership | NodeSetattrer |
| **Utimes** | Change timestamps | NodeSetattrer |

#### Priority 3: Standard Go fs Interfaces
| Operation | Description | Go Interface |
|-----------|-------------|--------------|
| **Stat** | Get file info without opening | fs.StatFS |
| **ReadFile** | Read entire file | fs.ReadFileFS |

#### Priority 4: Symbolic Links (Deferred)
| Operation | Description | FUSE Interface |
|-----------|-------------|----------------|
| Symlink | Create symbolic link | NodeSymlinker |
| Readlink | Read symbolic link target | NodeReadlinker |

---

## Task 1: Add Handler Interface Methods for Directory Operations

**Files:**
- Modify: `ragfs.go` (Handler interface)
- Test: `write_test.go`

### Step 1: Extend Handler interface with Mkdir and Rmdir

Add to Handler interface in `ragfs.go`:

```go
type Handler interface {
    // ... existing methods ...

    // Mkdir creates a directory at the given path.
    // Returns fs.ErrPermission if directory creation is not supported.
    Mkdir(ctx context.Context, path string, params map[string]string) error

    // Rmdir removes an empty directory at the given path.
    // Returns fs.ErrPermission if directory removal is not supported.
    Rmdir(ctx context.Context, path string, params map[string]string) error
}
```

### Step 2: Update ReadOnlyHandler with default implementations

```go
func (h *ReadOnlyHandler) Mkdir(ctx context.Context, path string, params map[string]string) error {
    return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrPermission}
}

func (h *ReadOnlyHandler) Rmdir(ctx context.Context, path string, params map[string]string) error {
    return &fs.PathError{Op: "rmdir", Path: path, Err: fs.ErrPermission}
}
```

### Step 3: Update metricsWrapper

Add pass-through methods in metricsWrapper for Mkdir and Rmdir.

### Step 4: Run tests

Run: `go test -v ./...`
Expected: All existing tests pass (new methods have default implementations)

---

## Task 2: Add Mkdir Method to FS

**Files:**
- Modify: `ragfs.go`
- Test: `write_test.go`

### Step 1: Write failing test

Add to `write_test.go`:

```go
func TestMkdir(t *testing.T) {
    fsys := New()

    dirs := make(map[string]bool)

    handler := &testWriteHandler{
        readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
            if dirs[path] {
                return []fs.DirEntry{}, nil // Empty directory
            }
            return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
        },
        mkdirFunc: func(ctx context.Context, path string, params map[string]string) error {
            dirs[path] = true
            return nil
        },
    }

    fsys.Map("/*", handler)

    // Create directory
    err := fsys.Mkdir("/newdir")
    if err != nil {
        t.Fatalf("Mkdir failed: %v", err)
    }

    // Verify directory exists
    entries, err := fsys.ReadDir("/newdir")
    if err != nil {
        t.Fatalf("ReadDir failed: %v", err)
    }
    if len(entries) != 0 {
        t.Errorf("Expected empty directory, got %d entries", len(entries))
    }
}
```

### Step 2: Implement Mkdir method

Add to `ragfs.go`:

```go
// Mkdir creates a new directory at the specified path.
// It matches the path against registered patterns and calls the handler's Mkdir method.
// Returns fs.ErrNotExist if no pattern matches.
// Returns fs.ErrPermission if the handler doesn't support directory creation.
func (f *FS) Mkdir(name string) error {
    matched := f.findLongestMatch(name)
    if matched == nil {
        return &fs.PathError{Op: "mkdir", Path: name, Err: fs.ErrNotExist}
    }

    err := matched.route.handler.Mkdir(context.Background(), name, matched.params)
    if err != nil {
        return err
    }

    // Invalidate parent directory cache
    parent := filepath.Dir(name)
    if f.cache != nil {
        f.cache.invalidate(parent)
    }
    if f.boltDBCache != nil {
        f.boltDBCache.invalidate(parent)
    }

    return nil
}
```

### Step 3: Run test

Run: `go test -v -run TestMkdir`
Expected: PASS

---

## Task 3: Add Rmdir Method to FS

**Files:**
- Modify: `ragfs.go`
- Test: `write_test.go`

### Step 1: Write failing test

```go
func TestRmdir(t *testing.T) {
    fsys := New()

    dirs := map[string]bool{"/mydir": true}

    handler := &testWriteHandler{
        readFunc: func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
            if dirs[path] {
                return []fs.DirEntry{}, nil
            }
            return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
        },
        rmdirFunc: func(ctx context.Context, path string, params map[string]string) error {
            if !dirs[path] {
                return &fs.PathError{Op: "rmdir", Path: path, Err: fs.ErrNotExist}
            }
            delete(dirs, path)
            return nil
        },
    }

    fsys.Map("/*", handler)

    // Remove directory
    err := fsys.Rmdir("/mydir")
    if err != nil {
        t.Fatalf("Rmdir failed: %v", err)
    }

    // Verify directory is gone
    _, err = fsys.ReadDir("/mydir")
    if err == nil {
        t.Fatal("Directory should not exist after Rmdir")
    }
}
```

### Step 2: Implement Rmdir method

```go
// Rmdir removes an empty directory at the specified path.
// Returns fs.ErrNotExist if no pattern matches.
// Returns fs.ErrPermission if the handler doesn't support directory removal.
func (f *FS) Rmdir(name string) error {
    matched := f.findLongestMatch(name)
    if matched == nil {
        return &fs.PathError{Op: "rmdir", Path: name, Err: fs.ErrNotExist}
    }

    err := matched.route.handler.Rmdir(context.Background(), name, matched.params)
    if err != nil {
        return err
    }

    // Invalidate cache
    if f.cache != nil {
        f.cache.invalidate(name)
        f.cache.invalidate(filepath.Dir(name))
    }
    if f.boltDBCache != nil {
        f.boltDBCache.invalidate(name)
        f.boltDBCache.invalidate(filepath.Dir(name))
    }

    return nil
}
```

### Step 3: Run test

Run: `go test -v -run TestRmdir`
Expected: PASS

---

## Task 4: Add FUSE Mkdir Support

**Files:**
- Modify: `fuse.go`

### Step 1: Add NodeMkdirer interface assertion

```go
var _ fuseFS.NodeMkdirer = (*FUSENode)(nil)
```

### Step 2: Implement Mkdir method on FUSENode

```go
// Mkdir creates a new directory.
func (n *FUSENode) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fuseFS.Inode, syscall.Errno) {
    // Build the directory path
    var dirPath string
    if n.path == "" {
        dirPath = "/" + name
    } else {
        dirPath = "/" + n.path + "/" + name
    }

    // Create the directory via ragfs
    err := n.fsys.Mkdir(dirPath)
    if err != nil {
        return nil, syscall.EIO
    }

    // Create child node
    var childPath string
    if n.path == "" {
        childPath = name
    } else {
        childPath = n.path + "/" + name
    }

    child := &FUSENode{
        fsys:  n.fsys,
        path:  childPath,
        isDir: true,
        uid:   n.uid,
        gid:   n.gid,
    }

    // Set entry attributes
    out.Mode = syscall.S_IFDIR | (mode & 0777)
    out.Uid = n.uid
    out.Gid = n.gid

    return n.NewInode(ctx, child, fuseFS.StableAttr{Mode: syscall.S_IFDIR}), fuseFS.OK
}
```

### Step 3: Run tests and verify

Run: `go test -v ./...`

---

## Task 5: Add FUSE Rmdir Support

**Files:**
- Modify: `fuse.go`

### Step 1: Add NodeRmdirer interface assertion

```go
var _ fuseFS.NodeRmdirer = (*FUSENode)(nil)
```

### Step 2: Implement Rmdir method on FUSENode

```go
// Rmdir removes an empty directory.
func (n *FUSENode) Rmdir(ctx context.Context, name string) syscall.Errno {
    // Build the directory path
    var dirPath string
    if n.path == "" {
        dirPath = "/" + name
    } else {
        dirPath = "/" + n.path + "/" + name
    }

    // Remove the directory via ragfs
    err := n.fsys.Rmdir(dirPath)
    if err != nil {
        return syscall.EIO
    }

    return fuseFS.OK
}
```

### Step 3: Run tests

Run: `go test -v ./...`

---

## Task 6: Add Truncate Support

**Files:**
- Modify: `ragfs.go` (Handler interface)
- Modify: `fuse.go` (NodeSetattrer)
- Test: `write_test.go`

### Step 1: Add Truncate to Handler interface

```go
// Truncate changes the size of the file at path.
// Returns fs.ErrPermission if truncation is not supported.
Truncate(ctx context.Context, path string, size int64, params map[string]string) error
```

### Step 2: Add Truncate method to FS

```go
func (f *FS) Truncate(name string, size int64) error {
    matched := f.findLongestMatch(name)
    if matched == nil {
        return &fs.PathError{Op: "truncate", Path: name, Err: fs.ErrNotExist}
    }

    err := matched.route.handler.Truncate(context.Background(), name, size, matched.params)
    if err != nil {
        return err
    }

    // Invalidate cache
    if f.cache != nil {
        f.cache.invalidate(name)
    }
    if f.boltDBCache != nil {
        f.boltDBCache.invalidate(name)
    }

    return nil
}
```

### Step 3: Implement NodeSetattrer for FUSE

```go
var _ fuseFS.NodeSetattrer = (*FUSENode)(nil)

func (n *FUSENode) Setattr(ctx context.Context, fh fuseFS.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
    ragfsPath := "/" + n.path

    // Handle truncate
    if in.Valid&fuse.FATTR_SIZE != 0 {
        err := n.fsys.Truncate(ragfsPath, int64(in.Size))
        if err != nil {
            return syscall.EIO
        }
    }

    // Return updated attributes
    return n.Getattr(ctx, fh, out)
}
```

---

## Task 7: Update Writable Example with Mkdir/Rmdir

**Files:**
- Modify: `examples/writable/main.go`

### Step 1: Add directory support to the writable handler

Update the handler to track directories separately and implement Mkdir/Rmdir.

### Step 2: Test the writable example

```bash
./writable-mount -mount /tmp/writable-test &
sleep 2
mkdir /tmp/writable-test/newdir
ls /tmp/writable-test/
rmdir /tmp/writable-test/newdir
ls /tmp/writable-test/
```

---

## Task 8: Update Documentation

**Files:**
- Modify: `CLAUDE.md`

### Step 1: Update Handler interface documentation

Document the new Mkdir, Rmdir, and Truncate methods.

### Step 2: Update example patterns

Add examples for directory operations.

---

## Task 9: Final Verification

### Step 1: Run all tests
```bash
go test -v ./...
```

### Step 2: Build and test all examples
```bash
go build -o json-mount examples/json/main.go
go build -o sqlite-mount examples/sqlite/main.go
go build -o writable-mount examples/writable/main.go

# Test each with FUSE mount and Unix tools
```

### Step 3: Verify go fmt
```bash
go fmt ./...
```

---

## Summary of Changes

| Component | New Methods/Interfaces |
|-----------|----------------------|
| Handler interface | `Mkdir()`, `Rmdir()`, `Truncate()` |
| FS struct | `Mkdir()`, `Rmdir()`, `Truncate()` |
| FUSENode | `NodeMkdirer`, `NodeRmdirer`, `NodeSetattrer` |
| ReadOnlyHandler | Default implementations returning ErrPermission |

## Deferred to Future

- **Symbolic links** - Symlink/Readlink operations
- **Hard links** - Link operation
- **Extended attributes** - Setxattr/Getxattr/Removexattr
- **fs.StatFS interface** - Can be added when needed
- **fs.ReadFileFS interface** - Can be added when needed
