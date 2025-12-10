package ragfs

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"time"
)

// Handler is a function that handles filesystem operations for a given path.
// It receives the full path, extracted parameters, and returns directory entries.
type Handler func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)

// FS is a filesystem that maps path patterns to handlers.
type FS struct {
	routes []route
}

type route struct {
	pattern string
	handler Handler
}

// New creates a new ragfs filesystem.
func New() *FS {
	return &FS{
		routes: make([]route, 0),
	}
}

// Map registers a handler for the given path pattern.
func (f *FS) Map(pattern string, handler Handler) error {
	f.routes = append(f.routes, route{
		pattern: pattern,
		handler: handler,
	})
	return nil
}

// Open opens the named file.
func (f *FS) Open(name string) (fs.File, error) {
	// Find matching route with pattern matching
	var handler Handler
	var params map[string]string

	for _, r := range f.routes {
		if match, p := matchPattern(r.pattern, name); match {
			handler = r.handler
			params = p
			break
		}
	}

	if handler == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	// Call the handler to get entries
	entries, err := handler(context.Background(), name, params)
	if err != nil {
		return nil, err
	}

	// For now, assume we're opening a file and the first entry contains the content
	// We need to extract content from the DirEntry somehow
	if len(entries) == 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	entry := entries[0]

	// We need a way to get content from the entry
	// For now, we'll use a type assertion to a custom interface
	if ce, ok := entry.(interface{ Content() []byte }); ok {
		return &file{
			name:   entry.Name(),
			reader: bytes.NewReader(ce.Content()),
		}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
}

// file implements fs.File
type file struct {
	name   string
	reader *bytes.Reader
}

func (f *file) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name: f.name,
		size: int64(f.reader.Len()),
	}, nil
}

func (f *file) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *file) Close() error {
	return nil
}

// fileInfo implements fs.FileInfo
type fileInfo struct {
	name string
	size int64
}

func (i *fileInfo) Name() string       { return i.name }
func (i *fileInfo) Size() int64        { return i.size }
func (i *fileInfo) Mode() fs.FileMode  { return 0444 }
func (i *fileInfo) ModTime() time.Time { return time.Time{} }
func (i *fileInfo) IsDir() bool        { return false }
func (i *fileInfo) Sys() any           { return nil }

// matchPattern matches a path against a pattern and extracts parameters.
// Pattern format: /emails/{date} matches /emails/2025-10-07 and extracts date=2025-10-07
func matchPattern(pattern, path string) (bool, map[string]string) {
	params := make(map[string]string)

	// Split pattern and path into segments
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	// Must have same number of segments
	if len(patternParts) != len(pathParts) {
		return false, nil
	}

	// Match each segment
	for i, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, "{") && strings.HasSuffix(patternPart, "}") {
			// Extract parameter name
			paramName := patternPart[1 : len(patternPart)-1]
			params[paramName] = pathParts[i]
		} else if patternPart != pathParts[i] {
			// Static segment must match exactly
			return false, nil
		}
	}

	return true, params
}
