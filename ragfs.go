// Package ragfs provides a filesystem interface for mapping path patterns to custom handlers.
// It implements the fs.FS interface, allowing LLM agents and applications to interact with
// data sources (APIs, databases) using familiar filesystem operations.
//
// Pattern Syntax (inspired by net/http.ServeMux):
//   - /emails/{date} matches /emails/2025-10-07 and extracts date=2025-10-07
//   - /files/{path...} matches /files/a/b/c and captures the rest of the path
//   - /users/{$} matches only /users exactly (not /users/)
//   - 755 /users/{id} adds permission requirements (owner read=4, execute=1)
//
// Most-specific pattern matching is used (not first-match).
// Conflicting patterns will cause Map to panic.
//
// Example usage:
//
//	fsys := ragfs.New()
//	fsys.Map("/emails/{date}", func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error) {
//	    // Fetch emails for the given date
//	    emails := fetchEmails(params["date"])
//	    return emails, nil
//	})
//	f, _ := fsys.Open("/emails/2025-10-07")
package ragfs

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"slices"
	"strings"
	"time"
)

// Handler is a function that handles filesystem operations for a given path.
// It receives the full path, extracted parameters, and returns directory entries.
type Handler func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)

// FS is a filesystem that maps path patterns to handlers.
// It implements the fs.FS interface from the standard library.
// Routes are matched using most-specific pattern matching with optional
// permission-based access control.
type FS struct {
	routes []routeInternal
}

// routeInternal represents a parsed pattern-to-handler mapping.
type routeInternal struct {
	pattern *pattern
	handler Handler
}

// New creates a new ragfs filesystem.
func New() *FS {
	return &FS{
		routes: make([]routeInternal, 0),
	}
}

// Map registers a handler for the given path pattern.
// Patterns follow ServeMux-style syntax:
//   - {name} matches a single path segment
//   - {name...} matches the rest of the path
//   - {$} matches only the end of the path (exact match)
//   - Prefix with "755 " to add permission requirements (optional)
//
// Most-specific pattern wins when multiple patterns match.
// If two patterns conflict (match same paths with no specificity difference),
// Map panics.
//
// For backward compatibility, patterns using the old "**" syntax
// are automatically converted to {_rest...}.
func (f *FS) Map(patternStr string, handler Handler) error {
	patternStr = convertLegacyPattern(patternStr)

	p, err := parsePattern(patternStr)
	if err != nil {
		return err
	}

	for _, existing := range f.routes {
		if p.conflictsWith(existing.pattern) {
			panic("ragfs: conflicting patterns: " + patternStr + " and " + existing.pattern.str)
		}
	}

	f.routes = append(f.routes, routeInternal{
		pattern: p,
		handler: handler,
	})
	return nil
}

func convertLegacyPattern(s string) string {
	if strings.HasSuffix(s, "/**") {
		return strings.TrimSuffix(s, "/**") + "/{_rest...}"
	}
	if s == "/**" {
		return "/{_rest...}"
	}
	if s == "/*" {
		return "/{_rest...}"
	}
	return s
}

// ReadDir reads the named directory, implementing fs.ReadDirFS.
// It matches the path against registered patterns using most-specific matching,
// checks execute permission (owner bit 1), and returns the directory entries.
// Returns fs.ErrNotExist if no pattern matches or fs.ErrPermission if permission denied.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	best := selectBestMatch(f.routes, name, false)
	if best == nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	params, _ := best.pattern.match(name)
	entries, err := best.handler(context.Background(), name, params)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// Open opens the named file, implementing fs.FS.
// It matches the path against registered patterns using most-specific matching,
// checks read permission (owner bit 4), and returns a file containing the handler's response.
// Returns fs.ErrNotExist if no pattern matches or fs.ErrPermission if permission denied.
func (f *FS) Open(name string) (fs.File, error) {
	best := selectBestMatch(f.routes, name, true)
	if best == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	params, _ := best.pattern.match(name)
	entries, err := best.handler(context.Background(), name, params)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	// If we have multiple entries, this is a directory
	if len(entries) > 1 {
		return &dirFile{
			name:    name,
			entries: entries,
		}, nil
	}

	// Single entry - check if it's a directory or file
	entry := entries[0]

	// If entry is a directory, return dirFile
	if entry.IsDir() {
		return &dirFile{
			name:    name,
			entries: entries,
		}, nil
	}

	// It's a file - get content
	if ce, ok := entry.(interface{ Content() []byte }); ok {
		return &file{
			name:   entry.Name(),
			reader: bytes.NewReader(ce.Content()),
		}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
}

func selectBestMatch(routes []routeInternal, path string, isOpen bool) *routeInternal {
	var matches []routeInternal

	for _, r := range routes {
		if _, ok := r.pattern.match(path); ok {
			if isOpen && !r.pattern.allowsOpen() {
				continue
			}
			if !isOpen && !r.pattern.allowsReadDir() {
				continue
			}
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return nil
	}

	if len(matches) == 1 {
		return &matches[0]
	}

	slices.SortFunc(matches, func(a, b routeInternal) int {
		if a.pattern.moreSpecificThan(b.pattern) {
			return -1
		}
		if b.pattern.moreSpecificThan(a.pattern) {
			return 1
		}
		bitsA := countPermBits(a.pattern.perm)
		bitsB := countPermBits(b.pattern.perm)
		if bitsA < bitsB {
			return -1
		}
		if bitsA > bitsB {
			return 1
		}
		return 0
	})

	return &matches[0]
}

// file implements fs.File
type file struct {
	name   string
	reader *bytes.Reader
}

// Stat returns file information.
func (f *file) Stat() (fs.FileInfo, error) {
	return &fileInfo{
		name: f.name,
		size: int64(f.reader.Len()),
	}, nil
}

// Read reads up to len(p) bytes into p.
func (f *file) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

// Close closes the file.
func (f *file) Close() error {
	return nil
}

// dirFile implements fs.ReadDirFile for directories
type dirFile struct {
	name    string
	entries []fs.DirEntry
	offset  int
}

// Stat returns directory information.
func (d *dirFile) Stat() (fs.FileInfo, error) {
	return &dirInfo{name: d.name}, nil
}

// Read is not supported for directories.
func (d *dirFile) Read(p []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.name, Err: fs.ErrInvalid}
}

// Close closes the directory.
func (d *dirFile) Close() error {
	return nil
}

// ReadDir reads directory entries.
func (d *dirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.offset >= len(d.entries) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		// Return all remaining entries
		entries := d.entries[d.offset:]
		d.offset = len(d.entries)
		return entries, nil
	}

	// Return up to n entries
	end := min(d.offset+n, len(d.entries))

	entries := d.entries[d.offset:end]
	d.offset = end

	return entries, nil
}

// dirInfo implements fs.FileInfo for directories
type dirInfo struct {
	name string
}

func (i *dirInfo) Name() string       { return i.name }
func (i *dirInfo) Size() int64        { return 0 }
func (i *dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0755 }
func (i *dirInfo) ModTime() time.Time { return time.Time{} }
func (i *dirInfo) IsDir() bool        { return true }
func (i *dirInfo) Sys() any           { return nil }

// fileInfo implements fs.FileInfo
type fileInfo struct {
	name string
	size int64
}

// Name returns the base name of the file.
func (i *fileInfo) Name() string { return i.name }

// Size returns the length in bytes.
func (i *fileInfo) Size() int64 { return i.size }

// Mode returns the file mode bits (always 0444 for read-only).
func (i *fileInfo) Mode() fs.FileMode { return 0444 }

// ModTime returns the modification time (always zero value).
func (i *fileInfo) ModTime() time.Time { return time.Time{} }

// IsDir returns whether this is a directory (always false).
func (i *fileInfo) IsDir() bool { return false }

// Sys returns underlying data source (always nil).
func (i *fileInfo) Sys() any { return nil }
