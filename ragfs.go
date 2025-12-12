// Package ragfs provides a filesystem interface for mapping path patterns to custom handlers.
// It implements the fs.FS interface, allowing LLM agents and applications to interact with
// data sources (APIs, databases) using familiar filesystem operations.
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
	"strings"
	"time"
)

// Handler is a function that handles filesystem operations for a given path.
// It receives the full path, extracted parameters, and returns directory entries.
type Handler func(ctx context.Context, path string, params map[string]string) ([]fs.DirEntry, error)

// FS is a filesystem that maps path patterns to handlers.
// It implements the fs.FS interface from the standard library.
type FS struct {
	routes      []route
	cache       *Cache       // nil if caching is disabled
	boltDBCache *boltDBCache // nil if BoltDB caching is not enabled
}

// route represents a pattern-to-handler mapping.
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
// Patterns can include parameters in curly braces, e.g., "/emails/{date}".
// When a path is accessed, the first matching pattern's handler is called.
func (f *FS) Map(pattern string, handler Handler) error {
	f.routes = append(f.routes, route{
		pattern: pattern,
		handler: handler,
	})
	return nil
}

type routeMatch struct {
	route  route
	params map[string]string
}

// findLongestMatch finds the longest matching route pattern for the given path.
// Returns nil if no match is found.
func (f *FS) findLongestMatch(name string) *routeMatch {
	var matched routeMatch
	for _, r := range f.routes {
		if match, p := matchPattern(r.pattern, name); match {
			if len(matched.route.pattern) < len(r.pattern) {
				matched = routeMatch{
					route:  r,
					params: p,
				}
				continue
			}
		}
	}

	if matched.route.pattern == "" {
		return nil
	}

	return &matched
}

// ReadDir reads the named directory, implementing fs.ReadDirFS.
// It matches the path against registered patterns, calls the matching handler,
// and returns the directory entries directly.
// Returns fs.ErrNotExist if no pattern matches.
func (f *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	matched := f.findLongestMatch(name)
	if matched == nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	entries, err := matched.route.handler(context.Background(), name, matched.params)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

// Open opens the named file, implementing fs.FS.
// It matches the path against registered patterns, calls the matching handler,
// and returns a file containing the handler's response.
// If caching is enabled, results are cached for subsequent calls.
// Returns fs.ErrNotExist if no pattern matches.
func (f *FS) Open(name string) (fs.File, error) {
	matched := f.findLongestMatch(name)
	if matched == nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	// Layer 1: Check handler cache
	var entries []fs.DirEntry
	var err error

	if f.boltDBCache != nil {
		entries = f.boltDBCache.getHandler(name)
	} else if f.cache != nil {
		entries = f.cache.getHandler(name)
	}

	if entries == nil {
		// Cache miss - call the handler
		entries, err = matched.route.handler(context.Background(), name, matched.params)
		if err != nil {
			return nil, err
		}

		// Cache the result
		if f.boltDBCache != nil {
			f.boltDBCache.setHandler(name, entries)
		} else if f.cache != nil {
			f.cache.setHandler(name, entries)
		}
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
		// Layer 2: Check content cache
		var content []byte

		if f.boltDBCache != nil {
			content = f.boltDBCache.getContent(name)
		} else if f.cache != nil {
			content = f.cache.getContent(name)
		}

		if content == nil {
			// Cache miss - extract content
			content = ce.Content()

			// Cache the content
			if f.boltDBCache != nil {
				f.boltDBCache.setContent(name, content)
			} else if f.cache != nil {
				f.cache.setContent(name, content)
			}
		}

		return &file{
			name:   entry.Name(),
			reader: bytes.NewReader(content),
		}, nil
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
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
	end := d.offset + n
	if end > len(d.entries) {
		end = len(d.entries)
	}

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

// FileEntry implements fs.DirEntry for file entries with content.
// It is a convenience type for handlers that return file entries.
// The Content method provides the file's byte content.
//
// Example:
//
//	return []fs.DirEntry{
//	    ragfs.NewFileEntry("config.json", []byte(`{"version": "1.0"}`)),
//	}, nil
type FileEntry struct {
	name    string
	content []byte
}

// NewFileEntry creates a new FileEntry with the given name and content.
func NewFileEntry(name string, content []byte) *FileEntry {
	return &FileEntry{
		name:    name,
		content: content,
	}
}

// Name returns the name of the file entry.
func (e *FileEntry) Name() string { return e.name }

// IsDir returns false, indicating this is a file entry.
func (e *FileEntry) IsDir() bool { return false }

// Type returns the file mode (0 for regular files).
func (e *FileEntry) Type() fs.FileMode { return 0 }

// Info returns nil, nil (file info is not provided).
func (e *FileEntry) Info() (fs.FileInfo, error) { return nil, nil }

// Content returns the file's byte content.
func (e *FileEntry) Content() []byte { return e.content }

// DirEntry implements fs.DirEntry for directory entries.
// It is a convenience type for handlers that return directory entries.
// The Content method returns nil for directories.
//
// Example:
//
//	return []fs.DirEntry{
//	    ragfs.NewDirEntry("users", true),
//	    ragfs.NewDirEntry("config", true),
//	}, nil
type DirEntry struct {
	name  string
	isDir bool
}

// DOT is the current directory entry.
var DOT = &DirEntry{
	name:  ".",
	isDir: true,
}

// DOTDOT is the parent directory entry.
var DOTDOT = &DirEntry{
	name:  "..",
	isDir: true,
}

// NewDirectoryListing creates a new directory listing with the given entries.
func NewDirectoryListing(entries []fs.DirEntry) []fs.DirEntry {
	return append([]fs.DirEntry{DOT, DOTDOT}, entries...)
}

// NewDirEntry creates a new DirEntry with the given name and directory flag.
func NewDirEntry(name string, isDir bool) *DirEntry {
	return &DirEntry{
		name:  name,
		isDir: isDir,
	}
}

// Name returns the name of the directory entry.
func (e *DirEntry) Name() string { return e.name }

// IsDir returns whether this entry is a directory.
func (e *DirEntry) IsDir() bool { return e.isDir }

// Type returns fs.ModeDir if this is a directory, otherwise 0.
func (e *DirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

// Info returns nil, nil (file info is not provided).
func (e *DirEntry) Info() (fs.FileInfo, error) { return nil, nil }

// Content returns nil for directory entries.
func (e *DirEntry) Content() []byte { return nil }

// matchPattern matches a path against a pattern and extracts parameters.
// Pattern format:
//   - /emails/{date} matches /emails/2025-10-07 and extracts date=2025-10-07
//   - /** matches any path with any number of segments
//   - /*/** matches any path with at least one segment
func matchPattern(pattern, path string) (bool, map[string]string) {
	params := make(map[string]string)

	// Handle wildcard pattern /**
	if pattern == "/**" || pattern == "/*" {
		// Match any path
		return true, params
	}

	// Split pattern and path into segments
	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	pathParts := strings.Split(strings.Trim(path, "/"), "/")

	// Check if pattern ends with ** (match remaining segments)
	hasWildcard := len(patternParts) > 0 && patternParts[len(patternParts)-1] == "**"
	if hasWildcard {
		// Remove ** from pattern for matching
		patternParts = patternParts[:len(patternParts)-1]
		// Path must have at least as many segments as the pattern (without **)
		if len(pathParts) < len(patternParts) {
			return false, nil
		}
	} else {
		// Without wildcard, must have same number of segments
		if len(patternParts) != len(pathParts) {
			return false, nil
		}
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

// EnableCache enables caching for this filesystem with the given configuration.
// Caching is opt-in and disabled by default. Once enabled, all filesystem operations
// will use the cache. Use default config values by passing CacheConfig{} for sensible defaults.
//
// Default values:
//   - MaxEntries: 1000 (per cache layer)
//   - TTL: 30 seconds
//
// Example:
//
//	fsys := ragfs.New()
//	fsys.EnableCache(ragfs.CacheConfig{
//	    MaxEntries: 500,
//	    TTL:        60 * time.Second,
//	})
func (f *FS) EnableCache(config CacheConfig) {
	// Apply defaults if not specified
	if config.MaxEntries == 0 {
		config.MaxEntries = 1000
	}
	if config.TTL == 0 {
		config.TTL = 30 * time.Second
	}

	f.cache = newCache(config)
}

// Invalidate removes a specific path from the cache.
// Both handler results and file content caches are invalidated for the given path.
// Returns an error if the path is empty.
//
// Example:
//
//	// After updating user data externally
//	fsys.Invalidate("/users/123")
func (f *FS) Invalidate(path string) error {
	if path == "" {
		return &fs.PathError{Op: "invalidate", Path: path, Err: fs.ErrInvalid}
	}

	if f.cache != nil {
		f.cache.invalidate(path)
	}

	return nil
}

// InvalidatePrefix removes all cached entries whose paths start with the given prefix.
// This is useful for invalidating entire directory trees or all entries matching a pattern.
// Returns an error if the prefix is empty.
//
// Example:
//
//	// Invalidate all users
//	fsys.InvalidatePrefix("/users/")
//
//	// Invalidate everything
//	fsys.InvalidatePrefix("/")
func (f *FS) InvalidatePrefix(prefix string) error {
	if prefix == "" {
		return &fs.PathError{Op: "invalidate_prefix", Path: prefix, Err: fs.ErrInvalid}
	}

	if f.cache != nil {
		f.cache.invalidatePrefix(prefix)
	}

	return nil
}

// Stats returns current cache statistics.
// Returns zero values if caching is disabled.
//
// Example:
//
//	stats := fsys.Stats()
//	hitRate := float64(stats.Hits.Load()) / float64(stats.Hits.Load() + stats.Misses.Load())
//	fmt.Printf("Cache hit rate: %.2f%%\n", hitRate*100)
func (f *FS) Stats() CacheStats {
	if f.cache == nil {
		return CacheStats{}
	}

	return *f.cache.stats
}
