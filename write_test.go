package ragfs_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/brittcrawford/ragfs"
)

// WritableHandler implements Handler with Write support for testing
type WritableHandler struct {
	ragfs.DefaultHandler
	written map[string][]byte // stores written data
	dirs    map[string]bool   // stores created directories
}

func NewWritableHandler() *WritableHandler {
	return &WritableHandler{
		written: make(map[string][]byte),
		dirs:    make(map[string]bool),
	}
}

func (h *WritableHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
	// Check if it's a directory
	if h.dirs[path] {
		return []fs.DirEntry{}, nil // Empty directory
	}

	data, ok := h.written[path]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fs.ErrNotExist}
	}

	// Return the written data as a file entry
	return []fs.DirEntry{
		&testFileEntry{name: "data.txt", content: data},
	}, nil
}

func (h *WritableHandler) Write(ctx context.Context, path string, data []byte, params map[string]string) error {
	h.written[path] = data
	return nil
}

func (h *WritableHandler) Remove(ctx context.Context, path string, params map[string]string) error {
	if _, ok := h.written[path]; !ok {
		return &fs.PathError{Op: "remove", Path: path, Err: fs.ErrNotExist}
	}
	delete(h.written, path)
	return nil
}

func (h *WritableHandler) Rename(ctx context.Context, oldPath, newPath string, params map[string]string) error {
	data, ok := h.written[oldPath]
	if !ok {
		return &fs.PathError{Op: "rename", Path: oldPath, Err: fs.ErrNotExist}
	}
	h.written[newPath] = data
	delete(h.written, oldPath)
	return nil
}

func (h *WritableHandler) Mkdir(ctx context.Context, path string, params map[string]string) error {
	if h.dirs[path] {
		return &fs.PathError{Op: "mkdir", Path: path, Err: fs.ErrExist}
	}
	h.dirs[path] = true
	return nil
}

func (h *WritableHandler) Rmdir(ctx context.Context, path string, params map[string]string) error {
	if !h.dirs[path] {
		return &fs.PathError{Op: "rmdir", Path: path, Err: fs.ErrNotExist}
	}
	delete(h.dirs, path)
	return nil
}

// TestWriteFile tests basic write operations
func TestWriteFile(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a writable path
	fsys.Map("/data", handler)

	// Write data to the file
	testData := []byte("hello world")
	err := fsys.WriteFile("/data", testData)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify the data was written by reading it back
	if string(handler.written["/data"]) != "hello world" {
		t.Errorf("Expected 'hello world', got %q", handler.written["/data"])
	}
}

// TestWriteFileNotFound tests writing to unmapped path
func TestWriteFileNotFound(t *testing.T) {
	fsys := ragfs.New()

	err := fsys.WriteFile("/nonexistent", []byte("data"))
	if err == nil {
		t.Fatal("Expected error for unmapped path, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestWriteFileReadOnly tests writing to read-only handler
func TestWriteFileReadOnly(t *testing.T) {
	fsys := ragfs.New()

	// Map a read-only handler
	fsys.Map("/readonly", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{&testFileEntry{name: "test.txt", content: []byte("read-only")}}, nil
	}))

	// Attempt to write should fail
	err := fsys.WriteFile("/readonly", []byte("new data"))
	if err == nil {
		t.Fatal("Expected error for read-only handler, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected ErrPermission, got %v", pathErr.Err)
	}
}

// TestWriteFileWithParameters tests writing with path parameters
func TestWriteFileWithParameters(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a pattern with parameters
	fsys.Map("/files/{name}", handler)

	// Write data
	err := fsys.WriteFile("/files/test.txt", []byte("content"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify data was written
	if string(handler.written["/files/test.txt"]) != "content" {
		t.Errorf("Expected 'content', got %q", handler.written["/files/test.txt"])
	}
}

// TestRemove tests basic remove operations
func TestRemove(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a writable path
	fsys.Map("/data", handler)

	// Write data first
	err := fsys.WriteFile("/data", []byte("test data"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Verify data exists
	if _, ok := handler.written["/data"]; !ok {
		t.Fatal("Data should exist before removal")
	}

	// Remove the file
	err = fsys.Remove("/data")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify data was removed
	if _, ok := handler.written["/data"]; ok {
		t.Error("Data should be removed")
	}
}

// TestRemoveNotFound tests removing non-existent file
func TestRemoveNotFound(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a path
	fsys.Map("/data", handler)

	// Try to remove non-existent file
	err := fsys.Remove("/data")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestRemoveUnmappedPath tests removing from unmapped path
func TestRemoveUnmappedPath(t *testing.T) {
	fsys := ragfs.New()

	err := fsys.Remove("/nonexistent")
	if err == nil {
		t.Fatal("Expected error for unmapped path, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestRemoveReadOnly tests removing from read-only handler
func TestRemoveReadOnly(t *testing.T) {
	fsys := ragfs.New()

	// Map a read-only handler
	fsys.Map("/readonly", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{&testFileEntry{name: "test.txt", content: []byte("read-only")}}, nil
	}))

	// Attempt to remove should fail
	err := fsys.Remove("/readonly")
	if err == nil {
		t.Fatal("Expected error for read-only handler, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected ErrPermission, got %v", pathErr.Err)
	}
}

// TestRemoveWithParameters tests removing with path parameters
func TestRemoveWithParameters(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a pattern with parameters
	fsys.Map("/files/{name}", handler)

	// Write data first
	err := fsys.WriteFile("/files/test.txt", []byte("content"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Remove the file
	err = fsys.Remove("/files/test.txt")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify data was removed
	if _, ok := handler.written["/files/test.txt"]; ok {
		t.Error("Data should be removed")
	}
}

// TestRename tests basic rename operations
func TestRename(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a writable path
	fsys.Map("/data", handler)

	// Write data first
	testData := []byte("test data")
	err := fsys.WriteFile("/data", testData)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Rename the file
	err = fsys.Rename("/data", "/data-renamed")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Verify old path no longer exists
	if _, ok := handler.written["/data"]; ok {
		t.Error("Old path should not exist after rename")
	}

	// Verify new path exists with correct data
	if string(handler.written["/data-renamed"]) != "test data" {
		t.Errorf("Expected 'test data', got %q", handler.written["/data-renamed"])
	}
}

// TestRenameNotFound tests renaming non-existent file
func TestRenameNotFound(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a path
	fsys.Map("/data", handler)

	// Try to rename non-existent file
	err := fsys.Rename("/data", "/data-renamed")
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestRenameUnmappedPath tests renaming from unmapped path
func TestRenameUnmappedPath(t *testing.T) {
	fsys := ragfs.New()

	err := fsys.Rename("/nonexistent", "/somewhere")
	if err == nil {
		t.Fatal("Expected error for unmapped path, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestRenameReadOnly tests renaming from read-only handler
func TestRenameReadOnly(t *testing.T) {
	fsys := ragfs.New()

	// Map a read-only handler
	fsys.Map("/readonly", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{&testFileEntry{name: "test.txt", content: []byte("read-only")}}, nil
	}))

	// Attempt to rename should fail
	err := fsys.Rename("/readonly", "/somewhere")
	if err == nil {
		t.Fatal("Expected error for read-only handler, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected ErrPermission, got %v", pathErr.Err)
	}
}

// TestRenameWithParameters tests renaming with path parameters
func TestRenameWithParameters(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a pattern with parameters
	fsys.Map("/files/{name}", handler)

	// Write data first
	err := fsys.WriteFile("/files/old.txt", []byte("content"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Rename the file
	err = fsys.Rename("/files/old.txt", "/files/new.txt")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Verify old path no longer exists
	if _, ok := handler.written["/files/old.txt"]; ok {
		t.Error("Old path should not exist after rename")
	}

	// Verify new path exists with correct data
	if string(handler.written["/files/new.txt"]) != "content" {
		t.Errorf("Expected 'content', got %q", handler.written["/files/new.txt"])
	}
}

// TestMkdir tests basic mkdir operations
func TestMkdir(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a writable path
	fsys.Map("/data/**", handler)

	// Create directory
	err := fsys.Mkdir("/data/newdir")
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Verify directory was created in handler
	if !handler.dirs["/data/newdir"] {
		t.Error("Directory should exist in handler")
	}
}

// TestMkdirNotFound tests mkdir to unmapped path
func TestMkdirNotFound(t *testing.T) {
	fsys := ragfs.New()

	err := fsys.Mkdir("/nonexistent")
	if err == nil {
		t.Fatal("Expected error for unmapped path, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestMkdirReadOnly tests mkdir on read-only handler
func TestMkdirReadOnly(t *testing.T) {
	fsys := ragfs.New()

	// Map a read-only handler
	fsys.Map("/readonly/**", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}))

	// Attempt to mkdir should fail
	err := fsys.Mkdir("/readonly/newdir")
	if err == nil {
		t.Fatal("Expected error for read-only handler, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected ErrPermission, got %v", pathErr.Err)
	}
}

// TestMkdirWithParameters tests mkdir with path parameters
func TestMkdirWithParameters(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a pattern with parameters
	fsys.Map("/dirs/{name}/**", handler)

	// Create directory
	err := fsys.Mkdir("/dirs/myproject/subdir")
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Verify directory was created
	if !handler.dirs["/dirs/myproject/subdir"] {
		t.Error("Directory should exist in handler")
	}
}

// TestRmdir tests basic rmdir operations
func TestRmdir(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a writable path
	fsys.Map("/data/**", handler)

	// Create directory first
	err := fsys.Mkdir("/data/mydir")
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Verify directory exists
	if !handler.dirs["/data/mydir"] {
		t.Fatal("Directory should exist before rmdir")
	}

	// Remove directory
	err = fsys.Rmdir("/data/mydir")
	if err != nil {
		t.Fatalf("Rmdir failed: %v", err)
	}

	// Verify directory was removed
	if handler.dirs["/data/mydir"] {
		t.Error("Directory should not exist after rmdir")
	}
}

// TestRmdirNotFound tests rmdir to unmapped path
func TestRmdirNotFound(t *testing.T) {
	fsys := ragfs.New()

	err := fsys.Rmdir("/nonexistent")
	if err == nil {
		t.Fatal("Expected error for unmapped path, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestRmdirReadOnly tests rmdir on read-only handler
func TestRmdirReadOnly(t *testing.T) {
	fsys := ragfs.New()

	// Map a read-only handler
	fsys.Map("/readonly/**", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}))

	// Attempt to rmdir should fail
	err := fsys.Rmdir("/readonly/somedir")
	if err == nil {
		t.Fatal("Expected error for read-only handler, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected ErrPermission, got %v", pathErr.Err)
	}
}

// TestRmdirNonexistentDir tests rmdir on non-existent directory
func TestRmdirNonexistentDir(t *testing.T) {
	fsys := ragfs.New()
	handler := NewWritableHandler()

	// Map a writable path
	fsys.Map("/data/**", handler)

	// Try to remove non-existent directory
	err := fsys.Rmdir("/data/doesnotexist")
	if err == nil {
		t.Fatal("Expected error for non-existent directory, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TruncatableHandler implements Handler with Truncate support for testing
type TruncatableHandler struct {
	WritableHandler
}

func NewTruncatableHandler() *TruncatableHandler {
	return &TruncatableHandler{
		WritableHandler: WritableHandler{
			written: make(map[string][]byte),
			dirs:    make(map[string]bool),
		},
	}
}

func (h *TruncatableHandler) Truncate(ctx context.Context, path string, size int64, params map[string]string) error {
	data, ok := h.written[path]
	if !ok {
		return &fs.PathError{Op: "truncate", Path: path, Err: fs.ErrNotExist}
	}

	if size < 0 {
		return &fs.PathError{Op: "truncate", Path: path, Err: fs.ErrInvalid}
	}

	if int64(len(data)) > size {
		// Truncate to smaller size
		h.written[path] = data[:size]
	} else if int64(len(data)) < size {
		// Extend with zero bytes
		newData := make([]byte, size)
		copy(newData, data)
		h.written[path] = newData
	}
	return nil
}

// TestTruncate tests basic truncate operations
func TestTruncate(t *testing.T) {
	fsys := ragfs.New()
	handler := NewTruncatableHandler()

	// Map a writable path
	fsys.Map("/data/**", handler)

	// Write initial data
	err := fsys.WriteFile("/data/file.txt", []byte("hello world"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Truncate to smaller size
	err = fsys.Truncate("/data/file.txt", 5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify truncation
	if string(handler.written["/data/file.txt"]) != "hello" {
		t.Errorf("Expected 'hello', got %q", handler.written["/data/file.txt"])
	}
}

// TestTruncateExtend tests truncate that extends file
func TestTruncateExtend(t *testing.T) {
	fsys := ragfs.New()
	handler := NewTruncatableHandler()

	// Map a writable path
	fsys.Map("/data/**", handler)

	// Write initial data
	err := fsys.WriteFile("/data/file.txt", []byte("hi"))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Extend to larger size
	err = fsys.Truncate("/data/file.txt", 5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify extension (should be "hi" + 3 zero bytes)
	if len(handler.written["/data/file.txt"]) != 5 {
		t.Errorf("Expected length 5, got %d", len(handler.written["/data/file.txt"]))
	}
	if string(handler.written["/data/file.txt"][:2]) != "hi" {
		t.Errorf("Expected first 2 bytes to be 'hi', got %q", handler.written["/data/file.txt"][:2])
	}
}

// TestTruncateNotFound tests truncate to unmapped path
func TestTruncateNotFound(t *testing.T) {
	fsys := ragfs.New()

	err := fsys.Truncate("/nonexistent", 0)
	if err == nil {
		t.Fatal("Expected error for unmapped path, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("Expected ErrNotExist, got %v", pathErr.Err)
	}
}

// TestTruncateReadOnly tests truncate on read-only handler
func TestTruncateReadOnly(t *testing.T) {
	fsys := ragfs.New()

	// Map a read-only handler
	fsys.Map("/readonly/**", ragfs.NewReadOnlyHandler(func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}))

	// Attempt to truncate should fail
	err := fsys.Truncate("/readonly/file.txt", 0)
	if err == nil {
		t.Fatal("Expected error for read-only handler, got nil")
	}

	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("Expected fs.PathError, got %T", err)
	}

	if pathErr.Err != fs.ErrPermission {
		t.Errorf("Expected ErrPermission, got %v", pathErr.Err)
	}
}
