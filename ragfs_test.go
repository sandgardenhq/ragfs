package ragfs_test

import (
	"context"
	"io"
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

func TestWildcardPattern(t *testing.T) {
	fsys := ragfs.New()

	var capturedPath string
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		capturedPath = path
		content := []byte("wildcard match: " + path)
		return []fs.DirEntry{
			&testFileEntry{name: "file.txt", content: content},
		}, nil
	}

	// Register a wildcard pattern that matches any path
	fsys.Map("/*", handler)

	// Test single-level path
	f, err := fsys.Open("/test")
	if err != nil {
		t.Fatalf("Open('/test') error: %v", err)
	}
	f.Close()
	if capturedPath != "/test" {
		t.Errorf("expected path /test, got %s", capturedPath)
	}

	// Test multi-level path
	f, err = fsys.Open("/app/config/name")
	if err != nil {
		t.Fatalf("Open('/app/config/name') error: %v", err)
	}
	f.Close()
	if capturedPath != "/app/config/name" {
		t.Errorf("expected path /app/config/name, got %s", capturedPath)
	}
}

func TestReadDirFS(t *testing.T) {
	var _ fs.ReadDirFS = (*ragfs.FS)(nil)

	fsys := ragfs.New()

	entries := []fs.DirEntry{
		&testFileEntry{name: "file1.txt", content: []byte("content1")},
		&testFileEntry{name: "file2.txt", content: []byte("content2")},
		&testFileEntry{name: "subdir", isDir: true},
	}

	var capturedPath string
	var capturedParams map[string]string
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		capturedPath = path
		capturedParams = params
		return entries, nil
	}

	fsys.Map("/docs/{category}", handler)

	t.Run("basicReadDir", func(t *testing.T) {
		got, err := fsys.ReadDir("/docs/reports")
		if err != nil {
			t.Fatalf("ReadDir() error: %v", err)
		}
		if capturedPath != "/docs/reports" {
			t.Errorf("expected path /docs/reports, got %s", capturedPath)
		}
		if capturedParams["category"] != "reports" {
			t.Errorf("expected category param reports, got %s", capturedParams["category"])
		}
		if len(got) != len(entries) {
			t.Fatalf("expected %d entries, got %d", len(entries), len(got))
		}
		for i, entry := range got {
			if entry.Name() != entries[i].Name() {
				t.Errorf("entry[%d] name: expected %s, got %s", i, entries[i].Name(), entry.Name())
			}
			if entry.IsDir() != entries[i].IsDir() {
				t.Errorf("entry[%d] IsDir: expected %v, got %v", i, entries[i].IsDir(), entry.IsDir())
			}
		}
	})

	t.Run("notFound", func(t *testing.T) {
		_, err := fsys.ReadDir("/nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent path")
		}
		pathErr, ok := err.(*fs.PathError)
		if !ok {
			t.Fatalf("expected *fs.PathError, got %T", err)
		}
		if pathErr.Op != "readdir" {
			t.Errorf("expected Op 'readdir', got %s", pathErr.Op)
		}
	})
}

func TestReadDirHandlerError(t *testing.T) {
	fsys := ragfs.New()

	expectedErr := fs.ErrPermission
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return nil, expectedErr
	}

	fsys.Map("/protected/**", handler)

	_, err := fsys.ReadDir("/protected/secret")
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestDirFileReadDir(t *testing.T) {
	fsys := ragfs.New()

	entries := []fs.DirEntry{
		&testFileEntry{name: "a.txt", content: []byte("a")},
		&testFileEntry{name: "b.txt", content: []byte("b")},
		&testFileEntry{name: "c.txt", content: []byte("c")},
		&testFileEntry{name: "d.txt", content: []byte("d")},
		&testFileEntry{name: "e.txt", content: []byte("e")},
	}

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return entries, nil
	}

	fsys.Map("/items/**", handler)

	t.Run("readDirAllAtOnce", func(t *testing.T) {
		f, err := fsys.Open("/items/all")
		if err != nil {
			t.Fatalf("Open() error: %v", err)
		}
		defer f.Close()

		dirFile, ok := f.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected fs.ReadDirFile")
		}

		got, err := dirFile.ReadDir(-1)
		if err != nil {
			t.Fatalf("ReadDir(-1) error: %v", err)
		}
		if len(got) != len(entries) {
			t.Errorf("expected %d entries, got %d", len(entries), len(got))
		}

		got, err = dirFile.ReadDir(-1)
		if err != nil {
			t.Fatalf("ReadDir(-1) after exhaust error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 entries after exhaust, got %d", len(got))
		}
	})

	t.Run("readDirZero", func(t *testing.T) {
		f, err := fsys.Open("/items/zero")
		if err != nil {
			t.Fatalf("Open() error: %v", err)
		}
		defer f.Close()

		dirFile, ok := f.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected fs.ReadDirFile")
		}

		got, err := dirFile.ReadDir(0)
		if err != nil {
			t.Fatalf("ReadDir(0) error: %v", err)
		}
		if len(got) != len(entries) {
			t.Errorf("expected %d entries with n=0, got %d", len(entries), len(got))
		}
	})

	t.Run("readDirIncrementally", func(t *testing.T) {
		f, err := fsys.Open("/items/inc")
		if err != nil {
			t.Fatalf("Open() error: %v", err)
		}
		defer f.Close()

		dirFile, ok := f.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected fs.ReadDirFile")
		}

		got, err := dirFile.ReadDir(2)
		if err != nil {
			t.Fatalf("ReadDir(2) first call error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 entries, got %d", len(got))
		}
		if got[0].Name() != "a.txt" || got[1].Name() != "b.txt" {
			t.Errorf("expected a.txt and b.txt, got %s and %s", got[0].Name(), got[1].Name())
		}

		got, err = dirFile.ReadDir(2)
		if err != nil {
			t.Fatalf("ReadDir(2) second call error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 entries, got %d", len(got))
		}
		if got[0].Name() != "c.txt" || got[1].Name() != "d.txt" {
			t.Errorf("expected c.txt and d.txt, got %s and %s", got[0].Name(), got[1].Name())
		}

		got, err = dirFile.ReadDir(2)
		if err != nil {
			t.Fatalf("ReadDir(2) third call error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("expected 1 entry (last), got %d", len(got))
		}
		if got[0].Name() != "e.txt" {
			t.Errorf("expected e.txt, got %s", got[0].Name())
		}

		_, err = dirFile.ReadDir(1)
		if err != io.EOF {
			t.Errorf("expected io.EOF after exhausting entries, got %v", err)
		}
	})

	t.Run("readDirMoreThanAvailable", func(t *testing.T) {
		f, err := fsys.Open("/items/more")
		if err != nil {
			t.Fatalf("Open() error: %v", err)
		}
		defer f.Close()

		dirFile, ok := f.(fs.ReadDirFile)
		if !ok {
			t.Fatal("expected fs.ReadDirFile")
		}

		got, err := dirFile.ReadDir(100)
		if err != nil {
			t.Fatalf("ReadDir(100) error: %v", err)
		}
		if len(got) != len(entries) {
			t.Errorf("expected %d entries (all), got %d", len(entries), len(got))
		}

		_, err = dirFile.ReadDir(1)
		if err != io.EOF {
			t.Errorf("expected io.EOF after exhausting entries, got %v", err)
		}
	})
}

func TestDirFileStatReadClose(t *testing.T) {
	fsys := ragfs.New()

	entries := []fs.DirEntry{
		&testFileEntry{name: "file1.txt", content: []byte("content1")},
		&testFileEntry{name: "file2.txt", content: []byte("content2")},
	}

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return entries, nil
	}

	fsys.Map("/mydir/**", handler)

	f, err := fsys.Open("/mydir/test")
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}

	t.Run("stat", func(t *testing.T) {
		info, err := f.Stat()
		if err != nil {
			t.Fatalf("Stat() error: %v", err)
		}
		if info.Name() != "/mydir/test" {
			t.Errorf("expected name /mydir/test, got %s", info.Name())
		}
		if !info.IsDir() {
			t.Error("expected IsDir() to be true")
		}
		if info.Size() != 0 {
			t.Errorf("expected Size() 0, got %d", info.Size())
		}
		if info.Mode() != (fs.ModeDir | 0755) {
			t.Errorf("expected Mode() ModeDir|0755, got %v", info.Mode())
		}
		if !info.ModTime().IsZero() {
			t.Errorf("expected ModTime() to be zero, got %v", info.ModTime())
		}
		if info.Sys() != nil {
			t.Errorf("expected Sys() nil, got %v", info.Sys())
		}
	})

	t.Run("readError", func(t *testing.T) {
		buf := make([]byte, 10)
		_, err := f.Read(buf)
		if err == nil {
			t.Error("expected error when reading directory")
		}
		pathErr, ok := err.(*fs.PathError)
		if !ok {
			t.Fatalf("expected *fs.PathError, got %T", err)
		}
		if pathErr.Op != "read" {
			t.Errorf("expected Op 'read', got %s", pathErr.Op)
		}
	})

	t.Run("close", func(t *testing.T) {
		err := f.Close()
		if err != nil {
			t.Fatalf("Close() error: %v", err)
		}
	})
}

func TestFileStat(t *testing.T) {
	fsys := ragfs.New()

	content := []byte("test file content here")
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "data.txt", content: content},
		}, nil
	}

	fsys.Map("/single/**", handler)

	f, err := fsys.Open("/single/data")
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}

	if info.Name() != "data.txt" {
		t.Errorf("expected name data.txt, got %s", info.Name())
	}
	if info.Size() != int64(len(content)) {
		t.Errorf("expected Size() %d, got %d", len(content), info.Size())
	}
	if info.IsDir() {
		t.Error("expected IsDir() to be false")
	}
	if info.Mode() != 0444 {
		t.Errorf("expected Mode() 0444, got %v", info.Mode())
	}
	if !info.ModTime().IsZero() {
		t.Errorf("expected ModTime() to be zero, got %v", info.ModTime())
	}
	if info.Sys() != nil {
		t.Errorf("expected Sys() nil, got %v", info.Sys())
	}
}

func TestOpenSingleDirectoryEntry(t *testing.T) {
	fsys := ragfs.New()

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&testFileEntry{name: "subdir", isDir: true},
		}, nil
	}

	fsys.Map("/dirs/**", handler)

	f, err := fsys.Open("/dirs/parent")
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	defer f.Close()

	_, ok := f.(fs.ReadDirFile)
	if !ok {
		t.Error("expected single directory entry to return fs.ReadDirFile")
	}
}

func TestOpenEmptyHandlerResult(t *testing.T) {
	fsys := ragfs.New()

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}

	fsys.Map("/empty/**", handler)

	_, err := fsys.Open("/empty/nothing")
	if err == nil {
		t.Fatal("expected error for empty handler result")
	}
	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("expected *fs.PathError, got %T", err)
	}
	if pathErr.Err != fs.ErrNotExist {
		t.Errorf("expected ErrNotExist, got %v", pathErr.Err)
	}
}

func TestOpenFileWithoutContentMethod(t *testing.T) {
	fsys := ragfs.New()

	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{
			&noContentEntry{name: "broken.txt"},
		}, nil
	}

	fsys.Map("/broken/**", handler)

	_, err := fsys.Open("/broken/file")
	if err == nil {
		t.Fatal("expected error for entry without Content method")
	}
	pathErr, ok := err.(*fs.PathError)
	if !ok {
		t.Fatalf("expected *fs.PathError, got %T", err)
	}
	if pathErr.Err != fs.ErrInvalid {
		t.Errorf("expected ErrInvalid, got %v", pathErr.Err)
	}
}

type noContentEntry struct {
	name string
}

func (e *noContentEntry) Name() string               { return e.name }
func (e *noContentEntry) IsDir() bool                { return false }
func (e *noContentEntry) Type() fs.FileMode          { return 0 }
func (e *noContentEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestOpenHandlerError(t *testing.T) {
	fsys := ragfs.New()

	expectedErr := fs.ErrPermission
	handler := func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
		return nil, expectedErr
	}

	fsys.Map("/error/**", handler)

	_, err := fsys.Open("/error/path")
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}
