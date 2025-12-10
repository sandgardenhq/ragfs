package ragfs_test

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/brittcrawford/ragfs"
)

func TestNewFS(t *testing.T) {
	fsys := ragfs.New()
	if fsys == nil {
		t.Fatal("New() returned nil")
	}
}

func TestMapRoute(t *testing.T) {
	fsys := ragfs.New()

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return nil, nil
	}

	err := fsys.Map("/test", handler)
	if err != nil {
		t.Fatalf("Map() returned error: %v", err)
	}
}

func TestOpenStaticFile(t *testing.T) {
	fsys := ragfs.New()

	called := false
	content := []byte("hello world")

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		called = true
		if path != "/hello.txt" {
			t.Errorf("expected path /hello.txt, got %s", path)
		}
		// Return a single file entry
		return []fs.DirEntry{
			&testFileEntry{name: "hello.txt", content: content},
		}, nil
	}

	fsys.Map("/hello.txt", handler)

	f, err := fsys.Open("/hello.txt")
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer f.Close()

	if !called {
		t.Error("handler was not called")
	}

	// Verify we can read the file content
	data := make([]byte, len(content))
	n, err := f.Read(data)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if n != len(content) {
		t.Errorf("expected to read %d bytes, got %d", len(content), n)
	}
	if string(data) != string(content) {
		t.Errorf("expected content %q, got %q", content, data)
	}
}

// testFileEntry is a test helper that implements fs.DirEntry
type testFileEntry struct {
	name    string
	content []byte
	isDir   bool
}

func (e *testFileEntry) Name() string               { return e.name }
func (e *testFileEntry) IsDir() bool                { return e.isDir }
func (e *testFileEntry) Type() fs.FileMode          { return 0 }
func (e *testFileEntry) Info() (fs.FileInfo, error) { return &testFileInfo{e}, nil }
func (e *testFileEntry) Content() []byte            { return e.content }

type testFileInfo struct {
	entry *testFileEntry
}

func (i *testFileInfo) Name() string       { return i.entry.name }
func (i *testFileInfo) Size() int64        { return int64(len(i.entry.content)) }
func (i *testFileInfo) Mode() fs.FileMode  { return 0444 }
func (i *testFileInfo) ModTime() time.Time { return time.Time{} }
func (i *testFileInfo) IsDir() bool        { return i.entry.isDir }
func (i *testFileInfo) Sys() any           { return nil }

func TestPathParameters(t *testing.T) {
	fsys := ragfs.New()

	var capturedParams map[string]string
	var capturedPath string

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		capturedPath = path
		capturedParams = params
		content := []byte("email content for " + params["date"])
		return []fs.DirEntry{
			&testFileEntry{name: "email.txt", content: content},
		}, nil
	}

	fsys.Map("/emails/{date}", handler)

	f, err := fsys.Open("/emails/2025-10-07")
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer f.Close()

	if capturedPath != "/emails/2025-10-07" {
		t.Errorf("expected path /emails/2025-10-07, got %s", capturedPath)
	}

	if capturedParams["date"] != "2025-10-07" {
		t.Errorf("expected date param 2025-10-07, got %s", capturedParams["date"])
	}

	// Verify content includes the date
	data := make([]byte, 100)
	n, _ := f.Read(data)
	content := string(data[:n])
	if content != "email content for 2025-10-07" {
		t.Errorf("expected content with date, got %s", content)
	}
}
