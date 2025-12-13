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
}

func NewWritableHandler() *WritableHandler {
	return &WritableHandler{
		written: make(map[string][]byte),
	}
}

func (h *WritableHandler) Read(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
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
